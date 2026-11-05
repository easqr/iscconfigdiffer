package differ

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func schemaContextFixture(reverse bool) []any {
	schemas := []any{}
	for _, name := range []string{"account", "group", "role"} {
		// Deliberately share nativeObjectType: it must not determine correspondence.
		attributes := []any{}
		for _, attr := range []string{"memberOf", "manager"} {
			attributes = append(attributes, map[string]any{"name": attr, "type": "STRING", "schema": map[string]any{"id": "shared-reference", "name": "group"}})
		}
		if reverse {
			attributes[0], attributes[1] = attributes[1], attributes[0]
		}
		schemas = append(schemas, map[string]any{"name": name, "nativeObjectType": "SharedNativeType", "id": "schema-" + name, "created": "2026-01-01T00:00:00Z", "modified": "2026-01-02T00:00:00Z", "attributes": attributes})
	}
	if reverse {
		schemas[0], schemas[2] = schemas[2], schemas[0]
	}
	return schemas
}
func schemaContextInput(t *testing.T, env, typ string, schemas []any) snapshot.Input {
	t.Helper()
	return input(t, env, []string{typ}, obj(typ, "same-source-id", "Invented", map[string]any{"schemas": schemas}))
}
func schemaContextExpectations() []v2.Expectation {
	paths := []string{"/name", "/schemas/*/name", "/schemas/*/nativeObjectType", "/schemas/*/attributes/*/name", "/schemas/*/attributes/*/type", "/schemas/*/attributes/*/schema/id", "/schemas/*/attributes/*/schema/name"}
	out := []v2.Expectation{}
	for i, path := range paths {
		out = append(out, v2.Expectation{Rule: rule(fmt.Sprintf("schema-copy-%d", i)), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
	}
	return out
}
func TestSourceSchemaAndAttributeAlignment(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		t.Run(profile, func(t *testing.T) {
			p := v2.Policy{Version: 2, ID: "invented", Profile: profile}
			req := Request{Mode: "drift", Left: schemaContextInput(t, "dev", "SOURCE", schemaContextFixture(false)), Right: schemaContextInput(t, "prod", "SOURCE", schemaContextFixture(true)), IncludeMatching: true}
			r := compareTest(t, req, p)
			if r.ExitCode() != 0 {
				t.Fatal(r)
			}
			if r.RuleUsage["isc.SOURCE.schemas"] != 1 || r.RuleUsage["isc.SOURCE.schema-attributes"] != 3 {
				t.Fatal("built-in rules not used", r.RuleUsage)
			}
			found := false
			for _, d := range r.Decisions {
				if d.Locations["left"].Pointer == "/schemas/0/attributes/0/schema/id" {
					found = true
					if d.Locations["right"].Pointer != "/schemas/2/attributes/1/schema/id" || len(d.ItemKeys) != 2 {
						t.Fatal("alignment changed original locations", d)
					}
				}
			}
			if !found {
				t.Fatal("attribute reference decision absent")
			}
		})
	}
}
func TestSourceSchemaContextAndRedaction(t *testing.T) {
	for _, mode := range []string{"drift", "deployment"} {
		for _, omit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/omit=%t", mode, omit), func(t *testing.T) {
				right := schemaContextFixture(true)
				right[2].(map[string]any)["attributes"].([]any)[1].(map[string]any)["schema"].(map[string]any)["id"] = "changed-reference-sentinel"
				req := Request{Mode: mode, Left: schemaContextInput(t, "dev", "SOURCE", schemaContextFixture(false)), Right: schemaContextInput(t, "prod", "SOURCE", right), ExcludeValues: omit}
				p := portable()
				if mode == "deployment" {
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
					p.Expectations = schemaContextExpectations()
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 1 {
					t.Fatal("nested reference change hidden", r)
				}
				lrole, rrole := "left", "right"
				if mode == "deployment" {
					lrole, rrole = "source", "actual"
				}
				found := false
				for _, d := range r.Decisions {
					if d.Locations[lrole].Pointer != "/schemas/0/attributes/0/schema/id" {
						continue
					}
					found = true
					if d.Locations[rrole].Pointer != "/schemas/2/attributes/1/schema/id" {
						t.Fatal(d)
					}
					for _, role := range []string{lrole, rrole} {
						c := d.Locations[role].Context
						if len(c) != 2 || c[0].Label != "Schema" || c[1].Label != "Attribute" || contextFieldValue(c[0], "Name") != "account" || contextFieldValue(c[0], "Native object type") != "SharedNativeType" || contextFieldValue(c[1], "Name") != "memberOf" {
							t.Fatal("wrong schema context", d)
						}
					}
					if mode == "deployment" && (d.Derivation == nil || len(d.Derivation.Location.Context) != 2) {
						t.Fatal("derived schema context missing", d)
					}
				}
				if !found {
					t.Fatal("reference difference absent")
				}
				for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
					var b bytes.Buffer
					if err := render(&b); err != nil {
						t.Fatal(err)
					}
					for _, text := range []string{"Schema", "account", "Native object type", "SharedNativeType", "Attribute", "memberOf", "/schemas/0/attributes/0/schema/id", "/schemas/2/attributes/1/schema/id"} {
						if !strings.Contains(b.String(), text) {
							t.Fatal("human context missing", text)
						}
					}
				}
				data, _ := json.Marshal(r)
				if omit && strings.Contains(string(data), "changed-reference-sentinel") {
					t.Fatal("value leaked with omission")
				}
				p.Sensitive = []v2.Sensitive{{Rule: rule("hide-schema"), Selector: v2.Selector{Type: "SOURCE", Path: "/schemas/*/attributes/*/name"}}}
				r = compareTest(t, req, p)
				data, _ = json.Marshal(r)
				for _, secret := range []string{"SharedNativeType", "memberOf", "changed-reference-sentinel"} {
					if strings.Contains(string(data), secret) {
						t.Fatal("sensitive schema context leaked", secret)
					}
				}
			})
		}
	}
}
func TestSourceSchemaNamesValidateWithinTheirParent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]any)
	}{
		{"duplicate schemas", func(s []any) { s[1].(map[string]any)["name"] = "account" }},
		{"missing schema name", func(s []any) { delete(s[0].(map[string]any), "name") }},
		{"null schema name", func(s []any) { s[0].(map[string]any)["name"] = nil }},
		{"blank schema name", func(s []any) { s[0].(map[string]any)["name"] = " " }},
		{"numeric schema name", func(s []any) { s[0].(map[string]any)["name"] = 12 }},
		{"duplicate attributes", func(s []any) { s[0].(map[string]any)["attributes"].([]any)[1].(map[string]any)["name"] = "memberOf" }},
		{"missing attribute name", func(s []any) { delete(s[0].(map[string]any)["attributes"].([]any)[0].(map[string]any), "name") }},
		{"blank attribute name", func(s []any) { s[0].(map[string]any)["attributes"].([]any)[0].(map[string]any)["name"] = " " }},
		{"null attribute name", func(s []any) { s[0].(map[string]any)["attributes"].([]any)[0].(map[string]any)["name"] = nil }},
		{"numeric attribute name", func(s []any) { s[0].(map[string]any)["attributes"].([]any)[0].(map[string]any)["name"] = 12 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schemas := schemaContextFixture(false)
			tc.change(schemas)
			for _, mode := range []string{"drift", "deployment"} {
				req := Request{Mode: mode, Left: schemaContextInput(t, "dev", "SOURCE", schemaContextFixture(false)), Right: schemaContextInput(t, "prod", "SOURCE", schemas)}
				p := portable()
				if mode == "deployment" {
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
					p.Expectations = schemaContextExpectations()
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 2 || r.Counts["conflict"] == 0 {
					t.Fatal(mode, r)
				}
			}
		})
	}
}
func TestNativeObjectTypeAndSchemaMembershipRemainComparable(t *testing.T) {
	right := schemaContextFixture(true)
	right[2].(map[string]any)["nativeObjectType"] = "ChangedNativeType"
	req := Request{Mode: "drift", Left: schemaContextInput(t, "dev", "SOURCE", schemaContextFixture(false)), Right: schemaContextInput(t, "prod", "SOURCE", right)}
	r := compareTest(t, req, portable())
	if r.ExitCode() != 1 || r.Counts["different"] != 1 {
		t.Fatal("native object type did not remain data", r)
	}
	d := r.Decisions[0]
	found := false
	for _, decision := range r.Decisions {
		if decision.Status == "different" {
			d = decision
			found = true
		}
	}
	if !found || contextFieldValue(d.Locations["right"].Context[0], "Name") != "account" || contextFieldValue(d.Locations["right"].Context[0], "Native object type") != "ChangedNativeType" {
		t.Fatal(d)
	}
	// Missing attributes have no fabricated counterpart name.
	right = schemaContextFixture(true)
	right[2].(map[string]any)["attributes"] = right[2].(map[string]any)["attributes"].([]any)[:1]
	req.Right = schemaContextInput(t, "prod", "SOURCE", right)
	r = compareTest(t, req, portable())
	if r.ExitCode() != 1 {
		t.Fatal(r)
	}
	found = false
	for _, d := range r.Decisions {
		if d.Locations["left"].Pointer == "/schemas/0/attributes/0/name" {
			found = true
			if d.Locations["right"].Present || len(d.Locations["right"].Context) != 1 {
				t.Fatal("missing attribute borrowed context", d)
			}
		}
	}
	if !found {
		t.Fatal("missing attribute not reported")
	}
	right = schemaContextFixture(true)[:2]
	req.Right = schemaContextInput(t, "prod", "SOURCE", right)
	if r = compareTest(t, req, portable()); r.ExitCode() != 1 {
		t.Fatal("schema removal hidden", r)
	}
	// SOURCE rules do not apply to a similarly shaped unrelated object type.
	req.Left = schemaContextInput(t, "dev", "ROLE", schemaContextFixture(false))
	req.Right = schemaContextInput(t, "prod", "ROLE", schemaContextFixture(true))
	if r = compareTest(t, req, portable()); r.ExitCode() != 1 || r.RuleUsage["isc.SOURCE.schemas"] != 0 {
		t.Fatal("schema rules applied to another type", r)
	}
}
