package v2

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func exceptionPolicy() Policy {
	p := testPolicy()
	p.Exceptions = []Exception{{Equivalence: Equivalence{Rule: testRule("known-host"), Selector: Selector{Type: "THING", Path: "/host"}, LeftEnv: "dev", RightEnv: "prod", Pairs: []ValuePair{{Left: "old", Right: "new"}}}, Rationale: "Migration pending <approval>"}}
	return p
}

func TestExceptionsRemainDistinct(t *testing.T) {
	p := exceptionPolicy()
	p.Equivalences = []Equivalence{{Rule: testRule("environment"), Selector: Selector{Type: "THING", Path: "/region"}, LeftEnv: "dev", RightEnv: "prod", Groups: []map[string]any{{"dev": "east", "prod": "west"}}}}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"host": "old", "region": "east"})), Right: testInput("prod", doc(map[string]any{"host": "new", "region": "west"}))}
	before := stable(p)
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["exception"] != 1 || r.Counts["equivalent"] != 1 || stable(p) != before {
		t.Fatal(r)
	}
	for _, d := range r.Decisions {
		if d.RuleID == "known-host" && (d.Status != "exception" || d.Rationale != p.Exceptions[0].Rationale || d.Code != "accepted known difference") {
			t.Fatal(d)
		}
		if d.RuleID == "environment" && d.Rationale != "" {
			t.Fatal(d)
		}
	}
	var md, html bytes.Buffer
	if err := r.Markdown(&md); err != nil {
		t.Fatal(err)
	}
	if err := r.HTML(&html); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md.String(), "Rationale: Migration pending") || !strings.Contains(html.String(), "Migration pending &lt;approval&gt;") {
		t.Fatal("rationale missing or HTML unescaped")
	}
	compiled := compileTest(t, p)
	p.Exceptions[0].Rationale = "Another reason"
	if compiled.Digest() == compileTest(t, p).Digest() {
		t.Fatal("rationale missing from digest")
	}
	req.Right = testInput("prod", doc(map[string]any{"host": "unexpected", "region": "west"}))
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["exception"] != 0 {
		t.Fatal(r)
	}
	req.Right.Environment = "other"
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["exception"] != 0 {
		t.Fatal(r)
	}
}

func TestExceptionRationaleValidation(t *testing.T) {
	rule := `{"id":"known","type":"THING","path":"/host","left_env":"dev","right_env":"prod","pairs":[{"left":"old","right":"new"}]`
	for _, suffix := range []string{`}`, `,"rationale":null}`, `,"rationale":42}`, `,"rationale":false}`} {
		if _, err := Decode([]byte(`{"version":2,"id":"p","exceptions":[` + rule + suffix + `]}`)); err == nil {
			t.Fatal("invalid rationale accepted", suffix)
		}
	}
	for _, rationale := range []string{"", " \t\n"} {
		p := exceptionPolicy()
		p.Exceptions[0].Rationale = rationale
		if _, err := Compile(p); err == nil || !strings.Contains(err.Error(), "rationale") {
			t.Fatal(err)
		}
	}
	p := exceptionPolicy()
	b, _ := json.Marshal(p)
	if _, err := Decode(b); err != nil {
		t.Fatal(err)
	}
	// A rationale is not an equivalence property.
	if _, err := Decode([]byte(`{"version":2,"id":"p","equivalences":[` + rule + `,"rationale":"known"}]}`)); err == nil {
		t.Fatal("equivalence accepted rationale")
	}
}

func TestExceptionMatchingFormsAndScopes(t *testing.T) {
	for _, form := range []string{"pairs", "groups", "state_pairs"} {
		t.Run(form, func(t *testing.T) {
			p := exceptionPolicy()
			e := &p.Exceptions[0]
			if form == "groups" {
				e.Pairs = nil
				e.Groups = []map[string]any{{"dev": "old", "prod": "new"}}
			}
			if form == "state_pairs" {
				e.Pairs = nil
				e.Path = "/settings/host"
				e.StatePairs = []StatePair{{Left: *present("old"), Right: State{State: "absent"}}}
			}
			left, right := doc(map[string]any{"host": "old"}), doc(map[string]any{"host": "new"})
			if form == "state_pairs" {
				left = doc(map[string]any{"settings": map[string]any{"host": "old"}})
				right = doc(map[string]any{"settings": map[string]any{}})
			}
			req := Request{Mode: "drift", Left: testInput("dev", left), Right: testInput("prod", right), ExcludeValues: true}
			if r := runTest(t, req, p); r.ExitCode() != 0 || r.Counts["exception"] != 1 {
				t.Fatal(r)
			}
			e.Key = "other"
			if r := runTest(t, req, p); r.ExitCode() != 1 || r.Counts["exception"] != 0 {
				t.Fatal(r)
			}
			e.Key = ""
			e.RightTenant = "required"
			if r := runTest(t, req, p); r.ExitCode() != 2 || r.Counts["exception"] != 0 {
				t.Fatal(r)
			}
		})
	}
}

