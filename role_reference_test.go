package differ

import (
	"fmt"
	"testing"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func roleReferenceInput(t *testing.T, env, typ string, alter func(map[string]any)) snapshot.Input {
	t.Helper()
	profiles, entitlements := []any{}, []any{}
	for i := 0; i < 3; i++ {
		profiles = append(profiles, map[string]any{"id": fmt.Sprintf("%s-profile-%d", env, i), "name": fmt.Sprintf("Invented Access Profile %d", i), "type": "ACCESS_PROFILE"})
		entitlements = append(entitlements, map[string]any{"id": fmt.Sprintf("%s-entitlement-%d", env, i), "name": fmt.Sprintf("Invented Entitlement %d", i), "type": "ENTITLEMENT", "sourceId": env + "-source", "sourceName": "Invented Source"})
	}
	payload := map[string]any{"accessProfiles": profiles, "entitlements": entitlements}
	if alter != nil {
		alter(payload)
	}
	return input(t, env, []string{typ}, obj(typ, "same-object-id", "Invented", payload))
}

func TestRoleReferenceIDs(t *testing.T) {
	ids := []string{"isc.ROLE.access-profile-id", "isc.ROLE.entitlement-id", "isc.ROLE.entitlement-source-id"}
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "role", Profile: profile}
				req := Request{Mode: mode, Left: roleReferenceInput(t, "dev", "ROLE", nil), Right: roleReferenceInput(t, "prod", "ROLE", nil)}
				status, role := "different", "left"
				if mode == "deployment" {
					status, role = "violation", "source"
					req.Operations = []v2.Operation{{Type: "ROLE", Key: "Invented", Action: "update"}}
					for i, path := range []string{"/name", "/accessProfiles/*/name", "/accessProfiles/*/type", "/entitlements/*/name", "/entitlements/*/type", "/entitlements/*/sourceName"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule(fmt.Sprintf("copy-%d", i)), Selector: v2.Selector{Type: "ROLE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "ROLE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 {
					t.Fatal("ROLE reference IDs caused drift", r.Counts)
				}
				pointers := map[string]bool{}
				for _, id := range ids {
					if r.RuleUsage[id] != 3 {
						t.Fatal("array members not excluded", id, r.RuleUsage)
					}
				}
				for _, d := range r.Decisions {
					if d.RuleID != ids[0] && d.RuleID != ids[1] && d.RuleID != ids[2] {
						continue
					}
					if d.Status != "excluded" || d.Space != "content" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.role" || d.Evidence.Attributes["layer"] != "type" {
						t.Fatal("ROLE exclusion provenance absent", d)
					}
					pointers[d.Locations[role].Pointer] = true
				}
				for i := 0; i < 3; i++ {
					for _, path := range []string{fmt.Sprintf("/accessProfiles/%d/id", i), fmt.Sprintf("/entitlements/%d/id", i), fmt.Sprintf("/entitlements/%d/sourceId", i)} {
						if !pointers[path] {
							t.Fatal("reference exclusion missing", path)
						}
					}
				}
				req.Right = roleReferenceInput(t, "prod", "ROLE", func(o map[string]any) {
					for _, p := range o["accessProfiles"].([]any) {
						delete(p.(map[string]any), "id")
					}
					for _, e := range o["entitlements"].([]any) {
						delete(e.(map[string]any), "id")
						delete(e.(map[string]any), "sourceId")
					}
				})
				if r = compareTest(t, req, p); r.ExitCode() != 0 {
					t.Fatal("missing IDs caused drift", r.Counts)
				}
				for _, collection := range []string{"accessProfiles", "entitlements"} {
					fields := []string{"name", "type"}
					if collection == "entitlements" {
						fields = append(fields, "sourceName")
					}
					for _, field := range fields {
						for _, remove := range []bool{false, true} {
							req.Right = roleReferenceInput(t, "prod", "ROLE", func(o map[string]any) {
								m := o[collection].([]any)[1].(map[string]any)
								if remove {
									delete(m, field)
								} else {
									m[field] = "different"
								}
							})
							if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
								t.Fatal("reference field hidden", collection, field, remove, r.Counts)
							}
						}
					}
				}
				req.Right = roleReferenceInput(t, "prod", "ROLE", nil)
				for _, id := range ids {
					p.DisabledRules = []string{id}
					want := 1
					decision := "different"
					if mode == "deployment" {
						want = 2
						decision = "unverified"
					}
					if r = compareTest(t, req, p); r.ExitCode() != want || r.Counts[decision] != 3 || r.RuleUsage[id] != 0 {
						t.Fatal("disabled rule still active", id, r.Counts)
					}
				}
			})
		}
	}
}

func TestRoleReferenceExclusionsAreLimitedToRequestedPaths(t *testing.T) {
	for _, collection := range []string{"accessProfiles", "entitlements"} {
		for _, change := range []string{"member-missing", "ordering", "nested-id"} {
			t.Run(collection+"/"+change, func(t *testing.T) {
				left := roleReferenceInput(t, "dev", "ROLE", func(o map[string]any) {
					if change == "nested-id" {
						o[collection].([]any)[0].(map[string]any)["details"] = map[string]any{"id": "dev-detail"}
					}
				})
				right := roleReferenceInput(t, "prod", "ROLE", func(o map[string]any) {
					a := o[collection].([]any)
					switch change {
					case "member-missing":
						o[collection] = a[:2]
					case "ordering":
						a[0], a[1] = a[1], a[0]
					case "nested-id":
						a[0].(map[string]any)["details"] = map[string]any{"id": "prod-detail"}
					}
				})
				r := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, portable())
				if r.ExitCode() != 1 || r.Counts["different"] == 0 {
					t.Fatal("other difference hidden", r.Counts)
				}
			})
		}
	}
	for _, typ := range []string{"SOURCE", "WORKFLOW"} {
		r := compareTest(t, Request{Mode: "drift", Left: roleReferenceInput(t, "dev", typ, nil), Right: roleReferenceInput(t, "prod", typ, nil)}, portable())
		if r.ExitCode() != 1 || r.Counts["different"] != 9 {
			t.Fatal("ROLE exclusions escaped type scope", typ, r.Counts)
		}
		for _, id := range []string{"isc.ROLE.access-profile-id", "isc.ROLE.entitlement-id", "isc.ROLE.entitlement-source-id"} {
			if r.RuleUsage[id] != 0 {
				t.Fatal("ROLE rule applied to another type", typ, id)
			}
		}
	}
	r := compareTest(t, Request{Mode: "drift", Left: roleReferenceInput(t, "dev", "ROLE", func(o map[string]any) { o["source"] = map[string]any{"id": "dev-source"} }), Right: roleReferenceInput(t, "prod", "ROLE", func(o map[string]any) { o["source"] = map[string]any{"id": "prod-source"} })}, portable())
	if r.ExitCode() != 1 || r.Counts["different"] != 1 {
		t.Fatal("ROLE top-level source reference excluded", r.Counts)
	}
}
