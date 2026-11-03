package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func writeJSONV2(t *testing.T, v any) string {
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
func TestV2CLIModesMigrationAndReports(t *testing.T) {
	left, right := tree(t, "same"), tree(t, "same")
	args := []string{"--mode", "drift", "--left", left, "--right", right, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--format", "json"}
	var out, errOut bytes.Buffer
	if code := run(args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	var r v2.Report
	if e := json.Unmarshal(out.Bytes(), &r); e != nil || r.Version != 2 || r.Mode != "drift" {
		t.Fatal(e, r)
	}
	first := out.String()
	out.Reset()
	if code := run(args, &out, &errOut); code != 0 || out.String() != first {
		t.Fatal("nondeterministic report", code)
	}
	right = tree(t, "changed")
	args[5] = right
	out.Reset()
	if code := run(args, &out, &errOut); code != 1 {
		t.Fatal(code, errOut.String())
	}
	if strings.Contains(out.String(), "changed") {
		t.Fatal("payload leaked")
	}
	legacy := writeJSONV2(t, map[string]any{"version": 1, "id": "legacy", "profile": "isc-portable-v1"})
	out.Reset()
	if code := run([]string{"--translate-policy", legacy}, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	p, e := v2.Decode(out.Bytes())
	if e != nil || p.Version != 2 {
		t.Fatal(e)
	}
	args = append(args, "--policy", legacy)
	out.Reset()
	if code := run(args, &out, &errOut); code != 1 {
		t.Fatal(code, errOut.String())
	}
	// Explicit deployment scopes and independently declared expectations.
	policy := writeJSONV2(t, map[string]any{"version": 2, "id": "deployment", "profile": "isc-portable-v2", "expectations": []any{map[string]any{"id": "name", "type": "ROLE", "path": "/name", "operation": "copy_source", "authority": "approved", "evidence": map[string]any{"kind": "invented-release"}}, map[string]any{"id": "secret", "type": "ROLE", "path": "/secret", "operation": "copy_source", "authority": "approved", "evidence": map[string]any{"kind": "invented-release"}}}})
	ops := writeJSONV2(t, []v2.Operation{{Type: "ROLE", Key: "Invented", Action: "update"}})
	deployment := []string{"--mode", "deployment", "--source", left, "--actual", left, "--source-mode", "tree", "--actual-mode", "tree", "--source-types", "ROLE", "--actual-types", "ROLE", "--operations", ops, "--policy", policy, "--format", "json"}
	out.Reset()
	if code := run(deployment, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String(), out.String())
	}
	deployment[5] = right
	out.Reset()
	if code := run(deployment, &out, &errOut); code != 1 {
		t.Fatal(code, errOut.String(), out.String())
	}
	// Without explicit field expectations even equal drafts remain unverified.
	deployment = deployment[:len(deployment)-4]
	out.Reset()
	if code := run(deployment, &out, &errOut); code != 2 || !strings.Contains(out.String(), "unverified") {
		t.Fatal(code, errOut.String(), out.String())
	}
}
func TestV2CLIRejectsAmbiguousRolesAndScope(t *testing.T) {
	root := tree(t, "v")
	base := []string{"--mode", "drift", "--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE"}
	var out, errOut bytes.Buffer
	for _, extra := range [][]string{{"--actual-env", "prod"}, {"--baseline-env", "prod"}, {"--operations", "missing.json"}, {"--hub-source-tenant", "unbound"}, {"--mode", "unknown"}, {"--scope", "SOURCE"}, {"--hub-deployment-authority", "approval"}} {
		args := append(append([]string{}, base...), extra...)
		if code := run(args, &out, &errOut); code != 2 {
			t.Fatal(extra, code)
		}
	}
	if code := run([]string{"--mode", "deployment", "--source", root, "--actual", root}, &out, &errOut); code != 2 {
		t.Fatal(code)
	}
	if code := run([]string{"--translate-policy", "missing", "--mode", "drift"}, &out, &errOut); code != 2 {
		t.Fatal(code)
	}
}

func TestLegacyLabelDoesNotSelectV2(t *testing.T) {
	root := tree(t, "v")
	var out, errOut bytes.Buffer
	args := []string{"--left", root, "--right", root, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--left-label", "--mode", "--format", "json"}
	if code := run(args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	if !strings.Contains(out.String(), `"version": 1`) {
		t.Fatal("label selected a different executor", out.String())
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
	policy := writeJSONV2(t, map[string]any{"version": 2, "id": "bound", "profile": "isc-portable-v2", "bindings": map[string]string{"left": "dev", "right": "prod"}})
	hub := writeJSONV2(t, []any{map[string]any{"objectMappingId": "invented-map", "objectType": "ROLE", "jsonPath": "$.secret", "sourceValue": "same", "targetValue": "same", "enabled": true}})
	args := []string{"--mode", "drift", "--left", left, "--right", right, "--left-mode", "snapshot", "--right-mode", "snapshot", "--policy", policy, "--hub", hub, "--hub-source-tenant", "invented-source", "--hub-target-tenant", "invented-target", "--format", "json"}
	var out, errOut bytes.Buffer
	if code := run(args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String(), out.String())
	}
	var r v2.Report
	if e := json.Unmarshal(out.Bytes(), &r); e != nil || r.Inputs["left"].Environment != "dev" || r.Inputs["right"].Environment != "prod" {
		t.Fatal(e, r)
	}
	args = append(args, "--left-env", "conflict")
	out.Reset()
	errOut.Reset()
	if code := run(args, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "conflicting environment binding") {
		t.Fatal(code, errOut.String())
	}
}
