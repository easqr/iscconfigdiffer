package differ

import (
	"fmt"
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
	"github.com/easqr/iscconfigexporter/snapshot"
)

func lifecycleStateInput(t *testing.T, env string, alter func(map[string]any)) snapshot.Input {
	t.Helper()
	refs := func(prefix string) []any {
		return []any{
			map[string]any{"id": env + prefix + "a", "name": "Invented Directory A", "type": "SOURCE"},
			map[string]any{"id": env + prefix + "b", "name": "Invented Directory B", "type": "SOURCE"},
		}
	}
	payload := map[string]any{
		"technicalName":      "active",
		"identityProfileRef": map[string]any{"id": env + "-profile", "name": "Invented People", "type": "IDENTITY_PROFILE"},
		"accountActionRefs": []any{
			map[string]any{"action": "ENABLE", "allSources": false, "sourceIdsRefs": refs("enable")},
			map[string]any{"action": "DISABLE", "allSources": false, "sourceIdsRefs": refs("disable")},
		},
		"accountActions": []any{map[string]any{"sourceIds": []any{env + "-source"}}},
	}
	if env == "prod" {
		for _, action := range payload["accountActionRefs"].([]any) {
			sources := action.(map[string]any)["sourceIdsRefs"].([]any)
			sources[0], sources[1] = sources[1], sources[0]
		}
	}
	if alter != nil {
		alter(payload)
	}
	return input(t, env, []string{"IDENTITY_PROFILE", "LIFECYCLE_STATE"},
		obj("IDENTITY_PROFILE", env+"-profile", "Invented People", nil),
		obj("LIFECYCLE_STATE", "same-state-id", "Active", payload))
}

func TestLifecycleStateAccountActionRules(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "lifecycle", Profile: profile}
				// Isolate the lifecycle rules from the parent's physical envelope ID.
				if profile != "isc-portable-v2" {
					p.Ignores = []v2.Ignore{{Rule: rule("parent-envelope-id"), Selector: v2.Selector{Type: "IDENTITY_PROFILE", Space: "metadata", Path: "/self/id"}}}
				}
				req := Request{Mode: "drift", Left: lifecycleStateInput(t, "dev", nil), Right: lifecycleStateInput(t, "prod", nil)}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 {
					t.Fatal("reordered sources or physical IDs affected equivalence", r.Counts)
				}
				if mode == "deployment" {
					key := ""
					for _, d := range r.Decisions {
						if d.Type == "LIFECYCLE_STATE" {
							key = d.Key
							break
						}
					}
					if key == "" {
						t.Fatal("lifecycle state not compared")
					}
					req.Mode = mode
					req.Operations = []v2.Operation{{Type: "LIFECYCLE_STATE", Key: key, Action: "update"}, {Type: "IDENTITY_PROFILE", Key: "Invented People", Action: "update"}}
					p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("parent-name"), Selector: v2.Selector{Type: "IDENTITY_PROFILE", Path: "/name"}, Operation: "copy_source", Authority: "invented-release"})
					for i, path := range []string{"/name", "/technicalName", "/identityProfileRef/name", "/identityProfileRef/type", "/accountActionRefs/*/action", "/accountActionRefs/*/allSources", "/accountActionRefs/*/sourceIdsRefs/*/name", "/accountActionRefs/*/sourceIdsRefs/*/type"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule(fmt.Sprintf("copy-%d", i)), Selector: v2.Selector{Type: "LIFECYCLE_STATE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "LIFECYCLE_STATE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
					r = compareTest(t, req, p)
				}
				for id, want := range map[string]int{"isc.LIFECYCLE_STATE.account-action-source-id": 4, "isc.LIFECYCLE_STATE.account-actions": 1, "isc.LIFECYCLE_STATE.identity-profile-id": 1} {
					if r.ExitCode() != 0 || r.RuleUsage[id] != want {
						t.Fatal("exclusions missing", id, r.Counts, r.RuleUsage)
					}
					found := false
					for _, d := range r.Decisions {
						if d.RuleID == id {
							found = true
							if d.Status != "excluded" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.lifecycle-state" {
								t.Fatal("missing exclusion provenance", d)
							}
						}
					}
					if !found {
						t.Fatal("missing exclusion decision", id)
					}
				}
				req.Right = lifecycleStateInput(t, "prod", func(o map[string]any) {
					delete(o, "accountActions")
					for _, a := range o["accountActionRefs"].([]any) {
						for _, s := range a.(map[string]any)["sourceIdsRefs"].([]any) {
							delete(s.(map[string]any), "id")
						}
					}
				})
				if r = compareTest(t, req, p); r.ExitCode() != 0 {
					t.Fatal("absent ignored values affected equivalence", r.Counts)
				}
				for _, path := range []string{"source-type", "source-name", "profile-name", "profile-type", "action"} {
					req.Right = lifecycleStateInput(t, "prod", func(o map[string]any) {
						action := o["accountActionRefs"].([]any)[1].(map[string]any)
						source := action["sourceIdsRefs"].([]any)[0].(map[string]any)
						switch path {
						case "source-type":
							source["type"] = "CHANGED"
						case "source-name":
							source["name"] = "Changed Directory"
						case "profile-name":
							o["identityProfileRef"].(map[string]any)["name"] = "Changed People"
						case "profile-type":
							o["identityProfileRef"].(map[string]any)["type"] = "CHANGED"
						case "action":
							action["action"] = "DELETE"
						}
					})
					wantExit := 1
					if mode == "deployment" && path == "source-name" {
						// Renaming the key creates an unmatched item without whole-item authority.
						wantExit = 2
					}
					if r = compareTest(t, req, p); r.ExitCode() != wantExit {
						t.Fatal("meaningful change hidden", path, r.Counts)
					}
				}
			})
		}
	}
}

