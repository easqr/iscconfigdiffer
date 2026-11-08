package differ

import (
	"fmt"
	"strings"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
)

func schemaMetadataInput(t *testing.T, env, typ string, reverse, changeNestedID bool) snapshot.Input {
	t.Helper()
	schemas := []any{}
	for i := 0; i < 3; i++ {
		nestedID := "shared-reference"
		if changeNestedID && i == 2 {
			nestedID = "changed-reference"
		}
		schemas = append(schemas, map[string]any{
			"name": fmt.Sprintf("schema-%d", i), "id": fmt.Sprintf("%s-schema-%d", env, i),
			"created": env + "-created", "modified": env + "-modified",
			"attributes": []any{map[string]any{"name": "member", "schema": map[string]any{"id": nestedID}}},
		})
	}
	if reverse {
		schemas[0], schemas[2] = schemas[2], schemas[0]
	}
	return input(t, env, []string{typ}, obj(typ, "same-source-id", "Invented", map[string]any{"schemas": schemas}))
}

func TestPortableSourceSchemaMetadataExclusions(t *testing.T) {
	for _, keyed := range []bool{false, true} {
		t.Run(fmt.Sprintf("keyed=%t", keyed), func(t *testing.T) {
			left := schemaMetadataInput(t, "dev", "SOURCE", false, false)
			right := schemaMetadataInput(t, "prod", "SOURCE", keyed, false)
			p := portable()
			if keyed {
				p.Collections = []v2.Collection{{Rule: v2.Rule{ID: "schemas-by-name", Replaces: "isc.SOURCE.schemas", Evidence: &v2.Evidence{Kind: "invented-policy"}}, Selector: v2.Selector{Type: "SOURCE", Path: "/schemas"}, Mode: "keyed", Keys: []string{"/name"}}}
			}
			for _, mode := range []string{"drift", "deployment"} {
				t.Run(mode, func(t *testing.T) {
					req := Request{Mode: mode, Left: left, Right: right}
					policy := p
					if mode == "deployment" {
						req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
						for i, path := range []string{"/name", "/schemas/*/name", "/schemas/*/attributes/*/name"} {
							policy.Expectations = append(policy.Expectations, v2.Expectation{Rule: rule(fmt.Sprintf("copy-%d", i)), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-policy"})
						}
					}
					r := compareTest(t, req, policy)
					if r.ExitCode() != 0 {
						t.Fatal("schema metadata caused drift or required deployment authority", r)
					}
					role := "left"
					if mode == "deployment" {
						role = "source"
					}
					for _, field := range []string{"id", "created", "modified"} {
						ruleID := "isc.SOURCE.schema-" + field
						if r.RuleUsage[ruleID] != 3 {
							t.Fatal("not all schemas excluded", ruleID, r.RuleUsage[ruleID])
						}
						found := false
						for _, d := range r.Decisions {
							if d.RuleID == ruleID && d.Locations[role].Pointer == "/schemas/2/"+field {
								found = d.Status == "excluded" && d.Space == "content" && d.Evidence != nil && d.Evidence.Kind == "isc-profile"
							}
						}
						if !found {
							t.Fatal("missing exclusion provenance for third schema", field, r)
						}
					}
					req.Right = schemaMetadataInput(t, "prod", "SOURCE", keyed, true)
					r = compareTest(t, req, policy)
					if r.ExitCode() != 0 {
						t.Fatal("nested schema reference ID caused drift", r)
					}
				})
			}
		})
	}
}

func TestSchemaMetadataExclusionsAreLimitedToPortableSource(t *testing.T) {
	for _, tc := range []struct{ profile, typ string }{
		{"isc-envelope-v2", "SOURCE"}, {"isc-identity-v2", "SOURCE"}, {"isc-portable-v2", "ROLE"},
	} {
		t.Run(tc.profile+"-"+tc.typ, func(t *testing.T) {
			p := v2.Policy{Version: 2, ID: "invented", Profile: tc.profile}
			r := compareTest(t, Request{Mode: "drift", Left: schemaMetadataInput(t, "dev", tc.typ, false, false), Right: schemaMetadataInput(t, "prod", tc.typ, false, false)}, p)
			found := false
			for _, d := range r.Decisions {
				if d.Status == "different" && strings.HasPrefix(d.Locations["left"].Pointer, "/schemas/") {
					found = true
				}
			}
			if r.ExitCode() != 1 || !found {
				t.Fatal("schema exclusions leaked beyond portable SOURCE", r)
			}
		})
	}
}
