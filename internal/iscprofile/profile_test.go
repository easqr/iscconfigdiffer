package iscprofile

import (
	"encoding/json"
	"testing"
)

func TestShippedPackValidation(t *testing.T) {
	data, err := shipped.ReadFile("packs/active-directory.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(map[string]any){
		func(p map[string]any) { p["version"] = 99 },
		func(p map[string]any) { p["unknown"] = true },
		func(p map[string]any) { delete(p, "connector") },
		func(p map[string]any) { p["rules"].(map[string]any)["id"] = "wrong" },
		func(p map[string]any) {
			p["rules"].(map[string]any)["collections"].([]any)[0].(map[string]any)["variant"] = ""
		},
		func(p map[string]any) {
			p["connector"].(map[string]any)["identifiers"].([]any)[0].(map[string]any)["path"] = "/*"
		},
	} {
		var p map[string]any
		if err := json.Unmarshal(data, &p); err != nil {
			t.Fatal(err)
		}
		change(p)
		bad, _ := json.Marshal(p)
		if _, err := decodePack(bad); err == nil {
			t.Fatal("invalid shipped pack accepted")
		}
	}
}
