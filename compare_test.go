package differ

import (
	"context"
	"encoding/json"
	"github.com/elianderson/iscutilities/configdiffer/v2"
	exporter "github.com/elianderson/iscutilities/configexporter"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExporterPublicationAndReaders(t *testing.T) {
	root := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"tenant": "invented", "objects": []any{obj("ROLE", "one", "Synthetic", nil)}})
	snap, err := exporter.Organize("invented", exporter.Download{JobID: "synthetic-job", Types: []string{"ROLE", "SOURCE"}, Raw: raw}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (exporter.FileStore{Output: root}).Publish(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	left, err := snapshot.Read(filepath.Join(root, "invented"), snapshot.Options{Mode: snapshot.SnapshotMode})
	if err != nil {
		t.Fatal(err)
	}
	right, err := snapshot.Read(filepath.Join(root, "invented"), snapshot.Options{Mode: snapshot.TreeMode})
	if err != nil {
		t.Fatal(err)
	}
	report := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, DefaultPolicy())
	if report.ExitCode() != 0 || len(report.Matches) != 1 {
		t.Fatal(report)
	}
	if _, err = os.Stat(filepath.Join(root, "invented", "manifest.json")); err != nil {
		t.Fatal(err)
	}
}
func TestScopeMissingObjectsAndInvalidInputs(t *testing.T) {
	left := input(t, "e", []string{"ROLE"}, obj("ROLE", "a", "Invented", nil))
	right := input(t, "e", []string{"ROLE", "SOURCE"}, obj("ROLE", "b", "Invented", nil))
	if _, err := Compare(Request{Mode: "drift", Left: left, Right: right}, portable()); err == nil {
		t.Fatal("different scopes accepted")
	}
	report := compareTest(t, Request{Mode: "drift", Left: left, Right: right, Scope: []string{"ROLE"}}, portable())
	if report.ExitCode() != 0 {
		t.Fatal(report)
	}
	missing := input(t, "e", []string{"ROLE"})
	report = compareTest(t, Request{Mode: "drift", Left: left, Right: missing}, portable())
	if report.ExitCode() != 1 || report.Counts["different"] != 1 {
		t.Fatal(report)
	}
	report, err := Compare(Request{Mode: "drift"}, portable())
	if err == nil || report.ExitCode() != 2 {
		t.Fatal("invalid inputs passed")
	}
}
func TestKeyedSelectorsAndKnownReferencePrecedence(t *testing.T) {
	p := portable()
	p.Collections = []v2.Collection{{Rule: rule("items"), Selector: v2.Selector{Type: "ROLE", Path: "/items"}, Mode: "keyed", Keys: []string{"/key"}}}
	p.Expectations = []v2.Expectation{{Rule: rule("assert"), Selector: v2.Selector{Type: "ROLE", Path: "/items/0/value"}, Operation: "literal", Expected: &v2.State{State: "present", Value: json.RawMessage(`"x"`)}, Authority: "policy"}}
	if _, err := Compile(p); err == nil {
		t.Fatal("positional assertion under keyed array accepted")
	}
	p = portable()
	p.References = []v2.Reference{{Rule: rule("ref"), Selector: v2.Selector{Type: "ROLE", Path: "/ref"}, TargetType: "SOURCE", External: map[string]string{"ls": "wrong"}}}
	types := []string{"ROLE", "SOURCE"}
	left := input(t, "e", types, obj("ROLE", "lr", "Role", map[string]any{"ref": "ls"}), obj("SOURCE", "ls", "Directory", nil))
	right := input(t, "e", types, obj("ROLE", "rr", "Role", map[string]any{"ref": "wrong"}), obj("SOURCE", "rs", "Directory", nil))
	report := compareTest(t, Request{Mode: "drift", Left: left, Right: right}, p)
	if report.ExitCode() == 0 {
		t.Fatal("external mapping overrode known correspondence", report)
	}
}

func TestObjectFilterAdapter(t *testing.T) {
	types := []string{"SOURCE"}
	left := input(t, "dev", types, obj("SOURCE", "a", "Directory", nil), obj("SOURCE", "b", "Other", map[string]any{"value": "dev"}))
	right := input(t, "prod", types, obj("SOURCE", "c", "Directory", nil), obj("SOURCE", "d", "Other", map[string]any{"value": "prod"}))
	r := compareTest(t, Request{Mode: "drift", Left: left, Right: right, Scope: types, ObjectKey: "Directory"}, portable())
	if r.ExitCode() != 0 || len(r.Matches) != 1 || r.ObjectKey != "Directory" {
		t.Fatal(r)
	}
}