func TestExceptionConflictsAndItemValidation(t *testing.T) {
	for _, kind := range []string{"equivalence", "ignore", "identity", "deployment", "ambiguous", "item"} {
		t.Run(kind, func(t *testing.T) {
			p := exceptionPolicy()
			switch kind {
			case "equivalence":
				e := p.Exceptions[0].Equivalence
				e.ID = "environment"
				p.Equivalences = []Equivalence{e}
			case "ignore":
				p.Ignores = []Ignore{{Rule: testRule("ignore"), Selector: p.Exceptions[0].Selector}}
			case "identity":
				p.Exceptions[0].Path = "/name"
			case "deployment":
				p.Exceptions[0].Modes = []string{"deployment"}
			case "ambiguous":
				p.Exceptions[0].Pairs = append(p.Exceptions[0].Pairs, p.Exceptions[0].Pairs[0])
			case "item":
				p.Exceptions[0].Item = &CollectionItemSelector{Collection: "unknown"}
			}
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid exception accepted")
			}
		})
	}
	// Shared item syntax still distinguishes disjoint exception/equivalence scopes.
	p := scopedItemPolicy()
	for _, e := range p.Equivalences {
		p.Exceptions = append(p.Exceptions, Exception{Equivalence: e, Rationale: "Known item discrepancy"})
	}
	p.Equivalences = nil
	if _, err := Compile(p); err != nil {
		t.Fatal(err)
	}
}

