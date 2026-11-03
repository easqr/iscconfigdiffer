package v2

import (
	"encoding/json"
	"strings"
	"testing"
)

func testInput(env string, content ...any) Input {
	in := Input{Environment: env, Types: []string{"THING"}}
	for i, c := range content {
		in.Documents = append(in.Documents, Document{Type: "THING", ID: env + string(rune('a'+i)), Path: "invented.json", Content: c, Metadata: map[string]any{}})
	}
	return in
}
func doc(extra map[string]any) map[string]any {
	out := map[string]any{"name": "invented"}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
func runTest(t *testing.T, req Request, p Policy) Report {
	t.Helper()
	r, e := Run(req, compileTest(t, p))
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestDriftEquivalenceAssertionsAndImmutability(t *testing.T) {
	p := testPolicy()
	p.Equivalences = []Equivalence{{Rule: testRule("host"), Selector: Selector{Type: "THING", Path: "/host"}, LeftEnv: "dev", RightEnv: "prod", Pairs: []ValuePair{{Left: "dev-host", Right: "prod-host"}}}}
	left, right := testInput("dev", doc(map[string]any{"host": "dev-host"})), testInput("prod", doc(map[string]any{"host": "prod-host"}))
	req := Request{Mode: "drift", Left: left, Right: right}
	before := stable(left.Documents)
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 {
		t.Fatal(r)
	}
	if before != stable(left.Documents) {
		t.Fatal("input mutated")
	}
	req.Right = testInput("prod", doc(map[string]any{"host": "wrong-host"}))
	if r = runTest(t, req, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
	p.Equivalences = nil
	p.Expectations = []Expectation{{Rule: testRule("host"), Selector: Selector{Type: "THING", Path: "/host"}, Operation: "literal", Authority: "approved", Values: map[string]State{"dev": *present("dev-host"), "prod": *present("prod-host")}}}
	req.Left = testInput("dev", doc(map[string]any{"host": "wrong-host"}))
	if r = runTest(t, req, p); r.Counts["violation"] != 2 || r.ExitCode() != 1 {
		t.Fatal(r)
	}
	p.Expectations[0].Authority = ""
	if r = runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal(r)
	}
}
func TestNestedCollectionsAndPhysicalPointers(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("outer"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/key"}}, {Rule: testRule("inner"), Selector: Selector{Type: "THING", Path: "/items/*/children"}, Mode: "keyed", Keys: []string{"/key"}}}
	p.Expectations = []Expectation{{Rule: testRule("assert"), Selector: Selector{Type: "THING", Path: "/items/*/children/*/value"}, Operation: "literal", Values: map[string]State{"dev": *present("d"), "prod": *present("p")}, Authority: "approved"}}
	item := func(k, v string) any {
		return map[string]any{"key": k, "children": []any{map[string]any{"key": "child", "value": v}}}
	}
	l := testInput("dev", doc(map[string]any{"items": []any{item("a", "d"), item("b", "d")}}))
	rr := testInput("prod", doc(map[string]any{"items": []any{item("b", "p"), item("a", "p")}}))
	r := runTest(t, Request{Mode: "drift", Left: l, Right: rr}, p)
	if r.ExitCode() != 0 || r.Counts["verified"] != 4 {
		t.Fatal(r)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "assert" && d.Locations["left"].Pointer == "/items/0/children/0/value" && d.Locations["right"].Pointer == "/items/1/children/0/value" {
			found = true
		}
	}
	if !found {
		t.Fatal("lost physical pointers", r.Decisions)
	}
	rr = testInput("prod", doc(map[string]any{"items": []any{item("a", "p"), item("a", "p")}}))
	if r = runTest(t, Request{Mode: "drift", Left: l, Right: rr}, p); r.ExitCode() != 2 {
		t.Fatal(r)
	}
}
func TestMultisetsAndExactValues(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("bag"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "multiset"}}
	l := testInput("e", doc(map[string]any{"items": []any{json.Number("1"), json.Number("1.0"), "x"}}))
	rr := testInput("e", doc(map[string]any{"items": []any{"x", json.Number("1e0"), json.Number("1")}}))
	req := Request{Mode: "drift", Left: l, Right: rr}
	if r := runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal(r)
	}
	req.Right = testInput("e", doc(map[string]any{"items": []any{"x", json.Number("1")}}))
	if r := runTest(t, req, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
	for _, pair := range [][2]any{{nil, ""}, {json.Number("9007199254740992"), json.Number("9007199254740993")}, {json.Number("1"), "1"}, {map[string]any{}, []any{}}} {
		req.Left = testInput("e", doc(map[string]any{"value": pair[0]}))
		req.Right = testInput("e", doc(map[string]any{"value": pair[1]}))
		if r := runTest(t, req, testPolicy()); r.ExitCode() != 1 {
			t.Fatal(pair, r)
		}
	}
}
func TestDeploymentExpectations(t *testing.T) {
	base := testPolicy()
	base.Expectations = []Expectation{{Rule: testRule("content"), Selector: Selector{Type: "THING", Path: ""}, Operation: "copy_source", Authority: "approved"}, {Rule: testRule("meta"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"}}
	req := Request{Mode: "deployment", Left: testInput("dev", doc(nil)), Right: testInput("prod", doc(nil)), Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}
	if r := runTest(t, req, base); r.ExitCode() != 0 {
		t.Fatal(r)
	}
	// Cover the name separately, allowing explicit field assertions below.
	base.Expectations[0].Path = "/name"
	base.Expectations = append(base.Expectations, Expectation{Rule: testRule("value"), Selector: Selector{Type: "THING", Path: "/value"}, Operation: "literal", Expected: present("expected"), Authority: "approved"})
	for _, tc := range []struct {
		op      string
		actual  any
		missing bool
		want    int
	}{{"literal", "expected", false, 0}, {"literal", "source", false, 1}, {"absent", nil, true, 0}, {"absent", nil, false, 1}, {"copy_source", "source", false, 0}, {"preserve_target", "baseline", false, 0}} {
		p := base
		p.Expectations = append([]Expectation{}, base.Expectations...)
		p.Expectations[2].Operation = tc.op
		if tc.op != "literal" {
			p.Expectations[2].Expected = nil
		}
		req.Left = testInput("dev", doc(map[string]any{"value": "source"}))
		extra := map[string]any{}
		if !tc.missing {
			extra["value"] = tc.actual
		}
		req.Right = testInput("prod", doc(extra))
		b := testInput("prod", doc(map[string]any{"value": "baseline"}))
		req.Baseline = &b
		r := runTest(t, req, p)
		if r.ExitCode() != tc.want {
			t.Fatalf("%s: %+v", tc.op, r)
		}
		if tc.op == "preserve_target" {
			req.Baseline = nil
			if r = runTest(t, req, p); r.ExitCode() != 2 {
				t.Fatal(r)
			}
		}
	}
	req.Baseline = nil
	req.Left = testInput("dev", doc(nil))
	req.Right = testInput("prod", doc(nil))
	if r := runTest(t, req, testPolicy()); r.ExitCode() != 2 || r.Counts["unverified"] == 0 {
		t.Fatal(r)
	}
}
func TestMissingLiteralRequiredCoverageAndSensitive(t *testing.T) {
	p := testPolicy()
	p.Expectations = []Expectation{{Rule: testRule("null"), Selector: Selector{Type: "THING", Path: "/missing"}, Operation: "literal", Expected: present(nil), Authority: "approved"}}
	req := Request{Mode: "drift", Left: testInput("e", doc(nil)), Right: testInput("e", doc(nil))}
	if r := runTest(t, req, p); r.ExitCode() != 1 || r.Counts["violation"] != 2 {
		t.Fatal(r)
	}
	p.Expectations[0].Path = "/items/*/missing"
	p.Expectations[0].Required = true
	if r := runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal(r)
	}
	p.Expectations = nil
	p.Sensitive = []Sensitive{{Rule: testRule("secret"), Selector: Selector{Type: "THING", Path: "/name"}}}
	r := runTest(t, req, p)
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "invented.json") || strings.Contains(string(b), `"key":"invented"`) {
		t.Fatal("identity leaked", string(b))
	}
}
