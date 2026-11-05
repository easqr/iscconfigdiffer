package v2

import (
	"encoding/json"
	"strings"
	"testing"
)

func scopedItemPolicy() Policy {
	p := testPolicy()
	c := mappedCollection()
	c.Keys = []string{"/key", "/kind"}
	c.KeyMappings = nil
	p.Collections = []Collection{c}
	mappings := mappedCollection().KeyMappings
	mappings[0].Groups = append(mappings[0].Groups, CollectionKeyMappingGroup{Key: "regional", Values: map[string]any{"dev": "dev-regional", "prod": "prod-regional"}})
	p.CollectionMappings = []CollectionMapping{{Rule: testRule("item-mappings"), Collection: "items", KeyMappings: mappings}}
	for _, tc := range []struct{ id, group, dev, prod string }{{"a-user", "corporate", "a-dev", "a-prod"}, {"b-user", "regional", "b-dev", "b-prod"}} {
		p.Equivalences = append(p.Equivalences, Equivalence{Rule: testRule(tc.id), Selector: Selector{Type: "THING", Path: "/items/*/user"}, Item: &CollectionItemSelector{Collection: "items", Keys: map[string]CollectionItemKey{"/key": {Mapped: tc.group}, "/kind": {Value: json.RawMessage(`"node"`)}}}, LeftEnv: "dev", RightEnv: "prod", Groups: []map[string]any{{"dev": tc.dev, "prod": tc.prod}}})
	}
	return p
}
func scopedItem(key, user string) any {
	return map[string]any{"key": key, "kind": "node", "user": user}
}
func scopedItemRequest() Request {
	return Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{scopedItem("dev.example", "a-dev"), scopedItem("dev-regional", "b-dev")}})), Right: testInput("prod", doc(map[string]any{"items": []any{scopedItem("prod-regional", "b-prod"), scopedItem("prod.example", "a-prod")}}))}
}
func TestItemScopedEquivalences(t *testing.T) {
	p := scopedItemPolicy()
	req := scopedItemRequest()
	r := runTest(t, req, p)
	if r.ExitCode() != 1 || r.Counts["equivalent"] != 2 {
		t.Fatal("key values should still differ without their own approvals", r.Counts)
	}
	// These are independent approvals for the domain identity fields themselves.
	p.Equivalences = append(p.Equivalences, mappingEquivalence())
	p.Equivalences[2].Selector.Path = "/items/*/key"
	p.Equivalences[2].Groups = append(p.Equivalences[2].Groups, map[string]any{"dev": "dev-regional", "prod": "prod-regional"})
	c := compileTest(t, p)
	before := stable(req)
	r, err := Run(req, c)
	if err != nil || r.ExitCode() != 0 || r.Counts["equivalent"] != 4 {
		t.Fatal(err, r.Counts)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "a-user" {
			found = true
			if d.Locations["left"].Pointer != "/items/0/user" || d.Locations["right"].Pointer != "/items/1/user" || d.Item == nil || d.Item.Keys["/key"].Mapped != "corporate" || len(d.Extensions) != 1 {
				t.Fatal("scope/pointers/provenance lost", d)
			}
		}
	}
	if !found || stable(req) != before {
		t.Fatal("missing decision or request mutated")
	}
	// A pair approved for one identity must not be accepted under another identity.
	req.Left = testInput("dev", doc(map[string]any{"items": []any{scopedItem("dev.example", "b-dev"), scopedItem("dev-regional", "a-dev")}}))
	req.Right = testInput("prod", doc(map[string]any{"items": []any{scopedItem("prod-regional", "a-prod"), scopedItem("prod.example", "b-prod")}}))
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["different"] != 2 || r.RuleUsage["a-user"] != 0 || r.RuleUsage["b-user"] != 0 {
		t.Fatal("cross-item approval leaked", r.Counts)
	}
	req = scopedItemRequest()
	req.Left, req.Right = req.Right, req.Left
	if r = runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("symmetric mapping broke scope", r.Counts)
	}
	req = scopedItemRequest()
	req.Right = testInput("prod", doc(map[string]any{"items": []any{scopedItem("prod.example", "a-prod")}}))
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.RuleUsage["b-user"] != 0 {
		t.Fatal("missing item accepted", r.Counts)
	}
	req = scopedItemRequest()
	req.Right = testInput("prod", doc(map[string]any{"items": []any{scopedItem("prod.example", "a-prod"), scopedItem("prod.example", "a-prod")}}))
	if r = runTest(t, req, p); r.ExitCode() != 2 || r.Counts["conflict"] == 0 {
		t.Fatal("duplicate keys bypassed by scope", r.Counts)
	}
}
func TestItemScopeRawKeysAndRedaction(t *testing.T) {
	p := scopedItemPolicy()
	p.Equivalences = p.Equivalences[:1]
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{scopedItem("corporate", "a-dev")}})), Right: testInput("prod", doc(map[string]any{"items": []any{scopedItem("corporate", "a-prod")}}))}
	if r := runTest(t, req, p); r.ExitCode() != 1 || r.Counts["equivalent"] != 0 {
		t.Fatal("raw key collided with mapped alias", r.Counts)
	}
	p.Equivalences[0].Item.Keys["/key"] = CollectionItemKey{Value: json.RawMessage(`"corporate"`)}
	req.ExcludeValues = true
	r := runTest(t, req, p)
	data, _ := json.Marshal(r)
	if r.ExitCode() != 0 || !strings.Contains(string(data), "corporate") || strings.Contains(string(data), "a-dev") {
		t.Fatal("raw scope or value omission failed", r.Counts)
	}
	p.Sensitive = []Sensitive{{Rule: testRule("sensitive"), Selector: Selector{Type: "THING", Path: "/items"}}}
	r = runTest(t, req, p)
	data, _ = json.Marshal(r)
	if strings.Contains(string(data), "corporate") || strings.Contains(string(data), "a-prod") {
		t.Fatal("scoped identity leaked through sensitive redaction")
	}
}
func TestItemScopeValidationAndConflicts(t *testing.T) {
	for name, change := range map[string]func(*Policy){
		"unknown collection":      func(p *Policy) { p.Equivalences[0].Item.Collection = "missing" },
		"not ancestor":            func(p *Policy) { p.Equivalences[0].Path = "/elsewhere/user" },
		"incomplete tuple":        func(p *Policy) { delete(p.Equivalences[0].Item.Keys, "/kind") },
		"extra component":         func(p *Policy) { p.Equivalences[0].Item.Keys["/extra"] = CollectionItemKey{Mapped: "corporate"} },
		"unknown group":           func(p *Policy) { p.Equivalences[0].Item.Keys["/key"] = CollectionItemKey{Mapped: "typo"} },
		"group in another object": func(p *Policy) { p.Equivalences[0].Key = "one"; p.CollectionMappings[0].Key = "other" },
		"inactive mapping":        func(p *Policy) { p.CollectionMappings[0].Modes = []string{"deployment"} },
		"both key forms": func(p *Policy) {
			p.Equivalences[0].Item.Keys["/key"] = CollectionItemKey{Mapped: "corporate", Value: json.RawMessage(`"x"`)}
		},
		"neither key form": func(p *Policy) { p.Equivalences[0].Item.Keys["/key"] = CollectionItemKey{} },
		"null key": func(p *Policy) {
			p.Equivalences[0].Item.Keys["/kind"] = CollectionItemKey{Value: json.RawMessage(`null`)}
		},
		"array key": func(p *Policy) {
			p.Equivalences[0].Item.Keys["/kind"] = CollectionItemKey{Value: json.RawMessage(`[]`)}
		},
		"blank string": func(p *Policy) {
			p.Equivalences[0].Item.Keys["/kind"] = CollectionItemKey{Value: json.RawMessage(`" "`)}
		},
		"numeric string component": func(p *Policy) { p.Equivalences[0].Item.Keys["/kind"] = CollectionItemKey{Value: json.RawMessage(`1`)} },
		"duplicate scope":          func(p *Policy) { p.Equivalences[1].Item = p.Equivalences[0].Item },
		"broad competing rule":     func(p *Policy) { p.Equivalences[1].Item = nil },
	} {
		t.Run(name, func(t *testing.T) {
			p := scopedItemPolicy()
			change(&p)
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid item scope accepted")
			}
		})
	}
	// Canonically equal numeric raw keys have overlapping ownership.
	p := scopedItemPolicy()
	p.Collections[0].ComponentType = "scalar"
	for i := range p.Equivalences {
		p.Equivalences[i].Item.Keys["/key"] = CollectionItemKey{Mapped: "corporate"}
		p.Equivalences[i].Item.Keys["/kind"] = CollectionItemKey{Value: []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`1.0`)}[i]}
	}
	if _, err := Compile(p); err == nil {
		t.Fatal("equivalent numeric scopes treated as disjoint")
	}
	p.Equivalences[1].Item.Keys["/kind"] = CollectionItemKey{Value: json.RawMessage(`"1"`)}
	compileTest(t, p)
}
func TestItemScopeDetachmentAndRequiredCoverage(t *testing.T) {
	p := scopedItemPolicy()
	req := scopedItemRequest()
	before := stable(p)
	c := compileTest(t, p)
	if before != stable(p) {
		t.Fatal("compiler mutated item scopes")
	}
	r, err := Run(req, c)
	if err != nil {
		t.Fatal(err)
	}
	p.Equivalences[0].Item.Keys["/key"] = CollectionItemKey{Mapped: "regional"}
	results := make(chan bool, 4)
	for range 4 {
		go func() { again, err := Run(req, c); results <- err == nil && stable(r) == stable(again) }()
	}
	for range 4 {
		if !<-results {
			t.Fatal("compiled scope was mutated")
		}
	}
	p = scopedItemPolicy()
	p.Equivalences[1].Required = true
	req.Left = testInput("dev", doc(map[string]any{"items": []any{scopedItem("dev.example", "a-dev")}}))
	req.Right = testInput("prod", doc(map[string]any{"items": []any{scopedItem("prod.example", "a-prod")}}))
	if r = runTest(t, req, p); r.ExitCode() != 2 || r.RuleUsage["b-user"] != 0 {
		t.Fatal("unused required item approval was ignored", r.Counts)
	}
}

