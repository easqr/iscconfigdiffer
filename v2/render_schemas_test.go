package v2

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func schemaReportFixture(t *testing.T, mode string, reverse, omit, assert bool) Report {
	t.Helper()
	p := testPolicy()
	p.NullHandling = "null_equals_absent"
	p.Collections = []Collection{
		{Rule: testRule("schemas"), Selector: Selector{Type: "THING", Path: "/schemas"}, Mode: "keyed", Keys: []string{"/name"}, ItemLabel: "Schema", DisplayFields: []DisplayField{{Label: "Name", Path: "/name"}}},
		{Rule: testRule("attributes"), Selector: Selector{Type: "THING", Path: "/schemas/*/attributes"}, Mode: "keyed", Keys: []string{"/name"}, ItemLabel: "Attribute", DisplayFields: []DisplayField{{Label: "Name", Path: "/name"}}},
	}
	p.Ignores = []Ignore{{Rule: testRule("schema-id"), Selector: Selector{Type: "THING", Path: "/schemas/*/id"}}, {Rule: testRule("generated-id"), Selector: Selector{Type: "THING", Path: "/schemas/*/attributes/*/generatedId"}}}
	if assert {
		p.Expectations = []Expectation{{Rule: testRule("assert-attribute"), Selector: Selector{Type: "THING", Path: "/schemas/*/attributes/*/required"}, Operation: "literal", Expected: present(true), Authority: "approved"}}
	}
	schema := func(name string) any {
		return map[string]any{"name": name, "id": "generated-schema", "attributes": []any{map[string]any{"name": "first", "type": "STRING", "generatedId": "generated-field", "required": false, "nullable": nil}, map[string]any{"name": "second", "type": "STRING", "generatedId": "generated-field", "required": false}}}
	}
	left := testInput("dev", doc(map[string]any{"schemas": []any{schema("account")}, "connectorAttributes": map[string]any{"clientID": "dev-app", "state": strings.Repeat("LONG_STATE_", 40)}, "provisioningPolicies": []any{map[string]any{"name": "Create", "description": "dev"}}, "note": "dev"}))
	right := testInput("prod", doc(map[string]any{"schemas": []any{schema("missing-A"), schema("account"), schema("missing-B")}, "connectorAttributes": map[string]any{"clientID": "prod-app", "state": "prod"}, "provisioningPolicies": []any{map[string]any{"name": "Create", "description": "prod"}}, "note": "prod"}))
	right.Documents[0].Content.(map[string]any)["schemas"].([]any)[1].(map[string]any)["displayAttribute"] = "changed"
	right.Documents[0].Metadata = map[string]any{"version": 2}
	if reverse {
		left, right = right, left
	}
	req := Request{Mode: mode, Left: left, Right: right, ExcludeValues: omit}
	if mode == "deployment" {
		req.Operations = []Operation{{Type: "THING", Key: "invented", Action: "update"}}
	}
	return runTest(t, req, p)
}

