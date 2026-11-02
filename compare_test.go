package differ

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	exporter "github.com/elianderson/iscutilities/configexporter"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func obj(typ, id, name string, extra map[string]any) map[string]any {
	payload := map[string]any{}
	if name != "" {
		payload["name"] = name
	}
	for k, v := range extra {
		payload[k] = v
	}
	return map[string]any{"version": json.Number("1"), "self": map[string]any{"type": typ, "id": id}, "object": payload}
}
func input(t *testing.T, env string, types []string, objects ...map[string]any) snapshot.Input {
	t.Helper()
	files := map[string][]byte{}
	for _, o := range objects {
		s := o["self"].(map[string]any)
		p, e := snapshot.ObjectPath(s["type"].(string), s["id"].(string))
		if e != nil {
			t.Fatal(e)
		}
		files[p], _ = json.Marshal(o)
	}
	in, e := snapshot.FromFiles(files, snapshot.Options{Mode: snapshot.TreeMode, Label: env, Environment: env, Types: types})
	if e != nil {
		t.Fatal(e)
	}
	return in
}
func policy() Policy { p := DefaultPolicy(); p.Profile = "isc-portable-v1"; return p }
func runCompare(t *testing.T, l, r snapshot.Input, p Policy) Report {
	t.Helper()
	out, e := Compare(l, r, p, Options{IncludeMatching: true})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestLogicalMatchingAndMissingOnce(t *testing.T) {
	l := input(t, "same", []string{"ROLE"}, obj("ROLE", "a", "Name", nil), obj("ROLE", "missing", "Absent", nil))
	r := input(t, "same", []string{"ROLE"}, obj("ROLE", "different", "Name", nil))
	out := runCompare(t, l, r, policy())
	if out.ExitCode() != 1 || out.Counts != (Counts{MissingRight: 1, Matching: 1}) {
		t.Fatalf("%+v", out)
	}
	out2, e := Compare(l, r, policy(), Options{})
	if e != nil || out2.Counts != out.Counts || len(out2.Entries) != 1 {
		t.Fatal(out2, e)
	}
	p := policy()
	p.Pairings = []Pairing{{ID: "rename", Type: "ROLE", LeftID: "a", RightID: "different"}}
	r = input(t, "same", []string{"ROLE"}, obj("ROLE", "different", "Renamed", nil))
	out = runCompare(t, l, r, p)
	if out.Counts.Different != 1 || out.Counts.MissingRight != 1 || out.Entries[1].PairingRule != "rename" {
		t.Fatal(out)
	}
	dup := input(t, "same", []string{"ROLE"}, obj("ROLE", "a", "Name", nil), obj("ROLE", "b", "Name", nil))
	if runCompare(t, dup, dup, policy()).ExitCode() != 2 {
		t.Fatal("ambiguity looked complete")
	}
	unknown := input(t, "same", []string{"FUTURE"}, obj("FUTURE", "a", "Name", nil))
	if runCompare(t, unknown, unknown, policy()).ExitCode() != 2 {
		t.Fatal("unknown type guessed")
	}
	p = policy()
	p.Identities = []Identity{{ID: "future", Type: "FUTURE", Strategy: "fields", Paths: []string{"/object/name"}}}
	if runCompare(t, unknown, unknown, p).ExitCode() != 0 {
		t.Fatal("explicit identity failed")
	}
}
func TestDependentIdentity(t *testing.T) {
	types := []string{"IDENTITY_PROFILE", "LIFECYCLE_STATE", "SOURCE", "ATTR_SYNC_SOURCE_CONFIG", "NOTIFICATION_TEMPLATE", "AUTH_ORG"}
	makeInput := func(prefix string) snapshot.Input {
		return input(t, "env", types, obj("IDENTITY_PROFILE", prefix+"p1", "Employees", nil), obj("IDENTITY_PROFILE", prefix+"p2", "Guests", nil), obj("SOURCE", prefix+"s", "Directory", nil), obj("LIFECYCLE_STATE", prefix+"l1", "Active", map[string]any{"technicalName": "active", "identityProfileRef": map[string]any{"id": prefix + "p1"}}), obj("LIFECYCLE_STATE", prefix+"l2", "Active", map[string]any{"technicalName": "active", "identityProfileRef": map[string]any{"id": prefix + "p2"}}), obj("ATTR_SYNC_SOURCE_CONFIG", prefix+"a", "", map[string]any{"source": map[string]any{"id": prefix + "s"}}), obj("NOTIFICATION_TEMPLATE", prefix+"n", "", map[string]any{"key": "request", "locale": "en_US", "medium": "EMAIL"}), obj("AUTH_ORG", prefix+"o", "", nil))
	}
	l, r := makeInput("l"), makeInput("r")
	p := policy()
	p.References = []Reference{{ID: "parent", Selector: Selector{Type: "LIFECYCLE_STATE", Path: "/object/identityProfileRef/id"}, TargetType: "IDENTITY_PROFILE"}, {ID: "source", Selector: Selector{Type: "ATTR_SYNC_SOURCE_CONFIG", Path: "/object/source/id"}, TargetType: "SOURCE"}}
	out := runCompare(t, l, r, p)
	if out.ExitCode() != 0 || out.Counts.Matching != 8 {
		t.Fatalf("%+v", out)
	}
	missing := input(t, "env", types, obj("LIFECYCLE_STATE", "x", "Active", map[string]any{"technicalName": "active", "identityProfileRef": map[string]any{"id": "absent"}}))
	if runCompare(t, missing, missing, p).ExitCode() != 2 {
		t.Fatal("unresolved identity accepted")
	}
	single := input(t, "env", []string{"AUTH_ORG"}, obj("AUTH_ORG", "one", "", nil), obj("AUTH_ORG", "two", "", nil))
	if runCompare(t, single, single, policy()).ExitCode() != 2 {
		t.Fatal("singleton cardinality not enforced")
	}
}
func TestStructureNumbersAndIgnores(t *testing.T) {
	cases := []struct {
		name string
		a, b map[string]any
		kind string
	}{
		{"missing", nil, map[string]any{"x": nil}, "missing_left"}, {"null", map[string]any{"x": nil}, map[string]any{"x": ""}, "type"}, {"type", map[string]any{"x": 1}, map[string]any{"x": "1"}, "type"}, {"value", map[string]any{"x": "a"}, map[string]any{"x": "b"}, "value"}, {"precision", map[string]any{"x": json.Number("9007199254740993")}, map[string]any{"x": json.Number("9007199254740992")}, "value"}, {"decimal", map[string]any{"x": json.Number("1.00e2")}, map[string]any{"x": json.Number("100")}, ""}, {"unknown", map[string]any{"future": true}, map[string]any{"future": false}, "value"}, {"ordered", map[string]any{"x": []any{"a", "b"}}, map[string]any{"x": []any{"b", "a"}}, "value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := input(t, "e", []string{"ROLE"}, obj("ROLE", "l", "N", tc.a))
			r := input(t, "e", []string{"ROLE"}, obj("ROLE", "r", "N", tc.b))
			out := runCompare(t, l, r, policy())
			if tc.kind == "" {
				if out.ExitCode() != 0 {
					t.Fatal(out)
				}
			} else if out.Counts.Different != 1 || out.Entries[0].Differences[0].Kind != tc.kind {
				t.Fatal(out)
			}
		})
	}
	p := policy()
	p.Ignores = []Ignore{{ID: "ignore", Selector: Selector{Type: "ROLE", Path: "/object/config/created"}}}
	l := input(t, "e", []string{"ROLE"}, obj("ROLE", "l", "N", map[string]any{"config": map[string]any{"created": 1}, "nested": map[string]any{"id": "x"}}))
	r := input(t, "e", []string{"ROLE"}, obj("ROLE", "r", "N", map[string]any{"config": map[string]any{"created": 2}, "nested": map[string]any{"id": "y"}}))
	out := runCompare(t, l, r, p)
	if out.Counts.Different != 1 || len(out.Entries[0].Differences) != 1 || out.Entries[0].Differences[0].Path != "/object/nested/id" {
		t.Fatal(out)
	}
}
func TestDirectionalTruthTable(t *testing.T) {
	p := policy()
	p.Expectations = []Expectation{{ID: "dn", Selector: Selector{Type: "SOURCE", Path: "/object/dn"}, Values: map[string]any{"production": "dc=example,dc=com", "sandbox": "dc=example-dev,dc=com"}, Required: true}}
	values := []any{"dc=example,dc=com", "dc=example-dev,dc=com", "unrelated", nil, "", struct{}{}}
	for _, envs := range [][2]string{{"production", "sandbox"}, {"sandbox", "production"}, {"production", "production"}, {"sandbox", "sandbox"}} {
		for i, a := range values {
			for j, b := range values {
				fields := func(v any) map[string]any {
					if _, ok := v.(struct{}); ok {
						return nil
					}
					return map[string]any{"dn": v}
				}
				l := input(t, envs[0], []string{"SOURCE"}, obj("SOURCE", "l", "N", fields(a)))
				r := input(t, envs[1], []string{"SOURCE"}, obj("SOURCE", "r", "N", fields(b)))
				out := runCompare(t, l, r, p)
				wantClean := a == p.Expectations[0].Values[envs[0]] && b == p.Expectations[0].Values[envs[1]]
				if (out.ExitCode() == 0) != wantClean || !out.Complete {
					t.Fatalf("env %v values %d/%d: %+v", envs, i, j, out)
				}
				if !wantClean && out.Entries[0].Differences[0].RuleID != "dn" {
					t.Fatal(out)
				}
			}
		}
	}
	l := input(t, "unknown", []string{"SOURCE"}, obj("SOURCE", "l", "N", nil))
	if runCompare(t, l, l, p).ExitCode() != 2 {
		t.Fatal("missing environment accepted")
	}
	p.Expectations[0].Path = "/object/list/*/dn"
	p.Expectations[0].Required = true
	if runCompare(t, l, l, p).ExitCode() != 2 {
		t.Fatal("unused required accepted")
	}
	p.Expectations[0].Path = "/object/dn"
	p.Bindings = map[string]string{"left": "production", "right": "sandbox"}
	if _, e := Compare(l, l, p, Options{}); e == nil {
		t.Fatal("binding conflict accepted")
	}
}
func TestReferencesAndCollections(t *testing.T) {
	types := []string{"ROLE", "SOURCE"}
	mk := func(side, target string, items []any) snapshot.Input {
		return input(t, "e", types, obj("SOURCE", side+"a", "A", nil), obj("SOURCE", side+"b", "B", nil), obj("ROLE", side+"r", "R", map[string]any{"source": target, "items": items}))
	}
	items := []any{map[string]any{"key": "a", "value": 1}, map[string]any{"key": "b", "value": 2}}
	reverse := []any{items[1], items[0]}
	p := policy()
	p.References = []Reference{{ID: "ref", Selector: Selector{Type: "ROLE", Path: "/object/source"}, TargetType: "SOURCE", Required: true}}
	p.Collections = []Collection{{ID: "items", Selector: Selector{Type: "ROLE", Path: "/object/items"}, Keys: []string{"/key"}, Required: true}}
	l, r := mk("l", "la", items), mk("r", "ra", reverse)
	if out := runCompare(t, l, r, p); out.ExitCode() != 0 {
		t.Fatal(out)
	}
	r = mk("r", "rb", reverse)
	out := runCompare(t, l, r, p)
	if out.ExitCode() != 1 || out.Entries[0].Differences[0].Kind != "reference" {
		t.Fatal(out)
	}
	r = mk("r", "external", reverse)
	if runCompare(t, l, r, p).ExitCode() != 2 {
		t.Fatal("unresolved resolver accepted")
	}
	r = mk("r", "ra", []any{items[0], items[0]})
	if runCompare(t, l, r, p).ExitCode() != 2 {
		t.Fatal("duplicate collection key accepted")
	}
	p.Collections = nil
	r = mk("r", "ra", reverse)
	if runCompare(t, l, r, p).ExitCode() != 1 {
		t.Fatal("ordered arrays changed semantics")
	}
}
func TestPoliciesScopesAndDeterminism(t *testing.T) {
	for _, raw := range []string{`{"version":1,"id":"x","typo":true}`, `{"version":1,"id":"x","ignores":[{"id":"x","type":"ROLE","path":"/object/name"}]}`, `{"version":1,"id":"x","expectations":[{"id":"x","type":"ROLE","path":"/bad~3","values":{"e":1}}]}`, `{"version":1,"version":1,"id":"x"}`} {
		if _, e := DecodePolicy([]byte(raw)); e == nil {
			t.Fatal(raw)
		}
	}
	p := policy()
	p.Ignores = []Ignore{{ID: "i", Selector: Selector{Type: "ROLE", Path: "/object/x"}}}
	p.Expectations = []Expectation{{ID: "e", Selector: Selector{Type: "ROLE", Path: "/object/x"}, Values: map[string]any{"e": nil}}}
	if p.Validate() == nil {
		t.Fatal("overlap accepted")
	}
	l := input(t, "e", []string{"ROLE"}, obj("ROLE", "l", "N", nil))
	r := input(t, "e", []string{"ROLE", "SOURCE"}, obj("ROLE", "r", "N", nil))
	if _, e := Compare(l, r, policy(), Options{}); e == nil {
		t.Fatal("scope mismatch accepted")
	}
	if out, e := Compare(l, r, policy(), Options{Scope: []string{"ROLE"}}); e != nil || out.ExitCode() != 0 || !reflect.DeepEqual(out.ExcludedRight, []string{"SOURCE"}) {
		t.Fatal(out, e)
	}
	if _, e := Compare(l, r, policy(), Options{Scope: []string{"SOURCE"}}); e == nil {
		t.Fatal("absent scope treated empty")
	}
	r = input(t, "e", []string{"ROLE"}, obj("ROLE", "r", "N", map[string]any{"secret": "secret-do-not-print"}))
	a := runCompare(t, l, r, policy())
	b := runCompare(t, l, r, policy())
	ar, _ := json.Marshal(a)
	br, _ := json.Marshal(b)
	if string(ar) != string(br) || strings.Contains(string(ar), "secret-do-not-print") {
		t.Fatal("report not deterministic/redacted")
	}
}
func TestExporterEndToEndCompatibility(t *testing.T) {
	root := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"tenant": "invented", "objects": []any{obj("ROLE", "one", "Synthetic", nil)}})
	snap, e := exporter.Organize("invented", exporter.Download{JobID: "synthetic-job", Types: []string{"ROLE", "SOURCE"}, Raw: raw}, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = (exporter.FileStore{Output: root}).Publish(context.Background(), snap); e != nil {
		t.Fatal(e)
	}
	l, e := snapshot.Read(filepath.Join(root, "invented"), snapshot.Options{Mode: snapshot.SnapshotMode, Label: "export"})
	if e != nil {
		t.Fatal(e)
	}
	r, e := snapshot.Read(filepath.Join(root, "invented"), snapshot.Options{Mode: snapshot.TreeMode, Label: "git"})
	if e != nil {
		t.Fatal(e)
	}
	if out := runCompare(t, l, r, DefaultPolicy()); out.ExitCode() != 0 || out.Counts.Matching != 1 {
		t.Fatal(out)
	}
	if _, e = os.Stat(filepath.Join(root, "invented", "manifest.json")); e != nil {
		t.Fatal(e)
	}
}

