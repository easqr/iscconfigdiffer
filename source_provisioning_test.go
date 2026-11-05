package differ

import (
	"fmt"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func provisioningFixture(env string, reverse bool) []any {
	policies := []any{}
	for i, usage := range []string{"CREATE", "DISABLE", "CREATE_MACHINE_ACCOUNT", "CREATE_MACHINE_ACCOUNT"} {
		fields := []any{}
		for _, name := range []string{"username", "enabled"} {
			fields = append(fields, map[string]any{"name": name, "type": "string", "id": env + "-field-" + name, "created": env + "-created", "modified": env + "-modified", "transform": map[string]any{"type": "static", "attributes": map[string]any{"id": "stable-transform-reference", "values": []any{"first", "second"}}}})
		}
		if reverse {
			fields[0], fields[1] = fields[1], fields[0]
		}
		p := map[string]any{"usageType": usage, "name": fmt.Sprintf("Policy %d", i), "id": fmt.Sprintf("%s-policy-%d", env, i), "created": env + "-created", "modified": env + "-modified", "fields": fields}
		if i >= 2 {
			p["subtypeId"] = fmt.Sprintf("subtype-%d", i)
		}
		policies = append(policies, p)
	}
	if reverse {
		for i, j := 0, len(policies)-1; i < j; i, j = i+1, j-1 {
			policies[i], policies[j] = policies[j], policies[i]
		}
	}
	return policies
}
func provisioningInput(t *testing.T, env, typ string, policies []any) snapshot.Input {
	t.Helper()
	return input(t, env, []string{typ}, obj(typ, "same-source-id", "Invented", map[string]any{"provisioningPolicies": policies}))
}
func provisioningExpectations() []v2.Expectation {
	paths := []string{"/name", "/provisioningPolicies/*/name", "/provisioningPolicies/*/usageType", "/provisioningPolicies/*/subtypeId", "/provisioningPolicies/*/fields/*/name", "/provisioningPolicies/*/fields/*/type", "/provisioningPolicies/*/fields/*/transform"}
	out := []v2.Expectation{}
	for i, path := range paths {
		out = append(out, v2.Expectation{Rule: rule(fmt.Sprintf("provisioning-copy-%d", i)), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
	}
	return out
}

func TestSourceProvisioningAlignmentAndExclusions(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		t.Run(profile, func(t *testing.T) {
			left := provisioningInput(t, "dev", "SOURCE", provisioningFixture("dev", false))
			right := provisioningInput(t, "prod", "SOURCE", provisioningFixture("prod", true))
			p := v2.Policy{Version: 2, ID: "invented", Profile: profile}
			r := compareTest(t, Request{Mode: "drift", Left: left, Right: right, IncludeMatching: true}, p)
			if r.ExitCode() != 0 {
				t.Fatal(r)
			}
			for _, field := range []string{"id", "created", "modified"} {
				if r.RuleUsage["isc.SOURCE.provisioning-policy-"+field] != 4 || r.RuleUsage["isc.SOURCE.provisioning-field-"+field] != 8 {
					t.Fatal("metadata exclusions incomplete", r.RuleUsage)
				}
			}
			found := false
			for _, d := range r.Decisions {
				if d.Locations["left"].Pointer == "/provisioningPolicies/0/fields/0/name" {
					found = d.Status == "equal" && d.Locations["right"].Pointer == "/provisioningPolicies/3/fields/1/name" && len(d.ItemKeys) == 2
				}
			}
			if !found {
				t.Fatal("original pointers or nested alignment lost", r)
			}
		})
	}
}

func TestSourceProvisioningDeploymentAndTransforms(t *testing.T) {
	for _, mode := range []string{"drift", "deployment"} {
		t.Run(mode, func(t *testing.T) {
			policies := provisioningFixture("prod", true)
			req := Request{Mode: mode, Left: provisioningInput(t, "dev", "SOURCE", provisioningFixture("dev", false)), Right: provisioningInput(t, "prod", "SOURCE", policies)}
			p := portable()
			if mode == "deployment" {
				req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
				p.Expectations = provisioningExpectations()
			}
			if r := compareTest(t, req, p); r.ExitCode() != 0 {
				t.Fatal(r)
			}
			// Array order inside a transform remains significant.
			attrs := policies[3].(map[string]any)["fields"].([]any)[0].(map[string]any)["transform"].(map[string]any)["attributes"].(map[string]any)
			attrs["values"] = []any{"second", "first"}
			req.Right = provisioningInput(t, "prod", "SOURCE", policies)
			if r := compareTest(t, req, p); r.ExitCode() != 1 {
				t.Fatal("transform ordering was hidden", r)
			}
			attrs["values"] = []any{"first", "second"}
			attrs["id"] = "changed-transform-reference"
			req.Right = provisioningInput(t, "prod", "SOURCE", policies)
			if r := compareTest(t, req, p); r.ExitCode() != 1 {
				t.Fatal("nested transform ID was excluded", r)
			}
		})
	}
}

func TestSourceProvisioningRejectsAmbiguousKeys(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]any)
	}{
		{"duplicate usage", func(p []any) { p[1].(map[string]any)["usageType"] = "CREATE" }},
		{"duplicate machine subtype", func(p []any) { p[3].(map[string]any)["subtypeId"] = "subtype-2" }},
		{"missing usage", func(p []any) { delete(p[0].(map[string]any), "usageType") }},
		{"blank usage", func(p []any) { p[0].(map[string]any)["usageType"] = " " }},
		{"missing subtype", func(p []any) { delete(p[2].(map[string]any), "subtypeId") }},
		{"null subtype", func(p []any) { p[2].(map[string]any)["subtypeId"] = nil }},
		{"numeric subtype", func(p []any) { p[2].(map[string]any)["subtypeId"] = 42 }},
		{"duplicate field", func(p []any) {
			fields := p[0].(map[string]any)["fields"].([]any)
			fields[1].(map[string]any)["name"] = "username"
		}},
		{"missing field name", func(p []any) { delete(p[0].(map[string]any)["fields"].([]any)[0].(map[string]any), "name") }},
		{"blank field name", func(p []any) { p[0].(map[string]any)["fields"].([]any)[0].(map[string]any)["name"] = " " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policies := provisioningFixture("prod", false)
			tc.change(policies)
			right := provisioningInput(t, "prod", "SOURCE", policies)
			req := Request{Mode: "drift", Left: provisioningInput(t, "dev", "SOURCE", provisioningFixture("dev", false)), Right: right}
			for _, mode := range []string{"drift", "deployment"} {
				req.Mode = mode
				p := portable()
				if mode == "deployment" {
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
					p.Expectations = provisioningExpectations()
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 2 || r.Counts["conflict"] == 0 {
					t.Fatal(mode, r)
				}
			}
		})
	}
}