func TestLifecycleStateSourceKeyConflicts(t *testing.T) {
	for _, missing := range []bool{false, true} {
		req := Request{Mode: "drift", Left: lifecycleStateInput(t, "dev", nil), Right: lifecycleStateInput(t, "prod", func(o map[string]any) {
			sources := o["accountActionRefs"].([]any)[1].(map[string]any)["sourceIdsRefs"].([]any)
			if missing {
				delete(sources[0].(map[string]any), "name")
			} else {
				sources[0].(map[string]any)["name"] = sources[1].(map[string]any)["name"]
			}
		})}
		if r := compareTest(t, req, portable()); r.ExitCode() != 2 {
			t.Fatal("ambiguous key accepted", missing, r.Counts)
		}
	}
}

func TestLifecycleStateExclusionScopeAndOptOut(t *testing.T) {
	for _, id := range []string{"isc.LIFECYCLE_STATE.account-action-source-id", "isc.LIFECYCLE_STATE.account-actions", "isc.LIFECYCLE_STATE.identity-profile-id", "isc.LIFECYCLE_STATE.account-action-sources"} {
		p := portable()
		p.DisabledRules = []string{id}
		req := Request{Mode: "drift", Left: lifecycleStateInput(t, "dev", nil), Right: lifecycleStateInput(t, "prod", nil)}
		if r := compareTest(t, req, p); r.ExitCode() != 1 || r.RuleUsage[id] != 0 {
			t.Fatal("default cannot be disabled", id, r.Counts)
		}
	}
	for _, field := range []string{"accountActions", "identityProfileRef", "accountActionRefs"} {
		p := portable()
		req := Request{Mode: "drift", Left: sourceReferenceInput(t, "dev", "WORKFLOW", field, map[string]any{"id": "dev", "name": "A"}), Right: sourceReferenceInput(t, "prod", "WORKFLOW", field, map[string]any{"id": "prod", "name": "B"})}
		if r := compareTest(t, req, p); r.ExitCode() != 1 {
			t.Fatal("rule escaped type scope", field, r.Counts)
		}
	}
}
