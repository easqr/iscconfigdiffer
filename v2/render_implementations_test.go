package v2

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func implementationExample() (Input, Input) {
	left := testInput("dev", doc(map[string]any{"transform": map[string]any{"type": "concat", "attributes": map[string]any{"values": []any{"first", "last"}}}}))
	right := testInput("prod", doc(map[string]any{"transform": map[string]any{"type": "static", "attributes": map[string]any{"value": "invented-result", "calculate": map[string]any{"type": "identityAttribute", "attributes": map[string]any{"name": "invented-attribute"}}}}}))
	return left, right
}

func TestTransformImplementationHumanGrouping(t *testing.T) {
	left, right := implementationExample()
	req := Request{Mode: "drift", Left: left, Right: right}
	r := runTest(t, req, testPolicy())
	before := stable(r)
	v := r.humanView()
	if len(v.Groups) != 1 || len(v.Groups[0].Decisions) != 1 || !reflect.DeepEqual(v.FindingCounts, []statusCount{{"different", 1}}) {
		t.Fatal("implementation was split", v.FindingCounts)
	}
	d := v.Groups[0].Decisions[0]
	if d.FindingTitle != "Transform implementation differs" || d.DecisionCount != len(r.Decisions) || len(d.Children) != len(r.Decisions)-1 || v.NestedDecisions != len(d.Children) {
		t.Fatal("decisions lost", d.DecisionCount, r.Counts)
	}
	if d.Locations[0].Pointer != "/transform/type" || d.Locations[1].Pointer != "/transform/type" || d.Locations[0].Value != `"concat"` || d.Locations[1].Value != `"static"` {
		t.Fatal("type comparison lost", d.Locations)
	}
	for _, child := range d.Children {
		if !child.Nested || !strings.HasPrefix(child.Locations[0].Pointer, "/transform/attributes") || len(child.Children) != 0 {
			t.Fatal("wrong implementation child", child)
		}
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"Transform implementation differs", "Nested decisions (", "/transform/type", "/transform/attributes/values/0", "invented-result", "concat", "static", "attributes of the transform implementations"} {
			if !strings.Contains(b.String(), text) {
				t.Fatal("group detail missing", text)
			}
		}
	}
	if stable(r) != before {
		t.Fatal("rendering mutated JSON report")
	}
	req.ExcludeValues = true
	omitted := runTest(t, req, testPolicy())
	ov := omitted.humanView()
	if !reflect.DeepEqual(v.FindingCounts, ov.FindingCounts) || v.NestedDecisions != ov.NestedDecisions || r.RequestDigest != omitted.RequestDigest {
		t.Fatal("value exclusion changed grouping")
	}
	for _, render := range []func(io.Writer) error{omitted.HTML, omitted.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), "invented-result") || strings.Contains(b.String(), "concat") {
			t.Fatal("excluded value exposed")
		}
	}
	for a, b := 0, len(r.Decisions)-1; a < b; a, b = a+1, b-1 {
		r.Decisions[a], r.Decisions[b] = r.Decisions[b], r.Decisions[a]
	}
	if rv := r.humanView(); len(rv.Groups[0].Decisions) != 1 || rv.NestedDecisions != v.NestedDecisions {
		t.Fatal("grouping depends on order")
	}
}

func TestTransformImplementationKeepsPolicyChecksSeparate(t *testing.T) {
	left, right := implementationExample()
	p := testPolicy()
	p.Ignores = []Ignore{{Rule: testRule("ignore"), Selector: Selector{Type: "THING", Path: "/transform/attributes/calculate/type"}}}
	p.Expectations = []Expectation{{Rule: testRule("assert"), Selector: Selector{Type: "THING", Path: "/transform/attributes/value"}, Operation: "literal", Expected: present("approved-result"), Authority: "invented-approval"}}
	r := runTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
	v := r.humanView()
	found := map[string]int{}
	for _, d := range v.Groups[0].Decisions {
		found[d.RuleID]++
		for _, child := range d.Children {
			if child.RuleID != "" {
				t.Fatal("policy check hidden")
			}
		}
	}
	if found["ignore"] != 1 || found["assert"] != 2 || found[""] != 1 {
		t.Fatal("explicit checks lost", found)
	}
	// An approved type mapping does not imply that its attributes are equivalent.
	p = testPolicy()
	p.Equivalences = []Equivalence{{Rule: testRule("approved-type"), Selector: Selector{Type: "THING", Path: "/transform/type"}, LeftEnv: "dev", RightEnv: "prod", Pairs: []ValuePair{{Left: "concat", Right: "static"}}}}
	r = runTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
	for _, d := range r.humanView().Groups[0].Decisions {
		if d.FindingTitle == "Transform implementation differs" {
			t.Fatal("approved type relationship treated as unruled mismatch")
		}
	}
	if r.ExitCode() != 1 {
		t.Fatal("attribute differences were implicitly approved")
	}
}

