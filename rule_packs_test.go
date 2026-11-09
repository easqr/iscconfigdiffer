package differ

import (
	"testing"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func connectorInput(t *testing.T, env, name, connector string, extra map[string]any) snapshot.Input {
	t.Helper()
	attrs := map[string]any{"connectorName": connector}
	for key, value := range extra {
		attrs[key] = value
	}
	return input(t, env, []string{"SOURCE"}, obj("SOURCE", "same-id", name, map[string]any{"connectorAttributes": attrs}))
}
func TestConnectorPacksUseConnectorIdentityNotShapeOrObjectName(t *testing.T) {
	first := forestEntry("one.example.invalid", true)
	second := forestEntry("two.example.invalid", false)
	for _, tc := range []struct {
		name, connector string
		exit, usage     int
	}{
		{"Renamed directory", "Active Directory", 0, 1},
		{"Active Directory", "Unknown Connector", 1, 0},
		{"Active Directory", "Microsoft Entra", 1, 0},
	} {
		t.Run(tc.name+"/"+tc.connector, func(t *testing.T) {
			req := Request{Mode: "drift", Left: connectorInput(t, "dev", tc.name, tc.connector, map[string]any{"forestSettings": []any{first, second}}), Right: connectorInput(t, "prod", tc.name, tc.connector, map[string]any{"forestSettings": []any{second, first}})}
			r := compareTest(t, req, portable())
			if r.ExitCode() != tc.exit || r.RuleUsage["isc.SOURCE.ad-forests"] != tc.usage {
				t.Fatal(r.Counts)
			}
			for _, d := range r.Decisions {
				if d.RuleID == "isc.SOURCE.ad-forests" && (d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.active-directory" || d.Evidence.Attributes["layer"] != "connector" || d.Evidence.Digest == "") {
					t.Fatal("pack provenance absent", d)
				}
			}
		})
	}
	req := Request{Mode: "drift", Left: connectorInput(t, "dev", "Invented", "Active Directory", nil), Right: connectorInput(t, "prod", "Invented", "Microsoft Entra", nil)}
	if r := compareTest(t, req, portable()); r.ExitCode() != 2 || r.Counts["conflict"] != 1 {
		t.Fatal("connector change silently accepted", r.Counts)
	}
	contradictory := input(t, "dev", []string{"SOURCE"}, obj("SOURCE", "same-id", "Invented", map[string]any{"connectorClass": "sailpoint.connector.ADLDAPConnector", "connectorAttributes": map[string]any{"connectorName": "Microsoft Entra"}}))
	if _, err := AdaptSnapshot(contradictory); err == nil {
		t.Fatal("contradictory connector identifiers accepted")
	}
}
func TestExplicitConnectorOverride(t *testing.T) {
	req := Request{Mode: "drift"}
	var r v2.Report
	p := portable()
	p.Collections = []v2.Collection{{Rule: v2.Rule{ID: "ordered-forests", Replaces: "isc.SOURCE.ad-forests", Evidence: &v2.Evidence{Kind: "implementation-policy"}}, Selector: v2.Selector{Type: "SOURCE", Variant: "active-directory", Path: "/connectorAttributes/forestSettings"}, Mode: "ordered"}}
	req.Left = forestInput(t, "dev", "SOURCE", []any{forestEntry("a", true), forestEntry("b", true)})
	req.Right = forestInput(t, "prod", "SOURCE", []any{forestEntry("b", true), forestEntry("a", true)})
	r = compareTest(t, req, p)
	if r.ExitCode() != 1 || r.Replacements["isc.SOURCE.ad-forests"] != "ordered-forests" {
		t.Fatal("explicit override ignored", r.Counts)
	}
	p.Collections[0].Variant = ""
	if _, err := Compile(p); err == nil {
		t.Fatal("override silently widened connector scope")
	}
	p = portable()
	p.DisabledRules = []string{"isc.SOURCE.before-provisioning-rule-id"}
	req.Left = sourceRuleInput(t, "dev", "SOURCE", map[string]any{"id": "left", "name": "same", "type": "RULE"})
	req.Right = sourceRuleInput(t, "prod", "SOURCE", map[string]any{"id": "right", "name": "same", "type": "RULE"})
	r = compareTest(t, req, p)
	if r.ExitCode() != 1 || len(r.DisabledRules) != 1 {
		t.Fatal("SOURCE default cannot be disabled", r.Counts)
	}
}
func TestRulePackProfilesAreDetached(t *testing.T) {
	p := portable()
	a, err := ISCProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range a.Rules.Collections {
		r := &a.Rules.Collections[i]
		if r.ID == "isc.SOURCE.schemas" {
			found = true
			r.Keys[0] = "/mutated"
			r.Evidence.Attributes["rule_pack"] = "mutated"
		}
	}
	if !found {
		t.Fatal("SOURCE pack absent")
	}
	b, err := ISCProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range b.Rules.Collections {
		if r.ID == "isc.SOURCE.schemas" && (r.Keys[0] != "/name" || r.Evidence.Attributes["rule_pack"] != "isc.source") {
			t.Fatal("caller mutated cached pack")
		}
	}
	if len(b.Rules.Evidence) != 16 {
		t.Fatal("missing pack inventory", len(b.Rules.Evidence))
	}
}
