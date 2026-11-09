package differ

import (
	"strings"
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
)

var entraExcludedAttributes = []struct {
	attribute string
	id        string
}{
	{"subscribedSkus", "isc.SOURCE.entra-subscribed-skus"},
	{"deleteThresholdPercentage", "isc.SOURCE.entra-delete-threshold-percentage"},
	{"directoryRolesDeltaToken", "isc.SOURCE.entra-directory-roles-delta-token"},
	{"groupsDeltaToken", "isc.SOURCE.entra-groups-delta-token"},
	{"userDeltaToken", "isc.SOURCE.entra-user-delta-token"},
}

func TestEntraConnectorAttributesExcluded(t *testing.T) {
	for _, attribute := range entraExcludedAttributes {
		t.Run(attribute.attribute, func(t *testing.T) {
			testEntraAttributeExcluded(t, attribute.attribute, attribute.id)
		})
	}
}

func testEntraAttributeExcluded(t *testing.T, attribute, id string) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "inventory", Profile: profile}
				left := connectorInput(t, "dev", "Renamed Directory", "Microsoft Entra", map[string]any{attribute: []any{map[string]any{"skuId": "invented-a", "servicePlans": []any{map[string]any{"servicePlanId": "plan-a"}}}}})
				req := Request{Mode: mode, Left: left}
				if mode == "deployment" {
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Renamed Directory", Action: "update"}}
					for i, path := range []string{"/name", "/connectorAttributes/connectorName"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"name", "connector"}[i]), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "SOURCE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				for _, attrs := range []map[string]any{
					{attribute: 25},
					{attribute: "changed-token"},
					{attribute: []any{map[string]any{"skuId": "invented-b", "servicePlans": []any{map[string]any{"servicePlanId": "changed"}}}}},
					{},
					{attribute: "different-shape"},
					{attribute: []any{map[string]any{"skuId": "duplicate"}, map[string]any{"skuId": "duplicate"}}},
				} {
					req.Right = connectorInput(t, "prod", "Renamed Directory", "Microsoft Entra", attrs)
					r := compareTest(t, req, p)
					if r.ExitCode() != 0 || r.RuleUsage[id] != 1 {
						t.Fatal("inventory affected comparison", r.Counts)
					}
					found := false
					for _, d := range r.Decisions {
						if d.RuleID == id {
							found = true
							if d.Status != "excluded" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.microsoft-entra" {
								t.Fatal("exclusion provenance absent", d)
							}
						}
						for _, loc := range d.Locations {
							if strings.HasPrefix(loc.Pointer, "/connectorAttributes/"+attribute+"/") {
								t.Fatal("inventory descendants compared", d.Code)
							}
						}
					}
					if !found {
						t.Fatal("whole-subtree exclusion absent")
					}
				}
				// Opting out restores comparison; deployment still needs explicit authority.
				p.DisabledRules = []string{id}
				r := compareTest(t, req, p)
				want := 1
				if mode == "deployment" {
					want = 2
				}
				if r.ExitCode() != want || r.RuleUsage[id] != 0 {
					t.Fatal("disabled exclusion still active", r.Counts)
				}
			})
		}
	}
}

func TestEntraConnectorAttributeExclusionScope(t *testing.T) {
	for _, attribute := range entraExcludedAttributes {
		t.Run(attribute.attribute, func(t *testing.T) {
			testEntraAttributeExclusionScope(t, attribute.attribute, attribute.id)
		})
	}
}

func testEntraAttributeExclusionScope(t *testing.T, attribute, id string) {
	for _, connector := range []string{"Active Directory", "Unknown Connector", "Microsoft Entra"} {
		left := connectorInput(t, "dev", "Microsoft Entra", connector, map[string]any{attribute: "a", "timeout": 30})
		right := connectorInput(t, "prod", "Microsoft Entra", connector, map[string]any{attribute: "b", "timeout": 30})
		if connector == "Microsoft Entra" {
			right = connectorInput(t, "prod", "Microsoft Entra", connector, map[string]any{attribute: "b", "timeout": 60})
		}
		r := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, portable())
		if r.ExitCode() != 1 || r.Counts["different"] != 1 {
			t.Fatal("unrelated difference hidden", connector, r.Counts)
		}
		if connector != "Microsoft Entra" && r.RuleUsage[id] != 0 {
			t.Fatal("exclusion escaped connector scope")
		}
	}
}
