package differ

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func identityProfileTransformsInput(t *testing.T, env string, alter func([]any)) snapshot.Input {
	t.Helper()
	transforms := []any{}
	for _, name := range []string{"email", "displayName"} {
		transforms = append(transforms, map[string]any{
			"identityAttributeName": name,
			"transformDefinition": map[string]any{"type": "accountAttribute", "attributes": map[string]any{
				"sourceId": env + "-source", "sourceName": "Invented Directory", "attributeName": name,
				"input": map[string]any{"type": "static", "attributes": map[string]any{"value": "invented-value", "sourceId": env + "-nested-id"}},
			}},
		})
	}
	if env == "prod" {
		transforms[0], transforms[1] = transforms[1], transforms[0]
	}
	if alter != nil {
		alter(transforms)
	}
	return input(t, env, []string{"IDENTITY_PROFILE"}, obj("IDENTITY_PROFILE", "same-profile-id", "Invented Profile", map[string]any{"identityAttributeConfig": map[string]any{"enabled": true, "attributeTransforms": transforms}}))
}

func TestIdentityProfileAttributeTransformAlignment(t *testing.T) {
	const collection = "isc.IDENTITY_PROFILE.attribute-transforms"
	const exclusion = "isc.IDENTITY_PROFILE.attribute-transform-source-id"
	const inputExclusion = "isc.IDENTITY_PROFILE.attribute-transform-input-source-id"
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "transforms", Profile: profile}
				req := Request{Mode: mode, Left: identityProfileTransformsInput(t, "dev", nil), Right: identityProfileTransformsInput(t, "prod", nil), IncludeMatching: true}
				if mode == "deployment" {
					req.Operations = []v2.Operation{{Type: "IDENTITY_PROFILE", Key: "Invented Profile", Action: "update"}}
					for i, path := range []string{"/name", "/identityAttributeConfig/enabled", "/identityAttributeConfig/attributeTransforms/*/identityAttributeName", "/identityAttributeConfig/attributeTransforms/*/transformDefinition/type", "/identityAttributeConfig/attributeTransforms/*/transformDefinition/attributes/sourceName", "/identityAttributeConfig/attributeTransforms/*/transformDefinition/attributes/attributeName", "/identityAttributeConfig/attributeTransforms/*/transformDefinition/attributes/input/type", "/identityAttributeConfig/attributeTransforms/*/transformDefinition/attributes/input/attributes/value"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule(fmt.Sprintf("copy-%d", i)), Selector: v2.Selector{Type: "IDENTITY_PROFILE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "IDENTITY_PROFILE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage[collection] != 1 || r.RuleUsage[exclusion] != 2 || r.RuleUsage[inputExclusion] != 2 {
					t.Fatal("order or source IDs affected equivalence", r.Counts)
				}
				leftRole, rightRole := "left", "right"
				if mode == "deployment" {
					leftRole, rightRole = "source", "actual"
				}
				found := false
				for _, d := range r.Decisions {
					if d.Locations[leftRole].Pointer == "/identityAttributeConfig/attributeTransforms/0/transformDefinition/attributes/sourceName" {
						found = true
						if d.Locations[rightRole].Pointer != "/identityAttributeConfig/attributeTransforms/1/transformDefinition/attributes/sourceName" {
							t.Fatal("original pointers lost", d)
						}
						for _, role := range []string{leftRole, rightRole} {
							c := d.Locations[role].Context
							if len(c) != 1 || contextFieldValue(c[0], "Identity attribute") != "email" {
								t.Fatal("identity attribute context missing", d)
							}
						}
					}
					if (d.RuleID == exclusion || d.RuleID == inputExclusion) && (d.Status != "excluded" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.identity-profile") {
						t.Fatal("exclusion provenance missing", d)
					}
				}
				if !found {
					t.Fatal("nested setting not compared")
				}
				changes := []struct {
					name   string
					change func(map[string]any)
				}{
					{"transform type", func(o map[string]any) { o["type"] = "changed" }},
					{"source name", func(o map[string]any) { o["attributes"].(map[string]any)["sourceName"] = "changed" }},
					{"attribute name", func(o map[string]any) { o["attributes"].(map[string]any)["attributeName"] = "changed" }},
					{"nested value", func(o map[string]any) {
						o["attributes"].(map[string]any)["input"].(map[string]any)["attributes"].(map[string]any)["value"] = "changed"
					}},
					{"input transform type", func(o map[string]any) {
						o["attributes"].(map[string]any)["input"].(map[string]any)["type"] = "changed"
					}},
				}
				for _, change := range changes {
					req.Right = identityProfileTransformsInput(t, "prod", func(items []any) { change.change(items[1].(map[string]any)["transformDefinition"].(map[string]any)) })
					r = compareTest(t, req, p)
					if r.ExitCode() != 1 {
						t.Fatal("nested difference hidden", change.name, r.Counts)
					}
				}
				req.Right = identityProfileTransformsInput(t, "prod", func(items []any) {
					for _, item := range items {
						attrs := item.(map[string]any)["transformDefinition"].(map[string]any)["attributes"].(map[string]any)
						delete(attrs, "sourceId")
						delete(attrs["input"].(map[string]any)["attributes"].(map[string]any), "sourceId")
					}
				})
				if r = compareTest(t, req, p); r.ExitCode() != 0 {
					t.Fatal("absent ignored ID affected result", r.Counts)
				}
			})
		}
	}
}

