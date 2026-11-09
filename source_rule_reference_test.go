package differ

import (
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
	"github.com/easqr/iscconfigexporter/snapshot"
)

func sourceRuleInput(t *testing.T, env, typ string, reference map[string]any) snapshot.Input {
	t.Helper()
	return sourceReferenceInput(t, env, typ, "beforeProvisioningRule", reference)
}

func sourceReferenceInput(t *testing.T, env, typ, field string, reference map[string]any) snapshot.Input {
	t.Helper()
	return input(t, env, []string{typ}, obj(typ, "same-object-id", "Invented", map[string]any{field: reference}))
}
func sourceReferenceCases() []struct{ field, ruleID, referenceType string } {
	return []struct{ field, ruleID, referenceType string }{
		{"beforeProvisioningRule", "isc.SOURCE.before-provisioning-rule-id", "RULE"},
		{"managerCorrelationRule", "isc.SOURCE.manager-correlation-rule-id", "RULE"},
		{"correlationConfigDetails", "isc.SOURCE.correlation-config-details-id", "ACCOUNT_CORRELATION_CONFIG"},
	}
}

func TestSourceReferenceIDs(t *testing.T) {
	for _, ref := range sourceReferenceCases() {
		for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
			for _, mode := range []string{"drift", "deployment"} {
				t.Run(ref.field+"/"+profile+"/"+mode, func(t *testing.T) {
					reference := func(id string) map[string]any {
						return map[string]any{"id": id, "name": "Invented Reference", "type": ref.referenceType}
					}
					p := v2.Policy{Version: 2, ID: "invented", Profile: profile}
					req := Request{Mode: mode, Left: sourceReferenceInput(t, "dev", "SOURCE", ref.field, reference("dev-rule-id")), Right: sourceReferenceInput(t, "prod", "SOURCE", ref.field, reference("prod-rule-id"))}
					status, role := "different", "left"
					if mode == "deployment" {
						status, role = "violation", "source"
						req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
						for i, path := range []string{"/name", "/" + ref.field + "/name", "/" + ref.field + "/type"} {
							p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"copy-name", "copy-rule-name", "copy-rule-type"}[i]), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
						}
						if profile != "isc-portable-v2" {
							p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("copy-envelope-id"), Selector: v2.Selector{Type: "SOURCE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
						}
					}
					r := compareTest(t, req, p)
					if r.ExitCode() != 0 || r.RuleUsage[ref.ruleID] != 1 {
						t.Fatal("reference IDs caused drift", r.Counts)
					}
					found := false
					for _, d := range r.Decisions {
						if d.RuleID == ref.ruleID {
							found = true
							if d.Status != "excluded" || d.Space != "content" || d.Locations[role].Pointer != "/"+ref.field+"/id" || d.Evidence == nil || d.Evidence.Kind != "isc-profile" || d.Evidence.Attributes["rule_pack"] != "isc.source" {
								t.Fatal("missing exclusion provenance", d)
							}
						}
					}
					if !found {
						t.Fatal("reference ID exclusion absent")
					}
					right := reference("prod-rule-id")
					delete(right, "id")
					req.Right = sourceReferenceInput(t, "prod", "SOURCE", ref.field, right)
					if r = compareTest(t, req, p); r.ExitCode() != 0 {
						t.Fatal("missing reference ID caused drift", r.Counts)
					}
					for _, field := range []string{"name", "type"} {
						right = reference("prod-rule-id")
						right[field] = "different"
						req.Right = sourceReferenceInput(t, "prod", "SOURCE", ref.field, right)
						if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
							t.Fatal("reference field was hidden", field, r.Counts)
						}
						delete(right, field)
						req.Right = sourceReferenceInput(t, "prod", "SOURCE", ref.field, right)
						if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
							t.Fatal("missing reference field was hidden", field, r.Counts)
						}
					}
				})
			}
		}
		// Identical paths on other object types retain ordinary comparison behavior.
		req := Request{Mode: "drift", Left: sourceReferenceInput(t, "dev", "ROLE", ref.field, map[string]any{"id": "dev-rule-id"}), Right: sourceReferenceInput(t, "prod", "ROLE", ref.field, map[string]any{"id": "prod-rule-id"})}
		if r := compareTest(t, req, portable()); r.ExitCode() != 1 || r.RuleUsage[ref.ruleID] != 0 {
			t.Fatal("SOURCE exclusion leaked to ROLE", r.Counts)
		}
	}
}
