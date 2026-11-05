package differ

import (
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func sourceRuleInput(t *testing.T, env, typ string, reference map[string]any) snapshot.Input {
	t.Helper()
	return input(t, env, []string{typ}, obj(typ, "same-object-id", "Invented", map[string]any{"beforeProvisioningRule": reference}))
}

func TestSourceBeforeProvisioningRuleReference(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				reference := func(id string) map[string]any {
					return map[string]any{"id": id, "name": "Invented Before Provisioning", "type": "RULE"}
				}
				p := v2.Policy{Version: 2, ID: "invented", Profile: profile}
				req := Request{Mode: mode, Left: sourceRuleInput(t, "dev", "SOURCE", reference("dev-rule-id")), Right: sourceRuleInput(t, "prod", "SOURCE", reference("prod-rule-id"))}
				status, role := "different", "left"
				if mode == "deployment" {
					status, role = "violation", "source"
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
					for i, path := range []string{"/name", "/beforeProvisioningRule/name", "/beforeProvisioningRule/type"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"copy-name", "copy-rule-name", "copy-rule-type"}[i]), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("copy-envelope-id"), Selector: v2.Selector{Type: "SOURCE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage["isc.SOURCE.before-provisioning-rule-id"] != 1 {
					t.Fatal("reference IDs caused drift", r.Counts)
				}
				found := false
				for _, d := range r.Decisions {
					if d.RuleID == "isc.SOURCE.before-provisioning-rule-id" {
						found = true
						if d.Status != "excluded" || d.Space != "content" || d.Locations[role].Pointer != "/beforeProvisioningRule/id" || d.Evidence == nil || d.Evidence.Kind != "isc-profile" {
							t.Fatal("missing exclusion provenance", d)
						}
					}
				}
				if !found {
					t.Fatal("reference ID exclusion absent")
				}
				right := reference("prod-rule-id")
				delete(right, "id")
				req.Right = sourceRuleInput(t, "prod", "SOURCE", right)
				if r = compareTest(t, req, p); r.ExitCode() != 0 {
					t.Fatal("missing reference ID caused drift", r.Counts)
				}
				for _, field := range []string{"name", "type"} {
					right = reference("prod-rule-id")
					right[field] = "different"
					req.Right = sourceRuleInput(t, "prod", "SOURCE", right)
					if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
						t.Fatal("reference field was hidden", field, r.Counts)
					}
					delete(right, field)
					req.Right = sourceRuleInput(t, "prod", "SOURCE", right)
					if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
						t.Fatal("missing reference field was hidden", field, r.Counts)
					}
				}
			})
		}
	}
	// Identical paths on other object types retain ordinary comparison behavior.
	req := Request{Mode: "drift", Left: sourceRuleInput(t, "dev", "ROLE", map[string]any{"id": "dev-rule-id"}), Right: sourceRuleInput(t, "prod", "ROLE", map[string]any{"id": "prod-rule-id"})}
	if r := compareTest(t, req, portable()); r.ExitCode() != 1 || r.RuleUsage["isc.SOURCE.before-provisioning-rule-id"] != 0 {
		t.Fatal("SOURCE exclusion leaked to ROLE", r.Counts)
	}
}
