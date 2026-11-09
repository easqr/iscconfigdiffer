package differ

import (
	"context"
	"encoding/json"
	exporter "github.com/easqr/iscconfigexporter"
	"github.com/easqr/iscconfigexporter/snapshot"
	"testing"
)

func obj(typ, id, name string, extra map[string]any) map[string]any {
	payload := map[string]any{}
	if name != "" {
		payload["name"] = name
	}
	for k, v := range extra {
		payload[k] = v
	}
	return map[string]any{"version": json.Number("1"), "self": map[string]any{"type": typ, "id": id}, "object": payload}
}
func input(t *testing.T, env string, types []string, objects ...map[string]any) snapshot.Input {
	t.Helper()
	files := map[string][]byte{}
	for _, o := range objects {
		s := o["self"].(map[string]any)
		p, e := snapshot.ObjectPath(s["type"].(string), s["id"].(string))
		if e != nil {
			t.Fatal(e)
		}
		files[p], _ = json.Marshal(o)
	}
	in, e := snapshot.FromFiles(files, snapshot.Options{Mode: snapshot.TreeMode, Label: env, Environment: env, Types: types})
	if e != nil {
		t.Fatal(e)
	}
	return in
}
func hubJSON(path string, enabled bool) []byte {
	b, _ := json.Marshal([]map[string]any{{"objectMappingId": "map-a", "objectType": "SOURCE", "jsonPath": path, "sourceValue": "dc=example,dc=com", "targetValue": "dc=example-dev,dc=com", "enabled": enabled, "created": "2025-01-01", "modified": "2025-01-02"}})
	return b
}

type retrievedMappingAPI struct {
	data   json.RawMessage
	source string
}

func (a *retrievedMappingAPI) GetMappings(_ context.Context, source string) (exporter.MappingDownload, error) {
	a.source = source
	return exporter.MappingDownload{Data: a.data, APIOrigin: "https://invented-sb.example.invalid"}, nil
}