func TestMissingSchemasSummarizedAndSectioned(t *testing.T) {
	for _, mode := range []string{"drift", "deployment"} {
		for _, reverse := range []bool{false, true} {
			r := schemaReportFixture(t, mode, reverse, false, false)
			before := stable(r)
			v := r.humanView()
			g := v.Groups[0]
			whole, represented := 0, 0
			for _, d := range g.Decisions {
				represented += d.DecisionCount
				if !d.WholeSchema {
					continue
				}
				whole++
				if d.Category != "schemas" || !strings.Contains(d.FindingTitle, "missing-") || len(d.Children) < 5 || d.DecisionCount != 1+len(d.Children) {
					t.Fatal("schema not summarized", d)
				}
				if !strings.Contains(d.StatusTokens, "excluded") {
					t.Fatal("nested statuses unavailable to filter", d.StatusTokens)
				}
				for _, child := range d.Children {
					if !child.Nested || child.RuleID == "assert-attribute" {
						t.Fatal("assertion hidden or detail not marked", child)
					}
				}
			}
			if whole != 2 || represented != len(r.Decisions) || g.DecisionCount != len(r.Decisions) {
				t.Fatal("schema findings lost or duplicated", whole, represented, len(r.Decisions))
			}
			titles := []string{}
			sectionCount := 0
			for _, s := range g.Sections {
				titles = append(titles, s.Title)
				sectionCount += s.DecisionCount
				for _, d := range s.Decisions {
					if d.Category != s.Category {
						t.Fatal("category misplaced")
					}
				}
			}
			if !reflect.DeepEqual(titles, []string{"Connection settings", "Schemas", "Provisioning policies", "Other settings", "Export metadata"}) || sectionCount != len(r.Decisions) {
				t.Fatal("sections incorrect", titles, sectionCount)
			}
			for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
				var b bytes.Buffer
				if err := render(&b); err != nil {
					t.Fatal(err)
				}
				for _, s := range []string{"Schema details (", "Schema value", "missing-A", "missing-B", "Connection settings", "Provisioning policies", "/connectorAttributes/clientID", "/schemas/0/attributes/0/type"} {
					if !strings.Contains(b.String(), s) {
						t.Fatal("report detail absent", mode, reverse, s)
					}
				}
			}
			if stable(r) != before {
				t.Fatal("presentation changed comparison report")
			}
			omitted := schemaReportFixture(t, mode, reverse, true, false)
			ov := omitted.humanView()
			if !reflect.DeepEqual(v.FindingCounts, ov.FindingCounts) || v.NestedDecisions != ov.NestedDecisions {
				t.Fatal("omission changed grouping")
			}
			for _, render := range []func(io.Writer) error{omitted.HTML, omitted.Markdown} {
				var b bytes.Buffer
				if err := render(&b); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(b.String(), "LONG_STATE_") {
					t.Fatal("omitted state leaked")
				}
			}
			for a, b := 0, len(r.Decisions)-1; a < b; a, b = a+1, b-1 {
				r.Decisions[a], r.Decisions[b] = r.Decisions[b], r.Decisions[a]
			}
			if shuffled := r.humanView(); shuffled.NestedDecisions != v.NestedDecisions || !reflect.DeepEqual(shuffled.FindingCounts, v.FindingCounts) {
				t.Fatal("grouping depends on decision order")
			}
		}
	}
}

func TestMissingSchemasKeepAssertionsSeparate(t *testing.T) {
	r := schemaReportFixture(t, "drift", false, false, true)
	v := r.humanView()
	assertions, missing := 0, 0
	for _, d := range v.Groups[0].Decisions {
		if d.RuleID == "assert-attribute" {
			assertions++
		}
		if d.WholeSchema {
			missing++
			for _, child := range d.Children {
				if child.RuleID == "assert-attribute" {
					t.Fatal("explicit assertion collapsed")
				}
			}
		}
	}
	if missing != 2 || assertions == 0 || r.Counts["violation"] == 0 {
		t.Fatal("schema/assertion visibility lost", missing, assertions, r.Counts)
	}
}

func TestMissingSchemaConservativeBoundaries(t *testing.T) {
	roles := []string{"left", "right", "baseline"}
	parent := Decision{Type: "THING", Key: "one", Space: "content", Status: "different", Code: "container presence differs", Locations: map[string]Location{"left": {ID: "l", Artifact: "l.json", Pointer: "/schemas"}, "right": {ID: "r", Artifact: "r.json", Pointer: "/schemas/1", Present: true, ValueKind: "object"}, "baseline": {ID: "b", Artifact: "b.json", Pointer: "/schemas/2", Present: true}}}
	child := func() Decision {
		d := parent
		d.Code = "missing left"
		d.Locations = map[string]Location{"left": {ID: "l", Artifact: "l.json", Pointer: "/schemas/attributes"}, "right": {ID: "r", Artifact: "r.json", Pointer: "/schemas/1/attributes/0/name", Present: true, ValueKind: "string"}, "baseline": {ID: "b", Artifact: "b.json", Pointer: "/schemas/2/attributes/0/name", Present: true}}
		return d
	}
	for _, change := range []func(*Decision){
		func(d *Decision) { d.Key = "another" }, func(d *Decision) { d.Status = "conflict" }, func(d *Decision) { d.RuleID = "explicit" },
		func(d *Decision) {
			l := d.Locations["right"]
			l.Pointer = "/schemas/10/attributes/0/name"
			d.Locations["right"] = l
		},
		func(d *Decision) {
			l := d.Locations["baseline"]
			l.Pointer = "/schemas/3/attributes/0/name"
			d.Locations["baseline"] = l
		},
		func(d *Decision) { l := d.Locations["left"]; l.Present = true; d.Locations["left"] = l },
	} {
		c := child()
		change(&c)
		ds := []Decision{parent, c}
		vs := []humanDecision{decisionView(parent, roles, false), decisionView(c, roles, false)}
		_, out := groupMissingSchemas(ds, vs, roles, nil)
		if len(out) != 2 || len(out[0].Children) != 0 {
			t.Fatal("unrelated schema detail collapsed", c)
		}
	}
	// Redacted paths cannot establish schema ancestry or reveal a schema title.
	p := parent
	p.Key = "[redacted]"
	if _, _, ok := missingSchema(p, roles); ok {
		t.Fatal("redacted identity summarized")
	}
	p = parent
	l := p.Locations["right"]
	l.Pointer = "/schemas/1/attributes/0"
	p.Locations["right"] = l
	if _, _, ok := missingSchema(p, roles); ok {
		t.Fatal("missing attribute reported as missing schema")
	}
}

