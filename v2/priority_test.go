package v2

import "testing"

func TestRelativeOrderPolicyValidation(t *testing.T) {
	base := func() Policy {
		p := testPolicy()
		p.Equivalences = []Equivalence{{Rule: testRule("order"), Selector: Selector{Type: "THING", Path: "/priority"}, Operation: "relative_order"}}
		return p
	}
	if _, err := Compile(base()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Equivalence){
		func(r *Equivalence) { r.Operation = "unknown" },
		func(r *Equivalence) { r.Path = "/*" },
		func(r *Equivalence) { r.Path = "/name" },
		func(r *Equivalence) { r.Key = "One" },
		func(r *Equivalence) { r.Space = "metadata" },
		func(r *Equivalence) { r.Type = "UNKNOWN" },
		func(r *Equivalence) { r.Modes = []string{"deployment"} },
		func(r *Equivalence) { r.LeftEnv = "dev" },
		func(r *Equivalence) { r.Pairs = []ValuePair{{Left: 1, Right: 2}} },
	} {
		p := base()
		mutate(&p.Equivalences[0])
		if _, err := Compile(p); err == nil {
			t.Fatal("invalid relative ordering accepted", p.Equivalences)
		}
	}
	p := base()
	p.Expectations = []Expectation{{Rule: testRule("expect"), Selector: Selector{Type: "THING", Path: "/priority"}, Operation: "absent", Authority: "approved"}}
	if _, err := Compile(p); err == nil {
		t.Fatal("competing priority rule accepted")
	}
	// Ordinary value equivalences still require explicit environment context.
	if _, err := Decode([]byte(`{"version":2,"id":"p","equivalences":[{"id":"e","type":"THING","path":"/priority","pairs":[{"left":1,"right":2}]}]}`)); err == nil {
		t.Fatal("ordinary equivalence context became optional")
	}
	if _, err := Decode([]byte(`{"version":2,"id":"p","equivalences":[{"id":"e","type":"THING","path":"/priority","operation":"relative_order"}]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestCollectionPriorityPolicyValidation(t *testing.T) {
	base := func() Policy {
		p := testPolicy()
		p.Collections = []Collection{{Rule: testRule("items"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "keyed", Keys: []string{"/name"}, PriorityOrder: &PriorityOrder{Type: "CONTEXT", ReferencePath: "/context/id", MatchPath: "/parent/id", PriorityPath: "/priority"}}}
		return p
	}
	if _, err := Compile(base()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Collection){
		func(r *Collection) { r.Mode = "ordered"; r.Keys = nil },
		func(r *Collection) { r.PriorityOrder.Type = "" },
		func(r *Collection) { r.PriorityOrder.ReferencePath = "" },
		func(r *Collection) { r.PriorityOrder.MatchPath = "/*" },
		func(r *Collection) { r.PriorityOrder.PriorityPath = "/priority/*" },
	} {
		p := base()
		mutate(&p.Collections[0])
		if _, err := Compile(p); err == nil {
			t.Fatal("invalid contextual ordering accepted")
		}
	}
}
