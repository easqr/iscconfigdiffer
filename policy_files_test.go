package differ

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func policyFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const fileRule = `{"id":"host","type":"ROLE","key":"Invented","path":"/host","left_env":"dev","right_env":"prod","groups":[{"dev":"dev.example.invalid","prod":"prod.example.invalid"}]}`
const fileRoot = `{"version":2,"id":"files","profile":"isc-portable-v2","bindings":{"left":"dev","right":"prod"},"includes":["rules/source.json"]}`

func TestPolicyFilesCompositionAndProvenance(t *testing.T) {
	dir := t.TempDir()
	root := policyFile(t, dir, "policy.json", fileRoot)
	fragment := `{"equivalences":[` + fileRule + `]}`
	policyFile(t, dir, "rules/source.json", fragment)
	p, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Equivalences) != 1 || p.Bindings["left"] != "dev" || p.Equivalences[0].Evidence != nil {
		t.Fatal("settings/rule evidence changed", p)
	}
	if _, err := DecodePolicy([]byte(fileRoot)); err == nil {
		t.Fatal("byte decoder unexpectedly resolves files")
	}
	if len(p.Evidence) != 2 {
		t.Fatal(p.Evidence)
	}
	for i, body := range []string{fileRoot, fragment} {
		hash := sha256.Sum256([]byte(body))
		if p.Evidence[i].Digest != hex.EncodeToString(hash[:]) || strings.Contains(p.Evidence[i].RecordID, dir) {
			t.Fatal("incorrect file provenance", p.Evidence[i])
		}
	}
	if p.Evidence[1].Attributes["rule_ids"] != "host" {
		t.Fatal(p.Evidence)
	}
	flat, err := DecodePolicy([]byte(`{"version":2,"id":"files","profile":"isc-portable-v2","bindings":{"left":"dev","right":"prod"},"equivalences":[` + fileRule + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Mode: "drift", Left: input(t, "dev", []string{"ROLE"}, obj("ROLE", "a", "Invented", map[string]any{"host": "dev.example.invalid"})), Right: input(t, "prod", []string{"ROLE"}, obj("ROLE", "b", "Invented", map[string]any{"host": "prod.example.invalid"}))}
	got, want := compareTest(t, req, p), compareTest(t, req, flat)
	if !reflect.DeepEqual(got.Decisions, want.Decisions) || !reflect.DeepEqual(got.Counts, want.Counts) || got.Counts["equivalent"] != 1 {
		t.Fatal("composition changed comparison", got.Counts, want.Counts)
	}
	if got.PolicyDigest == want.PolicyDigest {
		t.Fatal("file provenance missing from digest")
	}
	for _, render := range []func(io.Writer) error{got.HTML, got.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"Policy file origin", "rules/source.json", p.Evidence[1].Digest} {
			if !strings.Contains(b.String(), s) {
				t.Fatal("report lacks provenance", s)
			}
		}
	}
	// Reloading is deterministic and relocatable with the same relative layout.
	other := t.TempDir()
	policyFile(t, other, "policy.json", fileRoot)
	policyFile(t, other, "rules/source.json", fragment)
	moved, err := LoadPolicy(filepath.Join(other, "policy.json"))
	if err != nil || !reflect.DeepEqual(moved, p) {
		t.Fatal("absolute location affected policy", err)
	}
}

func TestPolicyFilesRejectInvalidBundles(t *testing.T) {
	tests := []struct{ name, root, fragment, want string }{
		{"missing", "[\"missing.json\"]", "{}", "missing.json"},
		{"absolute", "[\"/tmp/fragment.json\"]", "{}", "relative"},
		{"escape", "[\"../fragment.json\"]", "{}", "relative"},
		{"empty", "[\" \"]", "{}", "nonblank"},
		{"null", "null", "{}", "array"},
		{"wrong type", "[42]", "{}", "nonblank"},
		{"duplicate", "[\"fragment.json\",\"./fragment.json\"]", "{}", "duplicate policy file"},
		{"self", "[\"policy.json\"]", "{}", "duplicate policy file"},
		{"array fragment", "[\"fragment.json\"]", "[]", "JSON object"},
		{"duplicate JSON", "[\"fragment.json\"]", `{"evidence":[],"evidence":[]}`, "fragment.json"},
		{"bad rule", "[\"fragment.json\"]", `{"ignores":[{"id":"x","type":"ROLE","path":"/x","unknown":true}]}`, "fragment.json"},
	}
	for _, key := range []string{"version", "id", "profile", "bindings", "null_handling", "modes", "includes", "unknown"} {
		tests = append(tests, struct{ name, root, fragment, want string }{"forbidden " + key, `["fragment.json"]`, `{"` + key + `":null}`, "property \"" + key + "\""})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			policyFile(t, dir, "fragment.json", tc.fragment)
			root := policyFile(t, dir, "policy.json", `{"version":2,"id":"files","includes":`+tc.root+`}`)
			if _, err := LoadPolicy(root); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal("unexpected result", err, tc.want)
			}
		})
	}
	// Root decoding remains strict as well.
	for _, body := range []string{`[]`, `{"version":2,"id":"a","id":"b"}`, `{"version":2,"id":"a","typo":true}`} {
		root := policyFile(t, t.TempDir(), "policy.json", body)
		if _, err := LoadPolicy(root); err == nil {
			t.Fatal("invalid root accepted", body)
		}
	}
}

