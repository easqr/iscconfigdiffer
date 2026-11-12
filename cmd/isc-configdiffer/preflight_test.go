package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyValidationWithoutInputs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	root := filepath.Join(dir, "policy.json")
	fragment := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(root, []byte(`{"version":2,"id":"check","profile":"isc-portable-v2","includes":["rules.json"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body string
		code int
		want string
	}{
		{`{"ignores":[{"id":"custom","type":"ROLE","path":"/description"}]}`, 0, "policy valid"},
		{`{"ignores":[{"id":"custom","type":"ROLE","path":"/description"},{"id":"custom","type":"ROLE","path":"/secret"}]}`, 2, "rules.json"},
		{`{"profile":"unknown"}`, 2, "rules.json"},
		{`{"ignores":[{"id":"custom","type":"ROLE","path":"/bad~2escape"}]}`, 2, "rules.json"},
	} {
		if err := os.WriteFile(fragment, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		var out, stderr bytes.Buffer
		code := runWithOutput([]string{"--validate-policy", "--policy", root}, &out, &stderr)
		if code != tc.code || !strings.Contains(out.String()+stderr.String(), tc.want) {
			t.Fatalf("exit %d: %s %s", code, &out, &stderr)
		}
		if code == 2 && out.Len() != 0 {
			t.Fatal("invalid policy reported success", &out)
		}
	}
	for _, args := range [][]string{
		{"--validate-policy"}, {"--validate-policy", "--policy", "absent.json"},
		{"--validate-policy", "--policy", root, "--left", "absent"},
		{"--validate-policy", "--policy", root, "--format", "json"},
		{"--validate-policy", "--policy", root, "extra"},
		{"--validate-policy", "--version"},
	} {
		var out, stderr bytes.Buffer
		if code := runWithOutput(args, &out, &stderr); code != 2 || out.Len() != 0 {
			t.Fatalf("%v: %d %s %s", args, code, &out, &stderr)
		}
	}
	if _, err := os.Stat("reports"); !os.IsNotExist(err) {
		t.Fatal("preflight created reports", err)
	}
}

func TestVersionWithoutConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, stderr bytes.Buffer
	if code := runWithOutput([]string{"--version", "--policy", "absent.json", "--left", "absent"}, &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatal(code, &stderr)
	}
	for _, label := range []string{"isc-configdiffer ", "\ncommit: ", "\nmodified: ", "\ngo: ", "\nplatform: "} {
		if !strings.Contains(out.String(), label) {
			t.Fatal("missing version field", label, &out)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }
func TestPreflightOutputFailure(t *testing.T) {
	policy := writeJSON(t, map[string]any{"version": 2, "id": "valid", "profile": "isc-portable-v2"})
	for _, args := range [][]string{{"--version"}, {"--validate-policy", "--policy", policy}} {
		var stderr bytes.Buffer
		if code := runWithOutput(args, failedOutput{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "output unavailable") {
			t.Fatal(code, &stderr)
		}
	}
}

func TestCompletionSummary(t *testing.T) {
	left := tree(t, "private-sentinel-a")
	right := tree(t, "private-sentinel-b")
	for _, tc := range []struct {
		name, right string
		extra       []string
		code        int
		want        string
	}{
		{"clean", left, nil, 0, "no unexpected differences; complete: true; unexpected decisions: 0"},
		{"drift", right, nil, 1, "unexpected differences found; complete: true; unexpected decisions: 1"},
		{"incomplete", left, []string{"--scope", "ROLE", "--name", "absent"}, 2, "incomplete comparison; complete: false; unexpected decisions: 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := []string{"--left", left, "--right", tc.right, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--output-dir", dir, "--format", "json"}
			args = append(args, tc.extra...)
			var out, stderr bytes.Buffer
			if code := runWithOutput(args, &out, &stderr); code != tc.code || stderr.Len() != 0 {
				t.Fatal(code, &stderr)
			}
			if !strings.Contains(out.String(), tc.want) || !strings.Contains(out.String(), "Report: "+filepath.Join(dir, "default.json")) || strings.Contains(out.String(), "private-sentinel") {
				t.Fatal(&out)
			}
			if _, err := os.Stat(filepath.Join(dir, "default.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
	var out, stderr bytes.Buffer
	if code := runWithOutput([]string{"--left", "absent"}, &out, &stderr); code != 2 || out.Len() != 0 {
		t.Fatal(code, &out, &stderr)
	}
}

func TestSummaryOnlyAfterReportSaved(t *testing.T) {
	root := tree(t, "same")
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--output-dir", blocked}
	var out, stderr bytes.Buffer
	if code := runWithOutput(args, &out, &stderr); code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), "cannot save report") {
		t.Fatal(code, &out, &stderr)
	}
	args[len(args)-1] = dir
	stderr.Reset()
	if code := runWithOutput(args, failedOutput{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "output unavailable") {
		t.Fatal(code, &stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "default.html")); err != nil {
		t.Fatal("summary failure lost saved report", err)
	}
}