func TestExceptionRelativeOrderAndDisjointCategories(t *testing.T) {
	p, err := Decode([]byte(`{"version":2,"id":"p","identities":[{"id":"identity","type":"THING","strategy":"fields","paths":["/name"]}],"exceptions":[{"id":"order","type":"THING","path":"/priority","operation":"relative_order","rationale":"Priority gaps accepted pending migration"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"priority": 1})), Right: testInput("prod", doc(map[string]any{"priority": 10}))}
	if r := runTest(t, req, p); r.ExitCode() != 0 || r.Counts["exception"] != 1 {
		t.Fatal(r)
	}
	p = scopedItemPolicy()
	p.Exceptions = []Exception{{Equivalence: p.Equivalences[0], Rationale: "Accepted item discrepancy"}}
	p.Equivalences = p.Equivalences[1:]
	r := runTest(t, scopedItemRequest(), p)
	if r.Counts["exception"] != 1 || r.Counts["equivalent"] != 1 {
		t.Fatal(r.Counts)
	}
	p.Exceptions[0].Item = p.Equivalences[0].Item
	if _, err := Compile(p); err == nil {
		t.Fatal("overlapping exception/equivalence accepted")
	}
}

func subtreeExceptionPolicy() (Policy, map[string]any) {
	p := testPolicy()
	action := map[string]any{"action": "DISABLE", "sources": []any{map[string]any{"id": "dev-source", "name": "Directory"}}}
	p.Collections = []Collection{{Rule: testRule("sources"), Selector: Selector{Type: "THING", Path: "/actions/*/sources"}, Mode: "keyed", Keys: []string{"/name"}}}
	p.Ignores = []Ignore{{Rule: testRule("source-id"), Selector: Selector{Type: "THING", Path: "/actions/*/sources/*/id"}}}
	p.Exceptions = []Exception{{Equivalence: Equivalence{Rule: testRule("pending-action"), Selector: Selector{Type: "THING", Key: "invented", Path: "/actions/0"}, LeftEnv: "dev", RightEnv: "prod", StatePairs: []StatePair{{Left: *present(action), Right: State{State: "absent"}}}}, Rationale: "Waiting deployment approval"}}
	return p, action
}

func TestExactSubtreeExceptionsWithNestedStructuralRules(t *testing.T) {
	p, action := subtreeExceptionPolicy()
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"actions": []any{action}})), Right: testInput("prod", doc(map[string]any{"actions": []any{}}))}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["exception"] != 1 || r.Counts["different"] != 0 {
		t.Fatal(r)
	}
	for _, d := range r.Decisions {
		if d.RuleID == "pending-action" && (d.Rationale != "Waiting deployment approval" || d.Locations["left"].Pointer != "/actions/0") {
			t.Fatal(d)
		}
	}
	// The reverse presence relationship has the same structural ownership rules.
	p.Exceptions[0].StatePairs = []StatePair{{Left: State{State: "absent"}, Right: *present(action)}}
	req.Left, req.Right = testInput("dev", doc(map[string]any{"actions": []any{}})), testInput("prod", doc(map[string]any{"actions": []any{action}}))
	if r = runTest(t, req, p); r.ExitCode() != 0 || r.Counts["exception"] != 1 {
		t.Fatal(r)
	}
}

func TestUnmatchedSubtreeExceptionRetainsNormalComparison(t *testing.T) {
	p, action := subtreeExceptionPolicy()
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"actions": []any{action}})), Right: testInput("prod", doc(map[string]any{"actions": []any{}}))}
	action["action"] = "DELETE"
	if r := runTest(t, req, p); r.ExitCode() != 1 || r.Counts["exception"] != 0 {
		t.Fatal(r)
	}
	// When both sides are present, ordinary nested alignment and ID exclusions run.
	action["action"] = "DISABLE"
	rightAction := map[string]any{"action": "DISABLE", "sources": []any{map[string]any{"id": "prod-source", "name": "Directory"}}}
	req.Right = testInput("prod", doc(map[string]any{"actions": []any{rightAction}}))
	if r := runTest(t, req, p); r.ExitCode() != 0 || r.Counts["exception"] != 0 || r.RuleUsage["sources"] != 1 || r.RuleUsage["source-id"] != 1 {
		t.Fatal(r)
	}
	// An unrelated field still fails even when the subtree matches.
	req.Left = testInput("dev", doc(map[string]any{"actions": []any{action}, "enabled": true}))
	req.Right = testInput("prod", doc(map[string]any{"actions": []any{}, "enabled": false}))
	if r := runTest(t, req, p); r.ExitCode() != 1 || r.Counts["exception"] != 1 || r.Counts["different"] != 1 {
		t.Fatal(r)
	}
}

func TestSubtreeExceptionDoesNotRelaxCompetingValueRules(t *testing.T) {
	for _, kind := range []string{"expectation", "equivalence", "exception", "same-path-ignore", "scalar-state", "value-pair"} {
		t.Run(kind, func(t *testing.T) {
			p, action := subtreeExceptionPolicy()
			switch kind {
			case "expectation":
				p.Expectations = []Expectation{{Rule: testRule("assert-action"), Selector: Selector{Type: "THING", Path: "/actions/*/action"}, Operation: "literal", Expected: present("DISABLE"), Authority: "approved"}}
			case "equivalence":
				p.Equivalences = []Equivalence{{Rule: testRule("other"), Selector: Selector{Type: "THING", Path: "/actions/*/action"}, LeftEnv: "dev", RightEnv: "prod", Pairs: []ValuePair{{Left: "DISABLE", Right: "DELETE"}}}}
			case "exception":
				e := p.Exceptions[0]
				e.ID = "other"
				e.Path = "/actions/0/action"
				e.StatePairs = []StatePair{{Left: *present("DISABLE"), Right: State{State: "absent"}}}
				p.Exceptions = append(p.Exceptions, e)
			case "same-path-ignore":
				p.Ignores = append(p.Ignores, Ignore{Rule: testRule("ignore-action"), Selector: Selector{Type: "THING", Path: "/actions/0"}})
			case "scalar-state":
				p.Exceptions[0].StatePairs = []StatePair{{Left: *present("DISABLE"), Right: State{State: "absent"}}}
			case "value-pair":
				p.Exceptions[0].StatePairs = nil
				p.Exceptions[0].Pairs = []ValuePair{{Left: action, Right: nil}}
			}
			if _, err := Compile(p); err == nil {
				t.Fatal("competing or broad value rule accepted")
			}
		})
	}
}