func TestTypedNullExpectationAndInvalidResult(t *testing.T) {
	p := policy()
	p.Expectations = []Expectation{{ID: "null", Selector: Selector{Type: "ROLE", Path: "/object/x"}, Values: map[string]any{"e": nil}}}
	present := input(t, "e", []string{"ROLE"}, obj("ROLE", "a", "N", map[string]any{"x": nil}))
	absent := input(t, "e", []string{"ROLE"}, obj("ROLE", "b", "N", nil))
	if runCompare(t, present, present, p).ExitCode() != 0 || runCompare(t, present, absent, p).ExitCode() != 1 {
		t.Fatal("null and missing collapsed")
	}
	bad, e := Compare(snapshot.Input{}, snapshot.Input{}, p, Options{})
	if e == nil || bad.ExitCode() != 2 || bad.Complete {
		t.Fatal("invalid comparison returned a clean result")
	}
	// Property aliases, null booleans, duplicate IDs, pair endpoints, custom
	// identity ignores, and active profile overlaps are rejected at policy load.
	for _, raw := range []string{
		`{"version":1,"id":"x","Version":2}`,
		`{"version":1,"id":"x","ignores":[{"id":"i","type":"ROLE","path":"/object/x","required":null}]}`,
		`{"version":1,"id":"x","ignores":[{"id":"i","type":"ROLE","path":"/object/x"},{"id":"i","type":"ROLE","path":"/object/y"}]}`,
		`{"version":1,"id":"x","pairings":[{"id":"a","type":"ROLE","left_id":"a","right_id":"x"},{"id":"b","type":"ROLE","left_id":"a","right_id":"y"}]}`,
		`{"version":1,"id":"x","identities":[{"id":"a","type":"FUTURE","strategy":"fields","paths":["/object/custom"]}],"ignores":[{"id":"i","type":"FUTURE","path":"/object/custom"}]}`,
		`{"version":1,"id":"x","profile":"isc-portable-v1","references":[{"id":"r","type":"ROLE","path":"/object/id","target_type":"ROLE"}]}`,
	} {
		if _, e := DecodePolicy([]byte(raw)); e == nil {
			t.Fatal(raw)
		}
	}
}
func TestExternalReferenceMapping(t *testing.T) {
	l := input(t, "e", []string{"ROLE"}, obj("ROLE", "a", "N", map[string]any{"owner": "external-left"}))
	r := input(t, "e", []string{"ROLE"}, obj("ROLE", "b", "N", map[string]any{"owner": "external-right"}))
	p := policy()
	p.References = []Reference{{ID: "owner", Selector: Selector{Type: "ROLE", Path: "/object/owner"}, TargetType: "IDENTITY", External: map[string]string{"external-left": "external-right"}}}
	if runCompare(t, l, r, p).ExitCode() != 0 {
		t.Fatal("external reference mapping failed")
	}
	p.References[0].External["external-left"] = "wrong"
	if runCompare(t, l, r, p).ExitCode() != 1 {
		t.Fatal("wrong external target accepted")
	}
}
