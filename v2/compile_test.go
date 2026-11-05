package v2

import (
	"encoding/json"
	"strings"
	"testing"
)

func testRule(id string) Rule { return Rule{ID: id, Evidence: &Evidence{Kind: "invented-policy"}} }
func testPolicy() Policy {
	return Policy{Version: 2, ID: "test", Identities: []Identity{{Rule: testRule("identity"), Type: "THING", Strategy: "fields", Paths: []string{"/name"}}}}
}
func compileTest(t *testing.T, p Policy) *Compiled {
	t.Helper()
	c, e := Compile(p)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func present(v any) *State { b, _ := json.Marshal(v); return &State{State: "present", Value: b} }
func TestStrictCompiler(t *testing.T) {
	for _, s := range []string{
		`{"version":2,"id":"p","id":"q"}`,
		`{"version":2,"ID":"p"}`,
		`{"version":2,"id":"p","unknown":true}`,
		`{"version":2,"id":"p","expectations":[{"id":"r","type":"THING","path":"/x","operation":"literal","authority":"a","expected":{"state":"present","value":{"x":1,"x":2}}}]}`,
		`{"version":2,"id":"p","identities":null}`,
		`{"version":2,"id":"p","expectations":[{"id":"r","type":"THING","operation":"absent","authority":"a"}]}`,
	} {
		if _, e := Decode([]byte(s)); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	p := testPolicy()
	p.Ignores = []Ignore{{Rule: testRule("hide"), Selector: Selector{Type: "THING", Path: "/name"}}}
	if _, e := Compile(p); e == nil {
		t.Fatal("identity hidden")
	}
	p = testPolicy()
	p.Expectations = []Expectation{{Rule: testRule("a"), Selector: Selector{Type: "THING", Path: "/x"}, Operation: "literal", Expected: present(nil), Authority: "approved"}, {Rule: testRule("b"), Selector: Selector{Type: "THING", Path: "/x/y"}, Operation: "absent", Authority: "approved"}}
	if _, e := Compile(p); e == nil {
		t.Fatal("overlap accepted")
	}
	p = testPolicy()
	p.Identities = append(p.Identities, Identity{Rule: testRule("dependent"), Type: "CHILD", Strategy: "dependent", ParentType: "CHILD", ParentPath: "/parent"})
	if _, e := Compile(p); e == nil {
		t.Fatal("cycle accepted")
	}
}
func TestProfilesReplacementsAndDigest(t *testing.T) {
	profile := Profile{ID: "generic-v2", Rules: Policy{Identities: testPolicy().Identities, Ignores: []Ignore{{Rule: testRule("metadata"), Selector: Selector{Type: "THING", Space: "metadata", Path: "/generated"}}}}}
	p := Policy{Version: 2, ID: "p", Profile: profile.ID}
	p.Ignores = []Ignore{{Rule: Rule{ID: "custom", Replaces: "metadata", Modes: []string{"drift"}}, Selector: Selector{Type: "THING", Space: "metadata", Path: "/generated"}}}
	c, e := Compile(p, profile)
	if e != nil {
		t.Fatal(e)
	}
	if c.replacements["metadata"] != "custom" {
		t.Fatal(c)
	}
	p.Ignores[0].Path = "/other"
	if _, e = Compile(p, profile); e == nil {
		t.Fatal("ownership changed")
	}
	p = testPolicy()
	p.Ignores = []Ignore{{Rule: testRule("z"), Selector: Selector{Type: "THING", Path: "/z"}}, {Rule: testRule("a"), Selector: Selector{Type: "THING", Path: "/a"}}}
	a := compileTest(t, p)
	p.Ignores[0], p.Ignores[1] = p.Ignores[1], p.Ignores[0]
	b := compileTest(t, p)
	if a.Digest() != b.Digest() {
		t.Fatal("rule ordering changed digest")
	}
	p.Equivalences = []Equivalence{{Rule: testRule("equiv"), Selector: Selector{Type: "THING", Path: "/host"}, LeftEnv: "dev", RightEnv: "prod", Pairs: []ValuePair{{Left: json.Number("1"), Right: json.Number("2")}}}}
	a = compileTest(t, p)
	p.Equivalences[0].Pairs[0].Left = json.Number("1.0")
	b = compileTest(t, p)
	if a.Digest() != b.Digest() {
		t.Fatal("numeric semantic digest")
	}
	p.Expectations = []Expectation{{Rule: testRule("absent"), Selector: Selector{Type: "THING", Path: "/none"}, Operation: "literal", Expected: &State{State: "absent", Value: json.RawMessage(`null`)}, Authority: "approved"}}
	if _, e = Compile(p); e == nil || !strings.Contains(e.Error(), "absent") {
		t.Fatal(e)
	}
}
