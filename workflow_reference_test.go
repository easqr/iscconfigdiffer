package differ

import (
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
)

func TestWorkflowReferenceExclusions(t *testing.T) {
	for _, ref := range []struct{ field, leaf, id string }{
		{"creator", "id", "isc.WORKFLOW.creator-id"},
		{"modifiedBy", "id", "isc.WORKFLOW.modified-by-id"},
		{"modifiedBy", "name", "isc.WORKFLOW.modified-by-name"},
	} {
		reference := func(id string) map[string]any {
			value := map[string]any{"id": "identity-id", "name": "Invented Identity", "type": "IDENTITY"}
			value[ref.leaf] = id
			return value
		}
		for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
			for _, mode := range []string{"drift", "deployment"} {
				t.Run(ref.field+"/"+ref.leaf+"/"+profile+"/"+mode, func(t *testing.T) {
					p := v2.Policy{Version: 2, ID: "workflow", Profile: profile}
					req := Request{Mode: mode, Left: sourceReferenceInput(t, "dev", "WORKFLOW", ref.field, reference("dev-identity")), Right: sourceReferenceInput(t, "prod", "WORKFLOW", ref.field, reference("prod-identity"))}
					status, role := "different", "left"
					if mode == "deployment" {
						status, role = "violation", "source"
						req.Operations = []v2.Operation{{Type: "WORKFLOW", Key: "Invented", Action: "update"}}
						for i, path := range []string{"/name", "/" + ref.field + "/name", "/" + ref.field + "/type"} {
							if path == "/modifiedBy/name" {
								continue
							}
							p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"name", "reference-name", "reference-type"}[i]), Selector: v2.Selector{Type: "WORKFLOW", Path: path}, Operation: "copy_source", Authority: "invented-release"})
						}
						if profile != "isc-portable-v2" {
							p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "WORKFLOW", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
						}
					}
					r := compareTest(t, req, p)
					if r.ExitCode() != 0 || r.RuleUsage[ref.id] != 1 {
						t.Fatal("workflow reference field compared", r.Counts)
					}
					found := false
					for _, d := range r.Decisions {
						if d.RuleID == ref.id {
							found = true
							if d.Status != "excluded" || d.Locations[role].Pointer != "/"+ref.field+"/"+ref.leaf || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.workflow" || d.Evidence.Attributes["layer"] != "type" {
								t.Fatal("exclusion provenance absent", d)
							}
						}
					}
					if !found {
						t.Fatal("exclusion decision missing")
					}
					missing := reference("prod-identity")
					delete(missing, ref.leaf)
					req.Right = sourceReferenceInput(t, "prod", "WORKFLOW", ref.field, missing)
					if r = compareTest(t, req, p); r.ExitCode() != 0 {
						t.Fatal("missing excluded field affected result", r.Counts)
					}
					for _, field := range []string{"name", "type"} {
						if ref.field == "modifiedBy" && field == "name" {
							continue
						}
						changed := reference("prod-identity")
						changed[field] = "different"
						req.Right = sourceReferenceInput(t, "prod", "WORKFLOW", ref.field, changed)
						if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
							t.Fatal("reference field ignored", field, r.Counts)
						}
						delete(changed, field)
						req.Right = sourceReferenceInput(t, "prod", "WORKFLOW", ref.field, changed)
						if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
							t.Fatal("missing reference field ignored", field, r.Counts)
						}
					}
					req.Right = sourceReferenceInput(t, "prod", "WORKFLOW", ref.field, reference("prod-identity"))
					p.DisabledRules = []string{ref.id}
					want := 1
					if mode == "deployment" {
						want = 2
					}
					if r = compareTest(t, req, p); r.ExitCode() != want || r.RuleUsage[ref.id] != 0 {
						t.Fatal("exclusion could not be disabled", r.Counts)
					}
				})
			}
		}
		for _, typ := range []string{"SOURCE", "ROLE"} {
			r := compareTest(t, Request{Mode: "drift", Left: sourceReferenceInput(t, "dev", typ, ref.field, reference("dev")), Right: sourceReferenceInput(t, "prod", typ, ref.field, reference("prod"))}, portable())
			if r.ExitCode() != 1 || r.RuleUsage[ref.id] != 0 {
				t.Fatal("WORKFLOW exclusion escaped type scope", typ, r.Counts)
			}
		}
	}
	// A similarly named property is ordinary configuration, not the creator reference.
	r := compareTest(t, Request{Mode: "drift", Left: sourceReferenceInput(t, "dev", "WORKFLOW", "create", map[string]any{"id": "dev"}), Right: sourceReferenceInput(t, "prod", "WORKFLOW", "create", map[string]any{"id": "prod"})}, portable())
	if r.ExitCode() != 1 || r.Counts["different"] != 1 {
		t.Fatal("unrelated create/id ignored", r.Counts)
	}
}
