package v2

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func TestContainerPresenceCannotBeHiddenByChildIgnore(t *testing.T) {
	p := testPolicy()
	p.Ignores = []Ignore{{Rule: testRule("ignore"), Selector: Selector{Type: "THING", Path: "/branch/value"}}}
	req := Request{Mode: "drift", Left: testInput("e", doc(map[string]any{"branch": map[string]any{"value": "ignored"}})), Right: testInput("e", doc(nil))}
	if r := runTest(t, req, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
	p.Collections = []Collection{{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/key"}}}
	p.Ignores[0].Path = "/items/*/key"
	req.Left = testInput("e", doc(map[string]any{"items": []any{map[string]any{"key": "a"}}}))
	req.Right = testInput("e", doc(map[string]any{"items": []any{map[string]any{"key": "b"}}}))
	if r := runTest(t, req, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
	p = testPolicy()
	p.Expectations = []Expectation{{Rule: testRule("absent"), Selector: Selector{Type: "THING", Path: "/scalar/child"}, Operation: "absent", Authority: "approved"}}
	req.Left = testInput("e", doc(map[string]any{"scalar": 1}))
	req.Right = testInput("e", doc(map[string]any{"scalar": 2}))
	if r := runTest(t, req, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
}
func TestTypedIdentityAndDuplicateNumbers(t *testing.T) {
	p := testPolicy()
	p.Identities[0].Paths = []string{"/key"}
	req := Request{Mode: "drift", Left: testInput("e", map[string]any{"key": json.Number("1")}), Right: testInput("e", map[string]any{"key": valueKey([]any{json.Number("1")})})}
	if r := runTest(t, req, p); r.ExitCode() == 0 {
		t.Fatal("typed identity collision", r)
	}
	req.Left = testInput("e", map[string]any{"key": json.Number("1")}, map[string]any{"key": json.Number("1.0")})
	req.Right = req.Left
	if r := runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal("duplicate numeric identity", r)
	}
}
func TestIndependentReferenceExpectationAndEvidence(t *testing.T) {
	p := testPolicy()
	p.Identities = append(p.Identities, Identity{Rule: testRule("target-identity"), Type: "TARGET", Strategy: "fields", Paths: []string{"/name"}})
	p.Expectations = []Expectation{{Rule: testRule("name"), Selector: Selector{Type: "THING", Path: "/name"}, Operation: "copy_source", Authority: "approved"}, {Rule: testRule("reference"), Selector: Selector{Type: "THING", Path: "/ref"}, Operation: "reference_target", TargetType: "TARGET", Authority: "approved"}, {Rule: testRule("metadata"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"}}
	makeIn := func(prefix, ref, target string) Input {
		in := testInput(prefix, doc(map[string]any{"ref": ref}))
		in.Types = append(in.Types, "TARGET")
		in.Documents = append(in.Documents, Document{Type: "TARGET", ID: target, Content: map[string]any{"name": "target"}, Metadata: map[string]any{}})
		return in
	}
	req := Request{Mode: "deployment", Left: makeIn("source", "source-target", "source-target"), Right: makeIn("actual", "draft-wrong", "draft-wrong"), Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}
	// Ignore the out-of-operation target draft as an additional diagnostic, but
	// independently verify that the reference itself is never called correct.
	r := runTest(t, req, p)
	if r.Counts["verified"] != 2 || r.Counts["unverified"] == 0 {
		t.Fatal(r)
	}
	b := makeIn("baseline", "baseline-target", "baseline-target")
	req.Baseline = &b
	r = runTest(t, req, p)
	if r.Counts["violation"] != 1 {
		t.Fatal("self-consistent draft concealed wrong ID", r)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "reference" && d.Status == "violation" {
			found = d.Derivation != nil && d.Derivation.Role == "baseline" && d.Derivation.Location.ID == "baseline-target"
		}
	}
	if !found {
		t.Fatal("lost independent expectation evidence", r.Decisions)
	}
}
func TestDeleteAndMissingOperationCoverage(t *testing.T) {
	p := testPolicy()
	baseline := testInput("e", doc(nil))
	req := Request{Mode: "deployment", Left: testInput("e"), Right: testInput("e"), Baseline: &baseline, Operations: []Operation{{Type: "THING", Key: "invented", Action: "delete"}}}
	if r := runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal(r)
	}
	req.Baseline = nil
	if r := runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal(r)
	}
	req.Right = baseline
	if r := runTest(t, req, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
}
func TestCompiledPlanAndReportsAreDetached(t *testing.T) {
	p := testPolicy()
	p.Ignores = []Ignore{{Rule: testRule("ignore"), Selector: Selector{Type: "THING", Path: "/value"}}}
	profile := Profile{ID: "generic", Rules: Policy{Identities: p.Identities}}
	p.Identities = nil
	p.Profile = "generic"
	c, e := Compile(p, profile)
	if e != nil {
		t.Fatal(e)
	}
	req := Request{Mode: "drift", Left: testInput("e", doc(map[string]any{"value": "a"})), Right: testInput("e", doc(map[string]any{"value": "b"}))}
	r, e := Run(req, c)
	if e != nil || r.ExitCode() != 0 {
		t.Fatal(r, e)
	}
	before := stable(r)
	p.Ignores[0].Evidence.Kind = "mutated"
	profile.Rules.Identities[0].Paths[0] = "/mutated"
	r.Decisions[0].Evidence.Kind = "report-mutated"
	next, e := Run(req, c)
	if e != nil || stable(next) != before {
		t.Fatal("compiled plan shared caller/report state", e)
	}
}
func TestUsageOmissionAndDeterminism(t *testing.T) {
	p := testPolicy()
	p.Sensitive = []Sensitive{{Rule: Rule{ID: "sensitive", Required: true}, Selector: Selector{Type: "THING", Path: "/name"}}}
	req := Request{Mode: "drift", Left: testInput("e", doc(nil)), Right: testInput("e", doc(nil))}
	a := runTest(t, req, p)
	req.IncludeMatching = true
	b := runTest(t, req, p)
	if a.ExitCode() != 0 || b.ExitCode() != 0 || !reflect.DeepEqual(a.Counts, b.Counts) || a.RequestDigest != b.RequestDigest {
		t.Fatal(a, b)
	}
	if a.RuleUsage["sensitive"] == 0 {
		t.Fatal("sensitive rule incorrectly unused")
	}
	if stable(a) != stable(runTest(t, Request{Mode: "drift", Left: req.Left, Right: req.Right}, p)) {
		t.Fatal("nondeterministic")
	}
}
func TestMultisetCannotHideNestedCollectionRules(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("outer"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "multiset"}, {Rule: testRule("inner"), Selector: Selector{Type: "THING", Path: "/items/*/children"}, Mode: "keyed", Keys: []string{"/key"}}}
	if _, e := Compile(p); e == nil {
		t.Fatal("multiset hid nested collection")
	}
}

func TestCompiledPlanConcurrentReuse(t *testing.T) {
	p := testPolicy()
	c := compileTest(t, p)
	req := Request{Mode: "drift", Left: testInput("e", doc(nil)), Right: testInput("e", doc(nil))}
	want, e := Run(req, c)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				r, e := Run(req, c)
				if e != nil || stable(r) != stable(want) {
					t.Error("concurrent result changed", e)
				}
			}
		}()
	}
	wg.Wait()
}

func TestInvalidRequestReportCannotPass(t *testing.T) {
	req := Request{Mode: "deployment", Left: testInput("e", doc(nil)), Right: testInput("e", doc(nil)), Operations: []Operation{{Type: "THING", Key: "invented", Action: "invented-action"}}}
	r, e := Run(req, compileTest(t, testPolicy()))
	if e == nil || r.ExitCode() != 2 {
		t.Fatal(r, e)
	}
	req.Mode = "drift"
	req.Operations = nil
	r, e = Run(req, &Compiled{})
	if e == nil || r.ExitCode() != 2 {
		t.Fatal(r, e)
	}
}
func TestModeDisjointArrayRules(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: Rule{ID: "bag", Modes: []string{"drift"}}, Selector: Selector{Type: "THING", Path: "/items"}, Mode: "multiset"}}
	p.Expectations = []Expectation{{Rule: Rule{ID: "expected", Modes: []string{"deployment"}, Evidence: &Evidence{Kind: "approved"}}, Selector: Selector{Type: "THING", Path: "/items"}, Operation: "literal", Expected: present([]any{}), Authority: "approved"}}
	compileTest(t, p)
}
