package v2

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type failedReportWriter struct{}

func (failedReportWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestHumanReportFormats(t *testing.T) {
	r := Report{Version: 2, Mode: "drift", Complete: true, Correct: false, Scope: []string{"SOURCE"}, ObjectKey: "Directory", Counts: map[string]int{"equal": 5, "different": 1},
		Decisions:   []Decision{{Status: "different", Type: "SOURCE", Key: "Directory", RuleID: "host-rule", Code: "value mismatch"}},
		Diagnostics: []Diagnostic{{Code: "unused rule", RuleID: "optional"}, {Code: "unused rule", RuleID: "mandatory", Required: true, Type: "SOURCE"}, {Code: "example diagnostic", RuleID: "diagnostic-rule", Type: "SOURCE"}}}
	for _, tc := range []struct {
		name   string
		render func(io.Writer) error
		start  string
	}{
		{"HTML", r.HTML, "<!doctype html>"}, {"Markdown", r.Markdown, "# Comparison v2 (drift)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := tc.render(&b); err != nil {
				t.Fatal(err)
			}
			s := b.String()
			if !strings.HasPrefix(s, tc.start) {
				t.Fatal(s)
			}
			for _, part := range []string{"Complete", "Correct", "Exit code", "Object filter", "Directory", "Decision counts", "different", "equal", "Decisions", "SOURCE", "host-rule", "value mismatch", "Diagnostics", "mandatory", "example diagnostic", "diagnostic-rule", "Unused optional rules: 1"} {
				if !strings.Contains(s, part) {
					t.Fatalf("missing %q in %s", part, s)
				}
			}
			if strings.Contains(s, "<script src=") || strings.Contains(s, "http://") || strings.Contains(s, "https://") {
				t.Fatal("unexpected external asset/script", s)
			}
			if err := tc.render(failedReportWriter{}); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatal("writer error lost", err)
			}
		})
	}
}

func TestHumanReportsEscapeUntrustedText(t *testing.T) {
	untrusted := "<script>alert('x')</script>|`*[link](https://example.invalid)\n# injected"
	r := Report{Version: 2, Mode: untrusted, Scope: []string{untrusted}, ObjectKey: untrusted, Counts: map[string]int{untrusted: 1}, Decisions: []Decision{{Status: untrusted, Type: untrusted, Key: untrusted, RuleID: untrusted, Code: untrusted}}, Diagnostics: []Diagnostic{{Code: untrusted, Type: untrusted, RuleID: untrusted}}}
	var h, m bytes.Buffer
	if err := r.HTML(&h); err != nil {
		t.Fatal(err)
	}
	if err := r.Markdown(&m); err != nil {
		t.Fatal(err)
	}
	if strings.Count(h.String(), "<script>") != 1 || !strings.Contains(h.String(), "&lt;script&gt;") {
		t.Fatal("HTML not escaped", h.String())
	}
	if strings.Contains(m.String(), "<script>") || strings.Contains(m.String(), "\n# injected") || strings.Contains(m.String(), "[link](https:") || !strings.Contains(m.String(), "&#124;") {
		t.Fatal("Markdown not escaped", m.String())
	}
}

func TestEmptyHumanReports(t *testing.T) {
	r := Report{Version: 2, Mode: "drift", Complete: true, Correct: true}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"No decisions counted.", "No decisions to display.", "No diagnostics to display."} {
			if !strings.Contains(b.String(), s) {
				t.Fatal("missing empty section", s)
			}
		}
	}
}

