package differ

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func domainEntry(forest, dn string) map[string]any {
	return map[string]any{"forestName": forest, "domainDN": dn, "useSSL": true, "port": 636, "authorizationType": "simple", "user": "domain-user-sentinel", "servers": []any{"first.example.invalid", "second.example.invalid"}}
}
func domainInput(t *testing.T, env, typ string, domains []any) snapshot.Input {
	t.Helper()
	return input(t, env, []string{typ}, obj(typ, "same-source-id", "Invented Directory", map[string]any{"connectorAttributes": map[string]any{"connectorName": "Active Directory", "domainSettings": domains}}))
}
func domainExpectations() []v2.Expectation {
	out := []v2.Expectation{{Rule: rule("copy-name"), Selector: v2.Selector{Type: "SOURCE", Path: "/name"}, Operation: "copy_source", Authority: "invented-release"}}
	out = append(out, v2.Expectation{Rule: rule("copy-connector"), Selector: v2.Selector{Type: "SOURCE", Path: "/connectorAttributes/connectorName"}, Operation: "copy_source", Authority: "invented-release"})
	for _, field := range []string{"forestName", "domainDN", "useSSL", "port", "authorizationType", "user", "servers"} {
		out = append(out, v2.Expectation{Rule: rule("copy-domain-" + field), Selector: v2.Selector{Type: "SOURCE", Path: "/connectorAttributes/domainSettings/*/" + field}, Operation: "copy_source", Authority: "invented-release"})
	}
	return out
}
func TestSourceActiveDirectoryDomainCompositeAlignment(t *testing.T) {
	// Each component repeats independently; only the complete tuple is unique.
	a := domainEntry("forest-a", "DC=one")
	b := domainEntry("forest-a", "DC=two")
	c := domainEntry("forest-b", "DC=one")
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				p := v2.Policy{Version: 2, ID: "invented", Profile: profile}
				req := Request{Mode: mode, IncludeMatching: true, Left: domainInput(t, "dev", "SOURCE", []any{a, b, c}), Right: domainInput(t, "prod", "SOURCE", []any{c, a, b})}
				left, right := "left", "right"
				if mode == "deployment" {
					left, right = "source", "actual"
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented Directory", Action: "update"}}
					p.Expectations = domainExpectations()
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("copy-envelope-id"), Selector: v2.Selector{Type: "SOURCE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage["isc.SOURCE.ad-domains"] != 1 {
					t.Fatal(r.Counts)
				}
				found := false
				for _, d := range r.Decisions {
					if d.Locations[left].Pointer != "/connectorAttributes/domainSettings/0/port" {
						continue
					}
					found = true
					loc := d.Locations[right]
					if loc.Pointer != "/connectorAttributes/domainSettings/1/port" || len(loc.Context) != 1 || loc.Context[0].Label != "Domain" || contextFieldValue(loc.Context[0], "Forest name") != "forest-a" || contextFieldValue(loc.Context[0], "Domain DN") != "DC=one" {
						t.Fatal(d)
					}
					if mode == "deployment" && (d.Derivation == nil || len(d.Derivation.Location.Context) != 1) {
						t.Fatal("derivation context absent", d)
					}
				}
				if !found {
					t.Fatal("domain setting decision absent")
				}
			})
		}
	}
}
func TestSourceDomainValidationAndBoundaries(t *testing.T) {
	for _, mode := range []string{"drift", "deployment"} {
		p := portable()
		req := Request{Mode: mode, Left: domainInput(t, "dev", "SOURCE", []any{domainEntry("forest-a", "DC=one")})}
		if mode == "deployment" {
			p.Expectations = domainExpectations()
			req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented Directory", Action: "update"}}
		}
		for _, field := range []string{"forestName", "domainDN"} {
			for _, bad := range []any{nil, "", " ", 42} {
				domain := domainEntry("forest-a", "DC=one")
				domain[field] = bad
				req.Right = domainInput(t, "prod", "SOURCE", []any{domain})
				if r := compareTest(t, req, p); r.ExitCode() != 2 || r.Counts["conflict"] == 0 {
					t.Fatal("invalid domain key accepted", mode, field, bad, r.Counts)
				}
			}
			domain := domainEntry("forest-a", "DC=one")
			delete(domain, field)
			req.Right = domainInput(t, "prod", "SOURCE", []any{domain})
			if r := compareTest(t, req, p); r.ExitCode() != 2 {
				t.Fatal("missing domain key accepted", mode, field)
			}
		}
		req.Right = domainInput(t, "prod", "SOURCE", []any{domainEntry("forest-a", "DC=one"), domainEntry("forest-a", "DC=one")})
		if r := compareTest(t, req, p); r.ExitCode() != 2 || r.Counts["conflict"] == 0 {
			t.Fatal("duplicate tuple accepted", mode, r.Counts)
		}
	}
	left := []any{domainEntry("forest-a", "DC=one")}
	req := Request{Mode: "drift", Left: domainInput(t, "dev", "SOURCE", left)}
	for _, domains := range [][]any{{}, {domainEntry("forest-a", "DC=one"), domainEntry("forest-a", "DC=two")}} {
		req.Right = domainInput(t, "prod", "SOURCE", domains)
		if r := compareTest(t, req, portable()); r.ExitCode() != 1 {
			t.Fatal("membership change hidden", r.Counts)
		}
	}
	right := domainEntry("forest-a", "DC=one")
	right["servers"] = []any{"second.example.invalid", "first.example.invalid"}
	req.Right = domainInput(t, "prod", "SOURCE", []any{right})
	if r := compareTest(t, req, portable()); r.ExitCode() != 1 || r.Counts["different"] != 2 {
		t.Fatal("server priority lost", r.Counts)
	}
	req.Left = domainInput(t, "dev", "ROLE", []any{domainEntry("a", "DC=one"), domainEntry("b", "DC=two")})
	req.Right = domainInput(t, "prod", "ROLE", []any{domainEntry("b", "DC=two"), domainEntry("a", "DC=one")})
	if r := compareTest(t, req, portable()); r.ExitCode() != 1 || r.RuleUsage["isc.SOURCE.ad-domains"] != 0 {
		t.Fatal("domain rules leaked to other types", r.Counts)
	}
}
func TestDocumentedDomainMappingAndContext(t *testing.T) {
	data, err := os.ReadFile("examples/policy-drift.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := v2.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	left := []any{domainEntry("dev.example.invalid", "DC=dev,DC=example,DC=invalid"), domainEntry("shared.example.invalid", "DC=shared")}
	right := []any{domainEntry("shared.example.invalid", "DC=shared"), domainEntry("prod.example.invalid", "DC=prod,DC=example,DC=invalid")}
	req := Request{Mode: "drift", Left: domainInput(t, "dev", "SOURCE", left), Right: domainInput(t, "prod", "SOURCE", right)}
	r := compareTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 2 || r.RuleUsage["directory-domains"] != 1 {
		t.Fatal("example mapping failed", r.Counts, r.Replacements)
	}
	for _, role := range []string{"left", "right"} {
		found := false
		for _, d := range r.Decisions {
			if d.RuleID != "directory-domain-domainDN" {
				continue
			}
			found = true
			loc := d.Locations[role]
			index := "0"
			forest := "dev.example.invalid"
			dn := "DC=dev,DC=example,DC=invalid"
			if role == "right" {
				index = "1"
				forest = "prod.example.invalid"
				dn = "DC=prod,DC=example,DC=invalid"
			}
			if loc.Pointer != "/connectorAttributes/domainSettings/"+index+"/domainDN" || len(loc.Context) != 1 || contextFieldValue(loc.Context[0], "Forest name") != forest || contextFieldValue(loc.Context[0], "Domain DN") != dn {
				t.Fatal("incorrect domain context", d)
			}
		}
		if !found {
			t.Fatal("domain equivalence absent")
		}
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"Domain DN", "Forest name", "DC=dev,DC=example,DC=invalid", "/connectorAttributes/domainSettings/1/domainDN", "directory-domains"} {
			if !strings.Contains(b.String(), text) {
				t.Fatal("missing domain report context", text)
			}
		}
	}
	p.Equivalences = nil
	if r = compareTest(t, req, p); r.Counts["different"] != 2 {
		t.Fatal("key mapping accepted different values", r.Counts)
	}
	p.Expectations = domainExpectations()
	req.Mode = "deployment"
	req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented Directory", Action: "update"}}
	req.Left = domainInput(t, "dev", "SOURCE", left[:1])
	req.Right = domainInput(t, "prod", "SOURCE", right[1:])
	if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts["violation"] != 2 {
		t.Fatal("mapping changed copy_source authority", r.Counts)
	}
	for i := range p.Expectations {
		e := &p.Expectations[i]
		var value string
		switch e.ID {
		case "copy-domain-forestName":
			value = "prod.example.invalid"
		case "copy-domain-domainDN":
			value = "DC=prod,DC=example,DC=invalid"
		default:
			continue
		}
		raw, _ := json.Marshal(value)
		e.Operation = "literal"
		e.Expected = &v2.State{State: "present", Value: raw}
	}
	if r = compareTest(t, req, p); r.ExitCode() != 0 {
		t.Fatal("destination assertions failed", r.Counts)
	}
	req.ExcludeValues = true
	r = compareTest(t, req, p)
	data, _ = json.Marshal(r)
	if strings.Contains(string(data), "domain-user-sentinel") || !strings.Contains(string(data), "DC=prod,DC=example,DC=invalid") {
		t.Fatal("domain omission behavior changed")
	}
	p.Sensitive = []v2.Sensitive{{Rule: rule("hide-domain"), Selector: v2.Selector{Type: "SOURCE", Path: "/connectorAttributes/domainSettings"}}}
	r = compareTest(t, req, p)
	data, _ = json.Marshal(r)
	if strings.Contains(string(data), "example.invalid") || strings.Contains(string(data), "DC=prod") {
		t.Fatal("sensitive domain identifiers leaked")
	}
}
