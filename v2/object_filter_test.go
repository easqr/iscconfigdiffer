package v2

import (
	"strings"
	"testing"
)

func TestObjectFilterDrift(t *testing.T) {
	p := testPolicy()
	left := testInput("dev", doc(map[string]any{"value": "same"}), map[string]any{"name": "other", "value": "dev"})
	right := testInput("prod", doc(map[string]any{"value": "same"}), map[string]any{"name": "other", "value": "prod"})
	req := Request{Mode: "drift", Left: left, Right: right, Scope: []string{"THING"}, ObjectKey: "invented"}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || len(r.Matches) != 1 || r.Matches[0].Key != "invented" || r.ObjectKey != "invented" || r.ObjectCoverage["left_outside_object_filter"] != 1 {
		t.Fatal(r)
	}
	for _, d := range r.Decisions {
		if d.Key != "invented" {
			t.Fatal("unselected decision", d)
		}
	}
	filteredDigest := r.RequestDigest
	req.ObjectKey = ""
	r = runTest(t, req, p)
	if r.ExitCode() != 1 || r.RequestDigest == filteredDigest {
		t.Fatal("filter absent from behavior or digest", r)
	}
	req.ObjectKey = "Invented"
	r = runTest(t, req, p)
	if r.ExitCode() != 2 || r.Complete {
		t.Fatal("missing exact key passed", r)
	}
	req.ObjectKey = "invented"
	req.Right = testInput("prod", map[string]any{"name": "other"})
	r = runTest(t, req, p)
	if r.ExitCode() != 1 || r.Counts["different"] != 1 {
		t.Fatal("one-sided absence hidden", r)
	}
	req.Scope = nil
	if _, err := Run(req, compileTest(t, p)); err == nil {
		t.Fatal("implicit scope accepted")
	}
}

func TestObjectFilterKeepsReferenceContext(t *testing.T) {
	p := testPolicy()
	p.References = []Reference{{Rule: testRule("ref"), Selector: Selector{Type: "THING", Path: "/ref"}, TargetType: "THING"}}
	req := Request{Mode: "drift", Scope: []string{"THING"}, ObjectKey: "invented",
		Left:  testInput("dev", doc(map[string]any{"ref": "devb"}), map[string]any{"name": "target", "value": "dev"}),
		Right: testInput("prod", doc(map[string]any{"ref": "prodb"}), map[string]any{"name": "target", "value": "prod"})}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 || len(r.Matches) != 1 {
		t.Fatal(r)
	}
}

func TestObjectFilterDeploymentAndRedaction(t *testing.T) {
	p := testPolicy()
	p.Expectations = []Expectation{
		{Rule: testRule("copy"), Selector: Selector{Type: "THING", Path: ""}, Operation: "copy_source", Authority: "approved"},
		{Rule: testRule("metadata"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"},
	}
	req := Request{Mode: "deployment", Scope: []string{"THING"}, ObjectKey: "invented",
		Left:       testInput("dev", doc(nil), map[string]any{"name": "other", "value": "dev"}),
		Right:      testInput("prod", doc(nil), map[string]any{"name": "other", "value": "prod"}),
		Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}, {Type: "THING", Key: "other", Action: "update"}}}
	r := runTest(t, req, p)
	if r.ExitCode() != 0 || len(r.Matches) != 1 || r.ObjectCoverage["selected_operations"] != 1 {
		t.Fatal(r)
	}
	req.ObjectKey = "other"
	savedOps := req.Operations
	req.Operations = req.Operations[:1]
	if _, err := Run(req, compileTest(t, p)); err == nil {
		t.Fatal("selected deployment without operation passed")
	}
	req.Operations = savedOps
	req.ObjectKey = "absent"
	req.Operations = append(req.Operations, Operation{Type: "THING", Key: "absent", Action: "update"})
	if r = runTest(t, req, p); r.ExitCode() != 2 {
		t.Fatal(r)
	}
	req.ObjectKey = "invented"
	p.Sensitive = []Sensitive{{Rule: testRule("hide"), Selector: Selector{Type: "THING", Path: "/name"}}}
	r = runTest(t, req, p)
	if r.ObjectKey != "[redacted]" || strings.Contains(stable(r), `"invented"`) {
		t.Fatal("filter leaked", r)
	}
}

func TestObjectFilterRetainsScheduledDeleteContext(t *testing.T) {
	p := testPolicy()
	p.Expectations = []Expectation{
		{Rule: testRule("name"), Selector: Selector{Type: "THING", Path: "/name"}, Operation: "copy_source", Authority: "approved"},
		{Rule: testRule("ref"), Selector: Selector{Type: "THING", Path: "/ref"}, Operation: "reference_target", TargetType: "THING", Authority: "approved"},
		{Rule: testRule("meta"), Selector: Selector{Type: "THING", Space: "metadata", Path: ""}, Operation: "copy_source", Authority: "approved"},
	}
	req := Request{Mode: "deployment", Scope: []string{"THING"}, ObjectKey: "invented",
		Left:       testInput("dev", doc(map[string]any{"ref": "devb"}), map[string]any{"name": "target"}),
		Right:      testInput("prod", doc(map[string]any{"ref": "prodb"}), map[string]any{"name": "target"}),
		Operations: []Operation{{Type: "THING", Key: "invented", Action: "update"}, {Type: "THING", Key: "target", Action: "delete"}}}
	r := runTest(t, req, p)
	if r.ExitCode() != 1 || r.Counts["violation"] != 1 || len(r.Matches) != 1 {
		t.Fatal("scheduled delete context lost", r)
	}
}
