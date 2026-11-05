package v2

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestReportValuesAndOmission(t *testing.T) {
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"value": "dev-value-sentinel", "null": nil, "false": false, "zero": json.Number("0"), "empty": "", "missing": "only-dev"})), Right: testInput("prod", doc(map[string]any{"value": "prod-value-sentinel", "null": "non-null", "false": true, "zero": json.Number("9007199254740993"), "empty": "filled"}))}
	before := stable(req.Left)
	full := runTest(t, req, testPolicy())
	if full.ExitCode() != 1 || full.ValuesExcluded {
		t.Fatal(full)
	}
	want := map[string]string{"/value": `"dev-value-sentinel"`, "/null": "null", "/false": "false", "/zero": "0", "/empty": `""`}
	for path, raw := range want {
		found := false
		for _, d := range full.Decisions {
			loc := d.Locations["left"]
			if loc.Pointer == path {
				found = true
				if loc.Value == nil || loc.Value.State != "present" || string(loc.Value.Value) != raw {
					t.Fatal(path, loc)
				}
			}
		}
		if !found {
			t.Fatal("missing decision", path)
		}
	}
	for _, d := range full.Decisions {
		if d.Locations["left"].Pointer == "/missing" {
			v := d.Locations["right"].Value
			if v == nil || v.State != "absent" || len(v.Value) != 0 {
				t.Fatal("absence indistinguishable from null", d)
			}
		}
	}
	req.ExcludeValues = true
	omitted := runTest(t, req, testPolicy())
	if !omitted.ValuesExcluded || omitted.ExitCode() != full.ExitCode() || omitted.RequestDigest != full.RequestDigest || !reflect.DeepEqual(omitted.Counts, full.Counts) || !reflect.DeepEqual(omitted.RuleUsage, full.RuleUsage) {
		t.Fatal("omission changed comparison semantics", omitted)
	}
	if strings.Contains(stable(omitted), "value-sentinel") {
		t.Fatal("omitted value leaked")
	}
	for _, d := range omitted.Decisions {
		for _, loc := range d.Locations {
			if loc.Value != nil {
				t.Fatal("value retained", loc)
			}
		}
	}
	if before != stable(req.Left) {
		t.Fatal("report mutated input")
	}
}

