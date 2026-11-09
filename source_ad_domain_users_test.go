package differ

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
)

func multipleDomainUsers(env string) []any {
	domains := []any{}
	for _, logical := range []string{"corporate", "regional"} {
		forest := env + ".example.invalid"
		dn := "DC=" + env + ",DC=example,DC=invalid"
		suffix := forest
		if logical == "regional" {
			dn = "DC=regional," + dn
			suffix = "regional." + forest
		}
		d := domainEntry(forest, dn)
		d["user"] = "svc-" + logical + "@" + suffix
		domains = append(domains, d)
	}
	if env == "prod" {
		domains[0], domains[1] = domains[1], domains[0]
	}
	return domains
}
func TestDocumentedMultipleDomainUsers(t *testing.T) {
	data, err := os.ReadFile("examples/policy-drift.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := v2.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	left, right := multipleDomainUsers("dev"), multipleDomainUsers("prod")
	req := Request{Mode: "drift", Left: domainInput(t, "dev", "SOURCE", left), Right: domainInput(t, "prod", "SOURCE", right)}
	r := compareTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 6 {
		t.Fatal("multi-domain example failed", r.Counts)
	}
	for _, tc := range []struct{ id, dn, lp, rp string }{{"directory-corporate-domain-user", "corporate-domain", "/connectorAttributes/domainSettings/0/user", "/connectorAttributes/domainSettings/1/user"}, {"directory-regional-domain-user", "regional-domain", "/connectorAttributes/domainSettings/1/user", "/connectorAttributes/domainSettings/0/user"}} {
		found := false
		for _, d := range r.Decisions {
			if d.RuleID != tc.id {
				continue
			}
			found = true
			if d.Item == nil || d.Item.Keys["/forestName"].Mapped != "corporate-forest" || d.Item.Keys["/domainDN"].Mapped != tc.dn || d.Locations["left"].Pointer != tc.lp || d.Locations["right"].Pointer != tc.rp {
				t.Fatal("wrong domain approved", d)
			}
		}
		if !found {
			t.Fatal("missing scoped service-account approval", tc.id)
		}
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"Collection item scope", "corporate-domain", "regional-domain", "isc.SOURCE.ad-domains"} {
			if !strings.Contains(b.String(), text) {
				t.Fatal("scope provenance absent", text)
			}
		}
	}
	// Both account pairs are approved somewhere, but neither belongs in the other domain.
	for _, domains := range [][]any{left, right} {
		a, b := domains[0].(map[string]any), domains[1].(map[string]any)
		a["user"], b["user"] = b["user"], a["user"]
	}
	req.Left = domainInput(t, "dev", "SOURCE", left)
	req.Right = domainInput(t, "prod", "SOURCE", right)
	if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts["different"] != 2 || r.RuleUsage["directory-corporate-domain-user"] != 0 || r.RuleUsage["directory-regional-domain-user"] != 0 {
		t.Fatal("service-account approval leaked between domains", r.Counts)
	}
}
