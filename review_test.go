package differ

import (
	"encoding/json"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"testing"
	"time"
)

func TestReviewLegacyExternalCannotOverrideKnownCorrespondence(t *testing.T) {
	types := []string{"ROLE", "SOURCE"}
	left := input(t, "dev", types, obj("ROLE", "lrole", "Role", map[string]any{"ref": "lsource"}), obj("SOURCE", "lsource", "Directory", nil))
	right := input(t, "prod", types, obj("ROLE", "rrole", "Role", map[string]any{"ref": "wrong-external-id"}), obj("SOURCE", "rsource", "Directory", nil))
	p := policy()
	p.References = []Reference{{ID: "ref", Selector: Selector{Type: "ROLE", Path: "/object/ref"}, TargetType: "SOURCE", External: map[string]string{"lsource": "wrong-external-id"}}}
	r := runCompare(t, left, right, p)
	if r.ExitCode() != 2 {
		t.Fatal("external mapping overrode established reference correspondence", r)
	}
}

func TestReviewLegacyPositionalAssertionUnderKeyedArrayRejected(t *testing.T) {
	p := policy()
	p.Collections = []Collection{{ID: "items", Selector: Selector{Type: "ROLE", Path: "/object/items"}, Keys: []string{"/key"}}}
	p.Expectations = []Expectation{{ID: "assert", Selector: Selector{Type: "ROLE", Path: "/object/items/0/value"}, Values: map[string]any{"dev": "d", "prod": "p"}}}
	if p.Validate() == nil {
		t.Fatal("unsafe positional assertion accepted below keyed alignment")
	}
}

func TestReviewHubSidecarRequiredCountAndRetrievalTime(t *testing.T) {
	raw := []byte(`[]`)
	opts := HubV2Options{LeftEnvironment: "dev", RightEnvironment: "prod"}
	base := map[string]any{"schema": 1, "retrieval_id": "invented", "source_org": "invented-source", "target_tenant": "invented-target", "api_origin": "https://invented.example.invalid", "api_version": "v2026", "endpoint": "/v2026/configuration-hub/object-mappings/invented-source", "retrieved": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), "count": 0, "sha256": snapshot.Checksum(raw)}
	for _, tc := range []string{"missing-count", "zero-time"} {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		if tc == "missing-count" {
			delete(m, "count")
		} else {
			m["retrieved"] = "0001-01-01T00:00:00Z"
		}
		sidecar, _ := json.Marshal(m)
		if _, err := ImportHubV2(raw, sidecar, opts); err == nil {
			t.Fatal("malformed sidecar accepted", tc)
		}
	}
}

func TestReviewISCIdentityTypesPreserveV1Constraints(t *testing.T) {
	in := input(t, "e", []string{"ROLE"}, obj("ROLE", "id", "", map[string]any{"name": 42}))
	legacy := policy()
	if r := runCompare(t, in, in, legacy); r.ExitCode() != 2 {
		t.Fatal("legacy unexpectedly accepted numeric name", r)
	}
	r := compareV2Test(t, V2Request{Mode: "drift", Left: in, Right: in}, portableV2())
	if r.ExitCode() != 2 {
		t.Fatal("ISC v2 profile accepted numeric name", r)
	}
	legacy.Identities = []Identity{{ID: "custom", Type: "ROLE", Strategy: "fields", Paths: []string{"/object/name"}}}
	migrated, e := TranslateV1(legacy)
	if e != nil {
		t.Fatal(e)
	}
	r = compareV2Test(t, V2Request{Mode: "drift", Left: in, Right: in}, migrated)
	if r.ExitCode() != 2 {
		t.Fatal("migration relaxed string-only identity", r)
	}
}
