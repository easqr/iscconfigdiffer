package v2

import (
	"encoding/json"
	"strings"
	"testing"
)

func rawItemScope(collection, path, value string) CollectionItemSelector {
	raw, _ := json.Marshal(value)
	return CollectionItemSelector{Collection: collection, Keys: map[string]CollectionItemKey{path: {Value: raw}}}
}
func nestedItemsPolicy() Policy {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("parents"), Selector: Selector{Type: "THING", Path: "/parents"}, Mode: "keyed", Keys: []string{"/kind"}}, {Rule: testRule("fields"), Selector: Selector{Type: "THING", Path: "/parents/*/fields"}, Mode: "keyed", Keys: []string{"/name"}}}
	p.Equivalences = []Equivalence{{Rule: testRule("login-template"), Selector: Selector{Type: "THING", Path: "/parents/*/fields/*/value"}, Items: []CollectionItemSelector{rawItemScope("parents", "/kind", "new"), rawItemScope("fields", "/name", "login")}, LeftEnv: "dev", RightEnv: "prod", Groups: []map[string]any{{"dev": "dev-template", "prod": "prod-template"}}}}
	return p
}
func nestedItemsInput(env string, reverse bool, otherApprovals bool) Input {
	parents := []any{}
	for _, kind := range []string{"new", "modify"} {
		fields := []any{}
		for _, name := range []string{"login", "email"} {
			value := "same"
			if kind == "new" && name == "login" || otherApprovals {
				value = env + "-template"
			}
			fields = append(fields, map[string]any{"name": name, "value": value})
		}
		if reverse {
			fields[0], fields[1] = fields[1], fields[0]
		}
		parents = append(parents, map[string]any{"kind": kind, "fields": fields})
	}
	if reverse {
		parents[0], parents[1] = parents[1], parents[0]
	}
	return testInput(env, doc(map[string]any{"parents": parents}))
}
func TestConjunctiveItemsAndOrderIndependentInputs(t *testing.T) {
	p := nestedItemsPolicy()
	req := Request{Mode: "drift", Left: nestedItemsInput("dev", false, false), Right: nestedItemsInput("prod", true, false)}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 {
		t.Fatal("nested approval did not apply", r.Counts)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID != "login-template" {
			continue
		}
		found = true
		if d.Item != nil || len(d.Items) != 2 || d.Items[0].Collection != "parents" || d.Items[1].Collection != "fields" || d.Locations["left"].Pointer != "/parents/0/fields/0/value" || d.Locations["right"].Pointer != "/parents/1/fields/1/value" {
			t.Fatal("lost joint scope or original pointers", d)
		}
	}
	if !found {
		t.Fatal("joint scope decision missing")
	}
	req.Left = nestedItemsInput("dev", false, true)
	req.Right = nestedItemsInput("prod", true, true)
	r = runTest(t, req, p)
	if r.ExitCode() != 1 || r.Counts["equivalent"] != 1 || r.Counts["different"] != 3 {
		t.Fatal("approval escaped parent/field constraints", r.Counts)
	}
	// The ancestor identities remain meaningful when input environments reverse.
	req.Left, req.Right = req.Right, req.Left
	r = runTest(t, req, p)
	if r.Counts["equivalent"] != 1 || r.Counts["different"] != 3 {
		t.Fatal("reversed inputs changed scope", r.Counts)
	}
}
func TestJointScopeOwnershipAndValidation(t *testing.T) {
	for name, change := range map[string]func(*Policy){
		"both forms":              func(p *Policy) { item := p.Equivalences[0].Items[0]; p.Equivalences[0].Item = &item },
		"empty items":             func(p *Policy) { p.Equivalences[0].Items = []CollectionItemSelector{} },
		"duplicate collection":    func(p *Policy) { p.Equivalences[0].Items = append(p.Equivalences[0].Items, p.Equivalences[0].Items[0]) },
		"unknown collection":      func(p *Policy) { p.Equivalences[0].Items[1].Collection = "missing" },
		"missing tuple component": func(p *Policy) { p.Equivalences[0].Items[1].Keys = map[string]CollectionItemKey{} },
		"overlapping narrower rule": func(p *Policy) {
			r := p.Equivalences[0]
			r.ID = "broad"
			item := r.Items[0]
			r.Items = nil
			r.Item = &item
			p.Equivalences = append(p.Equivalences, r)
		},
		"duplicate reordered conditions": func(p *Policy) {
			r := p.Equivalences[0]
			r.ID = "duplicate"
			r.Items = []CollectionItemSelector{r.Items[1], r.Items[0]}
			p.Equivalences = append(p.Equivalences, r)
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := nestedItemsPolicy()
			change(&p)
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid joint scopes accepted")
			}
		})
	}
	// Sharing one constraint does not cause conflict if another constraint is disjoint.
	p := nestedItemsPolicy()
	for _, scope := range [][]CollectionItemSelector{{rawItemScope("parents", "/kind", "modify"), rawItemScope("fields", "/name", "login")}, {rawItemScope("parents", "/kind", "new"), rawItemScope("fields", "/name", "email")}} {
		r := p.Equivalences[0]
		r.ID = []string{"modify-login", "new-email"}[len(p.Equivalences)-1]
		r.Items = scope
		p.Equivalences = append(p.Equivalences, r)
	}
	r := runTest(t, Request{Mode: "drift", Left: nestedItemsInput("dev", false, true), Right: nestedItemsInput("prod", true, true)}, p)
	if r.Counts["equivalent"] != 3 || r.Counts["different"] != 1 {
		t.Fatal("disjoint joint rules rejected or over-applied", r.Counts)
	}
	// References to incompatible sibling collections cannot form a joint ancestor chain.
	p = nestedItemsPolicy()
	p.Collections = append(p.Collections, Collection{Rule: testRule("alternate"), Selector: Selector{Type: "THING", Path: "/elsewhere"}, Mode: "keyed", Keys: []string{"/name"}})
	p.Equivalences[0].Items = append(p.Equivalences[0].Items, rawItemScope("alternate", "/name", "login"))
	if _, err := Compile(p); err == nil {
		t.Fatal("non-ancestor joint reference accepted")
	}
}
func TestJointScopeDigestsDetachmentAndRedaction(t *testing.T) {
	p := nestedItemsPolicy()
	before := stable(p)
	a := compileTest(t, p)
	if stable(p) != before {
		t.Fatal("compiler changed caller scopes")
	}
	p.Equivalences[0].Items[0], p.Equivalences[0].Items[1] = p.Equivalences[0].Items[1], p.Equivalences[0].Items[0]
	b := compileTest(t, p)
	if a.digest != b.digest {
		t.Fatal("AND condition order changed digest")
	}
	req := Request{Mode: "drift", ExcludeValues: true, Left: nestedItemsInput("dev", false, false), Right: nestedItemsInput("prod", true, false)}
	r, err := Run(req, a)
	if err != nil || r.ExitCode() != 0 {
		t.Fatal(err, r.Counts)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "dev-template") || !strings.Contains(string(data), `"items"`) {
		t.Fatal("joint scope/value omission changed")
	}
	p.Equivalences[0].Items[0].Keys["/name"] = CollectionItemKey{Value: json.RawMessage(`"caller-mutation"`)}
	results := make(chan bool, 4)
	for range 4 {
		go func() { again, err := Run(req, a); results <- err == nil && stable(r) == stable(again) }()
	}
	for range 4 {
		if !<-results {
			t.Fatal("compiled joint scopes shared mutable state")
		}
	}
	p = nestedItemsPolicy()
	p.Sensitive = []Sensitive{{Rule: testRule("hide"), Selector: Selector{Type: "THING", Path: "/parents"}}}
	r = runTest(t, req, p)
	for _, d := range r.Decisions {
		if len(d.Items) != 0 || d.Item != nil {
			t.Fatal("sensitive joint scope retained")
		}
	}
}
