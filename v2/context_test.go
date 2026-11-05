package v2

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestCollectionContextValidation(t *testing.T) {
	for _, tc := range []Collection{
		{DisplayFields: []DisplayField{{Label: "Name", Path: "/name"}}},
		{ItemLabel: " ", DisplayFields: []DisplayField{{Label: "Name", Path: "/name"}}},
		{ItemLabel: "Item", DisplayFields: []DisplayField{{Label: "Name", Path: "/*"}}},
		{ItemLabel: "Item", DisplayFields: []DisplayField{{Label: "Name", Path: "/name"}, {Label: "Name", Path: "/id"}}},
		{ItemLabel: "Item", DisplayFields: []DisplayField{{Label: " ", Path: "/name"}}},
	} {
		tc.Rule = testRule("items")
		tc.Selector = Selector{Type: "THING", Path: "/items"}
		tc.Mode = "ordered"
		p := testPolicy()
		p.Collections = []Collection{tc}
		if _, err := Compile(p); err == nil {
			t.Fatal("invalid context accepted", tc)
		}
	}
}
func TestContextOriginAndEscaping(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "ordered", ItemLabel: "Item", DisplayFields: []DisplayField{{Label: "Name", Path: "/name"}}}}
	sourceName := `<script>alert(1)</script>|[name](https://example.invalid)`
	items := []any{map[string]any{"name": "Target", "value": "target-value"}, map[string]any{"name": sourceName, "value": "source-value"}}
	req := Request{Mode: "deployment", Left: testInput("dev", doc(map[string]any{"items": items})), Right: testInput("prod", doc(map[string]any{"items": items})), Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}
	from := "/items/1/value"
	p.Expectations = []Expectation{{Rule: testRule("copy"), Selector: Selector{Type: "THING", Path: "/items/0/value"}, Operation: "copy_source", From: &from, Authority: "approved"}}
	r := runTest(t, req, p)
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "copy" {
			found = true
			if d.Derivation == nil || len(d.Derivation.Location.Context) != 1 || d.Derivation.Location.Context[0].Fields[0].Value != sourceName || d.Locations["actual"].Context[0].Fields[0].Value != "Target" {
				t.Fatal("origin borrowed target context", d)
			}
		}
	}
	if !found {
		t.Fatal("derived decision absent")
	}
	before := stable(r)
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		s := b.String()
		if strings.Contains(s, "<script>alert") || !strings.Contains(s, "&lt;script&gt;") {
			t.Fatal("context not escaped")
		}
	}
	if before != stable(r) {
		t.Fatal("renderer mutated context")
	}
	data, _ := json.Marshal(r)
	var copy Report
	if err := json.Unmarshal(data, &copy); err != nil {
		t.Fatal(err)
	}
	if stable(copy) != stable(r) {
		t.Fatal("context report round trip changed")
	}
}
