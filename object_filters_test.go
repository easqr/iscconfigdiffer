package differ

import (
	"strings"
	"testing"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func filteredConfigurationInput(t *testing.T, env string) snapshot.Input {
	t.Helper()
	return input(t, env, []string{"SOURCE", "ROLE"},
		obj("SOURCE", "directory", "Primary Directory", nil),
		obj("SOURCE", "development", "Development Directory", map[string]any{"changed": env}),
		obj("ROLE", "employee", "Employee Access", nil),
		obj("ROLE", "other", "Other Access", map[string]any{"changed": env}))
}

func TestPerTypeObjectFiltersAndPolicyFiles(t *testing.T) {
	example, err := LoadPolicy("examples/policy-filtered-drift.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Compile(example); err != nil {
		t.Fatal("filtered example does not compile", err)
	}
	dir := t.TempDir()
	root := policyFile(t, dir, "policy.json", `{"version":2,"id":"filtered","profile":"isc-portable-v2","includes":["rules.json"],"object_filters":{"SOURCE":{"exclude":["Development Directory"]},"ROLE":{"include":["Employee Access"]}}}`)
	policyFile(t, dir, "rules.json", `{"ignores":[{"id":"fixture-note","type":"ROLE","path":"/note"}]}`)
	p, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Mode: "drift", Left: filteredConfigurationInput(t, "dev"), Right: filteredConfigurationInput(t, "prod"), Scope: []string{"SOURCE", "ROLE"}}
	r := compareTest(t, req, p)
	if r.ExitCode() != 0 || len(r.Matches) != 2 || len(r.ExcludedObjects) != 2 {
		t.Fatal("per-type include/exclude failed", r.Counts, r.ExcludedObjects)
	}
	if r.ObjectFilters["SOURCE"].Exclude[0] != "Development Directory" || len(r.Evidence) == 0 {
		t.Fatal("selection provenance absent")
	}
	req.Scope = []string{"ROLE"}
	if r = compareTest(t, req, p); r.ExitCode() != 0 || len(r.Matches) != 1 || len(r.ExcludedObjects) != 1 {
		t.Fatal("filters widened explicit type scope", r)
	}
	req.ObjectKey = "Other Access"
	if r = compareTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal("name selection widened policy", r)
	}
	policyFile(t, dir, "rules.json", `{"object_filters":{"ROLE":{"include":["Employee Access"]}}}`)
	if _, err = LoadPolicy(root); err == nil || !strings.Contains(err.Error(), "root policy") {
		t.Fatal("rule-only fragment supplied selection settings", err)
	}
}

func TestExcludedSourceRemainsDependencyContext(t *testing.T) {
	left, right := syncAttributePair()
	p := portable()
	p.ObjectFilters = map[string]v2.ObjectFilter{"SOURCE": {Exclude: []string{"Invented Source"}}}
	req := Request{Mode: "drift", Left: attributeSyncInput(t, "dev", left), Right: attributeSyncInput(t, "prod", right), Scope: []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}}
	r := compareTest(t, req, p)
	if r.ExitCode() != 0 || len(r.Matches) != 1 || r.Matches[0].Type != "ATTR_SYNC_SOURCE_CONFIG" || len(r.ExcludedObjects) != 1 {
		t.Fatal("excluded parent removed from matching context", r)
	}
}