func TestPolicyFilesRejectAliasesAndEscapingSymlinks(t *testing.T) {
	for _, kind := range []string{"hardlink", "symlink", "escape", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			target := policyFile(t, dir, "fragment.json", "{}")
			includes := `["fragment.json","alias.json"]`
			var err error
			switch kind {
			case "hardlink":
				err = os.Link(target, filepath.Join(dir, "alias.json"))
			case "symlink":
				err = os.Symlink("fragment.json", filepath.Join(dir, "alias.json"))
			case "escape":
				target = policyFile(t, t.TempDir(), "outside.json", "{}")
				err = os.Symlink(target, filepath.Join(dir, "alias.json"))
				includes = `["alias.json"]`
			case "directory":
				err = os.Mkdir(filepath.Join(dir, "alias.json"), 0700)
				includes = `["alias.json"]`
			}
			if err != nil {
				t.Fatal(err)
			}
			root := policyFile(t, dir, "policy.json", `{"version":2,"id":"files","includes":`+includes+`}`)
			if _, err := LoadPolicy(root); err == nil {
				t.Fatal("alias or escaping/nonregular include accepted")
			}
		})
	}
}

func TestPolicyFilesGlobalConflictsAndAuthority(t *testing.T) {
	dir := t.TempDir()
	root := policyFile(t, dir, "policy.json", `{"version":2,"id":"files","profile":"isc-portable-v2","includes":["one.json","two.json"]}`)
	policyFile(t, dir, "one.json", `{"equivalences":[`+fileRule+`]}`)
	for _, other := range []string{`{"equivalences":[` + fileRule + `]}`, `{"ignores":[{"id":"host","type":"ROLE","path":"/host"}]}`} {
		policyFile(t, dir, "two.json", other)
		_, err := LoadPolicy(root)
		if err == nil || !strings.Contains(err.Error(), "one.json") || !strings.Contains(err.Error(), "two.json") {
			t.Fatal("duplicate ID lacks owners", err)
		}
	}
	// Different IDs do not acquire precedence from manifest order.
	policyFile(t, dir, "two.json", `{"equivalences":[`+strings.Replace(fileRule, `"id":"host"`, `"id":"other"`, 1)+`]}`)
	p, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Compile(p); err == nil || !strings.Contains(err.Error(), "one.json") || !strings.Contains(err.Error(), "two.json") {
		t.Fatal("overlap lacks owners", err)
	}
	for _, name := range []string{"one.json", "two.json"} {
		policyFile(t, dir, name, `{"disabled_rules":["isc.ROLE.id"]}`)
	}
	if _, err = LoadPolicy(root); err == nil || !strings.Contains(err.Error(), "duplicate disabled rule") {
		t.Fatal("duplicate disabling accepted", err)
	}
	// File attribution cannot supply the independent approval required by deployment.
	policyFile(t, dir, "one.json", `{"expectations":[{"id":"unapproved","type":"ROLE","path":"/name","operation":"copy_source","authority":"approved"}]}`)
	policyFile(t, dir, "two.json", `{}`)
	p, err = LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Mode: "deployment", Left: input(t, "dev", []string{"ROLE"}, obj("ROLE", "a", "Invented", nil)), Right: input(t, "prod", []string{"ROLE"}, obj("ROLE", "b", "Invented", nil)), Operations: []v2.Operation{{Type: "ROLE", Key: "Invented", Action: "update"}}}
	r, err := Compare(req, p)
	if err == nil && (r.ExitCode() != 2 || r.Counts["unverified"] == 0) {
		t.Fatal("file evidence granted authority", r.Counts)
	}
	if err != nil && !strings.Contains(err.Error(), "one.json") {
		t.Fatal("semantic error lacks fragment context", err)
	}
	policyFile(t, dir, "one.json", `{"expectations":[{"id":"approved","type":"ROLE","path":"/name","operation":"copy_source","authority":"approved","evidence":{"kind":"invented-release"}}]}`)
	p, err = LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	r = compareTest(t, req, p)
	if r.ExitCode() != 0 || r.RuleUsage["approved"] != 1 {
		t.Fatal("explicit fragment expectation failed", r.Counts)
	}
}

