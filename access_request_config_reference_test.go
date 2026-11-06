package differ

import (
	"fmt"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

const grantApprovalIDRule = "isc.ACCESS_REQUEST_CONFIG.grant-approval-scheme-id"
const grantApprovalPath = "/entitlementRequestConfig/grantRequestApprovalSchemes"

func accessRequestConfigReferenceInput(t *testing.T, env, typ string, alter func(map[string]any)) snapshot.Input {
	t.Helper()
	schemes := []any{}
	for i := 0; i < 3; i++ {
		schemes = append(schemes, map[string]any{"id": fmt.Sprintf("%s-scheme-%d", env, i), "name": fmt.Sprintf("Invented Approval Scheme %d", i), "type": "MANAGER"})
	}
	config := map[string]any{"enabled": true, "grantRequestApprovalSchemes": schemes}
	if alter != nil {
		alter(config)
	}
	name := "Invented"
	if typ == "ACCESS_REQUEST_CONFIG" {
		name = ""
	}
	return input(t, env, []string{typ}, obj(typ, "same-object-id", name, map[string]any{"entitlementRequestConfig": config}))
}

func TestAccessRequestConfigApprovalSchemeIDs(t *testing.T) {
	const typ = "ACCESS_REQUEST_CONFIG"
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "access-request", Profile: profile}
				req := Request{Mode: mode, Left: accessRequestConfigReferenceInput(t, "dev", typ, nil), Right: accessRequestConfigReferenceInput(t, "prod", typ, nil)}
				status, role := "different", "left"
				if mode == "deployment" {
					status, role = "violation", "source"
					req.Operations = []v2.Operation{{Type: typ, Key: "singleton", Action: "update"}}
					for i, path := range []string{"/entitlementRequestConfig/enabled", grantApprovalPath + "/*/name", grantApprovalPath + "/*/type"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule(fmt.Sprintf("copy-%d", i)), Selector: v2.Selector{Type: typ, Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: typ, Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage[grantApprovalIDRule] != 3 {
					t.Fatal("scheme IDs caused findings", r.Counts)
				}
				paths := map[string]bool{}
				for _, d := range r.Decisions {
					if d.RuleID == grantApprovalIDRule {
						if d.Status != "excluded" || d.Space != "content" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.access-request-config" || d.Evidence.Attributes["layer"] != "type" {
							t.Fatal("exclusion provenance absent", d)
						}
						paths[d.Locations[role].Pointer] = true
					}
				}
				for i := 0; i < 3; i++ {
					if !paths[fmt.Sprintf("%s/%d/id", grantApprovalPath, i)] {
						t.Fatal("scheme member missed", i)
					}
				}
				req.Right = accessRequestConfigReferenceInput(t, "prod", typ, func(o map[string]any) {
					for _, s := range o["grantRequestApprovalSchemes"].([]any) {
						delete(s.(map[string]any), "id")
					}
				})
				if r = compareTest(t, req, p); r.ExitCode() != 0 {
					t.Fatal("absent scheme IDs caused findings", r.Counts)
				}
				for _, field := range []string{"name", "type"} {
					for _, remove := range []bool{false, true} {
						req.Right = accessRequestConfigReferenceInput(t, "prod", typ, func(o map[string]any) {
							s := o["grantRequestApprovalSchemes"].([]any)[1].(map[string]any)
							if remove {
								delete(s, field)
							} else {
								s[field] = "different"
							}
						})
						if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
							t.Fatal("scheme field hidden", field, remove, r.Counts)
						}
					}
				}
				req.Right = accessRequestConfigReferenceInput(t, "prod", typ, func(o map[string]any) { o["enabled"] = false })
				if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
					t.Fatal("adjacent setting hidden", r.Counts)
				}
				req.Right = accessRequestConfigReferenceInput(t, "prod", typ, nil)
				p.DisabledRules = []string{grantApprovalIDRule}
				want, decision := 1, "different"
				if mode == "deployment" {
					want, decision = 2, "unverified"
				}
				if r = compareTest(t, req, p); r.ExitCode() != want || r.Counts[decision] != 3 || r.RuleUsage[grantApprovalIDRule] != 0 {
					t.Fatal("scheme exclusion could not be disabled", r.Counts)
				}
			})
		}
	}
}

func TestAccessRequestConfigApprovalSchemeScope(t *testing.T) {
	const typ = "ACCESS_REQUEST_CONFIG"
	for _, change := range []string{"member-missing", "ordering", "nested-id", "other-id", "other-schemes"} {
		t.Run(change, func(t *testing.T) {
			makeInput := func(env string, changed bool) snapshot.Input {
				return accessRequestConfigReferenceInput(t, env, typ, func(o map[string]any) {
					if change == "nested-id" {
						id := "original"
						if changed {
							id = "changed"
						}
						o["grantRequestApprovalSchemes"].([]any)[0].(map[string]any)["details"] = map[string]any{"id": id}
					}
					if change == "other-id" {
						id := "original"
						if changed {
							id = "changed"
						}
						o["id"] = id
					}
					if change == "other-schemes" {
						id := "original"
						if changed {
							id = "changed"
						}
						o["customApprovalSchemes"] = []any{map[string]any{"id": id}}
					}
					if !changed {
						return
					}
					switch change {
					case "member-missing":
						o["grantRequestApprovalSchemes"] = o["grantRequestApprovalSchemes"].([]any)[:2]
					case "ordering":
						a := o["grantRequestApprovalSchemes"].([]any)
						a[0], a[1] = a[1], a[0]
					}
				})
			}
			r := compareTest(t, Request{Mode: "drift", Left: makeInput("dev", false), Right: makeInput("prod", true)}, portable())
			if r.ExitCode() != 1 || r.Counts["different"] == 0 {
				t.Fatal("other difference hidden", change, r.Counts)
			}
		})
	}
	for _, otherType := range []string{"SOURCE", "ROLE", "WORKFLOW"} {
		r := compareTest(t, Request{Mode: "drift", Left: accessRequestConfigReferenceInput(t, "dev", otherType, nil), Right: accessRequestConfigReferenceInput(t, "prod", otherType, nil)}, portable())
		if r.ExitCode() != 1 || r.Counts["different"] != 3 || r.RuleUsage[grantApprovalIDRule] != 0 {
			t.Fatal("scheme exclusion escaped type scope", otherType, r.Counts)
		}
	}
}
