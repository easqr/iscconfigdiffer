package v2

import (
	"encoding/json"
	"strings"
	"testing"
)

func mappedCollection() Collection {
	return Collection{
		Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"},
		Mode: "keyed", Keys: []string{"/key"}, ComponentType: "string",
		ItemLabel: "Item", DisplayFields: []DisplayField{{Label: "Name", Path: "/key"}},
		KeyMappings: []CollectionKeyMapping{{Path: "/key", Groups: []CollectionKeyMappingGroup{
			{Key: "corporate", Values: map[string]any{"dev": "dev.example", "prod": "prod.example"}},
		}}},
	}
}
func mappedItem(key, value string) any { return map[string]any{"key": key, "value": value} }
func mappingEquivalence() Equivalence {
	return Equivalence{Rule: testRule("names"), Selector: Selector{Type: "THING", Path: "/items/*/key"}, LeftEnv: "dev", RightEnv: "prod", Groups: []map[string]any{{"dev": "dev.example", "prod": "prod.example"}}}
}
func TestCollectionKeyMappingsAlignWithoutGrantingEquivalence(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{mappedCollection()}
	req := Request{Mode: "drift", IncludeMatching: true,
		Left:  testInput("dev", doc(map[string]any{"items": []any{mappedItem("dev.example", "shared"), mappedItem("unchanged", "other")}})),
		Right: testInput("prod", doc(map[string]any{"items": []any{mappedItem("unchanged", "other"), mappedItem("prod.example", "shared")}})),
	}
	before := stable(req)
	r := runTest(t, req, p)
	if r.ExitCode() != 1 || r.Counts["different"] != 1 {
		t.Fatalf("mapping must align but not accept differing names: %v", r.Counts)
	}
	for _, d := range r.Decisions {
		if d.Status == "different" && (d.Locations["left"].Pointer != "/items/0/key" || d.Locations["right"].Pointer != "/items/1/key" || len(d.ItemKeys) != 1 || len(d.Locations["right"].Context) != 1) {
			t.Fatal("lost original paths/context", d)
		}
	}
	if before != stable(req) {
		t.Fatal("request mutated")
	}
	p.Equivalences = []Equivalence{mappingEquivalence()}
	r = runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 {
		t.Fatalf("explicit equivalence not applied: %v", r.Counts)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "items" && strings.Contains(d.Code, "key mapping") {
			found = d.Evidence != nil && d.Evidence.Kind == "invented-policy"
		}
	}
	if !found {
		t.Fatal("mapping alignment provenance missing")
	}
	req.Left, req.Right = req.Right, req.Left
	if r = runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("group mappings must be symmetric", r.Counts)
	}
	req.Left = testInput("prod", doc(map[string]any{"items": []any{mappedItem("prod.example", "changed"), mappedItem("unchanged", "other")}}))
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["different"] != 1 {
		t.Fatal("other settings hidden", r.Counts)
	}
}
func TestCollectionKeyMappingContextsAndCollisions(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{mappedCollection()}
	for _, tc := range []struct {
		name, leftEnv, rightEnv string
		left, right             []any
		exit                    int
	}{
		{"exact fallback", "dev", "prod", []any{mappedItem("unmapped", "a")}, []any{mappedItem("unmapped", "a")}, 0},
		{"alias is not a raw name", "dev", "prod", []any{mappedItem("dev.example", "a"), mappedItem("corporate", "a")}, []any{mappedItem("corporate", "a"), mappedItem("prod.example", "a")}, 1},
		{"wrong environment value stays raw", "dev", "prod", []any{mappedItem("dev.example", "a")}, []any{mappedItem("dev.example", "a")}, 1},
		{"missing environment", "", "prod", []any{mappedItem("dev.example", "a")}, []any{mappedItem("prod.example", "a")}, 2},
		{"unknown environment", "test", "prod", []any{mappedItem("unmapped", "a")}, []any{mappedItem("unmapped", "a")}, 2},
		{"duplicate mapped key", "dev", "prod", []any{mappedItem("dev.example", "a"), mappedItem("dev.example", "b")}, []any{mappedItem("prod.example", "a")}, 2},
		{"duplicate raw key", "dev", "prod", []any{mappedItem("unmapped", "a"), mappedItem("unmapped", "b")}, []any{mappedItem("unmapped", "a")}, 2},
		{"missing key", "dev", "prod", []any{map[string]any{"value": "a"}}, []any{mappedItem("prod.example", "a")}, 2},
		{"missing forest", "dev", "prod", []any{mappedItem("dev.example", "a")}, []any{}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runTest(t, Request{Mode: "drift", Left: testInput(tc.leftEnv, doc(map[string]any{"items": tc.left})), Right: testInput(tc.rightEnv, doc(map[string]any{"items": tc.right}))}, p)
			if r.ExitCode() != tc.exit {
				t.Fatalf("exit=%d counts=%v", r.ExitCode(), r.Counts)
			}
		})
	}
	p.Bindings = map[string]string{"left": "dev", "right": "prod"}
	p.Equivalences = []Equivalence{mappingEquivalence()}
	req := Request{Mode: "drift", Left: testInput("", doc(map[string]any{"items": []any{mappedItem("dev.example", "a")}})), Right: testInput("", doc(map[string]any{"items": []any{mappedItem("prod.example", "a")}}))}
	if r := runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("bindings not honored", r.Counts)
	}
}
func TestCollectionMappingCompilerValidation(t *testing.T) {
	for name, change := range map[string]func(*Collection){
		"ordered":        func(c *Collection) { c.Mode = "ordered"; c.Keys = nil; c.ComponentType = "" },
		"unknown path":   func(c *Collection) { c.KeyMappings[0].Path = "/other" },
		"wildcard path":  func(c *Collection) { c.KeyMappings[0].Path = "/*" },
		"duplicate path": func(c *Collection) { c.KeyMappings = append(c.KeyMappings, c.KeyMappings[0]) },
		"no groups":      func(c *Collection) { c.KeyMappings[0].Groups = nil },
		"blank group":    func(c *Collection) { c.KeyMappings[0].Groups[0].Key = " " },
		"duplicate group": func(c *Collection) {
			c.KeyMappings[0].Groups = append(c.KeyMappings[0].Groups, c.KeyMappings[0].Groups[0])
		},
		"one environment":    func(c *Collection) { delete(c.KeyMappings[0].Groups[0].Values, "prod") },
		"blank environment":  func(c *Collection) { c.KeyMappings[0].Groups[0].Values[" "] = "x" },
		"null":               func(c *Collection) { c.KeyMappings[0].Groups[0].Values["dev"] = nil },
		"object":             func(c *Collection) { c.KeyMappings[0].Groups[0].Values["dev"] = map[string]any{} },
		"array":              func(c *Collection) { c.KeyMappings[0].Groups[0].Values["dev"] = []any{} },
		"numeric string key": func(c *Collection) { c.KeyMappings[0].Groups[0].Values["dev"] = 42 },
		"blank string key":   func(c *Collection) { c.KeyMappings[0].Groups[0].Values["dev"] = " " },
		"ambiguous value": func(c *Collection) {
			c.KeyMappings[0].Groups = append(c.KeyMappings[0].Groups, CollectionKeyMappingGroup{Key: "other", Values: map[string]any{"dev": "dev.example", "prod": "other"}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := testPolicy()
			c := mappedCollection()
			change(&c)
			p.Collections = []Collection{c}
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid mapping accepted")
			}
		})
	}
}
func TestCollectionMappingTypedCompositeAndConditionalKeys(t *testing.T) {
	c := conditionalCollection()
	c.KeyMappings = []CollectionKeyMapping{{Path: "/scope", Groups: []CollectionKeyMappingGroup{{Key: "scope", Values: map[string]any{"dev": "one", "prod": "two"}}}}}
	if err := validateCollectionKeys(c); err != nil {
		t.Fatal(err)
	}
	a, used, err := collectionKey(map[string]any{"kind": "scoped", "scope": "one"}, c, "dev")
	b, _, _ := collectionKey(map[string]any{"kind": "scoped", "scope": "two"}, c, "prod")
	if err != nil || !used || a != b {
		t.Fatal("conditional mapped tuple not aligned")
	}
	if _, used, err := collectionKey(map[string]any{"kind": "plain"}, c, ""); err != nil || used {
		t.Fatal("inactive mapping required environment")
	}
	c = mappedCollection()
	c.ComponentType = "scalar"
	c.KeyMappings[0].Groups[0].Values = map[string]any{"dev": json.Number("1.0"), "prod": true}
	if err := validateCollectionKeys(c); err != nil {
		t.Fatal(err)
	}
	a, _, _ = collectionKey(map[string]any{"key": json.Number("1e0")}, c, "dev")
	b, _, _ = collectionKey(map[string]any{"key": true}, c, "prod")
	raw, used, _ := collectionKey(map[string]any{"key": "1"}, c, "dev")
	if a != b || a == raw || used {
		t.Fatal("typed keys coerced")
	}
}
func TestCollectionMappingDeploymentRequiresIndependentExpectations(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{mappedCollection()}
	p.Expectations = []Expectation{
		{Rule: testRule("metadata"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"},
		{Rule: testRule("root-name"), Selector: Selector{Type: "THING", Path: "/name"}, Operation: "copy_source", Authority: "approved"},
		{Rule: testRule("value"), Selector: Selector{Type: "THING", Path: "/items/*/value"}, Operation: "preserve_target", Authority: "approved"},
	}
	baseline := testInput("prod", doc(map[string]any{"items": []any{mappedItem("unchanged", "b"), mappedItem("prod.example", "target")}}))
	req := Request{Mode: "deployment", Left: testInput("dev", doc(map[string]any{"items": []any{mappedItem("dev.example", "source"), mappedItem("unchanged", "b")}})), Right: baseline, Baseline: &baseline, Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}
	r := runTest(t, req, p)
	if r.ExitCode() != 2 || r.Counts["unverified"] != 2 {
		t.Fatal("alignment granted name authority", r.Counts)
	}
	p.Expectations = append(p.Expectations, Expectation{Rule: testRule("key"), Selector: Selector{Type: "THING", Path: "/items/*/key"}, Operation: "copy_source", Authority: "approved"})
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["violation"] != 1 {
		t.Fatal("mapping changed copy_source semantics", r.Counts)
	}
	p.Expectations[len(p.Expectations)-1] = Expectation{Rule: testRule("key"), Selector: Selector{Type: "THING", Path: "/items/*/key"}, Operation: "preserve_target", Authority: "approved"}
	if r = runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("baseline correspondence failed", r.Counts)
	}
	// Source and actual must align even if the optional baseline is absent.
	req.Baseline = nil
	p.Expectations[2].Operation = "literal"
	p.Expectations[2].Expected = present("target")
	p.Expectations[3].Operation = "literal"
	p.Expectations[3].Expected = present("prod.example")
	req.Left = testInput("dev", doc(map[string]any{"items": []any{mappedItem("dev.example", "source")}}))
	req.Right = testInput("prod", doc(map[string]any{"items": []any{mappedItem("prod.example", "target")}}))
	if r = runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("literal destination assertion failed", r.Counts)
	}
}

