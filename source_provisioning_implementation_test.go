package differ

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestSourceProvisioningImplementationPresentation(t *testing.T) {
	policies := func(prod bool) []any {
		out := []any{}
		for _, usage := range []string{"ENABLE", "DISABLE"} {
			transform := map[string]any{"type": "concat", "attributes": map[string]any{"values": []any{"invented-first", "invented-last"}}}
			if usage == "DISABLE" {
				transform = map[string]any{"type": "lookup", "attributes": map[string]any{"input": "invented-input", "table": map[string]any{"a": "invented-ou"}}}
			}
			if prod {
				transform = map[string]any{"type": "static", "attributes": map[string]any{"calculate": map[string]any{"type": "identityAttribute", "attributes": map[string]any{"name": "invented-attribute"}}, "value": "invented-template"}}
			}
			fields := []any{map[string]any{"name": "target", "transform": transform}, map[string]any{"name": "unchanged", "transform": map[string]any{"type": "static", "attributes": map[string]any{"value": "same"}}}}
			if prod {
				fields[0], fields[1] = fields[1], fields[0]
			}
			out = append(out, map[string]any{"usageType": usage, "name": "Invented " + usage, "fields": fields})
		}
		if prod {
			out[0], out[1] = out[1], out[0]
		}
		return out
	}
	r := compareTest(t, Request{Mode: "drift", Left: provisioningInput(t, "dev", "SOURCE", policies(false)), Right: provisioningInput(t, "prod", "SOURCE", policies(true))}, portable())
	before, _ := json.Marshal(r)
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		if strings.Count(b.String(), "Transform implementation differs") != 2 {
			t.Fatal("policies combined or not grouped")
		}
		for _, text := range []string{"Invented ENABLE", "Invented DISABLE", "target", "concat", "lookup", "static", "/provisioningPolicies/0/fields/0/transform/type", "/provisioningPolicies/1/fields/1/transform/type", "Nested decisions"} {
			if !strings.Contains(b.String(), text) {
				t.Fatal("context or physical pointers missing", text)
			}
		}
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("presentation changed JSON")
	}
}
