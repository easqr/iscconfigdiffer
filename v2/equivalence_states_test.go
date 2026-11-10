package v2

import (
	"encoding/json"
	"strings"
	"testing"
)

func stateEquivalencePolicy() Policy {
	p := testPolicy()
	p.Equivalences = []Equivalence{{
		Rule: testRule("optional-setting"), Selector: Selector{Type: "THING", Path: "/settings/optional"},
		LeftEnv: "dev", RightEnv: "prod",
		StatePairs: []StatePair{
			{Left: *present(false), Right: State{State: "absent"}},
			{Left: State{State: "absent"}, Right: *present(false)},
		},
	}}
	return p
}

func TestStateEquivalenceMatching(t *testing.T) {
	for _, tc := range []struct {
		name        string
		left, right map[string]any
		want        string
	}{
		{"false-absent", map[string]any{"optional": false}, map[string]any{}, "equivalent"},
		{"absent-false", map[string]any{}, map[string]any{"optional": false}, "equivalent"},
		{"true-absent", map[string]any{"optional": true}, map[string]any{}, "different"},
		{"absent-true", map[string]any{}, map[string]any{"optional": true}, "different"},
		{"false-null", map[string]any{"optional": false}, map[string]any{"optional": nil}, "different"},
		{"null-false", map[string]any{"optional": nil}, map[string]any{"optional": false}, "different"},
		{"string-absent", map[string]any{"optional": "false"}, map[string]any{}, "different"},
		{"object-is-not-state", map[string]any{"optional": map[string]any{"state": "absent"}}, map[string]any{}, "different"},
		{"both-false", map[string]any{"optional": false}, map[string]any{"optional": false}, "equal"},
		{"both-absent", map[string]any{}, map[string]any{}, "equal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := stateEquivalencePolicy()
			// Explicit state pairs distinguish null from absence even in ISC mode.
			p.NullHandling = "null_equals_absent"
			req := Request{Mode: "drift", IncludeMatching: true,
				Left:  testInput("dev", doc(map[string]any{"settings": tc.left})),
				Right: testInput("prod", doc(map[string]any{"settings": tc.right}))}
			before := stable(req)
			r := runTest(t, req, p)
			found := false
			for _, d := range r.Decisions {
				if d.Locations["left"].Pointer == "/settings/optional" {
					found = true
					if d.Status != tc.want || (tc.want == "equivalent" && d.RuleID != "optional-setting") {
						t.Fatal(d)
					}
				}
			}
			if (!found && tc.name != "both-absent") || before != stable(req) || (tc.name == "both-absent" && r.ExitCode() != 0) {
				t.Fatal("missing decision or mutated input", r)
			}
		})
	}
}

func TestStateEquivalenceScopeAndCompatibility(t *testing.T) {
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"settings": map[string]any{"optional": false}})), Right: testInput("prod", doc(map[string]any{"settings": map[string]any{}}))}
	for _, scope := range []string{"environment", "object", "tenant", "legacy-pair"} {
		t.Run(scope, func(t *testing.T) {
			p := stateEquivalencePolicy()
			switch scope {
			case "environment":
				p.Equivalences[0].LeftEnv = "other"
			case "object":
				p.Equivalences[0].Key = "other"
			case "tenant":
				p.Equivalences[0].RightTenant = "other"
			case "legacy-pair":
				p.Equivalences[0].StatePairs = nil
				p.Equivalences[0].Pairs = []ValuePair{{Left: false, Right: nil}}
			}
			r := runTest(t, req, p)
			if r.Counts["equivalent"] != 0 || r.ExitCode() == 0 {
				t.Fatal("state equivalence leaked", r.Counts)
			}
		})
	}
	p := stateEquivalencePolicy()
	p.Equivalences[0].StatePairs = []StatePair{{Left: *present(nil), Right: State{State: "absent"}}}
	req.Left = testInput("dev", doc(map[string]any{"settings": map[string]any{"optional": nil}}))
	if r := runTest(t, req, p); r.Counts["equivalent"] != 1 {
		t.Fatal("explicit null/absence pair failed", r.Counts)
	}
	p.Equivalences[0].Modes = []string{"deployment"}
	if _, err := Compile(p); err == nil {
		t.Fatal("deployment equivalence accepted")
	}
}