func TestCollectionMappingDetachmentAndDeterministicDigests(t *testing.T) {
	p := testPolicy()
	c := mappedCollection()
	c.KeyMappings[0].Groups = append(c.KeyMappings[0].Groups, CollectionKeyMappingGroup{Key: "secondary", Values: map[string]any{"dev": "dev-secondary", "prod": "prod-secondary"}})
	c.Keys = append(c.Keys, "/value")
	c.KeyMappings = append(c.KeyMappings, CollectionKeyMapping{Path: "/value", Groups: []CollectionKeyMappingGroup{{Key: "value", Values: map[string]any{"dev": "dev-value", "prod": "prod-value"}}}})
	p.Collections = []Collection{c}
	p.Equivalences = []Equivalence{mappingEquivalence()}
	before := stable(p)
	a := compileTest(t, p)
	if before != stable(p) {
		t.Fatal("compiler mutated caller mappings")
	}
	p.Collections[0].KeyMappings[0].Groups[0], p.Collections[0].KeyMappings[0].Groups[1] = p.Collections[0].KeyMappings[0].Groups[1], p.Collections[0].KeyMappings[0].Groups[0]
	p.Collections[0].KeyMappings[0], p.Collections[0].KeyMappings[1] = p.Collections[0].KeyMappings[1], p.Collections[0].KeyMappings[0]
	b := compileTest(t, p)
	if a.digest != b.digest {
		t.Fatal("mapping order changed effective digest")
	}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"items": []any{mappedItem("dev.example", "same")}})), Right: testInput("prod", doc(map[string]any{"items": []any{mappedItem("prod.example", "same")}}))}
	r, err := Run(req, a)
	if err != nil || r.ExitCode() != 0 {
		t.Fatal("compiled mapping failed", err, r.Counts)
	}
	p.Collections[0].KeyMappings[1].Groups[0].Values["dev"] = "caller-mutation"
	if changed := compileTest(t, p); changed.digest == a.digest {
		t.Fatal("mapping values omitted from digest")
	}
	r2, err := Run(req, a)
	if err != nil || stable(r) != stable(r2) {
		t.Fatal("compiled mapping retained caller-owned state")
	}
	results := make(chan bool, 4)
	for range 4 {
		go func() {
			result, err := Run(req, a)
			results <- err == nil && stable(result) == stable(r)
		}()
	}
	for range 4 {
		if !<-results {
			t.Fatal("concurrent mapping run changed the result")
		}
	}
}
