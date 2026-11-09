package differ

import (
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
)

func TestADRecommendationStatusExcluded(t *testing.T) {
	const id = "isc.SOURCE.ad-recommendation-status"
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "recommendation", Profile: profile}
				req := Request{Mode: mode, Left: connectorInput(t, "dev", "Renamed directory", "Active Directory", map[string]any{"recommendationStatus": "old"})}
				if mode == "deployment" {
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Renamed directory", Action: "update"}}
					for i, path := range []string{"/name", "/connectorAttributes/connectorName"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"name", "connector"}[i]), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "SOURCE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				for _, attrs := range []map[string]any{{"recommendationStatus": "new"}, {}, {"recommendationStatus": nil}, {"recommendationStatus": map[string]any{"nested": "new"}}} {
					req.Right = connectorInput(t, "prod", "Renamed directory", "Active Directory", attrs)
					r := compareTest(t, req, p)
					if r.ExitCode() != 0 || r.RuleUsage[id] != 1 {
						t.Fatal("recommendation status affected comparison", r.Counts)
					}
					found := false
					for _, d := range r.Decisions {
						if d.RuleID != id {
							continue
						}
						found = true
						if d.Status != "excluded" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.active-directory" {
							t.Fatal("exclusion provenance absent", d)
						}
						role := "left"
						if mode == "deployment" {
							role = "source"
						}
						if d.Locations[role].Pointer != "/connectorAttributes/recommendationStatus" {
							t.Fatal("original path absent", d)
						}
					}
					if !found {
						t.Fatal("exclusion absent")
					}
				}
				p.DisabledRules = []string{id}
				want := 1
				if mode == "deployment" {
					want = 2
				}
				if r := compareTest(t, req, p); r.ExitCode() != want || r.RuleUsage[id] != 0 {
					t.Fatal("disable did not restore comparison", r.Counts)
				}
			})
		}
	}
}

func TestADRecommendationStatusScope(t *testing.T) {
	for _, connector := range []string{"Active Directory", "Microsoft Entra", "Unknown Connector"} {
		left := connectorInput(t, "dev", "Active Directory", connector, map[string]any{"recommendationStatus": "old", "timeout": 30})
		attrs := map[string]any{"recommendationStatus": "new", "timeout": 30}
		if connector == "Active Directory" {
			attrs["timeout"] = 60
		}
		right := connectorInput(t, "prod", "Active Directory", connector, attrs)
		r := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, portable())
		if r.ExitCode() != 1 || r.Counts["different"] != 1 {
			t.Fatal("scope or adjacent value incorrect", connector, r.Counts)
		}
		if connector != "Active Directory" && r.RuleUsage["isc.SOURCE.ad-recommendation-status"] != 0 {
			t.Fatal("exclusion escaped connector scope")
		}
	}
}
