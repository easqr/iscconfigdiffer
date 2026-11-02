package differ

import (
	"encoding/json"
	exporter "github.com/elianderson/iscutilities/configexporter"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"reflect"
	"strings"
	"testing"
	"time"
)

func hubJSON(path string, enabled bool) []byte {
	b, _ := json.Marshal([]map[string]any{{"objectMappingId": "map-a", "objectType": "SOURCE", "jsonPath": path, "sourceValue": "dc=example,dc=com", "targetValue": "dc=example-dev,dc=com", "enabled": enabled, "created": "2025-01-01", "modified": "2025-01-02"}})
	return b
}
func TestHubPathsAndImport(t *testing.T) {
	for path, want := range map[string]string{"$": "/object", "$.owner.name": "/object/owner/name", `$['a/b']["~key"][0]`: "/object/a~1b/~0key/0", "$.items[*].name": "/object/items/*/name"} {
		got, e := HubPointer(path)
		if e != nil || got != want {
			t.Fatal(path, got, e)
		}
	}
	for _, path := range []string{"owner.name", "$..name", "$.x[?(@.id)]", "$.x[0:2]", "$.x[1,2]", "$.*", "$['*']"} {
		if _, e := HubPointer(path); e == nil {
			t.Fatal(path)
		}
	}
	ctx := HubContext{SourceTenant: "invented-prod", TargetTenant: "invented-sb", Provenance: "saved-fixture"}
	rules, e := ImportHub(hubJSON("$.dn", true), ctx)
	if e != nil || len(rules) != 1 || rules[0].Created != "2025-01-01" || rules[0].JSONPath != "$.dn" {
		t.Fatal(rules, e)
	}
	if _, e = ImportHub(hubJSON("$..dn", true), ctx); e == nil {
		t.Fatal("active unsupported expression dropped")
	}
	disabled, e := ImportHub(hubJSON("$..dn", false), ctx)
	if e != nil || disabled[0].Enabled {
		t.Fatal(disabled, e)
	}
	if _, e = ImportHub(hubJSON("$.dn", true), HubContext{}); e == nil {
		t.Fatal("missing context accepted")
	}
	raw := strings.Replace(string(hubJSON("$.dn", true)), `"sourceValue":"dc=example,dc=com"`, `"sourceValue":123`, 1)
	if _, e = ImportHub([]byte(raw), ctx); e == nil {
		t.Fatal("inferred string type")
	}
	p := policy()
	p.Hub = rules
	a := input(t, "production", []string{"SOURCE"}, obj("SOURCE", "a", "S", map[string]any{"dn": "dc=example,dc=com"}))
	b := input(t, "sandbox", []string{"SOURCE"}, obj("SOURCE", "b", "S", map[string]any{"dn": "dc=example-dev,dc=com"}))
	// Tree provenance is deliberately absent; Hub context cannot be guessed from labels.
	if runCompare(t, a, b, p).ExitCode() != 2 {
		t.Fatal("missing Hub tenant context accepted")
	}
	withTenant := func(in map[string]any, tenant string) snapshot.Input {
		raw, _ := json.Marshal(map[string]any{"tenant": tenant, "objects": []any{in}})
		snap, e := exporter.Organize(tenant, exporter.Download{JobID: "fixture", Types: []string{"SOURCE"}, Raw: raw}, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		if e != nil {
			t.Fatal(e)
		}
		files := map[string][]byte{}
		for _, artifact := range snap.Artifacts {
			files[artifact.Path] = artifact.Data
		}
		files["manifest.json"], _ = json.Marshal(snap.Manifest)
		input, e := snapshot.FromFiles(files, snapshot.Options{Mode: snapshot.SnapshotMode})
		if e != nil {
			t.Fatal(e)
		}
		return input
	}
	a = withTenant(obj("SOURCE", "a", "S", map[string]any{"dn": "dc=example,dc=com"}), ctx.SourceTenant)
	b = withTenant(obj("SOURCE", "b", "S", map[string]any{"dn": "dc=example-dev,dc=com"}), ctx.TargetTenant)
	if out := runCompare(t, a, b, p); out.ExitCode() != 0 {
		t.Fatal(out)
	}
	// A Hub substitution is applicable only to its source/target value pair;
	// unrelated equal values remain ordinary equality rather than assertions.
	b = withTenant(obj("SOURCE", "b", "S", map[string]any{"dn": "wrong"}), ctx.TargetTenant)
	if out := runCompare(t, a, b, p); out.ExitCode() != 1 {
		t.Fatal(out)
	}
	p.Hub = append(rules, rules[0])
	p.Hub[1].ID = "map-b"
	if p.Validate() == nil {
		t.Fatal("ambiguous conversion accepted")
	}
	p = policy()
	p.Hub = disabled
	if out := runCompare(t, a, b, p); out.ExitCode() != 1 {
		t.Fatal(out)
	}
}
func TestRuleOrderAndSensitiveProvenance(t *testing.T) {
	l := input(t, "e", []string{"ROLE"}, obj("ROLE", "l", "SensitiveName", map[string]any{"a": 1, "b": 1}))
	r := input(t, "e", []string{"ROLE"}, obj("ROLE", "r", "SensitiveName", map[string]any{"a": 2, "b": 2}))
	p := policy()
	p.Ignores = []Ignore{{ID: "b", Selector: Selector{Type: "ROLE", Path: "/object/b"}}, {ID: "a", Selector: Selector{Type: "ROLE", Path: "/object/a"}}}
	first := runCompare(t, l, r, p)
	p.Ignores[0], p.Ignores[1] = p.Ignores[1], p.Ignores[0]
	second := runCompare(t, l, r, p)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("rule ordering affected report")
	}
	p.Sensitive = []Sensitive{{ID: "secret-name", Selector: Selector{Type: "ROLE", Path: "/object/name"}}}
	out := runCompare(t, l, r, p)
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "SensitiveName") || strings.Contains(string(raw), "id-6c") {
		t.Fatal(string(raw))
	}
}

func TestSensitiveDerivedIdentityAndDiagnostics(t *testing.T) {
	in := input(t, "e", []string{"IDENTITY_PROFILE", "LIFECYCLE_STATE"}, obj("IDENTITY_PROFILE", "sensitive-physical-parent", "SecretParent", nil), obj("LIFECYCLE_STATE", "child", "State", map[string]any{"technicalName": "active", "identityProfileRef": map[string]any{"id": "sensitive-physical-parent"}}))
	p := policy()
	p.Sensitive = []Sensitive{{ID: "parent-secret", Selector: Selector{Type: "IDENTITY_PROFILE", Path: "/object/name"}}}
	out := runCompare(t, in, in, p)
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "SecretParent") || strings.Contains(string(raw), "sensitive-physical-parent") {
		t.Fatal("derived key exposed sensitive identity")
	}
	bad := input(t, "e", []string{"FUTURE"}, obj("FUTURE", "private-id", "", nil))
	p = policy()
	p.Sensitive = []Sensitive{{ID: "own-id", Selector: Selector{Type: "FUTURE", Path: "/self/id"}}}
	out = runCompare(t, bad, bad, p)
	raw, _ = json.Marshal(out)
	if strings.Contains(string(raw), "private-id") || strings.Contains(string(raw), "id-707269") {
		t.Fatal("diagnostic exposed sensitive provenance")
	}
}
