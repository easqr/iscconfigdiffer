package differ

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func contextFieldValue(context v2.ItemContext, label string) string {
	for _, field := range context.Fields {
		if field.Label == label {
			return field.Value
		}
	}
	return ""
}
func TestSourceProvisioningContextPerInput(t *testing.T) {
	for _, mode := range []string{"drift", "deployment"} {
		for _, omit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/omit=%t", mode, omit), func(t *testing.T) {
				left, right := provisioningFixture("dev", false), provisioningFixture("prod", true)
				for _, item := range []struct {
					policy map[string]any
					field  int
					value  string
				}{{left[0].(map[string]any), 0, "dev-ou-sentinel"}, {right[3].(map[string]any), 1, "prod-ou-sentinel"}} {
					attrs := item.policy["fields"].([]any)[item.field].(map[string]any)["transform"].(map[string]any)["attributes"].(map[string]any)
					attrs["calculateOu"] = map[string]any{"attributes": map[string]any{"values": []any{"first", item.value}}}
				}
				right[3].(map[string]any)["name"] = "Production account policy"
				req := Request{Mode: mode, Left: provisioningInput(t, "dev", "SOURCE", left), Right: provisioningInput(t, "prod", "SOURCE", right), ExcludeValues: omit, IncludeMatching: true}
				p := portable()
				if mode == "deployment" {
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented", Action: "update"}}
					p.Expectations = provisioningExpectations()
				}
				report := compareTest(t, req, p)
				if report.ExitCode() != 1 {
					t.Fatal(report)
				}
				lrole, rrole := "left", "right"
				suffix := "/transform/attributes/calculateOu/attributes/values/1"
				if mode == "deployment" {
					lrole, rrole = "source", "actual"
					suffix = "/transform"
				}
				found := false
				for _, d := range report.Decisions {
					if d.Locations[lrole].Pointer != "/provisioningPolicies/0/fields/0"+suffix {
						continue
					}
					found = true
					l, r := d.Locations[lrole], d.Locations[rrole]
					if r.Pointer != "/provisioningPolicies/3/fields/1"+suffix {
						t.Fatal("input index rewritten", d)
					}
					if len(l.Context) != 2 || len(r.Context) != 2 {
						t.Fatal("containing context missing", d)
					}
					if contextFieldValue(l.Context[0], "Name") != "Policy 0" || contextFieldValue(r.Context[0], "Name") != "Production account policy" || contextFieldValue(l.Context[0], "Usage type") != "CREATE" || contextFieldValue(l.Context[1], "Name") != "username" {
						t.Fatal("wrong input context", d)
					}
					if l.Context[0].Pointer != "/provisioningPolicies/0" || r.Context[1].Pointer != "/provisioningPolicies/3/fields/1" {
						t.Fatal("context pointer wrong", d)
					}
					if mode == "deployment" && (d.Derivation == nil || len(d.Derivation.Location.Context) != 2 || contextFieldValue(d.Derivation.Location.Context[0], "Name") != "Policy 0") {
						t.Fatal("derivation context missing", d)
					}
				}
				if !found {
					t.Fatal("target decision absent", report)
				}
				for _, render := range []func(io.Writer) error{report.HTML, report.Markdown} {
					var b bytes.Buffer
					if err := render(&b); err != nil {
						t.Fatal(err)
					}
					for _, text := range []string{"Provisioning policy", "Policy 0", "Production account policy", "Usage type", "CREATE", "username", "/provisioningPolicies/3/fields/1"} {
						if !strings.Contains(b.String(), text) {
							t.Fatal("human context absent", text)
						}
					}
				}
				data, _ := json.Marshal(report)
				if omit && (strings.Contains(string(data), "dev-ou-sentinel") || strings.Contains(string(data), "prod-ou-sentinel")) {
					t.Fatal("payload value leaked with omission")
				}
				// Context identifiers must follow the same conservative sensitive redaction.
				p.Sensitive = []v2.Sensitive{{Rule: rule("private-context"), Selector: v2.Selector{Type: "SOURCE", Path: "/provisioningPolicies/*/fields/*/name"}}}
				redacted := compareTest(t, req, p)
				data, _ = json.Marshal(redacted)
				for _, name := range []string{"Policy 0", "Production account policy", "username"} {
					if strings.Contains(string(data), name) {
						t.Fatal("sensitive context leaked", name)
					}
				}
				for _, d := range redacted.Decisions {
					for _, loc := range d.Locations {
						if len(loc.Context) > 0 {
							t.Fatal("sensitive context retained")
						}
					}
					if d.Derivation != nil && len(d.Derivation.Location.Context) > 0 {
						t.Fatal("sensitive derivation context retained")
					}
				}
			})
		}
	}
}

func TestMissingProvisioningFieldHasNoBorrowedContext(t *testing.T) {
	right := provisioningFixture("prod", true)
	right[3].(map[string]any)["fields"] = right[3].(map[string]any)["fields"].([]any)[:1] // enabled remains; username is absent.
	report := compareTest(t, Request{Mode: "drift", Left: provisioningInput(t, "dev", "SOURCE", provisioningFixture("dev", false)), Right: provisioningInput(t, "prod", "SOURCE", right)}, portable())
	found := false
	for _, d := range report.Decisions {
		if d.Locations["left"].Pointer != "/provisioningPolicies/0/fields/0/name" {
			continue
		}
		found = true
		l, r := d.Locations["left"], d.Locations["right"]
		if len(l.Context) != 2 || len(r.Context) != 1 || r.Present || contextFieldValue(l.Context[1], "Name") != "username" {
			t.Fatal("missing field context misrepresented", d)
		}
	}
	if !found {
		t.Fatal("missing-field decision absent")
	}
}

func TestMachineProvisioningContextIncludesSubtype(t *testing.T) {
	r := compareTest(t, Request{Mode: "drift", Left: provisioningInput(t, "dev", "SOURCE", provisioningFixture("dev", false)), Right: provisioningInput(t, "prod", "SOURCE", provisioningFixture("prod", true)), IncludeMatching: true}, portable())
	found := false
	for _, d := range r.Decisions {
		if d.Locations["left"].Pointer == "/provisioningPolicies/2/usageType" {
			found = true
			for _, role := range []string{"left", "right"} {
				c := d.Locations[role].Context
				if len(c) != 1 || contextFieldValue(c[0], "Usage type") != "CREATE_MACHINE_ACCOUNT" || contextFieldValue(c[0], "Subtype ID") != "subtype-2" {
					t.Fatal(d)
				}
			}
		}
	}
	if !found {
		t.Fatal("machine policy context missing")
	}
}
