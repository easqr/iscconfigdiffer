package differ

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
	"github.com/easqr/iscconfigexporter/snapshot"
)

func nestedProvisioningInput(t *testing.T, env string, reverse, otherTemplates bool, changeTransform bool) snapshot.Input {
	t.Helper()
	policies := []any{}
	for _, usage := range []string{"CREATE", "UPDATE"} {
		fields := []any{}
		for _, name := range []string{"userPrincipalName", "displayName"} {
			value := "unchanged-template"
			if usage == "CREATE" && name == "userPrincipalName" || otherTemplates {
				value = "$uid@" + env + ".example.invalid"
			}
			transformType := "static"
			if changeTransform && usage == "CREATE" && name == "userPrincipalName" {
				transformType = "identityAttribute"
			}
			fields = append(fields, map[string]any{"name": name, "type": "string", "isRequired": false, "transform": map[string]any{"type": transformType, "attributes": map[string]any{"value": value, "uid": map[string]any{"type": "identityAttribute", "attributes": map[string]any{"name": "uid"}}}}})
		}
		if reverse {
			fields[0], fields[1] = fields[1], fields[0]
		}
		policies = append(policies, map[string]any{"usageType": usage, "name": "Invented " + usage, "fields": fields})
	}
	if reverse {
		policies[0], policies[1] = policies[1], policies[0]
	}
	return input(t, env, []string{"SOURCE"}, obj("SOURCE", "same-id", "Invented Directory", map[string]any{"provisioningPolicies": policies}))
}
func TestDocumentedCreateUPNMapping(t *testing.T) {
	data, err := os.ReadFile("examples/policy-drift.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := v2.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Mode: "drift", Left: nestedProvisioningInput(t, "dev", false, false, false), Right: nestedProvisioningInput(t, "prod", true, false, false)}
	r := compareTest(t, req, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 || r.RuleUsage["directory-create-user-principal-name"] != 1 {
		t.Fatal("CREATE/UPN approval failed", r.Counts)
	}
	found := false
	for _, d := range r.Decisions {
		if d.RuleID != "directory-create-user-principal-name" {
			continue
		}
		found = true
		if len(d.Items) != 2 || d.Items[0].Collection != "isc.SOURCE.provisioning-policies" || d.Items[1].Collection != "isc.SOURCE.provisioning-fields" || d.Locations["left"].Pointer != "/provisioningPolicies/0/fields/0/transform/attributes/value" || d.Locations["right"].Pointer != "/provisioningPolicies/1/fields/1/transform/attributes/value" || len(d.Locations["left"].Context) != 2 {
			t.Fatal("nested identities/locations missing", d)
		}
	}
	if !found {
		t.Fatal("nested approval decision absent")
	}
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"Collection item scope", "isc.SOURCE.provisioning-policies", "isc.SOURCE.provisioning-fields", "CREATE", "userPrincipalName"} {
			if !strings.Contains(b.String(), text) {
				t.Fatal("nested scope missing from human report", text)
			}
		}
	}
	// The same pair must not be accepted on another field or another policy.
	req.Left = nestedProvisioningInput(t, "dev", false, true, false)
	req.Right = nestedProvisioningInput(t, "prod", true, true, false)
	if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts["different"] != 3 || r.Counts["equivalent"] != 1 {
		t.Fatal("nested approval leaked", r.Counts)
	}
	// Approving the value must not approve the rest of the transform.
	req.Left = nestedProvisioningInput(t, "dev", false, false, false)
	req.Right = nestedProvisioningInput(t, "prod", true, false, true)
	if r = compareTest(t, req, p); r.ExitCode() != 1 || r.Counts["different"] != 1 || r.Counts["equivalent"] != 1 {
		t.Fatal("transform changes hidden", r.Counts)
	}
	req.Right = nestedProvisioningInput(t, "prod", true, false, false)
	req.ExcludeValues = true
	r = compareTest(t, req, p)
	data, _ = json.Marshal(r)
	if strings.Contains(string(data), "$uid@") {
		t.Fatal("excluded template value leaked")
	}
	p.Sensitive = []v2.Sensitive{{Rule: rule("hide-provisioning"), Selector: v2.Selector{Type: "SOURCE", Path: "/provisioningPolicies"}}}
	r = compareTest(t, req, p)
	data, _ = json.Marshal(r)
	if strings.Contains(string(data), "userPrincipalName") {
		t.Fatal("sensitive nested identity leaked")
	}
}
