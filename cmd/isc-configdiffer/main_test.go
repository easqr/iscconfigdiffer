package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/easqr/iscconfigdiffer/v2"
	"github.com/easqr/iscconfigexporter/snapshot"
)

// Existing report tests inspect saved files and discard the terminal summary.
func run(args []string, errOut io.Writer) int {
	return runWithOutput(args, io.Discard, errOut)
}

func tree(t *testing.T, value string) string {
	t.Helper()
	root := t.TempDir()
	path, _ := snapshot.ObjectPath("ROLE", "same")
	if e := os.Mkdir(filepath.Join(root, "ROLE"), 0700); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(map[string]any{"version": 1, "self": map[string]any{"type": "ROLE", "id": "same"}, "object": map[string]any{"name": "Invented", "secret": value}})
	if e := os.WriteFile(filepath.Join(root, path), b, 0600); e != nil {
		t.Fatal(e)
	}
	return root
}
func writeJSON(t *testing.T, v any) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "invented.json")
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestCLIHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--mode", "deployment", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			var output bytes.Buffer
			if code := run(args, &output); code != 0 {
				t.Fatalf("help exited %d: %s", code, output.String())
			}
			if !strings.Contains(output.String(), "Usage of isc-configdiffer:") || !strings.Contains(output.String(), "-policy") {
				t.Fatalf("missing usage or options: %s", output.String())
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("help created files: %v, %v", entries, err)
			}
		})
	}
	for _, args := range [][]string{{"--unknown-option"}, {"--mode"}, {}} {
		var output bytes.Buffer
		if code := run(args, &output); code != 2 {
			t.Fatalf("invalid request %q exited %d: %s", args, code, output.String())
		}
	}
}

