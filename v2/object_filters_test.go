package v2

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestPolicyObjectFilters(t *testing.T) {
	p := testPolicy()
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Include: []string{"invented", "other"}, Exclude: []string{"other"}}}
	req := Request{Mode: "drift", Left: testInput("dev", doc(nil), map[string]any{"name": "other", "value": "dev"}, map[string]any{"name": "unlisted", "value": "dev"}), Right: testInput("prod", doc(nil), map[string]any{"name": "other", "value": "prod"}, map[string]any{"name": "unlisted", "value": "prod"})}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || len(r.Matches) != 1 || len(r.ExcludedObjects) != 2 || r.ObjectCoverage["left_excluded_by_policy"] != 2 {
		t.Fatal("excluded objects affected validation", r)
	}
	for _, d := range r.Decisions {
		if d.Key != "invented" {
			t.Fatal("excluded decision", d)
		}
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var out bytes.Buffer
		if err := render(&out); err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"Configurations excluded by policy", "not in object filter include list", "explicitly excluded by object filter", "unlisted"} {
			if !strings.Contains(out.String(), s) {
				t.Fatal("missing filter explanation", s)
			}
		}
	}
	oldDigest := r.PolicyDigest
	p.ObjectFilters = nil
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.PolicyDigest == oldDigest {
		t.Fatal("filter behavior or digest missing", r)
	}
	// Exclusion-only leaves other objects eligible. Names are exact and case-sensitive.
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Exclude: []string{"other"}}}
	if r = runTest(t, req, p); r.ExitCode() != 1 || len(r.Matches) != 2 {
		t.Fatal("blacklist became whitelist", r)
	}
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Include: []string{"Invented"}}}
	r = runTest(t, req, p)
	if r.ExitCode() != 2 {
		t.Fatal("misspelled include passed", r)
	}
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == "included object unavailable" && d.Key == "Invented" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing include diagnostic")
	}
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Exclude: []string{"invented", "other", "unlisted"}}}
	if r = runTest(t, req, p); r.ExitCode() != 2 || len(r.Matches) != 0 {
		t.Fatal("empty selection passed", r)
	}
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Include: []string{"invented"}, Exclude: []string{"unused"}}}
	if r = runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("unused exclusion failed comparison", r)
	}
	// A requested include present on only one side must still report object absence.
	req.Right = testInput("prod", map[string]any{"name": "other"})
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["different"] != 1 {
		t.Fatal("one-sided missing included object hidden", r)
	}
}

func TestObjectFiltersKeepContextAndRespectName(t *testing.T) {
	p := testPolicy()
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Exclude: []string{"target"}}}
	p.References = []Reference{{Rule: testRule("ref"), Selector: Selector{Type: "THING", Path: "/ref"}, TargetType: "THING"}}
	req := Request{Mode: "drift", Scope: []string{"THING"}, Left: testInput("dev", doc(map[string]any{"ref": "devb"}), map[string]any{"name": "target", "value": "dev"}), Right: testInput("prod", doc(map[string]any{"ref": "prodb"}), map[string]any{"name": "target", "value": "prod"})}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 || len(r.ExcludedObjects) != 1 {
		t.Fatal("excluded reference context lost", r)
	}
	req.ObjectKey = "target"
	if r = runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal("name widened policy selection", r)
	}
	req.ObjectKey = "invented"
	p.ObjectFilters["THING"] = ObjectFilter{Include: []string{"invented", "absent"}, Exclude: []string{"target"}}
	if r = runTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("unselected include typo affected narrower name request", r)
	}
}

