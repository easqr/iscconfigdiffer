package v2

import (
	"testing"
)

func extensionPolicy() Policy {
	p := testPolicy()
	c := mappedCollection()
	c.KeyMappings = nil
	c.Variant = "kind-a"
	p.Collections = []Collection{c}
	p.CollectionMappings = []CollectionMapping{{Rule: testRule("environment-items"), Collection: c.ID, Key: "invented", KeyMappings: mappedCollection().KeyMappings}}
	p.Equivalences = []Equivalence{mappingEquivalence()}
	return p
}
func variantInput(env, variant, key string) Input {
	in := testInput(env, doc(map[string]any{"items": []any{mappedItem(key, "shared")}}))
	in.Documents[0].Variant = variant
	return in
}
func TestCollectionExtensionsAreScopedAndAttributed(t *testing.T) {
	p := extensionPolicy()
	p.CollectionMappings[0].Required = true
	req := Request{Mode: "drift", IncludeMatching: true, Left: variantInput("dev", "kind-a", "dev.example"), Right: variantInput("prod", "kind-a", "prod.example")}
	before := stable(p)
	c := compileTest(t, p)
	if before != stable(p) {
		t.Fatal("policy mutated")
	}
	r, err := Run(req, c)
	if err != nil || r.ExitCode() != 0 || r.RuleUsage["environment-items"] != 1 {
		t.Fatal(err, r.Counts)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "items" {
			found = true
			if d.Replaces != "" || len(d.Extensions) != 1 || d.Extensions[0].RuleID != "environment-items" || d.Extensions[0].Collection != "items" || d.Extensions[0].Evidence.Kind != "invented-policy" {
				t.Fatal(d)
			}
		}
	}
	if !found {
		t.Fatal("collection attribution missing")
	}
	p.CollectionMappings[0].KeyMappings[0].Groups[0].Values["dev"] = "mutation"
	r2, err := Run(req, c)
	if err != nil || stable(r) != stable(r2) {
		t.Fatal("compiled extensions retained caller state")
	}
	// A mapped value in another named object does not inherit this extension.
	p = extensionPolicy()
	p.Equivalences = nil
	req.Left.Documents[0].Content.(map[string]any)["name"] = "other"
	req.Right.Documents[0].Content.(map[string]any)["name"] = "other"
	r = runTest(t, req, p)
	if r.ExitCode() != 1 || r.RuleUsage["environment-items"] != 0 {
		t.Fatal("extension escaped object scope", r.Counts)
	}
	// Connector-specific collections never activate from array shape alone.
	req.Left.Documents[0].Variant = "kind-b"
	req.Right.Documents[0].Variant = "kind-b"
	r = runTest(t, req, p)
	if r.RuleUsage["items"] != 0 || r.RuleUsage["environment-items"] != 0 {
		t.Fatal("collection escaped variant scope")
	}
	for _, d := range r.Decisions {
		if len(d.Extensions) > 0 {
			t.Fatal("inactive extension provenance retained")
		}
		for _, l := range d.Locations {
			if len(l.Context) > 0 {
				t.Fatal("inactive variant context retained")
			}
		}
	}
	req.Right.Documents[0].Variant = "kind-a"
	r = runTest(t, req, p)
	if r.ExitCode() != 2 || r.Counts["conflict"] != 1 {
		t.Fatal("variant mismatch silently accepted", r.Counts)
	}
}
func TestCollectionExtensionValidation(t *testing.T) {
	for name, change := range map[string]func(*Policy){
		"unknown collection": func(p *Policy) { p.CollectionMappings[0].Collection = "missing" },
		"non-keyed collection": func(p *Policy) {
			p.Collections[0].Mode = "ordered"
			p.Collections[0].Keys = nil
			p.Collections[0].ComponentType = ""
		},
		"missing mappings":      func(p *Policy) { p.CollectionMappings[0].KeyMappings = nil },
		"unknown key component": func(p *Policy) { p.CollectionMappings[0].KeyMappings[0].Path = "/other" },
		"duplicate extension": func(p *Policy) {
			e := p.CollectionMappings[0]
			e.ID = "another"
			p.CollectionMappings = append(p.CollectionMappings, e)
		},
		"inline overlap":        func(p *Policy) { p.Collections[0].KeyMappings = mappedCollection().KeyMappings },
		"object scope mismatch": func(p *Policy) { p.Collections[0].Key = "other" },
		"mode expansion":        func(p *Policy) { p.Collections[0].Modes = []string{"drift"} },
		"duplicate ID":          func(p *Policy) { p.CollectionMappings[0].ID = "items" },
		"invalid variant":       func(p *Policy) { p.Collections[0].Variant = " " },
	} {
		t.Run(name, func(t *testing.T) {
			p := extensionPolicy()
			change(&p)
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid extension accepted")
			}
		})
	}
	// Independent object scopes can use independent mapping cases.
	p := extensionPolicy()
	e := p.CollectionMappings[0]
	e.ID = "other-items"
	e.Key = "other"
	p.CollectionMappings = append(p.CollectionMappings, e)
	compileTest(t, p)
	// Explicitly disjoint modes are also independent scopes.
	p = extensionPolicy()
	p.CollectionMappings[0].Modes = []string{"drift"}
	e = p.CollectionMappings[0]
	e.ID = "deployment-items"
	e.Modes = []string{"deployment"}
	p.CollectionMappings = append(p.CollectionMappings, e)
	compileTest(t, p)
}