func TestDocumentedModularPolicy(t *testing.T) {
	p, err := LoadPolicy("examples/policy-includes.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Compile(p); err != nil {
		t.Fatal(err)
	}
	types := []string{"SOURCE", "TRANSFORM"}
	left := input(t, "dev", types, obj("SOURCE", "a", "Invented Directory", map[string]any{"connectorAttributes": map[string]any{"host": "directory-dev.example.invalid"}}), obj("SOURCE", "b", "Invented Entra", map[string]any{"connectorAttributes": map[string]any{"clientID": "invented-dev-client"}}), obj("TRANSFORM", "c", "Invented Email Domain", map[string]any{"type": "static", "attributes": map[string]any{"value": "dev.example.invalid"}}))
	right := input(t, "prod", types, obj("SOURCE", "d", "Invented Directory", map[string]any{"connectorAttributes": map[string]any{"host": "directory-prod.example.invalid"}}), obj("SOURCE", "e", "Invented Entra", map[string]any{"connectorAttributes": map[string]any{"clientID": "invented-prod-client"}}), obj("TRANSFORM", "f", "Invented Email Domain", map[string]any{"type": "static", "attributes": map[string]any{"value": "prod.example.invalid"}}))
	r := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 3 {
		b, _ := json.Marshal(r.Diagnostics)
		t.Fatal(r.Counts, string(b))
	}
	// The AD fragment retains the complete single-file example's keyed mappings.
	req := Request{Mode: "drift", Left: domainInput(t, "dev", "SOURCE", multipleDomainUsers("dev")), Right: domainInput(t, "prod", "SOURCE", multipleDomainUsers("prod"))}
	r = compareTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 6 {
		t.Fatal(r.Counts)
	}
}

func TestPolicyFilesRootRulesAndFragmentOverrides(t *testing.T) {
	dir := t.TempDir()
	rootBody := `{"version":2,"id":"overrides","profile":"isc-portable-v2","modes":["drift"],"null_handling":"strict","includes":["override.json","disable.json"],"ignores":[{"id":"local-extra","type":"SOURCE","path":"/extra"}],"evidence":[{"kind":"root-note"}]}`
	root := policyFile(t, dir, "policy.json", rootBody)
	policyFile(t, dir, "override.json", `{"ignores":[{"id":"replacement","replaces":"isc.SOURCE.before-provisioning-rule-id","type":"SOURCE","path":"/beforeProvisioningRule/id","modes":["drift"],"evidence":{"kind":"invented-approved-policy"}}],"evidence":[{"kind":"fragment-note"}]}`)
	policyFile(t, dir, "disable.json", `{"disabled_rules":["isc.SOURCE.correlation-config-details-id"]}`)
	p, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ignores) != 2 || p.Ignores[0].ID != "local-extra" || p.NullHandling != "strict" || !reflect.DeepEqual(p.Modes, []string{"drift"}) {
		t.Fatal("root rules/settings lost")
	}
	notes := map[string]bool{}
	for _, e := range p.Evidence {
		notes[e.Kind] = true
	}
	if !notes["root-note"] || !notes["fragment-note"] {
		t.Fatal("authored evidence lost")
	}
	req := Request{Mode: "drift", Left: input(t, "dev", []string{"SOURCE"}, obj("SOURCE", "a", "Invented", map[string]any{"extra": "left", "beforeProvisioningRule": map[string]any{"id": "a"}, "correlationConfigDetails": map[string]any{"id": "a"}})), Right: input(t, "prod", []string{"SOURCE"}, obj("SOURCE", "b", "Invented", map[string]any{"extra": "right", "beforeProvisioningRule": map[string]any{"id": "b"}, "correlationConfigDetails": map[string]any{"id": "b"}}))}
	before := compareTest(t, req, p)
	if before.Counts["different"] != 1 || before.RuleUsage["local-extra"] != 1 || before.RuleUsage["replacement"] != 1 || before.Replacements["isc.SOURCE.before-provisioning-rule-id"] != "replacement" || len(before.DisabledRules) != 1 {
		t.Fatal("override/disable changed", before.Counts)
	}
	// File order is not precedence, including for replacements and disabling.
	policyFile(t, dir, "policy.json", strings.Replace(rootBody, `["override.json","disable.json"]`, `["disable.json","override.json"]`, 1))
	p, err = LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	after := compareTest(t, req, p)
	if !reflect.DeepEqual(before.Decisions, after.Decisions) || !reflect.DeepEqual(before.Counts, after.Counts) {
		t.Fatal("include order changed decisions")
	}
	if before.PolicyDigest == after.PolicyDigest {
		t.Fatal("changed manifest not reflected in provenance")
	}
}
