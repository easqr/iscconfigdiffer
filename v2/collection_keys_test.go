package v2

import (
	"encoding/json"
	"testing"
)

func conditionalCollection() Collection {
	return Collection{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/kind"}, ComponentType: "string", KeyCases: []CollectionKeyCase{{WhenPath: "/kind", WhenValue: json.RawMessage(`"scoped"`), Keys: []string{"/kind", "/scope"}}}}
}
func TestConditionalCollectionKeys(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{conditionalCollection()}
	a := map[string]any{"kind": "plain"}
	b := map[string]any{"kind": "scoped", "scope": "one"}
	c := map[string]any{"kind": "scoped", "scope": "two"}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{a, b, c}})), Right: testInput("prod", doc(map[string]any{"items": []any{c, a, b}}))}
	if r := runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal(r)
	}
	for _, value := range []any{nil, 42, "", " "} {
		bad := map[string]any{"kind": "scoped", "scope": value}
		req.Right = testInput("prod", doc(map[string]any{"items": []any{a, bad}}))
		if r := runTest(t, req, p); r.ExitCode() != 2 {
			t.Fatal("invalid conditional key accepted", value, r)
		}
	}
}
func TestConditionalCollectionCompilerValidation(t *testing.T) {
	for _, change := range []func(*Collection){
		func(c *Collection) { c.Mode = "ordered"; c.Keys = nil },
		func(c *Collection) { c.ComponentType = "invalid" },
		func(c *Collection) { c.KeyCases[0].WhenValue = nil },
		func(c *Collection) { c.KeyCases[0].WhenValue = json.RawMessage(`{}`) },
		func(c *Collection) { c.KeyCases[0].WhenPath = "/*" },
		func(c *Collection) { c.KeyCases[0].Keys = []string{"/*"} },
		func(c *Collection) { c.KeyCases[0].Keys = nil },
		func(c *Collection) { c.KeyCases = append(c.KeyCases, c.KeyCases[0]) },
		func(c *Collection) {
			c.KeyCases = append(c.KeyCases, CollectionKeyCase{WhenPath: "/other", WhenValue: json.RawMessage(`"other"`), Keys: []string{"/key"}})
		},
	} {
		c := conditionalCollection()
		change(&c)
		p := testPolicy()
		p.Collections = []Collection{c}
		if _, err := Compile(p); err == nil {
			t.Fatal("invalid key case accepted", c)
		}
	}
	// Existing generic scalar keys continue to permit empty strings.
	c := conditionalCollection()
	c.KeyCases = nil
	c.ComponentType = ""
	if _, _, err := collectionKey(map[string]any{"kind": ""}, c, ""); err != nil {
		t.Fatal(err)
	}
}
