package v2

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func schemaExclusionPolicy(t *testing.T) Policy {
	t.Helper()
	p, err := Decode([]byte(`{
 "version":2,"id":"schema-exclusion",
 "identities":[{"id":"source","type":"SOURCE","strategy":"fields","paths":["/name"]}],
 "collections":[
  {"id":"schemas","type":"SOURCE","path":"/schemas","mode":"keyed","keys":["/name"]},
  {"id":"attributes","type":"SOURCE","path":"/schemas/*/attributes","mode":"keyed","keys":["/name"]}
 ],
 "ignores":[{"id":"schema-id","type":"SOURCE","path":"/schemas/*/id"}],
 "exceptions":[{"id":"excluded-schema","type":"SOURCE","key":"Entra","path":"/schemas/*","operation":"exclude","item":{"collection":"schemas","keys":{"/name":{"value":"production-only"}}},"rationale":"Production-only schema"}]
 }`))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func sourceSchemas(env, source string, schemas ...any) Input {
	return Input{Environment: env, Types: []string{"SOURCE"}, Documents: []Document{{Type: "SOURCE", ID: env + "-source", Path: "source.json", Content: map[string]any{"name": source, "schemas": schemas}, Metadata: map[string]any{}}}}
}
func schema(name, description string) any {
	return map[string]any{"name": name, "description": description, "attributes": []any{map[string]any{"name": "id", "type": "STRING"}}}
}
func TestExceptionExcludesEntireNamedSchema(t *testing.T) {
	for _, presence := range []string{"right-only", "left-only", "both", "neither"} {
		t.Run(presence, func(t *testing.T) {
			p := schemaExclusionPolicy(t)
			shared := schema("account", "same")
			left, right := []any{shared}, []any{shared}
			if presence == "left-only" || presence == "both" {
				left = append(left, schema("production-only", "left settings"))
			}
			if presence == "right-only" || presence == "both" {
				right = append([]any{schema("production-only", "different right settings")}, right...)
			}
			req := Request{Mode: "drift", Left: sourceSchemas("dev", "Entra", left...), Right: sourceSchemas("prod", "Entra", right...)}
			before := stable(req)
			r := runTest(t, req, p)
			want := 1
			if presence == "neither" {
				want = 0
			}
			if r.ExitCode() != 0 || r.Counts["exception"] != want || r.RuleUsage["excluded-schema"] != want || stable(req) != before {
				t.Fatal(r)
			}
			for _, d := range r.Decisions {
				if d.RuleID != "excluded-schema" {
					continue
				}
				if d.Rationale != "Production-only schema" || d.Code != "explicit exception exclusion" || d.Item == nil {
					t.Fatal(d)
				}
				if presence == "both" && (d.Locations["left"].Pointer != "/schemas/1" || d.Locations["right"].Pointer != "/schemas/0") {
					t.Fatal("lost physical positions", d)
				}
				var md, html bytes.Buffer
				if err := r.Markdown(&md); err != nil {
					t.Fatal(err)
				}
				if err := r.HTML(&html); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(md.String(), "Production-only schema") || !strings.Contains(html.String(), "explicit exception exclusion") {
					t.Fatal("missing exclusion rationale")
				}
			}
		})
	}
}
func TestSchemaExclusionDoesNotHideOtherDifferences(t *testing.T) {
	p := schemaExclusionPolicy(t)
	req := Request{Mode: "drift", Left: sourceSchemas("dev", "Entra", schema("account", "left")), Right: sourceSchemas("prod", "Entra", schema("production-only", "anything"), schema("account", "right")), ExcludeValues: true}
	r := runTest(t, req, p)
	if r.ExitCode() != 1 || r.Counts["exception"] != 1 || r.Counts["different"] != 1 {
		t.Fatal(r)
	}
	req.Left = sourceSchemas("dev", "Other Source", schema("account", "same"))
	req.Right = sourceSchemas("prod", "Other Source", schema("production-only", "anything"), schema("account", "same"))
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["exception"] != 0 {
		t.Fatal(r)
	}
	req.Left = sourceSchemas("dev", "Entra", schema("account", "same"))
	req.Right = sourceSchemas("prod", "Entra", schema("production-only", "anything"), schema("account", "same"))
	p.Exceptions[0].LeftEnv = "different"
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["exception"] != 0 {
		t.Fatal(r)
	}
	p.Exceptions[0].LeftEnv = "dev"
	p.Exceptions[0].RightEnv = "prod"
	p.Exceptions[0].RightTenant = "expected"
	if r = runTest(t, req, p); r.ExitCode() != 2 || r.Counts["exception"] != 0 {
		t.Fatal(r)
	}
}
func TestSchemaExclusionValidation(t *testing.T) {
	for _, change := range []string{"pairs", "groups", "states", "deployment", "identity", "rationale", "equivalence", "competing-rule"} {
		t.Run(change, func(t *testing.T) {
			p := schemaExclusionPolicy(t)
			switch change {
			case "pairs":
				p.Exceptions[0].Pairs = []ValuePair{{Left: 1, Right: 2}}
			case "groups":
				p.Exceptions[0].Groups = []map[string]any{{"dev": 1, "prod": 2}}
			case "states":
				p.Exceptions[0].StatePairs = []StatePair{{Left: State{State: "absent"}, Right: *present(1)}}
			case "deployment":
				p.Exceptions[0].Modes = []string{"deployment"}
			case "identity":
				p.Exceptions[0].Path = "/name"
				p.Exceptions[0].Item = nil
			case "rationale":
				p.Exceptions[0].Rationale = " "
			case "equivalence":
				p.Equivalences = []Equivalence{p.Exceptions[0].Equivalence}
				p.Exceptions = nil
			case "competing-rule":
				p.Expectations = []Expectation{{Rule: testRule("assertion"), Selector: Selector{Type: "SOURCE", Path: "/schemas/*/description"}, Operation: "literal", Expected: present("required"), Authority: "release"}}
			}
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid exclusion accepted")
			}
		})
	}
	p := schemaExclusionPolicy(t)
	data, _ := json.Marshal(p)
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	delete(body["exceptions"].([]any)[0].(map[string]any), "rationale")
	data, _ = json.Marshal(body)
	if _, err := Decode(data); err == nil {
		t.Fatal("missing rationale accepted")
	}
	// Disjoint named schema exclusions share the wildcard path without conflict.
	p = schemaExclusionPolicy(t)
	e := p.Exceptions[0]
	e.ID = "second-schema"
	e.Item = &CollectionItemSelector{Collection: "schemas", Keys: map[string]CollectionItemKey{"/name": {Value: json.RawMessage(`"another-schema"`)}}}
	p.Exceptions = append(p.Exceptions, e)
	req := Request{Mode: "drift", Left: sourceSchemas("dev", "Entra"), Right: sourceSchemas("prod", "Entra", schema("production-only", "one"), schema("another-schema", "two"))}
	// Use an empty JSON array, rather than a null schemas property.
	req.Left.Documents[0].Content.(map[string]any)["schemas"] = []any{}
	if r := runTest(t, req, p); r.ExitCode() != 0 || r.Counts["exception"] != 2 {
		t.Fatal(r)
	}
}