func TestTransformImplementationNestedAndSameType(t *testing.T) {
	makeInput := func(env, outer, inner string) Input {
		return testInput(env, doc(map[string]any{"transform": map[string]any{"type": outer, "attributes": map[string]any{"wrapper": map[string]any{"transform": map[string]any{"type": inner, "attributes": map[string]any{"value": env}}}}}}))
	}
	r := runTest(t, Request{Mode: "drift", Left: makeInput("dev", "outer-a", "inner-a"), Right: makeInput("prod", "outer-b", "inner-b")}, testPolicy())
	v := r.humanView()
	if len(v.Groups[0].Decisions) != 1 || v.Groups[0].Decisions[0].DecisionCount != len(r.Decisions) {
		t.Fatal("nested implementations duplicated or lost")
	}
	r = runTest(t, Request{Mode: "drift", Left: makeInput("dev", "same", "inner-a"), Right: makeInput("prod", "same", "inner-b")}, testPolicy())
	v = r.humanView()
	if len(v.Groups[0].Decisions) != 1 || v.Groups[0].Decisions[0].Locations[0].Pointer != "/transform/attributes/wrapper/transform/type" {
		t.Fatal("unchanged outer type grouped", v.Groups[0].Counts)
	}
	r = runTest(t, Request{Mode: "drift", Left: makeInput("dev", "same", "same"), Right: makeInput("prod", "same", "same")}, testPolicy())
	for _, d := range r.humanView().Groups[0].Decisions {
		if d.FindingTitle == "Transform implementation differs" {
			t.Fatal("same type reported as different implementation")
		}
	}
}

func TestTransformImplementationConservativeBoundaries(t *testing.T) {
	for _, name := range []string{"other metadata", "different field", "different section", "different artifact", "redacted", "type not string", "deployment", "same prefix", "ambiguous"} {
		t.Run(name, func(t *testing.T) {
			left, right := implementationExample()
			r := runTest(t, Request{Mode: "drift", Left: left, Right: right}, testPolicy())
			var parent, child Decision
			for _, d := range r.Decisions {
				if d.Locations["left"].Pointer == "/transform/type" {
					parent = d
				}
				if d.Locations["left"].Pointer == "/transform/attributes/value" {
					child = d
				}
			}
			switch name {
			case "other metadata":
				for role, loc := range child.Locations {
					loc.Pointer = "/transform/description"
					child.Locations[role] = loc
				}
			case "different field":
				for role, loc := range child.Locations {
					loc.Pointer = "/other/transform/attributes/value"
					child.Locations[role] = loc
				}
			case "different section":
				child.Space = "metadata"
			case "different artifact":
				loc := child.Locations["right"]
				loc.Artifact = "another.json"
				child.Locations["right"] = loc
			case "redacted":
				parent.Key = "[redacted]"
				child.Key = "[redacted]"
			case "type not string":
				loc := parent.Locations["right"]
				loc.ValueKind = "object"
				parent.Locations["right"] = loc
			case "deployment":
				parent.Status = "unverified"
				parent.Code = "no authoritative expectation"
				child.Status = "unverified"
				child.Code = "no authoritative expectation"
			case "same prefix":
				for role, loc := range child.Locations {
					loc.Pointer = "/transform/attributesOther/value"
					child.Locations[role] = loc
				}
			}
			r.Decisions = []Decision{parent, child}
			if name == "ambiguous" {
				r.Decisions = append(r.Decisions, parent)
			}
			v := r.humanView()
			if v.NestedDecisions != 0 {
				t.Fatal("unrelated/ambiguous decisions grouped", name)
			}
		})
	}
}

func TestTransformImplementationSensitiveRedaction(t *testing.T) {
	left, right := implementationExample()
	p := testPolicy()
	p.Sensitive = []Sensitive{{Rule: testRule("private"), Selector: Selector{Type: "THING", Path: "/transform"}}}
	r := runTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
	if v := r.humanView(); v.NestedDecisions != 0 {
		t.Fatal("combined redacted identities grouped")
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), "invented-result") || strings.Contains(b.String(), "concat") || strings.Contains(b.String(), "/transform/") {
			t.Fatal("redacted implementation exposed")
		}
	}
}

func TestTransformImplementationTypeOnlyAndWholeAttributes(t *testing.T) {
	for _, whole := range []bool{false, true} {
		leftTransform := map[string]any{"type": "concat", "attributes": map[string]any{"value": "same"}}
		rightTransform := map[string]any{"type": "static", "attributes": map[string]any{"value": "same"}}
		if whole {
			delete(rightTransform, "attributes")
		}
		r := runTest(t, Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"transform": leftTransform})), Right: testInput("prod", doc(map[string]any{"transform": rightTransform}))}, testPolicy())
		v := r.humanView()
		if len(v.Groups[0].Decisions) != 1 || v.Groups[0].Decisions[0].FindingTitle != "Transform implementation differs" || v.Groups[0].Decisions[0].DecisionCount != len(r.Decisions) {
			t.Fatal("type-only/whole attributes not represented", whole, v.Groups[0].Counts)
		}
		if !whole && (len(v.Groups[0].Decisions[0].Children) != 0 || strings.Contains(v.Groups[0].Decisions[0].Explanation, "nested")) {
			t.Fatal("nonexistent child details claimed")
		}
	}
}