func TestCLIModesAndReports(t *testing.T) {
	left, right := tree(t, "same"), tree(t, "same")
	args := []string{"--mode", "drift", "--left", left, "--right", right, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--format", "json"}
	var out, errOut bytes.Buffer
	if code := runWithReports(t, args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	var r v2.Report
	if e := json.Unmarshal(out.Bytes(), &r); e != nil || r.Version != 2 || r.Mode != "drift" {
		t.Fatal(e, r)
	}
	first := out.String()
	out.Reset()
	if code := runWithReports(t, args, &out, &errOut); code != 0 || out.String() != first {
		t.Fatal("nondeterministic report", code)
	}
	right = tree(t, "changed")
	args[5] = right
	out.Reset()
	if code := runWithReports(t, args, &out, &errOut); code != 1 {
		t.Fatal(code, errOut.String())
	}
	if !strings.Contains(out.String(), "changed") {
		t.Fatal("observed value missing")
	}
	// Explicit deployment scopes and independently declared expectations.
	policy := writeJSON(t, map[string]any{"version": 2, "id": "deployment", "profile": "isc-portable-v2", "expectations": []any{map[string]any{"id": "name", "type": "ROLE", "path": "/name", "operation": "copy_source", "authority": "approved", "evidence": map[string]any{"kind": "invented-release"}}, map[string]any{"id": "secret", "type": "ROLE", "path": "/secret", "operation": "copy_source", "authority": "approved", "evidence": map[string]any{"kind": "invented-release"}}}})
	ops := writeJSON(t, []v2.Operation{{Type: "ROLE", Key: "Invented", Action: "update"}})
	deployment := []string{"--mode", "deployment", "--source", left, "--actual", left, "--source-mode", "tree", "--actual-mode", "tree", "--source-types", "ROLE", "--actual-types", "ROLE", "--operations", ops, "--policy", policy, "--format", "json"}
	out.Reset()
	if code := runWithReports(t, deployment, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String(), out.String())
	}
	deployment[5] = right
	out.Reset()
	if code := runWithReports(t, deployment, &out, &errOut); code != 1 {
		t.Fatal(code, errOut.String(), out.String())
	}
	// Without explicit field expectations even equal drafts remain unverified.
	deployment = deployment[:len(deployment)-4]
	out.Reset()
	if code := runWithReports(t, deployment, &out, &errOut); code != 2 || !strings.Contains(out.String(), "unverified") {
		t.Fatal(code, errOut.String(), out.String())
	}
}
func TestCLIRejectsAmbiguousRolesAndScope(t *testing.T) {
	root := tree(t, "v")
	base := []string{"--mode", "drift", "--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE"}
	var out, errOut bytes.Buffer
	for _, extra := range [][]string{{"--actual-env", "prod"}, {"--baseline-env", "prod"}, {"--operations", "missing.json"}, {"--hub-source-tenant", "unbound"}, {"--mode", "unknown"}, {"--scope", "SOURCE"}, {"--hub-deployment-authority", "approval"}} {
		args := append(append([]string{}, base...), extra...)
		if code := runWithReports(t, args, &out, &errOut); code != 2 {
			t.Fatal(extra, code)
		}
	}
	if code := runWithReports(t, []string{"--mode", "deployment", "--source", root, "--actual", root}, &out, &errOut); code != 2 {
		t.Fatal(code)
	}
}

func TestLabelIsOnlyMetadata(t *testing.T) {
	root := tree(t, "v")
	var out, errOut bytes.Buffer
	args := []string{"--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--left-label", "--mode", "--format", "json"}
	if code := runWithReports(t, args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	if !strings.Contains(out.String(), `"version": 2`) {
		t.Fatal("label changed the report schema", out.String())
	}
}

func TestDefaultDriftAndPolicySchema(t *testing.T) {
	root := tree(t, "value")
	base := []string{"--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--format", "json"}
	var out, errOut bytes.Buffer
	if code := runWithReports(t, base, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	var report v2.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Version != 2 || report.Mode != "drift" {
		t.Fatal(report, err)
	}
	unsupported := writeJSON(t, map[string]any{"version": 3, "id": "unsupported", "profile": "isc-portable-v2"})
	out.Reset()
	errOut.Reset()
	if code := runWithReports(t, append(append([]string{}, base...), "--policy", unsupported), &out, &errOut); code != 2 || out.Len() != 0 {
		t.Fatal("unsupported schema was executed", code, out.String(), errOut.String())
	}
	if code := runWithReports(t, append(append([]string{}, base...), "--unknown"), &out, &errOut); code != 2 {
		t.Fatal("unknown option accepted")
	}
}

func TestReviewHubUsesPolicyEnvironmentBindings(t *testing.T) {
	left, right := tree(t, "same"), tree(t, "same")
	for tenant, root := range map[string]string{"invented-source": left, "invented-target": right} {
		p, _ := snapshot.ObjectPath("ROLE", "same")
		b, e := os.ReadFile(filepath.Join(root, p))
		if e != nil {
			t.Fatal(e)
		}
		m := snapshot.Manifest{Schema: 1, Tenant: tenant, JobID: "invented-job", Completed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Types: []string{"ROLE"}, Counts: map[string]int{"ROLE": 1}, Artifacts: []snapshot.Artifact{{Path: p, SHA256: snapshot.Checksum(b)}}}
		b, _ = json.Marshal(m)
		if e = os.WriteFile(filepath.Join(root, "manifest.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	policy := writeJSON(t, map[string]any{"version": 2, "id": "bound", "profile": "isc-portable-v2", "bindings": map[string]string{"left": "dev", "right": "prod"}})
	hub := writeJSON(t, []any{map[string]any{"objectMappingId": "invented-map", "objectType": "ROLE", "jsonPath": "$.secret", "sourceValue": "same", "targetValue": "same", "enabled": true}})
	args := []string{"--mode", "drift", "--left", left, "--right", right, "--left-mode", "snapshot", "--right-mode", "snapshot", "--policy", policy, "--hub", hub, "--hub-source-tenant", "invented-source", "--hub-target-tenant", "invented-target", "--format", "json"}
	var out, errOut bytes.Buffer
	if code := runWithReports(t, args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String(), out.String())
	}
	var r v2.Report
	if e := json.Unmarshal(out.Bytes(), &r); e != nil || r.Inputs["left"].Environment != "dev" || r.Inputs["right"].Environment != "prod" {
		t.Fatal(e, r)
	}
	args = append(args, "--left-env", "conflict")
	out.Reset()
	errOut.Reset()
	if code := runWithReports(t, args, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "conflicting environment binding") {
		t.Fatal(code, errOut.String())
	}
}

func TestCLIObjectNameFilter(t *testing.T) {
	root := tree(t, "same")
	base := []string{"--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--format", "json"}
	for _, tc := range []struct {
		extra []string
		code  int
	}{
		{[]string{"--scope", "ROLE", "--name", "Invented"}, 0},
		{[]string{"--scope", "ROLE", "--name", "invented"}, 2},
		{[]string{"--name", "Invented"}, 2},
		{[]string{"--scope", "ROLE", "--name", ""}, 2},
	} {
		var out, errOut bytes.Buffer
		if code := runWithReports(t, append(append([]string{}, base...), tc.extra...), &out, &errOut); code != tc.code {
			t.Fatal(tc, code, errOut.String(), out.String())
		}
		if tc.code == 0 {
			var report v2.Report
			if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.ObjectKey != "Invented" || len(report.Matches) != 1 {
				t.Fatal(err, report)
			}
		}
	}
}

func TestCLIPolicyObjectFilters(t *testing.T) {
	root := tree(t, "same")
	base := []string{"--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--scope", "ROLE", "--format", "json"}
	for _, tc := range []struct {
		filter  string
		code    int
		skipped int
	}{
		{`{"include":["Invented"]}`, 0, 0},
		{`{"exclude":["Invented"]}`, 2, 1},
		{`{"include":["invented"]}`, 2, 1},
	} {
		policyPath := filepath.Join(t.TempDir(), "policy.json")
		body := `{"version":2,"id":"filtered","profile":"isc-portable-v2","object_filters":{"ROLE":` + tc.filter + `}}`
		if err := os.WriteFile(policyPath, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		if code := runWithReports(t, append(append([]string{}, base...), "--policy", policyPath), &out, &errOut); code != tc.code {
			t.Fatal(code, errOut.String())
		}
		var r v2.Report
		if err := json.Unmarshal(out.Bytes(), &r); err != nil || len(r.ExcludedObjects) != tc.skipped {
			t.Fatal("filter report unavailable", err, r)
		}
	}
}

// Read the saved report for assertions while isolating each CLI invocation.
func runWithReports(t *testing.T, args []string, reportContents, errOut *bytes.Buffer) int {
	t.Helper()
	dir := t.TempDir()
	code := run(append(append([]string{}, args...), "--output-dir", dir, "--output-file", "report"), errOut)
	data, err := os.ReadFile(filepath.Join(dir, "report"))
	if err == nil {
		reportContents.Write(data)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return code
}

func TestCLIIncludedPolicy(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(root, []byte(`{"version":2,"id":"included","profile":"isc-portable-v2","bindings":{"left":"dev","right":"prod"},"includes":["rules.json"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`{"equivalences":[{"id":"secret-mapping","type":"ROLE","key":"Invented","path":"/secret","left_env":"dev","right_env":"prod","groups":[{"dev":"dev-value","prod":"prod-value"}]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--left", tree(t, "dev-value"), "--right", tree(t, "prod-value"), "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--policy", root, "--format", "json"}
	var out, errOut bytes.Buffer
	if code := runWithReports(t, args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	var r v2.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Counts["equivalent"] != 1 || !strings.Contains(out.String(), "rules.json") {
		t.Fatal(r.Counts)
	}
	// Invalid fragment configuration fails before writing a report.
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`{"profile":"isc-portable-v2"}`), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := runWithReports(t, args, &out, &errOut); code != 2 || out.Len() != 0 || !strings.Contains(errOut.String(), "rules.json") {
		t.Fatal(code, errOut.String())
	}
}

func TestCLIExceptionsAndRequiredRationale(t *testing.T) {
	left, right := tree(t, "old"), tree(t, "new")
	for _, rationale := range []any{nil, "", " \t", 42, "Pending migration"} {
		rule := map[string]any{"id": "known", "type": "ROLE", "path": "/secret", "left_env": "dev", "right_env": "prod", "pairs": []any{map[string]any{"left": "old", "right": "new"}}}
		if rationale != nil {
			rule["rationale"] = rationale
		}
		policy := writeJSON(t, map[string]any{"version": 2, "id": "exceptions", "profile": "isc-portable-v2", "exceptions": []any{rule}})
		args := []string{"--mode", "drift", "--left", left, "--right", right, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--left-env", "dev", "--right-env", "prod", "--policy", policy, "--format", "json"}
		var out, errOut bytes.Buffer
		code := runWithReports(t, args, &out, &errOut)
		if rationale != "Pending migration" {
			if code != 2 {
				t.Fatalf("invalid rationale %v: exit %d, %s", rationale, code, errOut.String())
			}
			continue
		}
		var report v2.Report
		if code != 0 || json.Unmarshal(out.Bytes(), &report) != nil || report.Counts["exception"] != 1 || report.Counts["equivalent"] != 0 {
			t.Fatalf("exit %d: %s %s", code, out.String(), errOut.String())
		}
	}
}

func TestCLIExceptionExclusionFragment(t *testing.T) {
	dir := t.TempDir()
	fragment := `{"exceptions":[{"id":"known-secret","type":"ROLE","key":"Invented","path":"/secret","operation":"exclude","rationale":"Accepted excluded setting"}]}`
	if err := os.WriteFile(filepath.Join(dir, "exceptions.json"), []byte(fragment), 0600); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(policy, []byte(`{"version":2,"id":"exclusions","profile":"isc-portable-v2","includes":["exceptions.json"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--mode", "drift", "--left", tree(t, "old"), "--right", tree(t, "new"), "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--policy", policy, "--format", "json"}
	var out, errOut bytes.Buffer
	if code := runWithReports(t, args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	var r v2.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Counts["exception"] != 1 || r.Counts["different"] != 0 {
		t.Fatal(r.Counts)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "known-secret" {
			found = true
			if d.Rationale != "Accepted excluded setting" || d.Code != "explicit exception exclusion" {
				t.Fatal(d)
			}
		}
	}
	if !found {
		t.Fatal("missing exclusion decision")
	}
}