func TestExpectedAndDerivedReportValues(t *testing.T) {
	p := testPolicy()
	p.Expectations = []Expectation{{Rule: testRule("literal"), Selector: Selector{Type: "THING", Path: "/setting"}, Operation: "literal", Expected: present("expected-sentinel"), Authority: "approved"}}
	req := Request{Mode: "drift", Left: testInput("dev", doc(map[string]any{"setting": "actual-left"})), Right: testInput("prod", doc(map[string]any{"setting": "actual-right"}))}
	r := runTest(t, req, p)
	seen := map[string]bool{}
	for _, d := range r.Decisions {
		if d.RuleID == "literal" {
			if d.Expected == nil || string(d.Expected.Value) != `"expected-sentinel"` {
				t.Fatal(d)
			}
			seen[d.ExpectedRole] = true
		}
	}
	if !seen["left"] || !seen["right"] {
		t.Fatal("expected role lost", r)
	}
	p.Expectations = []Expectation{{Rule: testRule("copy"), Selector: Selector{Type: "THING", Path: ""}, Operation: "copy_source", Authority: "approved"}, {Rule: testRule("meta"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"}}
	req.Mode = "deployment"
	req.Operations = []Operation{{Type: "THING", Key: "invented", Action: "update"}}
	r = runTest(t, req, p)
	for _, d := range r.Decisions {
		if d.RuleID == "copy" {
			if d.ExpectedRole != "actual" || d.Expected == nil || d.Derivation == nil || d.Derivation.Location.Value == nil || !strings.Contains(string(d.Expected.Value), "actual-left") {
				t.Fatal(d)
			}
		}
	}
	req.ExcludeValues = true
	r = runTest(t, req, p)
	for _, d := range r.Decisions {
		if d.Expected != nil || (d.Derivation != nil && d.Derivation.Location.Value != nil) {
			t.Fatal("derived value leaked with omission", d)
		}
	}
}

func TestContainerValuesAndSensitiveReferences(t *testing.T) {
	p := testPolicy()
	p.Identities = append(p.Identities, Identity{Rule: testRule("other-id"), Type: "OTHER", Strategy: "fields", Paths: []string{"/name"}})
	p.References = []Reference{{Rule: testRule("ref"), Selector: Selector{Type: "OTHER", Path: "/ref"}, TargetType: "THING"}}
	makeIn := func(env string) Input {
		in := testInput(env, doc(map[string]any{"secret": "sensitive-value-sentinel"}))
		in.Types = append(in.Types, "OTHER")
		in.Documents = append(in.Documents, Document{Type: "OTHER", ID: env + "-other", Content: map[string]any{"name": "Holder", "ref": env + "a"}})
		return in
	}
	req := Request{Mode: "drift", Left: makeIn("dev"), Right: makeIn("prod"), IncludeMatching: true}
	// One-sided whole-object decisions also carry values.
	req.Right.Documents = req.Right.Documents[:1]
	r := runTest(t, req, p)
	found := false
	for _, d := range r.Decisions {
		if d.Code == "object missing" {
			v := d.Locations["left"].Value
			found = v != nil && strings.Contains(string(v.Value), `"Holder"`)
		}
	}
	if !found {
		t.Fatal("whole-object value missing", r)
	}
	req.Right = makeIn("prod")
	p.Sensitive = []Sensitive{{Rule: testRule("hide"), Selector: Selector{Type: "THING", Path: "/secret"}}}
	r = runTest(t, req, p)
	for _, bad := range []string{"sensitive-value-sentinel", `"deva"`, `"proda"`, `"Holder"`} {
		if strings.Contains(stable(r), bad) {
			t.Fatal("sensitive value/reference leaked", bad)
		}
	}
	for _, d := range r.Decisions {
		for _, loc := range d.Locations {
			if loc.Value != nil && loc.Value.State != "redacted" {
				t.Fatal("sensitive value not redacted", d)
			}
		}
	}
}

func TestHumanReportValuesEscapeAndRenderStates(t *testing.T) {
	value := `<script>alert("x")</script>|[link](https://example.invalid)`
	raw, _ := json.Marshal(value)
	r := Report{Version: 2, Mode: "deployment", Complete: true, Correct: false, Decisions: []Decision{{Type: "THING", Key: "invented", Status: "violation", ExpectedRole: "actual", Expected: &State{State: "present", Value: raw}, Locations: map[string]Location{"source": {Present: true, Value: &State{State: "present", Value: json.RawMessage("null")}}, "actual": {Present: true, Value: &State{State: "present", Value: raw}}}}}}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		s := b.String()
		if !strings.Contains(s, "Expected") || !strings.Contains(s, "null") || strings.Contains(s, "<script>alert") {
			t.Fatal("unsafe or missing values", s)
		}
	}
	for _, tc := range []struct {
		state *State
		want  string
	}{{nil, "omitted"}, {&State{State: "absent"}, "absent"}, {&State{State: "redacted"}, "redacted"}, {&State{State: "unavailable"}, "unavailable"}, {&State{State: "present", Value: json.RawMessage("false")}, "false"}} {
		if got := valueView(tc.state); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
}

func TestSensitiveExpectedAndDerivedValues(t *testing.T) {
	p := testPolicy()
	p.Expectations = []Expectation{{Rule: testRule("copy"), Selector: Selector{Type: "THING", Path: ""}, Operation: "copy_source", Authority: "approved"}, {Rule: testRule("metadata"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"}}
	p.Sensitive = []Sensitive{{Rule: testRule("hide"), Selector: Selector{Type: "THING", Path: "/secret"}}}
	req := Request{Mode: "deployment", Left: testInput("dev", doc(map[string]any{"secret": "sensitive-source-sentinel"})), Right: testInput("prod", doc(map[string]any{"secret": "sensitive-actual-sentinel"})), Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}}}
	r := runTest(t, req, p)
	if strings.Contains(stable(r), "sensitive-source-sentinel") || strings.Contains(stable(r), "sensitive-actual-sentinel") {
		t.Fatal("sensitive expected/derived value leaked")
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "copy" {
			found = true
			if d.Expected == nil || d.Expected.State != "redacted" || d.Derivation == nil || d.Derivation.Location.Value == nil || d.Derivation.Location.Value.State != "redacted" {
				t.Fatal(d)
			}
		}
	}
	if !found {
		t.Fatal("copy decision absent")
	}
}