func TestStateEquivalenceMissingPathsAndUnavailable(t *testing.T) {
	p := stateEquivalencePolicy()
	p.Equivalences[0].StatePairs = []StatePair{{Left: State{State: "absent"}, Right: State{State: "absent"}}}
	req := Request{Mode: "drift", Left: testInput("dev", doc(nil)), Right: testInput("prod", doc(nil))}
	if r := runTest(t, req, p); r.Counts["equivalent"] != 1 {
		t.Fatal("literal absent path not evaluated", r.Counts)
	}
	req.Left = testInput("dev", doc(map[string]any{"settings": "not-a-container"}))
	if r := runTest(t, req, p); r.Counts["equivalent"] != 0 || r.ExitCode() != 1 {
		t.Fatal("unavailable descendant treated as absent", r.Counts)
	}
}

func TestStateEquivalenceValidationAndDigest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Equivalence)
	}{
		{"missing-state", func(e *Equivalence) { e.StatePairs[0].Right = State{} }},
		{"missing-value", func(e *Equivalence) { e.StatePairs[0].Left = State{State: "present"} }},
		{"absent-with-value", func(e *Equivalence) { e.StatePairs[0].Right.Value = json.RawMessage(`null`) }},
		{"invalid-state", func(e *Equivalence) { e.StatePairs[0].Right.State = "unavailable" }},
		{"duplicate", func(e *Equivalence) { e.StatePairs = append(e.StatePairs, e.StatePairs[0]) }},
		{"numeric-duplicate", func(e *Equivalence) {
			e.StatePairs = []StatePair{{Left: *present(json.Number("1")), Right: *present("a")}, {Left: *present(json.Number("1.0")), Right: *present("b")}}
		}},
		{"mixed-pairs", func(e *Equivalence) { e.Pairs = []ValuePair{{Left: false, Right: nil}} }},
		{"mixed-groups", func(e *Equivalence) { e.Groups = []map[string]any{{"dev": false, "prod": nil}} }},
		{"relative-order", func(e *Equivalence) { e.Operation = "relative_order" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := stateEquivalencePolicy()
			tc.change(&p.Equivalences[0])
			if _, err := Compile(p); err == nil || !strings.Contains(err.Error(), "optional-setting") {
				t.Fatal("invalid state pairs accepted or missing rule attribution", err)
			}
		})
	}
	p := stateEquivalencePolicy()
	a := compileTest(t, p)
	p.Equivalences[0].StatePairs[0], p.Equivalences[0].StatePairs[1] = p.Equivalences[0].StatePairs[1], p.Equivalences[0].StatePairs[0]
	if b := compileTest(t, p); a.Digest() != b.Digest() {
		t.Fatal("state pair ordering changed digest")
	}
}

func TestStateEquivalenceNestedCollectionAndPrivacy(t *testing.T) {
	p := stateEquivalencePolicy()
	p.Collections = []Collection{{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/key"}}}
	p.Equivalences[0].Path = "/items/*/optional"
	item := rawItemScope("items", "/key", "approved")
	p.Equivalences[0].Item = &item
	left := testInput("dev", doc(map[string]any{"items": []any{map[string]any{"key": "approved", "optional": false}, map[string]any{"key": "other", "optional": false}}}))
	right := testInput("prod", doc(map[string]any{"items": []any{map[string]any{"key": "other"}, map[string]any{"key": "approved"}}}))
	req := Request{Mode: "drift", Left: left, Right: right}
	r := runTest(t, req, p)
	if r.Counts["equivalent"] != 1 || r.Counts["different"] != 1 {
		t.Fatal("item scope not preserved", r.Counts)
	}
	for _, d := range r.Decisions {
		if d.Status == "equivalent" && (d.Locations["left"].Pointer != "/items/0/optional" || d.Locations["right"].Pointer != "/items/1/optional" || d.Locations["right"].Present) {
			t.Fatal("physical presence/locations lost", d)
		}
	}
	req.ExcludeValues = true
	r = runTest(t, req, p)
	for _, d := range r.Decisions {
		for _, loc := range d.Locations {
			if loc.Value != nil {
				t.Fatal("values leaked", loc)
			}
		}
	}
}
