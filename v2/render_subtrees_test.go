package v2

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func subtreeExample() (Input, Input) {
	branch := map[string]any{"type": "static", "attributes": map[string]any{"value": "invented-filter", "identityUpn": map[string]any{"type": "identityAttribute", "attributes": map[string]any{"name": "invented-name"}}}}
	return testInput("dev", doc(map[string]any{"attributes": map[string]any{"filter": branch, "flag": true, "application": "invented-app"}})), testInput("prod", doc(map[string]any{"attributes": map[string]any{"sourceName": "Invented Source"}}))
}

func TestMissingSubtreeHumanGrouping(t *testing.T) {
	left, right := subtreeExample()
	req := Request{Mode: "drift", Left: left, Right: right}
	r := runTest(t, req, testPolicy())
	if r.Counts["different"] != 11 {
		t.Fatal(r.Counts)
	}
	before := stable(r)
	v := r.humanView()
	if len(v.Groups) != 1 || len(v.Groups[0].Decisions) != 4 || v.NestedDecisions != 7 || v.Groups[0].DecisionCount != 11 || !reflect.DeepEqual(v.FindingCounts, []statusCount{{"different", 4}}) {
		t.Fatal("incorrect grouping counts", v)
	}
	var subtree humanDecision
	for _, d := range v.Groups[0].Decisions {
		if len(d.Children) > 0 {
			subtree = d
		}
	}
	if subtree.DecisionCount != 8 || len(subtree.Children) != 7 || subtree.Locations[0].Pointer != "/attributes/filter" || subtree.Locations[1].State != "absent" {
		t.Fatal("wrong subtree grouped", subtree)
	}
	if !strings.Contains(subtree.Locations[0].Value, "invented-filter") {
		t.Fatal("parent subtree value omitted")
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"Displayed findings", "Underlying decisions", "4 grouped findings representing 11 underlying displayed decisions", "Nested decisions (7)", "8 underlying decisions", "/attributes/filter/attributes/identityUpn/attributes/name", "invented-filter"} {
			if !strings.Contains(b.String(), text) {
				t.Fatal("missing grouped detail", text)
			}
		}
		if strings.Count(b.String(), "<summary>Nested decisions (7)</summary>") != 1 {
			t.Fatal("missing expandable subtree")
		}
	}
	if stable(r) != before {
		t.Fatal("rendering changed JSON report")
	}
	// Output omission changes no grouping and does not cause a payload reread.
	req.ExcludeValues = true
	omitted := runTest(t, req, testPolicy())
	ov := omitted.humanView()
	if !reflect.DeepEqual(v.FindingCounts, ov.FindingCounts) || ov.NestedDecisions != 7 {
		t.Fatal("value omission changed grouping")
	}
	for _, render := range []func(io.Writer) error{omitted.HTML, omitted.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), "invented-filter") {
			t.Fatal("omitted value exposed")
		}
	}
	// Grouping does not depend on the input decision order.
	for a, b := 0, len(r.Decisions)-1; a < b; a, b = a+1, b-1 {
		r.Decisions[a], r.Decisions[b] = r.Decisions[b], r.Decisions[a]
	}
	reordered := r.humanView()
	if reordered.NestedDecisions != 7 || len(reordered.Groups[0].Decisions) != 4 {
		t.Fatal("grouping depends on decision order")
	}
}

func TestMissingSubtreeKeepsPolicyChecksSeparate(t *testing.T) {
	left, right := subtreeExample()
	p := testPolicy()
	p.Ignores = []Ignore{{Rule: testRule("ignored-child"), Selector: Selector{Type: "THING", Path: "/attributes/filter/type"}}}
	p.Expectations = []Expectation{{Rule: testRule("assert-child"), Selector: Selector{Type: "THING", Path: "/attributes/filter/attributes/value"}, Operation: "literal", Expected: present("invented-filter"), Authority: "invented-approval"}}
	r := runTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
	v := r.humanView()
	if v.NestedDecisions != 5 {
		t.Fatal("explicit checks were grouped", v.Groups[0].Counts)
	}
	found := map[string]bool{}
	for _, d := range v.Groups[0].Decisions {
		if d.RuleID != "" {
			found[d.RuleID] = true
		}
		for _, child := range d.Children {
			if child.RuleID != "" {
				t.Fatal("rule-driven check hidden", child)
			}
		}
	}
	if !found["ignored-child"] || !found["assert-child"] {
		t.Fatal("policy findings disappeared", found)
	}
}

