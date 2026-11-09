package differ

import (
	"fmt"
	"testing"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func accessProfileReferenceInput(t *testing.T, env, typ string, alter func(map[string]any)) snapshot.Input {
	t.Helper()
	entitlements := []any{}
	for i := 0; i < 3; i++ {
		entitlements = append(entitlements, map[string]any{"id": fmt.Sprintf("%s-entitlement-%d", env, i), "name": fmt.Sprintf("Invented Entitlement %d", i), "type": "ENTITLEMENT"})
	}
	payload := map[string]any{"source": map[string]any{"id": env + "-source", "name": "Invented Source", "type": "SOURCE"}, "entitlements": entitlements}
	if alter != nil {
		alter(payload)
	}
	return input(t, env, []string{typ}, obj(typ, "same-object-id", "Invented", payload))
}

func TestAccessProfileReferenceIDs(t *testing.T) {
	const typ = "ACCESS_PROFILE"
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "access-profile", Profile: profile}
				req := Request{Mode: mode, Left: accessProfileReferenceInput(t, "dev", typ, nil), Right: accessProfileReferenceInput(t, "prod", typ, nil)}
				status, role := "different", "left"
				if mode == "deployment" {
					status, role = "violation", "source"
					req.Operations = []v2.Operation{{Type: typ, Key: "Invented", Action: "update"}}
					for i, path := range []string{"/name", "/source/name", "/source/type", "/entitlements/*/name", "/entitlements/*/type"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule(fmt.Sprintf("copy-%d", i)), Selector: v2.Selector{Type: typ, Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: typ, Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage["isc.ACCESS_PROFILE.entitlement-id"] != 3 || r.RuleUsage["isc.ACCESS_PROFILE.source-id"] != 1 {
					t.Fatal("reference IDs affected comparison", r.Counts)
				}
				pointers := map[string]bool{}
				for _, d := range r.Decisions {
					if d.RuleID != "isc.ACCESS_PROFILE.entitlement-id" && d.RuleID != "isc.ACCESS_PROFILE.source-id" {
						continue
					}
					if d.Status != "excluded" || d.Space != "content" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.access-profile" || d.Evidence.Attributes["layer"] != "type" {
						t.Fatal("exclusion provenance missing", d)
					}
					pointers[d.Locations[role].Pointer] = true
				}
				for _, path := range []string{"/source/id", "/entitlements/0/id", "/entitlements/1/id", "/entitlements/2/id"} {
					if !pointers[path] {
						t.Fatal("array member or source ID not excluded", path)
					}
				}
				req.Right = accessProfileReferenceInput(t, "prod", typ, func(o map[string]any) {
					delete(o["source"].(map[string]any), "id")
					for _, e := range o["entitlements"].([]any) {
						delete(e.(map[string]any), "id")
					}
				})
				if r = compareTest(t, req, p); r.ExitCode() != 0 {
					t.Fatal("absent reference IDs affected comparison", r.Counts)
				}
				for _, reference := range []string{"source", "entitlement"} {
					for _, field := range []string{"name", "type"} {
						for _, remove := range []bool{false, true} {
							req.Right = accessProfileReferenceInput(t, "prod", typ, func(o map[string]any) {
								target := o["source"].(map[string]any)
								if reference == "entitlement" {
									target = o["entitlements"].([]any)[1].(map[string]any)
								}
								if remove {
									delete(target, field)
								} else {
									target[field] = "different"
								}
							})
							if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
								t.Fatal("reference name/type was hidden", reference, field, remove, r.Counts)
							}
						}
					}
				}
				req.Right = accessProfileReferenceInput(t, "prod", typ, nil)
				for _, id := range []string{"isc.ACCESS_PROFILE.entitlement-id", "isc.ACCESS_PROFILE.source-id"} {
					p.DisabledRules = []string{id}
					want := 1
					if mode == "deployment" {
						want = 2
					}
					if r = compareTest(t, req, p); r.ExitCode() != want || r.RuleUsage[id] != 0 {
						t.Fatal("reference exclusion could not be disabled", id, r.Counts)
					}
				}
			})
		}
	}
}

func TestAccessProfileReferenceExclusionsPreserveOtherDifferences(t *testing.T) {
	for _, change := range []string{"missing-entitlement", "missing-source", "ordering", "other-id"} {
		t.Run(change, func(t *testing.T) {
			left := accessProfileReferenceInput(t, "dev", "ACCESS_PROFILE", func(o map[string]any) {
				if change == "other-id" {
					o["source"].(map[string]any)["details"] = map[string]any{"id": "original"}
				}
			})
			right := accessProfileReferenceInput(t, "prod", "ACCESS_PROFILE", func(o map[string]any) {
				switch change {
				case "missing-entitlement":
					o["entitlements"] = o["entitlements"].([]any)[:2]
				case "missing-source":
					delete(o, "source")
				case "ordering":
					e := o["entitlements"].([]any)
					e[0], e[1] = e[1], e[0]
				case "other-id":
					o["source"].(map[string]any)["details"] = map[string]any{"id": "other"}
				}
			})
			r := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, portable())
			if r.ExitCode() != 1 || r.Counts["different"] == 0 {
				t.Fatal("unrelated difference hidden", change, r.Counts)
			}
		})
	}
	for _, typ := range []string{"ROLE", "WORKFLOW", "SOURCE"} {
		r := compareTest(t, Request{Mode: "drift", Left: accessProfileReferenceInput(t, "dev", typ, nil), Right: accessProfileReferenceInput(t, "prod", typ, nil)}, portable())
		want := 4
		if typ == "ROLE" {
			want = 1
		} // ROLE has its own entitlement-ID exclusion; /source/id remains comparable.
		if r.ExitCode() != 1 || r.Counts["different"] != want || r.RuleUsage["isc.ACCESS_PROFILE.entitlement-id"] != 0 || r.RuleUsage["isc.ACCESS_PROFILE.source-id"] != 0 {
			t.Fatal("ACCESS_PROFILE rules escaped type scope", typ, r.Counts)
		}
	}
}
