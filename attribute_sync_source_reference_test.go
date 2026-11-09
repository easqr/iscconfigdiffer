package differ

import (
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
	"github.com/easqr/iscconfigexporter/snapshot"
)

const syncSourceIDRule = "isc.ATTR_SYNC_SOURCE_CONFIG.source-id"

func attributeSyncPhysicalReferenceInput(t *testing.T, env string, alter func(map[string]any)) snapshot.Input {
	t.Helper()
	ref := map[string]any{"id": env + "-source", "name": "Invented Source", "type": "SOURCE"}
	if alter != nil {
		alter(ref)
	}
	attrs := []any{syncMapping("mail", "email", "Email", true)}
	return input(t, env, []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"},
		obj("SOURCE", env+"-source", "Invented Source", nil),
		obj("SOURCE", env+"-other", "Other Source", nil),
		obj("ATTR_SYNC_SOURCE_CONFIG", "same-sync", "", map[string]any{"source": ref, "attributes": attrs}))
}

func TestAttributeSyncSourceReferenceExclusion(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "sync-reference", Profile: profile}
				// Isolate the parent-reference comparison from source envelope ID differences.
				if profile != "isc-portable-v2" {
					p.Ignores = []v2.Ignore{{Rule: rule("fixture-source-physical-id"), Selector: v2.Selector{Type: "SOURCE", Space: "metadata", Path: "/self/id"}}}
				}
				req := Request{Mode: mode, Left: attributeSyncPhysicalReferenceInput(t, "dev", nil), Right: attributeSyncPhysicalReferenceInput(t, "prod", nil), Scope: []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}}
				role, status := "left", "different"
				if mode == "deployment" {
					role, status = "source", "violation"
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented Source", Action: "update"}, {Type: "SOURCE", Key: "Other Source", Action: "update"}, {Type: "ATTR_SYNC_SOURCE_CONFIG", Key: "Invented Source", Action: "update"}}
					p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("parent-name"), Selector: v2.Selector{Type: "SOURCE", Path: "/name"}, Operation: "copy_source", Authority: "invented-release"})
					for i, path := range []string{"/source/name", "/source/type", "/attributes/*/target", "/attributes/*/name", "/attributes/*/displayName", "/attributes/*/enabled"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"source-name", "source-type", "target", "attribute-name", "display-name", "enabled"}[i]), Selector: v2.Selector{Type: "ATTR_SYNC_SOURCE_CONFIG", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("sync-envelope-id"), Selector: v2.Selector{Type: "ATTR_SYNC_SOURCE_CONFIG", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage[syncSourceIDRule] != 1 {
					t.Fatal("tenant-specific parent IDs caused drift", r.Counts)
				}
				matched, excluded := false, false
				for _, m := range r.Matches {
					if m.Type == "ATTR_SYNC_SOURCE_CONFIG" && m.Key == "Invented Source" {
						matched = true
					}
				}
				for _, d := range r.Decisions {
					if d.RuleID == syncSourceIDRule {
						excluded = true
						if d.Status != "excluded" || d.Locations[role].Pointer != "/source/id" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.attribute-sync" {
							t.Fatal("exclusion provenance absent", d)
						}
					}
				}
				if !matched || !excluded {
					t.Fatal("parent matching or exclusion absent")
				}
				for _, field := range []string{"name", "type"} {
					for _, remove := range []bool{false, true} {
						req.Right = attributeSyncPhysicalReferenceInput(t, "prod", func(ref map[string]any) {
							if remove {
								delete(ref, field)
							} else {
								ref[field] = "different"
							}
						})
						if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
							t.Fatal("parent reference field hidden", field, remove, r.Counts)
						}
					}
				}
				req.Right = attributeSyncPhysicalReferenceInput(t, "prod", nil)
				p.DisabledRules = []string{syncSourceIDRule}
				want, decision := 1, "different"
				if mode == "deployment" {
					want, decision = 2, "unverified"
				}
				if r = compareTest(t, req, p); r.ExitCode() != want || r.Counts[decision] != 1 || r.RuleUsage[syncSourceIDRule] != 0 {
					t.Fatal("source ID comparison could not be restored", r.Counts)
				}
			})
		}
	}
}

func TestAttributeSyncParentMatchingStillRequired(t *testing.T) {
	for _, kind := range []string{"missing", "null", "empty", "invalid-type", "unresolved"} {
		t.Run(kind, func(t *testing.T) {
			req := Request{Mode: "drift", Left: attributeSyncPhysicalReferenceInput(t, "dev", nil), Right: attributeSyncPhysicalReferenceInput(t, "prod", func(ref map[string]any) {
				switch kind {
				case "missing":
					delete(ref, "id")
				case "null":
					ref["id"] = nil
				case "empty":
					ref["id"] = ""
				case "invalid-type":
					ref["id"] = false
				case "unresolved":
					ref["id"] = "unknown-source"
				}
			}), Scope: []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}}
			r := compareTest(t, req, portable())
			found := false
			for _, d := range r.Diagnostics {
				if d.Type == "ATTR_SYNC_SOURCE_CONFIG" && d.Code == "identity dependency/component unavailable" {
					found = true
				}
			}
			if r.ExitCode() != 2 || !found || r.RuleUsage[syncSourceIDRule] != 0 {
				t.Fatal("exclusion bypassed parent resolution", kind, r.Counts)
			}
		})
	}
	req := Request{Mode: "drift", Left: attributeSyncPhysicalReferenceInput(t, "dev", nil), Right: attributeSyncPhysicalReferenceInput(t, "prod", nil), Scope: []string{"ATTR_SYNC_SOURCE_CONFIG"}}
	if r := compareTest(t, req, portable()); r.ExitCode() != 2 {
		t.Fatal("exclusion bypassed missing SOURCE scope")
	}
	req.Scope = []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}
	req.Right = attributeSyncPhysicalReferenceInput(t, "prod", func(ref map[string]any) { ref["id"] = "prod-other"; ref["name"] = "Other Source" })
	r := compareTest(t, req, portable())
	if r.ExitCode() != 1 || r.RuleUsage[syncSourceIDRule] != 0 {
		t.Fatal("change to a different logical parent was hidden", r.Counts)
	}
	// Identical paths on types without this exclusion remain comparable.
	for _, typ := range []string{"ROLE", "WORKFLOW"} {
		r = compareTest(t, Request{Mode: "drift", Left: sourceReferenceInput(t, "dev", typ, "source", map[string]any{"id": "dev", "name": "same", "type": "SOURCE"}), Right: sourceReferenceInput(t, "prod", typ, "source", map[string]any{"id": "prod", "name": "same", "type": "SOURCE"})}, portable())
		if r.ExitCode() != 1 || r.Counts["different"] != 1 || r.RuleUsage[syncSourceIDRule] != 0 {
			t.Fatal("attribute-sync exclusion escaped type scope", typ, r.Counts)
		}
	}
}