func TestMissingSubtreeKeyedAndOrderedArrays(t *testing.T) {
	for _, mode := range []string{"keyed", "ordered"} {
		t.Run(mode, func(t *testing.T) {
			p := testPolicy()
			c := Collection{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: mode}
			if mode == "keyed" {
				c.Keys = []string{"/name"}
			}
			p.Collections = []Collection{c}
			left := testInput("dev", doc(map[string]any{"items": []any{map[string]any{"name": "A", "config": map[string]any{"nested": map[string]any{"value": "a"}}}, map[string]any{"name": "B", "config": map[string]any{"value": "b"}}}}))
			right := testInput("prod", doc(map[string]any{"items": []any{}}))
			r := runTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
			v := r.humanView()
			missing := 0
			for _, d := range v.Groups[0].Decisions {
				if d.Code != "container presence differs" {
					continue
				}
				missing++
				if len(d.Children) == 0 {
					t.Fatal("item descendants were not grouped")
				}
				prefix := d.Locations[0].Pointer + "/"
				for _, child := range d.Children {
					if !strings.HasPrefix(child.Locations[0].Pointer, prefix) {
						t.Fatal("different array items combined")
					}
				}
			}
			if missing != 2 {
				t.Fatal("array item findings merged", missing, v.Groups[0].Counts)
			}
		})
	}
	// Reordering an existing keyed item changes each side's physical ancestor.
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/name"}}}
	a := map[string]any{"name": "A", "config": map[string]any{"nested": map[string]any{"value": "a"}}}
	b := map[string]any{"name": "B"}
	r := runTest(t, Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{a, b}})), Right: testInput("prod", doc(map[string]any{"items": []any{b, map[string]any{"name": "A"}}}))}, p)
	v := r.humanView()
	var missing []humanDecision
	for _, d := range v.Groups[0].Decisions {
		if d.Code == "container presence differs" {
			missing = append(missing, d)
		}
	}
	if len(missing) != 1 || len(missing[0].Children) != 2 {
		t.Fatal("reordered item ancestry failed", v.Groups[0].Counts)
	}
	if missing[0].Locations[0].Pointer != "/items/0/config" || missing[0].Locations[1].Pointer != "/items/1/config" {
		t.Fatal("physical paths lost")
	}
}

func TestMissingSubtreeEmptyRootAndEscapedPaths(t *testing.T) {
	for _, value := range []any{map[string]any{}, []any{}, map[string]any{"a/b": map[string]any{"~field": true}}} {
		r := runTest(t, Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"missing": value})), Right: testInput("prod", doc(nil))}, testPolicy())
		v := r.humanView()
		if len(v.Groups[0].Decisions) != 1 || v.Groups[0].DecisionCount < 2 {
			t.Fatal("empty/escaped subtree duplicated", v.Groups[0].Counts)
		}
	}
	// An empty JSON Pointer identifies a root container, not an unknown location.
	d := Decision{Type: "THING", Key: "root", Status: "different", Code: "container presence differs", Space: "content", Locations: map[string]Location{"left": {Present: true, ValueKind: "object"}, "right": {}}}
	child := d
	child.Code = "missing right"
	child.Locations = map[string]Location{"left": {Present: true, Pointer: "/a~1b", ValueKind: "string"}, "right": {Pointer: "/a~1b"}}
	v := (Report{Version: 2, Mode: "drift", Decisions: []Decision{d, child}}).humanView()
	if len(v.Groups[0].Decisions) != 1 || v.NestedDecisions != 1 {
		t.Fatal("root container not grouped")
	}
}

