package v2

import (
	"encoding/json"
	"testing"
)

func TestNullHandlingDefaultAndOptIn(t *testing.T) {
	p := testPolicy()
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"optional": nil})), Right: testInput("prod", doc(nil))}
	strict := runTest(t, req, p)
	if strict.ExitCode() != 1 || strict.NullHandling != "strict" {
		t.Fatal("neutral default not strict", strict)
	}
	p.NullHandling = "null_equals_absent"
	before := stable(req.Left)
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 || r.NullHandling != "null_equals_absent" || r.PolicyDigest == strict.PolicyDigest {
		t.Fatal(r)
	}
	d := r.Decisions[0]
	if d.Normalization != "null_equals_absent" || d.Locations["left"].Value.State != "present" || string(d.Locations["left"].Value.Value) != "null" || d.Locations["right"].Value.State != "absent" {
		t.Fatal("original representations lost", d)
	}
	if before != stable(req.Left) {
		t.Fatal("input mutated")
	}
	p.NullHandling = "invalid"
	if _, err := Compile(p); err == nil {
		t.Fatal("invalid setting accepted")
	}
	p.NullHandling = ""
	p.Profile = "invented-profile"
	if _, err := Compile(p, Profile{ID: p.Profile, Rules: Policy{NullHandling: "invalid"}}); err == nil {
		t.Fatal("invalid profile setting accepted")
	}
}

func TestNullHandlingPreservesArrayMembersAndOtherValues(t *testing.T) {
	p := testPolicy()
	p.NullHandling = "null_equals_absent"
	for _, value := range []any{"", false, json.Number("0"), map[string]any{}, []any{}, []any{nil}} {
		r := runTest(t, Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"field": value})), Right: testInput("prod", doc(nil))}, p)
		if r.ExitCode() != 1 {
			t.Fatalf("%#v became absent: %+v", value, r)
		}
	}
	r := runTest(t, Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{nil}})), Right: testInput("prod", doc(map[string]any{"items": []any{}}))}, p)
	if r.ExitCode() != 1 {
		t.Fatal("null array member disappeared", r)
	}
	// Metadata retains strict envelope semantics.
	left, right := testInput("dev", doc(nil)), testInput("prod", doc(nil))
	left.Documents[0].Metadata = map[string]any{"field": nil}
	r = runTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
	if r.ExitCode() != 1 {
		t.Fatal("metadata normalized", r)
	}
	// A null array key is still invalid, not an absent/nullable identity.
	p.Collections = []Collection{{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/key"}}}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{map[string]any{"key": nil}}})), Right: testInput("prod", doc(map[string]any{"items": []any{map[string]any{"key": nil}}}))}
	if r = runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal("null collection key accepted", r)
	}
}

func TestWholeExpectationsAndNullHandling(t *testing.T) {
	for _, operation := range []string{"literal", "copy_source", "preserve_target", "absent"} {
		t.Run(operation, func(t *testing.T) {
			p := testPolicy()
			p.NullHandling = "null_equals_absent"
			path := ""
			var expected *State
			if operation == "literal" {
				expected = present(map[string]any{"name": "invented", "optional": nil, "nested": map[string]any{"optional": nil}, "items": []any{map[string]any{"key": "one", "optional": nil}}})
			}
			if operation == "absent" {
				path = "/optional"
			}
			p.Expectations = []Expectation{{Rule: testRule("check"), Selector: Selector{Type: "THING", Path: path}, Operation: operation, Expected: expected, Authority: "approved"}}
			if path != "" {
				p.Expectations = append(p.Expectations, Expectation{Rule: testRule("name"), Selector: Selector{Type: "THING", Path: "/name"}, Operation: "copy_source", Authority: "approved"})
			}
			p.Expectations = append(p.Expectations, Expectation{Rule: testRule("meta"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"})
			left := testInput("dev", doc(map[string]any{"optional": nil, "nested": map[string]any{"optional": nil}, "items": []any{map[string]any{"key": "one", "optional": nil}}}))
			right := testInput("prod", doc(map[string]any{"nested": map[string]any{}, "items": []any{map[string]any{"key": "one"}}}))
			if operation == "absent" {
				left = testInput("dev", doc(nil))
				right = testInput("prod", doc(map[string]any{"optional": nil}))
			}
			req := Request{Mode: "deployment", Left: left, Right: right, Baseline: &left, Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}
			r := runTest(t, req, p)
			if r.ExitCode() != 0 {
				t.Fatal(r)
			}
			used := false
			for _, d := range r.Decisions {
				if d.RuleID == "check" && d.Normalization == "null_equals_absent" {
					used = true
				}
			}
			if !used {
				t.Fatal("normalization provenance absent", r)
			}
			p.NullHandling = "strict"
			if r = runTest(t, req, p); r.ExitCode() != 1 {
				t.Fatal("strict override ineffective", r)
			}
		})
	}
}

func TestNullHandlingMultisetsReferencesAndAuthority(t *testing.T) {
	p := testPolicy()
	p.NullHandling = "null_equals_absent"
	p.Collections = []Collection{{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "multiset"}}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{map[string]any{"name": "one", "optional": nil}}})), Right: testInput("prod", doc(map[string]any{"items": []any{map[string]any{"name": "one"}}}))}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Decisions[0].Normalization != "null_equals_absent" {
		t.Fatal(r)
	}
	p.Collections = nil
	p.References = []Reference{{Rule: testRule("optional-ref"), Selector: Selector{Type: "THING", Path: "/ref"}, TargetType: "THING"}}
	req.Left = testInput("dev", doc(map[string]any{"ref": nil}))
	req.Right = testInput("prod", doc(nil))
	if r = runTest(t, req, p); r.ExitCode() != 0 || r.Counts["equivalent"] != 1 {
		t.Fatal("optional reference not normalized", r)
	}
	p.References = nil
	req.Mode = "deployment"
	req.Operations = []Operation{{Type: "THING", Key: "invented", Action: "update"}}
	if r = runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal("normalization supplied deployment authority", r)
	}
	// Source guards use the same property semantics as expectation validation.
	guardRule := testRule("guard")
	guardRule.Modes = []string{"deployment"}
	p.Expectations = []Expectation{{Rule: guardRule, Selector: Selector{Type: "THING", Path: "/ref"}, Operation: "literal", Expected: present(nil), SourceEquals: &State{State: "absent"}, Authority: "approved"}, {Rule: testRule("name"), Selector: Selector{Type: "THING", Path: "/name"}, Operation: "copy_source", Authority: "approved"}, {Rule: testRule("meta"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"}}
	if r = runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("null/absent guard mismatch", r)
	}
}
