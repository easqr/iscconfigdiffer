package v2

import (
	"strings"
	"testing"
)

func deploymentPolicy() Policy {
	p := testPolicy()
	p.Expectations = []Expectation{{Rule: testRule("name"), Selector: Selector{Type: "THING", Path: "/name"}, Operation: "copy_source", Authority: "approved"}, {Rule: testRule("metadata"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"}}
	return p
}
func TestReviewLiteralPathsBeyondArrayInventory(t *testing.T) {
	p := testPolicy()
	p.Expectations = []Expectation{{Rule: testRule("expected"), Selector: Selector{Type: "THING", Path: "/items/1000000000/value"}, Operation: "literal", Expected: present("required"), Authority: "approved"}}
	in := testInput("e", doc(map[string]any{"items": []any{}}))
	r := runTest(t, Request{Mode: "drift", Left: in, Right: in}, p)
	if r.ExitCode() != 1 || r.Counts["violation"] != 2 {
		t.Fatalf("literal missing paths were skipped: %+v", r)
	}
}
func TestReviewBaselineOnlyWildcardPreservation(t *testing.T) {
	p := deploymentPolicy()
	p.Expectations = append(p.Expectations, Expectation{Rule: testRule("preserve"), Selector: Selector{Type: "THING", Path: "/items/*/value"}, Operation: "preserve_target", Authority: "approved"})
	in := testInput("e", doc(nil))
	base := testInput("e", doc(map[string]any{"items": []any{map[string]any{"value": "required"}}}))
	r := runTest(t, Request{Mode: "deployment", Left: in, Right: in, Baseline: &base, Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}, p)
	if r.ExitCode() == 0 || r.Counts["violation"] == 0 {
		t.Fatalf("baseline-only wildcard skipped: %+v", r)
	}
}
func TestReviewBaselineRenamePairing(t *testing.T) {
	p := deploymentPolicy()
	p.Pairings = []Pairing{{Rule: testRule("pair"), Type: "THING", LeftID: "sourcea", RightID: "actuala"}}
	p.Expectations[0].Operation = "literal"
	p.Expectations[0].Expected = present("renamed")
	left := testInput("source", doc(nil))
	right := testInput("actual", map[string]any{"name": "renamed"})
	base := testInput("baseline", map[string]any{"name": "renamed"})
	r := runTest(t, Request{Mode: "deployment", Left: left, Right: right, Baseline: &base, Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}, p)
	if r.ExitCode() != 0 {
		t.Fatalf("baseline exact draft name was not matched: %+v", r)
	}
}
func TestReviewOrderedCollectionAllowsLiteralItemSelector(t *testing.T) {
	p := testPolicy()
	p.Collections = []Collection{{Rule: testRule("ordered"), Selector: Selector{Type: "THING", Path: "/items"}, Mode: "ordered"}}
	p.Expectations = []Expectation{{Rule: testRule("item"), Selector: Selector{Type: "THING", Path: "/items/0"}, Operation: "literal", Expected: present("x"), Authority: "approved"}}
	if _, err := Compile(p); err != nil {
		t.Fatal("ordered array literal selector rejected", err)
	}
}
func TestReviewEquivalentReplacementSelectorSpace(t *testing.T) {
	profile := Profile{ID: "profile", Rules: Policy{Identities: testPolicy().Identities, Ignores: []Ignore{{Rule: testRule("original"), Selector: Selector{Type: "THING", Space: "content", Path: "/generated"}}}}}
	p := Policy{Version: 2, ID: "p", Profile: "profile", Ignores: []Ignore{{Rule: Rule{ID: "replacement", Replaces: "original"}, Selector: Selector{Type: "THING", Path: "/generated"}}}}
	if _, err := Compile(p, profile); err != nil {
		t.Fatal("equivalent default content space rejected", err)
	}
}

func TestReviewCannotReferenceExplicitlyDeletedTarget(t *testing.T) {
	p := deploymentPolicy()
	p.Identities = append(p.Identities, Identity{Rule: testRule("target-identity"), Type: "TARGET", Strategy: "fields", Paths: []string{"/name"}})
	p.Expectations = append(p.Expectations, Expectation{Rule: testRule("ref"), Selector: Selector{Type: "THING", Path: "/ref"}, Operation: "reference_target", TargetType: "TARGET", Authority: "approved"})
	left := testInput("source", doc(map[string]any{"ref": "source-target"}))
	left.Types = append(left.Types, "TARGET")
	left.Documents = append(left.Documents, Document{Type: "TARGET", ID: "source-target", Content: map[string]any{"name": "target"}})
	right := testInput("actual", doc(map[string]any{"ref": "baseline-target"}))
	right.Types = append(right.Types, "TARGET")
	base := right
	base.Documents = append(append([]Document{}, right.Documents...), Document{Type: "TARGET", ID: "baseline-target", Content: map[string]any{"name": "target"}})
	r := runTest(t, Request{Mode: "deployment", Left: left, Right: right, Baseline: &base, Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}, {Type: "TARGET", Key: "target", Action: "delete"}}}, p)
	if r.ExitCode() != 1 || r.Counts["violation"] == 0 {
		t.Fatal("reference to deleted target passed", r)
	}
}

func TestReviewRequiredIdentityIsUsedForExplicitPair(t *testing.T) {
	p := testPolicy()
	p.Identities[0].Required = true
	p.Pairings = []Pairing{{Rule: testRule("pair"), Type: "THING", LeftID: "la", RightID: "ra"}}
	r := runTest(t, Request{Mode: "drift", Left: testInput("l", doc(nil)), Right: testInput("r", doc(nil))}, p)
	if r.ExitCode() != 0 || r.RuleUsage["identity"] == 0 {
		t.Fatal(r)
	}
}

func TestReviewRequestDigestMeasuresDocuments(t *testing.T) {
	p := testPolicy()
	in := testInput("e", doc(nil))
	in.ContentHash = "upstream-artifact-digest"
	a := runTest(t, Request{Mode: "drift", Left: in, Right: in}, p)
	in.Documents[0].Content = doc(map[string]any{"changed": true})
	b := runTest(t, Request{Mode: "drift", Left: in, Right: in}, p)
	if a.RequestDigest == b.RequestDigest {
		t.Fatal("comparison digest trusted stale supplied content hash")
	}
}

func TestReviewDependentDeleteWithoutDraftParent(t *testing.T) {
	p := testPolicy()
	p.Identities = append(p.Identities, Identity{Rule: testRule("child"), Type: "CHILD", Strategy: "dependent", ParentType: "THING", ParentPath: "/parent", Paths: []string{"/technical"}})
	in := testInput("e")
	in.Types = append(in.Types, "CHILD")
	base := testInput("baseline", doc(nil))
	base.Types = append(base.Types, "CHILD")
	base.Documents = append(base.Documents, Document{Type: "CHILD", ID: "child", Content: map[string]any{"parent": "baselinea", "technical": "active"}})
	r := runTest(t, Request{Mode: "deployment", Left: in, Right: in, Baseline: &base, Operations: []Operation{{Type: "THING", Key: "invented", Action: "delete"}, {Type: "CHILD", Key: `["invented","active"]`, Action: "delete"}}}, p)
	if r.ExitCode() != 0 {
		t.Fatal(r)
	}
}

func TestReviewExplicitRootDerivation(t *testing.T) {
	p := deploymentPolicy()
	root := ""
	p.Expectations = append(p.Expectations, Expectation{Rule: testRule("root-copy"), Selector: Selector{Type: "THING", Path: "/copy"}, Operation: "copy_source", From: &root, Authority: "approved"})
	left := testInput("source", doc(nil))
	right := testInput("actual", doc(map[string]any{"copy": doc(nil)}))
	r := runTest(t, Request{Mode: "deployment", Left: left, Right: right, Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}, p)
	if r.ExitCode() != 0 {
		t.Fatal("explicit empty source pointer did not select root", r)
	}
}

func TestReviewSensitiveMissingOperationKey(t *testing.T) {
	p := testPolicy()
	p.Sensitive = []Sensitive{{Rule: testRule("sensitive"), Selector: Selector{Type: "THING", Path: "/name"}}}
	r := runTest(t, Request{Mode: "deployment", Left: testInput("e"), Right: testInput("e"), Operations: []Operation{{Type: "THING", Key: "private-operation-name", Action: "update"}}}, p)
	if strings.Contains(stable(r), "private-operation-name") {
		t.Fatal("missing operation disclosed sensitive key")
	}
}

func TestReviewRequiredSensitiveSelectorMustExist(t *testing.T) {
	p := testPolicy()
	p.Sensitive = []Sensitive{{Rule: Rule{ID: "sensitive", Required: true}, Selector: Selector{Type: "THING", Path: "/missing/*/secret"}}}
	in := testInput("e", doc(nil))
	r := runTest(t, Request{Mode: "drift", Left: in, Right: in}, p)
	if r.ExitCode() != 2 {
		t.Fatal("unmatched required sensitive selector passed", r)
	}
}

func TestReviewBaselineShapeDoesNotInvalidateIndependentCopy(t *testing.T) {
	p := deploymentPolicy()
	p.Expectations = append(p.Expectations, Expectation{Rule: testRule("values"), Selector: Selector{Type: "THING", Path: "/items/*"}, Operation: "copy_source", Authority: "approved"})
	in := testInput("e", doc(map[string]any{"items": []any{"x"}}))
	base := testInput("e", doc(map[string]any{"items": "old-shape"}))
	r := runTest(t, Request{Mode: "deployment", Left: in, Right: in, Baseline: &base, Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}, p)
	if r.ExitCode() != 0 {
		t.Fatal(r)
	}
}

func TestReviewProfileCannotSilentlyIgnoreBindings(t *testing.T) {
	profile := Profile{ID: "profile", Rules: Policy{Bindings: map[string]string{"left": "prod"}, Identities: testPolicy().Identities}}
	p := Policy{Version: 2, ID: "p", Profile: "profile"}
	if _, err := Compile(p, profile); err == nil {
		t.Fatal("profile environment binding silently ignored")
	}
}

func TestReviewErrorReportCannotMutateCompiledEvidence(t *testing.T) {
	p := testPolicy()
	p.Evidence = []Evidence{{Kind: "approval", Attributes: map[string]string{"original": "unchanged"}}}
	c := compileTest(t, p)
	in := testInput("e", doc(nil))
	r, err := Run(Request{Mode: "deployment", Left: in, Right: in, Operations: []Operation{{Type: "THING", Key: "invented", Action: "invalid"}}}, c)
	if err == nil {
		t.Fatal("invalid request accepted")
	}
	r.Evidence[0].Attributes["original"] = "changed"
	next, err := Run(Request{Mode: "drift", Left: in, Right: in}, c)
	if err != nil || next.Evidence[0].Attributes["original"] != "unchanged" {
		t.Fatal("error report changed compiled plan", err, next)
	}
}

func TestReviewRootModesLimitConflicts(t *testing.T) {
	p := testPolicy()
	p.Modes = []string{"deployment"}
	p.Equivalences = []Equivalence{{Rule: testRule("equiv"), Selector: Selector{Type: "THING", Path: "/host"}, LeftEnv: "dev", RightEnv: "prod", Pairs: []ValuePair{{Left: "dev", Right: "prod"}}}}
	p.Expectations = []Expectation{{Rule: testRule("host"), Selector: Selector{Type: "THING", Path: "/host"}, Operation: "literal", Expected: present("prod"), Authority: "approved"}}
	compileTest(t, p)
}