func TestExceptionExcludesCollectionAtSamePath(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{
		{Rule: testRule("attributes"), Selector: Selector{Type: "THING", Path: "/attributes"}, Mode: "keyed", Keys: []string{"/name"}},
		{Rule: testRule("sources"), Selector: Selector{Type: "THING", Path: "/attributes/*/sources"}, Mode: "keyed", Keys: []string{"/name"}},
	}
	p.Exceptions = []Exception{{Equivalence: Equivalence{Rule: testRule("manager-sources"), Selector: Selector{Type: "THING", Path: "/attributes/*/sources"}, Operation: "exclude", Item: &CollectionItemSelector{Collection: "attributes", Keys: map[string]CollectionItemKey{"/name": {Value: json.RawMessage(`"manager"`)}}}}, Rationale: "priority needs to be investigated and aligned"}}
	attribute := func(name, value string) any {
		return map[string]any{"name": name, "sources": []any{map[string]any{"name": "source", "value": value}}}
	}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"attributes": []any{attribute("manager", "left"), attribute("email", "same")}})), Right: testInput("prod", doc(map[string]any{"attributes": []any{attribute("email", "same"), attribute("manager", "right")}}))}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["exception"] != 1 || r.RuleUsage["sources"] != 1 {
		t.Fatal(r)
	}
	req.Right = testInput("prod", doc(map[string]any{"attributes": []any{attribute("email", "changed"), attribute("manager", "right")}}))
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["exception"] != 1 || r.Counts["different"] != 1 {
		t.Fatal(r)
	}
	// Same-path value rules still conflict; the exception is not a general override.
	p.Ignores = []Ignore{{Rule: testRule("hide"), Selector: p.Exceptions[0].Selector}}
	if _, err := Compile(p); err == nil {
		t.Fatal("same-path ignore/exclusion accepted")
	}
}
