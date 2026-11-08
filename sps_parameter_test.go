package differ

import (
	"strings"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func TestSPSParameterModificationMetadata(t *testing.T) {
	fields := []struct{ field, id string }{
		{"lastModifiedAt", "isc.SPS_PARAMETER.last-modified-at"},
		{"lastModifiedBy", "isc.SPS_PARAMETER.last-modified-by"},
		{"privateFieldsLastModifiedAt", "isc.SPS_PARAMETER.private-fields-last-modified-at"},
		{"privateFieldsLastModifiedBy", "isc.SPS_PARAMETER.private-fields-last-modified-by"},
	}
	for _, field := range fields {
		for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
			for _, mode := range []string{"drift", "deployment"} {
				t.Run(field.field+"/"+profile+"/"+mode, func(t *testing.T) {
					p := v2.Policy{Version: 2, ID: "parameter", Profile: profile}
					leftValue := any("2026-01-01T00:00:00Z")
					rightValue := any("2026-02-02T00:00:00Z")
					if strings.HasSuffix(field.field, "By") {
						leftValue = map[string]any{"id": "dev-user", "name": "Dev User", "type": "IDENTITY", "extra": map[string]any{"audit": "dev"}}
						rightValue = map[string]any{"id": "prod-user", "name": "Prod User", "type": "IDENTITY", "extra": map[string]any{"audit": "prod"}}
					}
					makeInput := func(env, typ string, value any, setting string) map[string]any {
						payload := map[string]any{"publicFields": map[string]any{"endpoint": setting}}
						if value != nil {
							payload[field.field] = value
						}
						return obj(typ, "same-id", "Invented Parameter", payload)
					}
					req := Request{Mode: mode, Left: input(t, "dev", []string{"SPS_PARAMETER"}, makeInput("dev", "SPS_PARAMETER", leftValue, "same"))}
					if mode == "deployment" {
						req.Operations = []v2.Operation{{Type: "SPS_PARAMETER", Key: "Invented Parameter", Action: "update"}}
						for _, path := range []string{"/name", "/publicFields/endpoint"} {
							p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("copy-" + strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", "-")), Selector: v2.Selector{Type: "SPS_PARAMETER", Path: path}, Operation: "copy_source", Authority: "invented-release"})
						}
						if profile != "isc-portable-v2" {
							p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "SPS_PARAMETER", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
						}
					}
					for _, value := range []any{rightValue, nil, []any{"changed-shape"}} {
						req.Right = input(t, "prod", []string{"SPS_PARAMETER"}, makeInput("prod", "SPS_PARAMETER", value, "same"))
						r := compareTest(t, req, p)
						if r.ExitCode() != 0 || r.RuleUsage[field.id] != 1 {
							t.Fatal("modification metadata affected comparison", r.Counts)
						}
						found := false
						for _, d := range r.Decisions {
							if d.RuleID == field.id {
								found = true
								if d.Status != "excluded" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.sps-parameter" {
									t.Fatal("exclusion provenance missing", d)
								}
							}
							for _, loc := range d.Locations {
								if strings.HasPrefix(loc.Pointer, "/"+field.field+"/") {
									t.Fatal("modifier descendants compared", d)
								}
							}
						}
						if !found {
							t.Fatal("exclusion decision missing")
						}
					}
					req.Right = input(t, "prod", []string{"SPS_PARAMETER"}, makeInput("prod", "SPS_PARAMETER", rightValue, "changed"))
					if r := compareTest(t, req, p); r.ExitCode() != 1 {
						t.Fatal("parameter setting hidden", r.Counts)
					}
					req.Right = input(t, "prod", []string{"SPS_PARAMETER"}, makeInput("prod", "SPS_PARAMETER", rightValue, "same"))
					p.DisabledRules = []string{field.id}
					want := 1
					if mode == "deployment" {
						want = 2
					}
					if r := compareTest(t, req, p); r.ExitCode() != want || r.RuleUsage[field.id] != 0 {
						t.Fatal("exclusion cannot be disabled", r.Counts)
					}
					p = portable()
					req = Request{Mode: "drift", Left: input(t, "dev", []string{"WORKFLOW"}, makeInput("dev", "WORKFLOW", leftValue, "same")), Right: input(t, "prod", []string{"WORKFLOW"}, makeInput("prod", "WORKFLOW", rightValue, "same"))}
					if r := compareTest(t, req, p); r.ExitCode() != 1 || r.RuleUsage[field.id] != 0 {
						t.Fatal("exclusion escaped parameter type", r.Counts)
					}
				})
			}
		}
	}
}
