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
			if strings.Contains(s, "<script") || strings.Contains(s, "http://") || strings.Contains(s, "https://") {
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
	if strings.Contains(h.String(), "<script>") || !strings.Contains(h.String(), "&lt;script&gt;") {
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
