package differ

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
)

func forestInput(t *testing.T, env, typ string, forests []any) snapshot.Input {
	t.Helper()
	return input(t, env, []string{typ}, obj(typ, "same-source-id", "Invented Directory", map[string]any{"connectorAttributes": map[string]any{"connectorName": "Active Directory", "forestSettings": forests}}))
}
func forestEntry(name string, ssl bool) any {
	return map[string]any{"forestName": name, "useSSL": ssl, "authorizationType": "simple", "user": "shared-user-sentinel"}
}
func TestSourceActiveDirectoryForestAlignment(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		t.Run(profile, func(t *testing.T) {
			req := Request{Mode: "drift", IncludeMatching: true, Left: forestInput(t, "dev", "SOURCE", []any{forestEntry("one.example.invalid", true), forestEntry("two.example.invalid", false)}), Right: forestInput(t, "prod", "SOURCE", []any{forestEntry("two.example.invalid", false), forestEntry("one.example.invalid", true)})}
			p := v2.Policy{Version: 2, ID: "invented", Profile: profile}
			r := compareTest(t, req, p)
			if r.ExitCode() != 0 || r.RuleUsage["isc.SOURCE.ad-forests"] != 1 {
				t.Fatal(r.Counts)
			}
			found := false
			for _, d := range r.Decisions {
				if d.Locations["left"].Pointer == "/connectorAttributes/forestSettings/0/useSSL" {
					found = true
					if d.Locations["right"].Pointer != "/connectorAttributes/forestSettings/1/useSSL" || len(d.Locations["left"].Context) != 1 || contextFieldValue(d.Locations["left"].Context[0], "Name") != "one.example.invalid" {
						t.Fatal(d)
					}
				}
			}
			if !found {
				t.Fatal("forest setting decision absent")
			}
			for _, name := range []any{nil, "", " ", 42, "two.example.invalid"} {
				bad := forestEntry("one", true).(map[string]any)
				bad["forestName"] = name
				req.Right = forestInput(t, "prod", "SOURCE", []any{bad, forestEntry("two.example.invalid", false)})
				if r = compareTest(t, req, p); r.ExitCode() != 2 || r.Counts["conflict"] == 0 {
					t.Fatal("invalid forest name accepted", name, r.Counts)
				}
			}
			bad := forestEntry("one", true).(map[string]any)
			delete(bad, "forestName")
			req.Right = forestInput(t, "prod", "SOURCE", []any{bad})
			if r = compareTest(t, req, p); r.ExitCode() != 2 {
				t.Fatal("missing forest name accepted")
			}
		})
	}
	// The domain provider does not change unrelated types with similar data.
	req := Request{Mode: "drift", Left: forestInput(t, "dev", "ROLE", []any{forestEntry("one", true), forestEntry("two", false)}), Right: forestInput(t, "prod", "ROLE", []any{forestEntry("two", false), forestEntry("one", true)})}
	if r := compareTest(t, req, portable()); r.ExitCode() != 1 || r.RuleUsage["isc.SOURCE.ad-forests"] != 0 {
		t.Fatal("SOURCE rule leaked to ROLE", r.Counts)
	}
}
func TestDocumentedForestMappingAndReportContext(t *testing.T) {
	data, err := os.ReadFile("examples/policy-drift.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := v2.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	left := []any{forestEntry("dev.example.invalid", true), forestEntry("shared.example.invalid", false)}
	right := []any{forestEntry("shared.example.invalid", false), forestEntry("prod.example.invalid", true)}
	req := Request{Mode: "drift", IncludeMatching: true, Left: forestInput(t, "dev", "SOURCE", left), Right: forestInput(t, "prod", "SOURCE", right)}
	r := compareTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 || r.RuleUsage["directory-forests"] != 1 {
		t.Fatal("example did not align mapped forests", r.Counts, r.Replacements)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID == "directory-forest-names" {
			found = true
			if d.Locations["left"].Pointer != "/connectorAttributes/forestSettings/0/forestName" || d.Locations["right"].Pointer != "/connectorAttributes/forestSettings/1/forestName" {
				t.Fatal(d)
			}
		}
	}
	if !found {
		t.Fatal("equivalence provenance absent")
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"Forest", "dev.example.invalid", "prod.example.invalid", "directory-forests", "key mapping"} {
			if !strings.Contains(b.String(), text) {
				t.Fatal("report missing forest mapping context", text)
			}
		}
	}
	right[1].(map[string]any)["useSSL"] = false
	req.Right = forestInput(t, "prod", "SOURCE", right)
	if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts["different"] != 1 {
		t.Fatal("forest mapping hid other settings", r.Counts)
	}
	// Alignment alone does not establish equivalence of the forest name values.
	p.Equivalences = nil
	if r = compareTest(t, req, p); r.Counts["different"] != 2 {
		t.Fatal("mapping granted value equivalence", r.Counts)
	}
	req.ExcludeValues = true
	r = compareTest(t, req, p)
	data, _ = json.Marshal(r)
	if strings.Contains(string(data), "shared-user-sentinel") {
		t.Fatal("omitted values leaked")
	}
	if !strings.Contains(string(data), "dev.example.invalid") {
		t.Fatal("forest identity context omitted")
	}
	p.Sensitive = []v2.Sensitive{{Rule: rule("hide-forests"), Selector: v2.Selector{Type: "SOURCE", Path: "/connectorAttributes/forestSettings"}}}
	r = compareTest(t, req, p)
	data, _ = json.Marshal(r)
	if strings.Contains(string(data), "dev.example.invalid") || strings.Contains(string(data), "prod.example.invalid") {
		t.Fatal("sensitive forest context leaked")
	}
}