func TestHumanReportContentAndCoverage(t *testing.T) {
	r := Report{Version: 2, Mode: "drift", Complete: true, Correct: false, PolicyID: "invented-policy", Profile: "invented-profile", Scope: []string{"THING"}, PolicyDigest: "policy-hash", RequestDigest: "request-hash", Counts: map[string]int{"equal": 99, "different": 3, "equivalent": 1},
		Inputs:         map[string]Input{"left": {Environment: "dev", Tenant: "invented-dev", Documents: []Document{{Content: map[string]any{"secret": "MUST-NOT-APPEAR"}}}}, "right": {Environment: "prod", Tenant: "invented-prod"}},
		ObjectCoverage: map[string]int{"left_in_scope": 3, "right_in_scope": 2, "compared_groups": 3},
		Matches:        []Match{{Type: "THING", Key: "Directory", RuleID: "identity", Locations: map[string]Location{"left": {ID: "a", Artifact: "left.json", Present: true}, "right": {ID: "b", Artifact: "right.json", Present: true}}}, {Type: "THING", Key: "Only dev", Locations: map[string]Location{"left": {ID: "c", Present: true}}}, {Type: "THING", Key: "Equal object", Locations: map[string]Location{"left": {Present: true}, "right": {Present: true}}}},
		Decisions: []Decision{
			{Type: "THING", Key: "Directory", Status: "different", Space: "content", Code: "value mismatch", ItemKeys: []string{`["account"]`}, Locations: map[string]Location{"left": {ID: "a", Artifact: "left.json", Pointer: "/schemas/0/name", Present: true, ValueKind: "string"}, "right": {ID: "b", Artifact: "right.json", Pointer: "/schemas/2/name", Present: true, ValueKind: "string"}}},
			{Type: "THING", Key: "Only dev", Status: "different", Code: "object missing", Locations: map[string]Location{"left": {Present: true}}},
			{Type: "THING", Key: "Directory", Status: "different", Space: "content", Code: "container presence differs", Locations: map[string]Location{"left": {Pointer: "/options", Present: false}, "right": {Pointer: "/options", Present: true, ValueKind: "object"}}},
			{Type: "THING", Key: "Directory", Status: "equivalent", Code: "approved value relationship", RuleID: "host-rule", Evidence: &Evidence{Kind: "invented-approval", RecordID: "record-1", Attributes: map[string]string{"z": "last", "a": "first"}}},
		}}
	v := r.humanView()
	if v.MissingObjects != 1 || v.ContainerDifferences != 1 || v.FieldDifferences != 1 || len(v.Groups) != 3 {
		t.Fatal(v)
	}
	if v.Groups[0].Key != "Directory" || v.Groups[1].Key != "Equal object" || v.Groups[2].Presence != "left: present; right: absent" {
		t.Fatal(v.Groups)
	}
	if len(v.Groups[1].Decisions) != 0 {
		t.Fatal("matching-only object gained decisions")
	}
	if v.Groups[0].Counts[0].Count != 2 {
		t.Fatal("shown counts mixed with report totals", v.Groups[0].Counts)
	}
	before := stable(r)
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		s := b.String()
		for _, part := range []string{"invented-policy", "invented-dev", "invented-prod", "policy-hash", "request-hash", "/schemas/0/name", "/schemas/2/name", "left.json", "right.json", "Collection item keys", "record-1", "Equal object", "Only dev", "object unavailable", "content", "99", "object counts"} {
			if !strings.Contains(s, part) {
				t.Fatalf("missing content %q", part)
			}
		}
		if strings.Contains(s, "MUST-NOT-APPEAR") {
			t.Fatal("renderer accessed payload")
		}
	}
	if before != stable(r) {
		t.Fatal("rendering mutated structured report")
	}
}

func TestHumanReportDeploymentAndRedaction(t *testing.T) {
	r := Report{Version: 2, Mode: "deployment", Complete: false, Correct: false, Inputs: map[string]Input{"baseline": {}}, Scope: []string{"THING"}, Matches: []Match{{Type: "THING", Key: "[redacted]"}, {Type: "THING", Key: "[redacted]"}}, Decisions: []Decision{{Type: "THING", Key: "[redacted]", Status: "unverified", Code: "expected unavailable", Space: "metadata", RuleID: "copy", Authority: "release-policy", Replaces: "old-rule", Locations: map[string]Location{"actual": {ID: "[redacted]", Artifact: "[redacted]", Pointer: "[redacted]", Present: true, ValueKind: "object"}}, Derivation: &Derivation{Role: "source", Type: "THING", Location: Location{ID: "[redacted]", Artifact: "[redacted]", Pointer: "[redacted]", Present: true}}}}}
	v := r.humanView()
	if len(v.Groups) != 1 || !strings.Contains(v.Groups[0].Presence, "combined") {
		t.Fatal("redacted groups falsely distinct", v.Groups)
	}
	locs := v.Groups[0].Decisions[0].Locations
	if len(locs) != 3 || locs[0].Role != "source" || locs[1].Role != "actual" || locs[2].Role != "baseline" || locs[1].Pointer != "[redacted]" {
		t.Fatal(locs)
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{"Incomplete:", "metadata", "[redacted]", "release-policy", "old-rule", "Expected result derived from", "baseline"} {
			if !strings.Contains(b.String(), part) && !strings.Contains(b.String(), markdownCell(part)) {
				t.Fatal("missing deployment context", part)
			}
		}
	}
}
