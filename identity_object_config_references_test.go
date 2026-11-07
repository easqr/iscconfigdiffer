package differ

import (
	"strconv"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func identityReferenceDetails(env string) []any {
	items := []any{}
	for _, name := range []string{"email", "phone"} {
		sources := []any{}
		for _, suffix := range []string{"primary", "fallback"} {
			sources = append(sources, map[string]any{
				"ruleRef":        map[string]any{"id": env + "-rule-" + suffix, "name": "Rule " + suffix, "type": "RULE"},
				"applicationRef": map[string]any{"id": env + "-app-" + suffix, "name": "Application " + suffix, "type": "SOURCE"},
			})
		}
		items = append(items, map[string]any{"name": name, "attributeSources": sources})
	}
	if env == "prod" {
		items[0], items[1] = items[1], items[0]
	}
	return items
}

func TestIdentityObjectConfigReferenceIDs(t *testing.T) {
	rules := []string{"isc.IDENTITY_OBJECT_CONFIG.attribute-source-rule-id", "isc.IDENTITY_OBJECT_CONFIG.attribute-source-application-id"}
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "identity-references", Profile: profile, DisabledRules: []string{"isc.IDENTITY_OBJECT_CONFIG.attribute-source-priority"}}
				req := Request{Mode: mode, Left: identityAttributeInput(t, "dev", "IDENTITY_OBJECT_CONFIG", identityReferenceDetails("dev")), Right: identityAttributeInput(t, "prod", "IDENTITY_OBJECT_CONFIG", identityReferenceDetails("prod"))}
				status := "different"
				if mode == "deployment" {
					status = "violation"
					req.Operations = []v2.Operation{{Type: "IDENTITY_OBJECT_CONFIG", Key: "Invented", Action: "update"}}
					paths := []string{"/name", "/objectAttributeDetails/*/name"}
					for _, ref := range []string{"ruleRef", "applicationRef"} {
						for _, field := range []string{"name", "type"} {
							paths = append(paths, "/objectAttributeDetails/*/attributeSources/*/"+ref+"/"+field)
						}
					}
					for i, path := range paths {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("copy-" + strconv.Itoa(i)), Selector: v2.Selector{Type: "IDENTITY_OBJECT_CONFIG", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "IDENTITY_OBJECT_CONFIG", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 {
					t.Fatal("reference IDs caused differences", r.Counts)
				}
				for _, id := range rules {
					if r.RuleUsage[id] != 4 {
						t.Fatal("exclusion did not cover all nested entries", id, r.RuleUsage)
					}
				}
				for _, d := range r.Decisions {
					if d.Status == "excluded" && (d.RuleID == rules[0] || d.RuleID == rules[1]) {
						if d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.identity-object-config" {
							t.Fatal("missing exclusion provenance", d)
						}
					}
				}
				for _, ref := range []string{"ruleRef", "applicationRef"} {
					for _, field := range []string{"name", "type"} {
						for _, remove := range []bool{false, true} {
							right := identityReferenceDetails("prod")
							value := right[1].(map[string]any)["attributeSources"].([]any)[1].(map[string]any)[ref].(map[string]any)
							if remove {
								delete(value, field)
							} else {
								value[field] = "Changed"
							}
							req.Right = identityAttributeInput(t, "prod", "IDENTITY_OBJECT_CONFIG", right)
							if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
								t.Fatal("reference field hidden", ref, field, remove, r.Counts)
							}
						}
					}
				}
				req.Right = identityAttributeInput(t, "prod", "IDENTITY_OBJECT_CONFIG", identityReferenceDetails("prod"))
				for _, id := range rules {
					p.DisabledRules = []string{"isc.IDENTITY_OBJECT_CONFIG.attribute-source-priority", id}
					want, decision := 1, "different"
					if mode == "deployment" {
						want, decision = 2, "unverified"
					}
					if r = compareTest(t, req, p); r.ExitCode() != want || r.Counts[decision] != 4 || r.RuleUsage[id] != 0 {
						t.Fatal("disable did not restore reference IDs", id, r.Counts)
					}
				}
			})
		}
	}
	// The same nested paths on unrelated types retain ID comparison.
	req := Request{Mode: "drift", Left: identityAttributeInput(t, "dev", "ROLE", identityReferenceDetails("dev")), Right: identityAttributeInput(t, "prod", "ROLE", identityReferenceDetails("prod"))}
	r := compareTest(t, req, portable())
	if r.ExitCode() != 1 || r.RuleUsage[rules[0]] != 0 || r.RuleUsage[rules[1]] != 0 {
		t.Fatal("exclusion escaped type scope", r.Counts)
	}
}
