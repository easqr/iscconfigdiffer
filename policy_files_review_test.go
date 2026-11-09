package differ

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
)

func TestPolicyFilePathsUseRecordedNormalizedNames(t *testing.T) {
	dir := t.TempDir()
	policyFile(t, dir, "rules.json", `{"ignores":[{"id":"top","type":"ROLE","path":"/top"}]}`)
	policyFile(t, dir, "nested/rules.json", `{"ignores":[{"id":"nested","type":"ROLE","path":"/nested"}]}`)
	if err := os.Mkdir(filepath.Join(dir, "nested/deep"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nested/deep", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	root := policyFile(t, dir, "policy.json", `{"version":2,"id":"paths","includes":["link/../rules.json"]}`)
	p, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if p.Ignores[0].ID != "top" || p.Evidence[1].RecordID != "rules.json" {
		t.Fatal("recorded path does not identify the file read", p.Ignores, p.Evidence)
	}
	policyFile(t, dir, "policy.json", `{"version":2,"id":"paths","includes":["rules.json","link/../rules.json"]}`)
	if _, err = LoadPolicy(root); err == nil || !strings.Contains(err.Error(), "duplicate policy file") {
		t.Fatal("normalized alias was not rejected", err)
	}
}

func TestPolicyRootSymlinkUsesTargetBundle(t *testing.T) {
	dir := t.TempDir()
	root := policyFile(t, dir, "policy.json", fileRoot)
	policyFile(t, dir, "rules/source.json", `{"equivalences":[`+fileRule+`]}`)
	alias := filepath.Join(t.TempDir(), "alias.json")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPolicy(alias)
	if err != nil {
		t.Fatal("root symlink broke file loading", err)
	}
	if p.Evidence[0].RecordID != "policy.json" || len(p.Equivalences) != 1 {
		t.Fatal("symlink did not load target bundle", p)
	}
	// Single-file policies were readable through external symlinks before includes.
	policyFile(t, dir, "policy.json", `{"version":2,"id":"flat"}`)
	if _, err = LoadPolicy(alias); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyFileErrorsPreserveFilesystemCause(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadPolicy(filepath.Join(dir, "missing.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("missing root cause lost", err)
	}
	root := policyFile(t, dir, "policy.json", fileRoot)
	if _, err := LoadPolicy(root); !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "rules/source.json") {
		t.Fatal("missing include cause lost", err)
	}
}

func TestPolicyFragmentSemanticErrorsIdentifyOwner(t *testing.T) {
	for _, rule := range []string{
		`"collections":[{"id":"broken","type":"ROLE","path":"/items","mode":"invalid"}]`,
		`"equivalences":[{"id":"broken","type":"ROLE","path":"/value","left_env":"dev","right_env":"prod"}]`,
		`"expectations":[{"id":"broken","type":"ROLE","path":"/value","operation":"invalid","authority":"approved"}]`,
		`"sensitive":[{"id":"broken","type":"ROLE","path":"invalid"}]`,
		`"identities":[{"id":"broken","type":"INVENTED","strategy":"invalid"}]`,
		`"pairings":[{"id":"broken","type":"ROLE","left_id":"","right_id":"right"}]`,
		`"references":[{"id":"broken","type":"ROLE","path":"/value","target_type":""}]`,
	} {
		t.Run(strings.Split(rule, ":")[0], func(t *testing.T) {
			dir := t.TempDir()
			// A valid rule ID matching an error word must not hijack the attribution.
			root := policyFile(t, dir, "policy.json", `{"version":2,"id":"errors","profile":"isc-portable-v2","includes":["broken.json"],"ignores":[{"id":"invalid","type":"ROLE","path":"/unused"}]}`)
			policyFile(t, dir, "broken.json", `{`+rule+`}`)
			p, err := LoadPolicy(root)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Compile(p)
			var ruleErr *v2.RuleError
			if !errors.As(err, &ruleErr) || len(ruleErr.RuleIDs) != 1 || ruleErr.RuleIDs[0] != "broken" {
				t.Fatal("structured rule ownership unavailable", err)
			}
			if err == nil || !strings.Contains(err.Error(), "broken.json") || strings.Contains(err.Error(), "policy files: policy.json") {
				t.Fatal("wrong semantic error owner", err)
			}
		})
	}
}

func TestPolicyFragmentMalformedOwnershipMetadata(t *testing.T) {
	for _, body := range []string{
		`{"ignores":[{"id":"bad id","type":"ROLE","path":"/unused"}]}`,
		`{"ignores":[{"id":"","type":"ROLE","path":"/unused"}]}`,
		`{"disabled_rules":["bad id"]}`,
		`{"evidence":[{"kind":""}]}`,
	} {
		dir := t.TempDir()
		root := policyFile(t, dir, "policy.json", fileRoot)
		policyFile(t, dir, "rules/source.json", body)
		if _, err := LoadPolicy(root); err == nil || !strings.Contains(err.Error(), "rules/source.json") {
			t.Fatal("invalid ownership metadata accepted or misattributed", err)
		}
	}
}

func TestPolicyGlobalErrorDoesNotMatchRuleIDWords(t *testing.T) {
	dir := t.TempDir()
	root := policyFile(t, dir, "policy.json", `{"version":2,"id":"bad policy id","profile":"isc-portable-v2","includes":["rules.json"]}`)
	policyFile(t, dir, "rules.json", `{"ignores":[{"id":"invalid","type":"ROLE","path":"/unused"}]}`)
	p, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Compile(p)
	if err == nil || !strings.Contains(err.Error(), "policy files: policy.json") || strings.Contains(err.Error(), "policy files: rules.json") {
		t.Fatal("root error misattributed to a rule", err)
	}
}

func TestPolicyCrossFileConflictsRetainBothOwners(t *testing.T) {
	cases := []struct{ one, two string }{
		{`{"pairings":[{"id":"pair-one","type":"ROLE","left_id":"a","right_id":"b"}]}`, `{"pairings":[{"id":"pair-two","type":"ROLE","left_id":"a","right_id":"c"}]}`},
		{`{"identities":[{"id":"identity-one","type":"INVENTED","strategy":"singleton"}]}`, `{"identities":[{"id":"identity-two","type":"INVENTED","strategy":"singleton"}]}`},
		{`{"ignores":[{"id":"replacement","replaces":"isc.SOURCE.before-provisioning-rule-id","type":"SOURCE","path":"/beforeProvisioningRule/id"}]}`, `{"disabled_rules":["isc.SOURCE.before-provisioning-rule-id"]}`},
		// Multiple conflicts from one file must not repeat its name in the diagnostic.
		{`{"ignores":[{"id":"one","type":"ROLE","path":"/x"},{"id":"two","type":"ROLE","path":"/x"}]}`, `{}`},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		root := policyFile(t, dir, "policy.json", `{"version":2,"id":"conflicts","profile":"isc-portable-v2","includes":["one.json","two.json"]}`)
		policyFile(t, dir, "one.json", tc.one)
		policyFile(t, dir, "two.json", tc.two)
		p, err := LoadPolicy(root)
		if err != nil {
			t.Fatal(err)
		}
		_, err = Compile(p)
		if err == nil || strings.Count(err.Error(), "one.json") != 1 {
			t.Fatal("missing/repeated first file", err)
		}
		if tc.two != "{}" && strings.Count(err.Error(), "two.json") != 1 {
			t.Fatal("missing second file", err)
		}
	}
}