func TestItemScopeNestedCollectionAndConditionalTuples(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("outer"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/key"}}, {Rule: testRule("inner"), Selector: Selector{Type: "THING", Path: "/items/*/children"}, Mode: "keyed", Keys: []string{"/key"}}}
	p.Equivalences = []Equivalence{{Rule: testRule("outer-user"), Selector: Selector{Type: "THING", Path: "/items/*/children/*/user"}, Item: &CollectionItemSelector{Collection: "outer", Keys: map[string]CollectionItemKey{"/key": {Value: json.RawMessage(`"a"`)}}}, LeftEnv: "dev", RightEnv: "prod", Groups: []map[string]any{{"dev": "dev-user", "prod": "prod-user"}}}}
	item := func(key, user string) any {
		return map[string]any{"key": key, "children": []any{map[string]any{"key": "child", "user": user}}}
	}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{item("a", "dev-user"), item("b", "dev-user")}})), Right: testInput("prod", doc(map[string]any{"items": []any{item("b", "prod-user"), item("a", "prod-user")}}))}
	r := runTest(t, req, p)
	if r.ExitCode() != 1 || r.Counts["equivalent"] != 1 || r.Counts["different"] != 1 {
		t.Fatal("ancestor scope lost across nested collection", r.Counts)
	}
	competing := p.Equivalences[0]
	competing.ID = "inner-user"
	competing.Item = &CollectionItemSelector{Collection: "inner", Keys: map[string]CollectionItemKey{"/key": {Value: json.RawMessage(`"child"`)}}}
	p.Equivalences = append(p.Equivalences, competing)
	if _, err := Compile(p); err == nil {
		t.Fatal("potentially overlapping ancestor scopes accepted")
	}
	p = testPolicy()
	c := conditionalCollection()
	c.KeyMappings = []CollectionKeyMapping{{Path: "/scope", Groups: []CollectionKeyMappingGroup{{Key: "branch", Values: map[string]any{"dev": "dev-branch", "prod": "prod-branch"}}}}}
	p.Collections = []Collection{c}
	for _, kind := range []string{"plain", "scoped"} {
		keys := map[string]CollectionItemKey{"/kind": {Value: json.RawMessage(`"` + kind + `"`)}}
		if kind == "scoped" {
			keys["/scope"] = CollectionItemKey{Mapped: "branch"}
		}
		p.Equivalences = append(p.Equivalences, Equivalence{Rule: testRule(kind + "-user"), Selector: Selector{Type: "THING", Path: "/items/*/user"}, Item: &CollectionItemSelector{Collection: "items", Keys: keys}, LeftEnv: "dev", RightEnv: "prod", Groups: []map[string]any{{"dev": "dev-user", "prod": "prod-user"}}})
	}
	req.Left = testInput("dev", doc(map[string]any{"items": []any{map[string]any{"kind": "plain", "user": "dev-user"}, map[string]any{"kind": "scoped", "scope": "dev-branch", "user": "dev-user"}}}))
	req.Right = testInput("prod", doc(map[string]any{"items": []any{map[string]any{"kind": "scoped", "scope": "prod-branch", "user": "prod-user"}, map[string]any{"kind": "plain", "user": "prod-user"}}}))
	r = runTest(t, req, p)
	if r.Counts["equivalent"] != 2 || r.RuleUsage["plain-user"] != 1 || r.RuleUsage["scoped-user"] != 1 {
		t.Fatal("conditional tuples not selected", r.Counts)
	}
}

