package main

import (
	"bytes"
	"encoding/json"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
func TestExitCodesAndRendering(t *testing.T) {
	left, right := tree(t, "sensitive-original"), tree(t, "sensitive-original")
	args := []string{"--left", left, "--right", right, "--left-mode", "tree", "--right-mode", "tree", "--left-types", "ROLE", "--right-types", "ROLE", "--format", "json"}
	var out, errOut bytes.Buffer
	if code := run(args, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	var report map[string]any
	if e := json.Unmarshal(out.Bytes(), &report); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), "sensitive-original") {
		t.Fatal("payload leaked")
	}
	first := out.String()
	out.Reset()
	if code := run(args, &out, &errOut); code != 0 || out.String() != first {
		t.Fatal("non deterministic CLI")
	}
	right = tree(t, "sensitive-changed")
	args[3] = right
	out.Reset()
	if code := run(args, &out, &errOut); code != 1 {
		t.Fatal(code, errOut.String())
	}
	if strings.Contains(out.String(), "sensitive-changed") {
		t.Fatal("payload leaked")
	}
	args[len(args)-1] = "text"
	out.Reset()
	if code := run(args, &out, &errOut); code != 1 || !strings.Contains(out.String(), "different: 1") {
		t.Fatal(code, out.String())
	}
	for _, bad := range [][]string{nil, {"--unknown"}, {"--left", left, "--right", right}, {"--left", left, "--right", right, "--format", "yaml"}} {
		if code := run(bad, &out, &errOut); code != 2 {
			t.Fatal(code)
		}
	}
	// A required unused selector produces an incomplete report and exit 2.
	p := filepath.Join(t.TempDir(), "policy.json")
	os.WriteFile(p, []byte(`{"version":1,"id":"test","expectations":[{"id":"required","type":"ROLE","path":"/object/items/*/dn","values":{"e":"value"},"required":true}]}`), 0600)
	args = append(args, "--policy", p)
	out.Reset()
	if code := run(args, &out, &errOut); code != 2 || !strings.Contains(out.String(), "Complete: false") {
		t.Fatal(code, out.String(), errOut.String())
	}
}
