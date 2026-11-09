package differ

import (
	"testing"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func runtimeMetadataInput(t *testing.T, env, typ, connector, field string, timestamp any, present bool, timeout int) snapshot.Input {
	t.Helper()
	attrs := map[string]any{"connectorName": connector, "timeout": timeout}
	if present {
		attrs[field] = timestamp
	}
	return input(t, env, []string{typ}, obj(typ, "same-id", "Invented", map[string]any{"connectorAttributes": attrs}))
}

func operationalMetadataCases() []struct {
	field, ruleID string
	left, right   any
} {
	return []struct {
		field, ruleID string
		left, right   any
	}{
		{"cloudCacheUpdate", "isc.SOURCE.cloud-cache-update", 1700000000000, 1800000000000},
		{"since", "isc.SOURCE.health-since", "2026-01-01T01:00:00Z", "2026-02-01T02:00:00Z"},
	}
}

func TestPortableSourceOperationalMetadata(t *testing.T) {
	for _, marker := range operationalMetadataCases() {
		for _, connector := range []string{"Active Directory", "Microsoft Entra", "Unknown Connector"} {
			for _, mode := range []string{"drift", "deployment"} {
				t.Run(marker.field+"/"+connector+"/"+mode, func(t *testing.T) {
					p := portable()
					req := Request{Mode: mode, Left: runtimeMetadataInput(t, "dev", "SOURCE", connector, marker.field, marker.left, true, 30), Right: runtimeMetadataInput(t, "prod", "SOURCE", connector, marker.field, marker.right, true, 30)}
					status, role := "different", "left"
					if mode == "deployment" {
						status, role = "violation", "source"
						req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
						for i, path := range []string{"/name", "/connectorAttributes/connectorName", "/connectorAttributes/timeout"} {
							p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"copy-name", "copy-connector", "copy-timeout"}[i]), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
						}
					}
					r := compareTest(t, req, p)
					if r.ExitCode() != 0 || r.RuleUsage[marker.ruleID] != 1 {
						t.Fatal("operational timestamp caused a finding", r.Counts)
					}
					found := false
					for _, d := range r.Decisions {
						if d.RuleID == marker.ruleID {
							found = true
							if d.Status != "excluded" || d.Space != "content" || d.Locations[role].Pointer != "/connectorAttributes/"+marker.field || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.source-portable" {
								t.Fatal("exclusion provenance absent", d)
							}
						}
					}
					if !found {
						t.Fatal("operational metadata exclusion decision missing")
					}
					// An absent marker is also outside portable comparison.
					req.Right = runtimeMetadataInput(t, "prod", "SOURCE", connector, marker.field, nil, false, 30)
					if r = compareTest(t, req, p); r.ExitCode() != 0 {
						t.Fatal("missing marker caused a finding", r.Counts)
					}
					// The exclusion must not hide adjacent configuration changes.
					req.Right = runtimeMetadataInput(t, "prod", "SOURCE", connector, marker.field, marker.right, true, 60)
					if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
						t.Fatal("adjacent setting was hidden", r.Counts)
					}
					// Disabling the rule restores comparison or missing authority.
					req.Right = runtimeMetadataInput(t, "prod", "SOURCE", connector, marker.field, marker.right, true, 30)
					p.DisabledRules = []string{marker.ruleID}
					exit, want := 1, "different"
					if mode == "deployment" {
						exit, want = 2, "unverified"
					}
					if r = compareTest(t, req, p); r.ExitCode() != exit || r.Counts[want] != 1 {
						t.Fatal("disabled rule did not restore normal coverage", r.Counts)
					}
				})
			}
		}
	}
}

func TestOperationalMetadataExclusionsAreLimitedToPortableSource(t *testing.T) {
	for _, marker := range operationalMetadataCases() {
		for _, tc := range []struct{ profile, typ string }{{"isc-identity-v2", "SOURCE"}, {"isc-envelope-v2", "SOURCE"}, {"isc-portable-v2", "ROLE"}} {
			t.Run(marker.field+"/"+tc.profile+"/"+tc.typ, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "invented", Profile: tc.profile}
				r := compareTest(t, Request{Mode: "drift", Left: runtimeMetadataInput(t, "dev", tc.typ, "Unknown Connector", marker.field, marker.left, true, 30), Right: runtimeMetadataInput(t, "prod", tc.typ, "Unknown Connector", marker.field, marker.right, true, 30)}, p)
				if r.ExitCode() != 1 || r.Counts["different"] != 1 || r.RuleUsage[marker.ruleID] != 0 {
					t.Fatal("operational metadata exclusion escaped its scope", r.Counts)
				}
			})
		}
	}
}

func TestHealthSinceExclusionKeepsOtherPathsComparable(t *testing.T) {
	makeInput := func(env, stamp, status string) snapshot.Input {
		return input(t, env, []string{"SOURCE"}, obj("SOURCE", "same-id", "Invented", map[string]any{
			"since":               stamp,
			"connectorAttributes": map[string]any{"since": stamp, "status": status},
		}))
	}
	r := compareTest(t, Request{Mode: "drift", Left: makeInput("dev", "2026-01-01T00:00:00Z", "SOURCE_STATE_HEALTHY"), Right: makeInput("prod", "2026-02-01T00:00:00Z", "SOURCE_STATE_ERROR_SOURCE")}, portable())
	if r.ExitCode() != 1 || r.Counts["different"] != 2 || r.RuleUsage["isc.SOURCE.health-since"] != 1 {
		t.Fatal("health timestamp exclusion hid other fields", r.Counts)
	}
}
