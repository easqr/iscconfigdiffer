package differ

import (
	"encoding/json"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"testing"
	"time"
)

func TestReviewHubSidecarRequiredCountAndRetrievalTime(t *testing.T) {
	raw := []byte(`[]`)
	opts := HubOptions{LeftEnvironment: "dev", RightEnvironment: "prod"}
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
		if _, err := ImportHub(raw, sidecar, opts); err == nil {
			t.Fatal("malformed sidecar accepted", tc)
		}
	}
}

func TestISCIdentityComponentsAreStrings(t *testing.T) {
	in := input(t, "e", []string{"ROLE"}, obj("ROLE", "id", "", map[string]any{"name": 42}))
	report := compareTest(t, Request{Mode: "drift", Left: in, Right: in}, portable())
	if report.ExitCode() != 2 {
		t.Fatal("ISC identity accepted a numeric name", report)
	}
}
