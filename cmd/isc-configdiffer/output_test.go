package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func TestReportOutputDefaultsAndOverrides(t *testing.T) {
	left, right := tree(t, "same"), tree(t, "same")
	policy := writeJSON(t, map[string]any{"version": 2, "id": "source-drift", "profile": "isc-portable-v2"})
	t.Chdir(t.TempDir())
	base := []string{"--left", left, "--right", right, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE"}
	for _, tc := range []struct {
		name  string
		extra []string
		path  string
	}{
		{"built-in default", nil, "reports/default.txt"},
		{"policy id", []string{"--policy", policy, "--format", "json"}, "reports/source-drift.json"},
		{"directory", []string{"--policy", policy, "--output-dir", "nested/results"}, "nested/results/source-drift.txt"},
		{"exact override", []string{"--output-dir", "custom", "--output-file", "comparison.data", "--format", "json"}, "custom/comparison.data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer
			if code := run(append(append([]string{}, base...), tc.extra...), &errOut); code != 0 {
				t.Fatal(code, errOut.String())
			}
			saved, err := os.ReadFile(tc.path)
			if err != nil || len(saved) == 0 {
				t.Fatal("saved report is missing or empty", err)
			}
			if strings.Contains(tc.path, "json") || strings.HasSuffix(tc.path, ".data") {
				var r v2.Report
				if err := json.Unmarshal(saved, &r); err != nil || !r.Complete {
					t.Fatal(err, r)
				}
			}
		})
	}
	// Repeated runs replace the same report, including when drift produces exit 1.
	changed := tree(t, "changed")
	base[3] = changed
	if err := os.WriteFile("reports/source-drift.json", []byte("old report"), 0600); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	if code := run(append(base, "--policy", policy, "--format", "json"), &errOut); code != 1 {
		t.Fatal(code, errOut.String())
	}
	saved, err := os.ReadFile("reports/source-drift.json")
	if err != nil || len(saved) == 0 {
		t.Fatal(err, "report not replaced")
	}
	var r v2.Report
	if err := json.Unmarshal(saved, &r); err != nil || r.Correct {
		t.Fatal(err, r)
	}
}

func TestReportOutputIncompleteAndFailures(t *testing.T) {
	root := tree(t, "same")
	t.Chdir(t.TempDir())
	base := []string{"--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE"}
	var errOut bytes.Buffer
	if code := run(append(append([]string{}, base...), "--scope", "ROLE", "--name", "absent", "--format", "json"), &errOut); code != 2 {
		t.Fatal(code, errOut.String())
	}
	saved, err := os.ReadFile("reports/default.json")
	if err != nil || len(saved) == 0 {
		t.Fatal(err, "incomplete diagnostic report not saved")
	}
	if err := os.WriteFile("blocked", []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("destination/existing", 0700); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{
		{"--output-dir", ""}, {"--output-file", ""}, {"--output-file", "../escape.json"},
		{"--output-file", "/absolute.json"}, {"--output-file", `nested\report.json`}, {"--output-file", "."}, {"--output-file", ".."},
		{"--output-dir", "blocked"}, {"--output-dir", "destination", "--output-file", "existing"},
	} {
		errOut.Reset()
		if code := run(append(append([]string{}, base...), extra...), &errOut); code != 2 || errOut.Len() == 0 {
			t.Fatal(extra, code, errOut.String())
		}
	}
	leftovers, err := filepath.Glob("destination/.isc-configdiffer-*")
	if err != nil || len(leftovers) != 0 {
		t.Fatal("temporary report left behind", err, leftovers)
	}
	errOut.Reset()
	if code := run(append(append([]string{}, base...), "--policy", "missing.json", "--output-dir", "not-created"), &errOut); code != 2 {
		t.Fatal(code)
	}
	if _, err := os.Stat("not-created"); !os.IsNotExist(err) {
		t.Fatal("invalid request created output", err)
	}
}