func TestIdentityProfileAttributeTransformConflictsAndOptOut(t *testing.T) {
	for _, missing := range []bool{false, true} {
		req := Request{Mode: "drift", Left: identityProfileTransformsInput(t, "dev", nil), Right: identityProfileTransformsInput(t, "prod", func(items []any) {
			if missing {
				delete(items[0].(map[string]any), "identityAttributeName")
			} else {
				items[0].(map[string]any)["identityAttributeName"] = items[1].(map[string]any)["identityAttributeName"]
			}
		})}
		if r := compareTest(t, req, portable()); r.ExitCode() != 2 {
			t.Fatal("ambiguous key accepted", missing, r.Counts)
		}
	}
	for _, id := range []string{"isc.IDENTITY_PROFILE.attribute-transforms", "isc.IDENTITY_PROFILE.attribute-transform-source-id", "isc.IDENTITY_PROFILE.attribute-transform-input-source-id"} {
		p := portable()
		p.DisabledRules = []string{id}
		req := Request{Mode: "drift", Left: identityProfileTransformsInput(t, "dev", nil), Right: identityProfileTransformsInput(t, "prod", nil)}
		if r := compareTest(t, req, p); r.ExitCode() != 1 || r.RuleUsage[id] != 0 {
			t.Fatal("rule cannot be disabled", id, r.Counts)
		}
	}
}

func TestIdentityProfileTransformReportSections(t *testing.T) {
	for _, omit := range []bool{false, true} {
		req := Request{Mode: "drift", Left: identityProfileTransformsInput(t, "dev", nil), Right: identityProfileTransformsInput(t, "prod", func(items []any) {
			for _, item := range items {
				attrs := item.(map[string]any)["transformDefinition"].(map[string]any)["attributes"].(map[string]any)
				attrs["sourceName"] = "changed-directory"
				attrs["attributeName"] = "changed-attribute"
			}
		}), ExcludeValues: omit}
		r := compareTest(t, req, portable())
		if r.Counts["different"] != 4 {
			t.Fatal("nested decisions lost", r.Counts)
		}
		for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
			var b bytes.Buffer
			if err := render(&b); err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{"Identity attribute transform: email", "Identity attribute transform: displayName", "/identityAttributeConfig/attributeTransforms/0/transformDefinition/attributes/sourceName", "/identityAttributeConfig/attributeTransforms/1/transformDefinition/attributes/attributeName"} {
				if !strings.Contains(b.String(), text) {
					t.Fatal("report grouping or original pointer missing", text)
				}
			}
			if omit && strings.Contains(b.String(), "changed-directory") {
				t.Fatal("omitted value leaked")
			}
		}
	}
}
