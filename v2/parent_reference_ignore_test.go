package v2

import "testing"

func TestIgnoreExactPhysicalParentReference(t *testing.T) {
	policy := func() Policy {
		p := testPolicy()
		p.Identities = append(p.Identities, Identity{Rule: testRule("child-identity"), Type: "CHILD", Strategy: "dependent", ParentType: "THING", ParentPath: "/parent/id", Paths: []string{"/code"}, ComponentType: "string"})
		return p
	}
	p := policy()
	p.Ignores = []Ignore{{Rule: testRule("parent-id"), Selector: Selector{Type: "CHILD", Path: "/parent/id"}}}
	if _, err := Compile(p); err != nil {
		t.Fatal("exact physical parent reference cannot be excluded", err)
	}
	for _, path := range []string{"", "/parent", "/parent/*", "/parent/id/child", "/code"} {
		p = policy()
		p.Ignores = []Ignore{{Rule: testRule("unsafe"), Selector: Selector{Type: "CHILD", Path: path}}}
		if _, err := Compile(p); err == nil {
			t.Fatal("broad or logical identity exclusion accepted", path)
		}
	}
	p = policy()
	p.Identities[1].Paths = append(p.Identities[1].Paths, "/parent/id")
	p.Ignores = []Ignore{{Rule: testRule("logical-id"), Selector: Selector{Type: "CHILD", Path: "/parent/id"}}}
	if _, err := Compile(p); err == nil {
		t.Fatal("reference also used as logical component was hidden")
	}
	p = policy()
	p.Ignores = []Ignore{{Rule: testRule("primary-name"), Selector: Selector{Type: "THING", Path: "/name"}}}
	if _, err := Compile(p); err == nil {
		t.Fatal("primary logical identity was hidden")
	}
	p = policy()
	p.Equivalences = []Equivalence{{Rule: testRule("physical-equivalence"), Selector: Selector{Type: "CHILD", Path: "/parent/id"}, LeftEnv: "dev", RightEnv: "prod", Pairs: []ValuePair{{Left: "dev", Right: "prod"}}}}
	if _, err := Compile(p); err == nil {
		t.Fatal("identity equivalence protection was relaxed")
	}
}
