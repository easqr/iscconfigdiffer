package differ

import (
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func cacheMetadataInput(t *testing.T, env, typ, connector string, timestamp any, present bool, timeout int) snapshot.Input {
	t.Helper()
	attrs := map[string]any{"connectorName": connector, "timeout": timeout}
	if present {
		attrs["cloudCacheUpdate"] = timestamp
	}
	return input(t, env, []string{typ}, obj(typ, "same-id", "Invented", map[string]any{"connectorAttributes": attrs}))
}

func TestPortableSourceCacheMetadata(t *testing.T) {
	for _, connector := range []string{"Active Directory", "Microsoft Entra", "Unknown Connector"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(connector+"/"+mode, func(t *testing.T) {
				p := portable()
				req := Request{Mode: mode, Left: cacheMetadataInput(t, "dev", "SOURCE", connector, 1700000000000, true, 30), Right: cacheMetadataInput(t, "prod", "SOURCE", connector, 1800000000000, true, 30)}
				status, role := "different", "left"
				if mode == "deployment" {
					status, role = "violation", "source"
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
					for i, path := range []string{"/name", "/connectorAttributes/connectorName", "/connectorAttributes/timeout"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule([]string{"copy-name", "copy-connector", "copy-timeout"}[i]), Selector: v2.Selector{Type: "SOURCE", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage["isc.SOURCE.cloud-cache-update"] != 1 {
					t.Fatal("cache timestamp caused a finding", r.Counts)
				}
				found := false
				for _, d := range r.Decisions {
					if d.RuleID == "isc.SOURCE.cloud-cache-update" {
						found = true
						if d.Status != "excluded" || d.Space != "content" || d.Locations[role].Pointer != "/connectorAttributes/cloudCacheUpdate" || d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.source-portable" {
							t.Fatal("exclusion provenance absent", d)
						}
					}
				}
				if !found {
					t.Fatal("cache exclusion decision missing")
				}
				// An absent marker is also outside portable comparison.
				req.Right = cacheMetadataInput(t, "prod", "SOURCE", connector, nil, false, 30)
				if r = compareTest(t, req, p); r.ExitCode() != 0 {
					t.Fatal("missing marker caused a finding", r.Counts)
				}
				// The exclusion must not hide adjacent configuration changes.
				req.Right = cacheMetadataInput(t, "prod", "SOURCE", connector, 1800000000000, true, 60)
				if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts[status] != 1 {
					t.Fatal("adjacent setting was hidden", r.Counts)
				}
				// Disabling the rule restores comparison or missing authority.
				req.Right = cacheMetadataInput(t, "prod", "SOURCE", connector, 1800000000000, true, 30)
				p.DisabledRules = []string{"isc.SOURCE.cloud-cache-update"}
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

func TestCacheExclusionIsLimitedToPortableSource(t *testing.T) {
	for _, tc := range []struct{ profile, typ string }{{"isc-identity-v2", "SOURCE"}, {"isc-envelope-v2", "SOURCE"}, {"isc-portable-v2", "ROLE"}} {
		t.Run(tc.profile+"/"+tc.typ, func(t *testing.T) {
			p := v2.Policy{Version: 2, ID: "invented", Profile: tc.profile}
			r := compareTest(t, Request{Mode: "drift", Left: cacheMetadataInput(t, "dev", tc.typ, "Unknown Connector", 1700000000000, true, 30), Right: cacheMetadataInput(t, "prod", tc.typ, "Unknown Connector", 1800000000000, true, 30)}, p)
			if r.ExitCode() != 1 || r.Counts["different"] != 1 || r.RuleUsage["isc.SOURCE.cloud-cache-update"] != 0 {
				t.Fatal("cache exclusion escaped its scope", r.Counts)
			}
		})
	}
}
