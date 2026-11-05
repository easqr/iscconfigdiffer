package differ

import (
	"encoding/json"
	"testing"
)

func TestHubPointer(t *testing.T) {
	for path, want := range map[string]string{"$": "", "$.owner.name": "/owner/name", `$['a/b']["~key"][0]`: "/a~1b/~0key/0", "$.items[*].name": "/items/*/name", "$.object.host": "/object/host"} {
		got, err := HubPointer(path)
		if err != nil || got != want {
			t.Fatal(path, got, err)
		}
	}
	for _, path := range []string{"owner.name", "$..name", "$.x[?(@.id)]", "$.x[0:2]", "$.x[1,2]", "$.*", "$['*']"} {
		if _, err := HubPointer(path); err == nil {
			t.Fatal("unsupported path accepted", path)
		}
	}
}

func TestHubAmbiguityAndPayloadPaths(t *testing.T) {
	opts := HubOptions{SourceTenant: "invented-source", TargetTenant: "invented-target", LeftEnvironment: "dev", RightEnvironment: "prod"}
	record := func(id, path, source, target string) map[string]any {
		return map[string]any{"objectMappingId": id, "objectType": "SOURCE", "jsonPath": path, "sourceValue": source, "targetValue": target, "enabled": true}
	}
	for _, records := range [][]map[string]any{{record("a", "$.host", "dev", "prod"), record("b", "$.host", "dev", "other")}, {record("a", "$.host", "dev", "middle"), record("b", "$.host", "middle", "prod")}, {record("a", "$.items[*].host", "dev", "prod"), record("b", "$.items[0].host", "other", "different")}, {record("a", "$.host", "dev", "prod"), record("a", "$.other", "x", "y")}} {
		data, _ := json.Marshal(records)
		if _, err := ImportHub(data, nil, opts); err == nil {
			t.Fatal("ambiguous mappings accepted")
		}
	}
	data, _ := json.Marshal([]map[string]any{record("a", "$.host", "dev-a", "prod-a"), record("b", "$.host", "dev-b", "prod-b")})
	result, err := ImportHub(data, nil, opts)
	if err != nil || len(result.Equivalences) != 1 || len(result.Equivalences[0].Pairs) != 2 {
		t.Fatal(result, err)
	}
	data, _ = json.Marshal([]map[string]any{record("a", "$.object.host", "dev", "prod")})
	result, err = ImportHub(data, nil, opts)
	if err != nil || result.Equivalences[0].Path != "/object/host" {
		t.Fatal("payload member was stripped", result, err)
	}
}