func TestProvisioningMembershipAndSourceOnlyRules(t *testing.T) {
	left := provisioningInput(t, "dev", "SOURCE", provisioningFixture("dev", false))
	policies := provisioningFixture("prod", false)
	policies[2].(map[string]any)["subtypeId"] = "another-subtype"
	right := provisioningInput(t, "prod", "SOURCE", policies)
	if r := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, portable()); r.ExitCode() != 1 {
		t.Fatal("different subtypes silently matched", r)
	}
	policies = provisioningFixture("prod", false)
	policies[0].(map[string]any)["fields"] = policies[0].(map[string]any)["fields"].([]any)[:1]
	right = provisioningInput(t, "prod", "SOURCE", policies)
	if r := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, portable()); r.ExitCode() != 1 {
		t.Fatal("missing field hidden", r)
	}
	l := provisioningInput(t, "dev", "ROLE", provisioningFixture("dev", false))
	rr := provisioningInput(t, "prod", "ROLE", provisioningFixture("prod", true))
	if r := compareTest(t, Request{Mode: "drift", Left: l, Right: rr}, portable()); r.ExitCode() != 1 || r.RuleUsage["isc.SOURCE.provisioning-policies"] != 0 {
		t.Fatal("SOURCE rules applied to ROLE", r)
	}
}