func TestObjectFiltersDeployment(t *testing.T) {
	p := testPolicy()
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Include: []string{"invented"}}}
	p.Expectations = []Expectation{
		{Rule: testRule("copy"), Selector: Selector{Type: "THING", Path: ""}, Operation: "copy_source", Authority: "approved"},
		{Rule: testRule("metadata"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"},
	}
	req := Request{Mode: "deployment", Scope: []string{"THING"}, Left: testInput("dev", doc(nil), map[string]any{"name": "other", "value": "dev"}), Right: testInput("prod", doc(nil), map[string]any{"name": "other", "value": "prod"}), Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}, {Type: "THING", Key: "other", Action: "update"}, {Type: "THING", Key: "missing-excluded", Action: "delete"}}}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.ObjectCoverage["selected_operations"] != 1 || len(r.Matches) != 1 {
		t.Fatal("excluded operations affected validation", r)
	}
	req.ObjectKey = "other"
	if r = runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal("deployment name widened filters", r)
	}
	// Excluding a referenced scheduled deletion does not authorize using it.
	req.ObjectKey = ""
	req.Left = testInput("dev", doc(map[string]any{"ref": "devb"}), map[string]any{"name": "other"})
	req.Right = testInput("prod", doc(map[string]any{"ref": "prodb"}), map[string]any{"name": "other"})
	req.Operations = []Operation{{Type: "THING", Key: "invented", Action: "update"}, {Type: "THING", Key: "other", Action: "delete"}}
	p.Expectations = []Expectation{{Rule: testRule("name"), Selector: Selector{Type: "THING", Path: "/name"}, Operation: "copy_source", Authority: "approved"}, {Rule: testRule("ref"), Selector: Selector{Type: "THING", Path: "/ref"}, Operation: "reference_target", TargetType: "THING", Authority: "approved"}, {Rule: testRule("meta"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"}}
	if r = runTest(t, req, p); r.ExitCode() != 1 || r.Counts["violation"] != 1 {
		t.Fatal("excluded delete lost safety context", r)
	}
}

func TestObjectFilterValidationDigestAndRedaction(t *testing.T) {
	for _, f := range []ObjectFilter{{}, {Include: []string{""}}, {Exclude: []string{" "}}, {Include: []string{"same", "same"}}, {Exclude: []string{"same", "same"}}, {Include: []string{}, Exclude: []string{"x"}}} {
		p := testPolicy()
		p.ObjectFilters = map[string]ObjectFilter{"THING": f}
		if _, err := Compile(p); err == nil {
			t.Fatal("invalid filters accepted", f)
		}
	}
	p := testPolicy()
	p.ObjectFilters = map[string]ObjectFilter{"UNKNOWN": {Include: []string{"x"}}}
	if _, err := Compile(p); err == nil {
		t.Fatal("unknown type accepted")
	}
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Include: []string{"other", "invented"}, Exclude: []string{"secret"}}}
	before := stable(p)
	c := compileTest(t, p)
	if stable(p) != before {
		t.Fatal("compile mutated authored filter lists")
	}
	p.ObjectFilters["THING"] = ObjectFilter{Include: []string{"invented", "other"}, Exclude: []string{"secret"}}
	if c.Digest() != compileTest(t, p).Digest() {
		t.Fatal("list order changed digest")
	}
	profile := Profile{ID: "generic", Rules: Policy{Identities: testPolicy().Identities, ObjectFilters: p.ObjectFilters}}
	if _, err := Compile(Policy{Version: 2, ID: "test", Profile: "generic"}, profile); err == nil {
		t.Fatal("profile supplied user selection")
	}
	p.Sensitive = []Sensitive{{Rule: testRule("hide"), Selector: Selector{Type: "THING", Path: "/name"}}}
	p.ObjectFilters = map[string]ObjectFilter{"THING": {Include: []string{"invented", "missing-secret"}, Exclude: []string{"other"}}}
	c = compileTest(t, p)
	req := Request{Mode: "drift", Left: testInput("dev", doc(nil), map[string]any{"name": "other"}), Right: testInput("prod", doc(nil), map[string]any{"name": "other"})}
	r, err := Run(req, c)
	if err != nil {
		t.Fatal(err)
	}
	encoded := stable(r)
	for _, secret := range []string{`"invented"`, `"other"`, `"missing-secret"`} {
		if strings.Contains(encoded, secret) {
			t.Fatal("filter name leaked", secret)
		}
	}
	if stable(c.policy.ObjectFilters) != stable(p.ObjectFilters) {
		t.Fatal("redaction mutated compiled filters")
	}
}