func TestSchemaSectionsAndRedaction(t *testing.T) {
	for _, tc := range []struct{ space, left, right, want string }{
		{"content", "/connectorAttributes/clientID", "/connectorAttributes/clientID", "connection"},
		{"content", "/schemas/1/attributes/0/name", "/schemas/0/attributes/1/name", "schemas"},
		{"content", "/schema~1s/x", "/schema~1s/x", "other"},
		{"content", "/schemas/x", "/connectorAttributes/x", "other"},
		{"metadata", "/schemas/0", "/schemas/0", "metadata"},
		{"content", "[redacted]", "[redacted]", "other"},
	} {
		d := Decision{Space: tc.space, Locations: map[string]Location{"left": {Pointer: tc.left}, "right": {Pointer: tc.right}}}
		if got := decisionCategory(d); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	r := schemaReportFixture(t, "drift", false, false, false)
	// Rendering a redacted report cannot infer names from omitted input data.
	for i := range r.Decisions {
		d := &r.Decisions[i]
		d.Key = "[redacted]"
		for role, loc := range d.Locations {
			loc.Pointer = "[redacted]"
			loc.Context = nil
			loc.Value = nil
			d.Locations[role] = loc
		}
	}
	for _, d := range r.humanView().Groups[0].Decisions {
		if d.WholeSchema {
			t.Fatal("redacted schema summarized")
		}
	}
}

func TestMissingSchemaNamesEscapedAndWholeCollectionAbsent(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("schemas"), Selector: Selector{Type: "THING", Path: "/schemas"}, Mode: "keyed", Keys: []string{"/name"}, ItemLabel: "Schema", DisplayFields: []DisplayField{{Label: "Name", Path: "/name"}}}}
	name := "<script>schema</script>|`*[link](https://example.invalid)\n# injected"
	r := runTest(t, Request{Mode: "drift", Left: testInput("dev", doc(nil)), Right: testInput("prod", doc(map[string]any{"schemas": []any{map[string]any{"name": name, "attributes": []any{}}, map[string]any{"name": "second", "attributes": []any{}}}}))}, p)
	whole := 0
	sum := 0
	for _, d := range r.humanView().Groups[0].Decisions {
		sum += d.DecisionCount
		if d.WholeSchema {
			whole++
		}
	}
	if whole != 2 || sum != len(r.Decisions) {
		t.Fatal("missing entire collection lost schema summaries", whole, sum, len(r.Decisions))
	}
	var h, m bytes.Buffer
	if err := r.HTML(&h); err != nil {
		t.Fatal(err)
	}
	if err := r.Markdown(&m); err != nil {
		t.Fatal(err)
	}
	if strings.Count(h.String(), "<script>") != 1 || strings.Contains(m.String(), "<script>") || strings.Contains(m.String(), "\n# injected") || strings.Contains(m.String(), "[link](https:") {
		t.Fatal("untrusted schema name not escaped")
	}
}
