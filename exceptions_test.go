package differ

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExceptionPolicyFragments(t *testing.T) {
	dir := t.TempDir()
	policyFile(t, dir, "root.json", `{"version":2,"id":"exceptions","profile":"isc-portable-v2","includes":["exceptions.json"]}`)
	policyFile(t, dir, "exceptions.json", `{"exceptions":[{"id":"known","type":"ROLE","path":"/description","left_env":"dev","right_env":"prod","pairs":[{"left":"old","right":"new"}],"rationale":"Pending migration"}]}`)
	p, err := LoadPolicy(filepath.Join(dir, "root.json"))
	if err != nil || len(p.Exceptions) != 1 || len(p.Equivalences) != 0 {
		t.Fatal(p, err)
	}
	if _, err = Compile(p); err != nil {
		t.Fatal(err)
	}
	p.Exceptions[0].Rationale = " "
	if _, err = Compile(p); err == nil || !strings.Contains(err.Error(), "exceptions.json") {
		t.Fatal(err)
	}
	policyFile(t, dir, "exceptions.json", `{"exceptions":[{"id":"known","type":"ROLE","path":"/description","left_env":"dev","right_env":"prod","pairs":[{"left":"old","right":"new"}]}]}`)
	if _, err = LoadPolicy(filepath.Join(dir, "root.json")); err == nil || !strings.Contains(err.Error(), "rationale") || !strings.Contains(err.Error(), "exceptions.json") {
		t.Fatal(err)
	}
}
