package differ

import (
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func TestPortableOwnerReferencesAcrossConfigurationTypes(t *testing.T) {
	for _, typ := range []string{"SOURCE", "ROLE", "TRANSFORM", "CUSTOM_CONFIG"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(typ+"/"+mode, func(t *testing.T) {
				owner := func(id string) map[string]any {
					return map[string]any{"id": id, "name": "Invented Owner", "type": "IDENTITY"}
				}
				p := portable()
				if typ == "CUSTOM_CONFIG" {
					p.Identities = []v2.Identity{{Rule: rule("custom-identity"), Type: typ, Strategy: "fields", Paths: []string{"/name"}, ComponentType: "string"}}
				}
				req := Request{Mode: mode, Left: sourceReferenceInput(t, "dev", typ, "owner", owner("dev-owner-id")), Right: sourceReferenceInput(t, "prod", typ, "owner", owner("prod-owner-id"))}
				status, role := "different", "left"
				if mode == "deployment" {
					status, role = "violation", "source"
					req.Operations = []v2.Operation{{Type: typ, Key: "Invented", Action: "update"}}
					for i, path := range []string{"/name", "/owner/name", "/owner/type"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"copy-name", "copy-owner-name", "copy-owner-type"}[i]), Selector: v2.Selector{Type: typ, Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				ruleID := "isc." + typ + ".owner-id"
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage[ruleID] != 1 {
					t.Fatal("owner IDs caused a finding", r.Counts)
				}
				found := false
				for _, d := range r.Decisions {
					if d.RuleID == ruleID {
						found = true
						if d.Status != "excluded" || d.Locations[role].Pointer != "/owner/id" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.portable" || d.Evidence.Attributes["layer"] != "common" {
							t.Fatal("common pack provenance missing", d)
						}
					}
				}
				if !found {
					t.Fatal("owner exclusion absent")
				}
				right := owner("prod-owner-id")
				delete(right, "id")
				req.Right = sourceReferenceInput(t, "prod", typ, "owner", right)
				if r = compareTest(t, req, p); r.ExitCode() != 0 {
					t.Fatal("missing owner ID caused a finding", r.Counts)
				}
				for _, field := range []string{"name", "type"} {
					right = owner("prod-owner-id")
					right[field] = "different"
					req.Right = sourceReferenceInput(t, "prod", typ, "owner", right)
					if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
						t.Fatal("owner field was hidden", field, r.Counts)
					}
				}
				req.Right = sourceReferenceInput(t, "prod", typ, "owner", owner("prod-owner-id"))
				p.DisabledRules = []string{ruleID}
				exit, want := 1, "different"
				if mode == "deployment" {
					exit, want = 2, "unverified"
				}
				if r = compareTest(t, req, p); r.ExitCode() != exit || r.Counts[want] != 1 {
					t.Fatal("owner ID comparison could not be restored", r.Counts)
				}
			})
		}
	}
}
func TestOwnerIDsRemainComparableInStricterProfiles(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2"} {
		p := v2.Policy{Version: 2, ID: "invented", Profile: profile}
		r := compareTest(t, Request{Mode: "drift", Left: sourceReferenceInput(t, "dev", "ROLE", "owner", map[string]any{"id": "dev-id", "name": "same", "type": "IDENTITY"}), Right: sourceReferenceInput(t, "prod", "ROLE", "owner", map[string]any{"id": "prod-id", "name": "same", "type": "IDENTITY"})}, p)
		if r.ExitCode() != 1 || r.Counts["different"] != 1 || r.RuleUsage["isc.ROLE.owner-id"] != 0 {
			t.Fatal("portable owner exclusion leaked into stricter profile", r.Counts)
		}
	}
}