func TestMissingSubtreeConservativeBoundaries(t *testing.T) {
	parent := Decision{Type: "THING", Key: "invented", Status: "different", Code: "container presence differs", Space: "content", Locations: map[string]Location{"left": {ID: "a", Artifact: "a.json", Pointer: "/a", Present: true, ValueKind: "object"}, "right": {ID: "b", Artifact: "b.json", Pointer: "/a"}}}
	makeChild := func() Decision {
		d := parent
		d.Code = "missing right"
		d.Locations = map[string]Location{"left": {ID: "a", Artifact: "a.json", Pointer: "/a/x", Present: true, ValueKind: "string"}, "right": {ID: "b", Artifact: "b.json", Pointer: "/a/x"}}
		return d
	}
	tests := []struct {
		name  string
		alter func(*Decision)
	}{
		{"segment boundary", func(d *Decision) {
			for role, loc := range d.Locations {
				loc.Pointer = "/ab/x"
				d.Locations[role] = loc
			}
		}},
		{"different section", func(d *Decision) { d.Space = "metadata" }},
		{"different object", func(d *Decision) { d.Key = "another" }},
		{"different artifact", func(d *Decision) {
			loc := d.Locations["right"]
			loc.Artifact = "other.json"
			d.Locations["right"] = loc
		}},
		{"other side ancestry", func(d *Decision) { loc := d.Locations["right"]; loc.Pointer = "/other/x"; d.Locations["right"] = loc }},
		{"different status", func(d *Decision) { d.Status = "conflict" }},
		{"explicit rule", func(d *Decision) { d.RuleID = "check" }},
		{"both present", func(d *Decision) { loc := d.Locations["right"]; loc.Present = true; d.Locations["right"] = loc }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			child := makeChild()
			tc.alter(&child)
			v := (Report{Version: 2, Mode: "drift", Decisions: []Decision{parent, child}}).humanView()
			if v.NestedDecisions != 0 {
				t.Fatal("unrelated finding nested", tc.name)
			}
		})
	}
	child := makeChild()
	parent.Key = "[redacted]"
	child.Key = "[redacted]"
	if v := (Report{Version: 2, Mode: "drift", Decisions: []Decision{parent, child}}).humanView(); v.NestedDecisions != 0 {
		t.Fatal("combined redacted objects grouped")
	}
}

func TestMissingSubtreeDeployment(t *testing.T) {
	left, right := subtreeExample()
	req := Request{Mode: "deployment", Left: left, Right: right, Baseline: &right, Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}
	r := runTest(t, req, testPolicy())
	before := stable(r)
	v := r.humanView()
	if r.ExitCode() != 2 || v.NestedDecisions != 7 {
		t.Fatal("deployment grouping failed", r.Counts, v.NestedDecisions)
	}
	for _, d := range v.Groups[0].Decisions {
		if len(d.Children) > 0 && (d.Status != "unverified" || len(d.Locations) != 3) {
			t.Fatal("deployment meaning or baseline lost")
		}
	}
	if stable(r) != before {
		t.Fatal("deployment report changed")
	}
}

func TestMissingSubtreePreservesSensitiveRedaction(t *testing.T) {
	left, right := subtreeExample()
	p := testPolicy()
	p.Sensitive = []Sensitive{{Rule: testRule("hide"), Selector: Selector{Type: "THING", Path: "/attributes/filter"}}}
	r := runTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
	v := r.humanView()
	if v.NestedDecisions != 0 || len(v.Groups) != 1 || v.Groups[0].Key != "[redacted]" {
		t.Fatal("redacted identities were grouped")
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"invented-filter", "invented-app", "/attributes/filter"} {
			if strings.Contains(b.String(), secret) {
				t.Fatal("sensitive subtree exposed", secret)
			}
		}
	}
}