func TestItemScopeRequiresConsistentComponentsAcrossInputs(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/a"}, KeyCases: []CollectionKeyCase{{WhenPath: "/kind", WhenValue: json.RawMessage(`"scoped"`), Keys: []string{"/b"}}}}}
	p.Equivalences = []Equivalence{{Rule: testRule("user"), Selector: Selector{Type: "THING", Path: "/items/*/user"}, Item: &CollectionItemSelector{Collection: "items", Keys: map[string]CollectionItemKey{"/b": {Value: json.RawMessage(`"same"`)}}}, LeftEnv: "dev", RightEnv: "prod", Groups: []map[string]any{{"dev": "dev-user", "prod": "prod-user"}}}}
	item := func(kind, user string) any {
		return map[string]any{"kind": kind, "a": "same", "b": "same", "user": user}
	}
	r := runTest(t, Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{item("plain", "dev-user")}})), Right: testInput("prod", doc(map[string]any{"items": []any{item("scoped", "prod-user")}}))}, p)
	if r.ExitCode() != 1 || r.RuleUsage["user"] != 0 {
		t.Fatal("scope borrowed key components from only one input", r.Counts)
	}
	p = scopedItemPolicy()
	p.Collections[0].KeyCases = []CollectionKeyCase{{WhenPath: "/flag", WhenValue: json.RawMessage(`true`), Keys: []string{"/kind", "/key"}}}
	if _, err := Compile(p); err == nil {
		t.Fatal("ambiguous component ordering accepted")
	}
}