func TestExtensionDigestsAndUnavailableTargets(t *testing.T) {
	p := extensionPolicy()
	p.Collections[0].Keys = append(p.Collections[0].Keys, "/value")
	p.CollectionMappings[0].KeyMappings = append(p.CollectionMappings[0].KeyMappings, CollectionKeyMapping{Path: "/value", Groups: []CollectionKeyMappingGroup{{Key: "value", Values: map[string]any{"dev": "a", "prod": "b"}}}})
	a := compileTest(t, p)
	p.CollectionMappings[0].KeyMappings[0], p.CollectionMappings[0].KeyMappings[1] = p.CollectionMappings[0].KeyMappings[1], p.CollectionMappings[0].KeyMappings[0]
	b := compileTest(t, p)
	if a.digest != b.digest {
		t.Fatal("extension path order changed effective digest")
	}
	p = extensionPolicy()
	profile := Profile{ID: "base", Rules: Policy{Collections: p.Collections}}
	p.Profile = "base"
	p.Collections = nil
	p.DisabledRules = []string{"items"}
	if _, err := Compile(p, profile); err == nil {
		t.Fatal("extension targeted disabled collection")
	}
	p.DisabledRules = nil
	c := profile.Rules.Collections[0]
	c.Rule = Rule{ID: "replacement", Replaces: "items"}
	p.Collections = []Collection{c}
	if _, err := Compile(p, profile); err == nil {
		t.Fatal("extension silently redirected to replacement")
	}
	p.CollectionMappings[0].Collection = "replacement"
	if _, err := Compile(p, profile); err != nil {
		t.Fatal("effective replacement cannot be extended", err)
	}
}
func TestDisabledShippedRules(t *testing.T) {
	profile := Profile{ID: "base", Rules: Policy{Identities: testPolicy().Identities, Ignores: []Ignore{{Rule: testRule("generated-id"), Selector: Selector{Type: "THING", Path: "/ref/id"}}}}}
	p := Policy{Version: 2, ID: "p", Profile: "base"}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"ref": map[string]any{"id": "left"}})), Right: testInput("prod", doc(map[string]any{"ref": map[string]any{"id": "right"}}))}
	a, err := Compile(p, profile)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Run(req, a)
	if err != nil || r.ExitCode() != 0 {
		t.Fatal(err, r.Counts)
	}
	p.DisabledRules = []string{"generated-id"}
	b, err := Compile(p, profile)
	if err != nil {
		t.Fatal(err)
	}
	r, err = Run(req, b)
	if err != nil || r.ExitCode() != 1 || len(r.DisabledRules) != 1 || r.DisabledRules[0] != "generated-id" || a.digest == b.digest || a.profileDigest != b.profileDigest {
		t.Fatal("explicit disabling failed", err, r.Counts)
	}
	for _, ids := range [][]string{{"unknown"}, {"generated-id", "generated-id"}} {
		p.DisabledRules = ids
		if _, err := Compile(p, profile); err == nil {
			t.Fatal("invalid disabled_rules accepted")
		}
	}
	p.DisabledRules = []string{"generated-id"}
	p.Ignores = []Ignore{{Rule: Rule{ID: "replacement", Replaces: "generated-id"}, Selector: Selector{Type: "THING", Path: "/ref/id"}}}
	if _, err := Compile(p, profile); err == nil {
		t.Fatal("disabled rule also replaced")
	}
}
func TestVariantSelectorsDoNotExpandInactiveAssertions(t *testing.T) {
	p := testPolicy()
	p.Expectations = []Expectation{{Rule: testRule("assertion"), Selector: Selector{Type: "THING", Variant: "kind-a", Path: "/absent/deep"}, Operation: "literal", Expected: present("required"), Authority: "approved"}}
	req := Request{Mode: "drift", Left: variantInput("dev", "kind-b", "same"), Right: variantInput("prod", "kind-b", "same")}
	if r := runTest(t, req, p); r.ExitCode() != 0 || r.RuleUsage["assertion"] != 0 {
		t.Fatal("inactive assertion expanded", r.Counts)
	}
}
