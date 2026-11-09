package differ

import (
	"strconv"
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
	"github.com/easqr/iscconfigexporter/snapshot"
)

const identityAttributesRule = "isc.IDENTITY_OBJECT_CONFIG.object-attribute-details"

func identityAttributeInput(t *testing.T, env, typ string, details []any) snapshot.Input {
	t.Helper()
	return input(t, env, []string{typ}, obj(typ, "same-id", "Invented", map[string]any{"objectAttributeDetails": details}))
}

func identityAttributePair() ([]any, []any) {
	makeItem := func(name string) any {
		return map[string]any{"name": name, "displayName": name, "attributeSources": []any{
			map[string]any{"ruleRef": map[string]any{"id": "first", "name": "First", "type": "RULE"}},
			map[string]any{"ruleRef": map[string]any{"id": "second", "name": "Second", "type": "RULE"}},
		}}
	}
	return []any{makeItem("email"), makeItem("phone")}, []any{makeItem("phone"), makeItem("email")}
}

func TestIdentityObjectConfigNamedAttributes(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				left, right := identityAttributePair()
				p := v2.Policy{Version: 2, ID: "identity-attributes", Profile: profile}
				req := Request{Mode: mode, Left: identityAttributeInput(t, "dev", "IDENTITY_OBJECT_CONFIG", left), Right: identityAttributeInput(t, "prod", "IDENTITY_OBJECT_CONFIG", right)}
				status, role, other := "different", "left", "right"
				if mode == "deployment" {
					status, role, other = "violation", "source", "actual"
					req.Operations = []v2.Operation{{Type: "IDENTITY_OBJECT_CONFIG", Key: "Invented", Action: "update"}}
					for i, path := range []string{"/name", "/objectAttributeDetails/*/name", "/objectAttributeDetails/*/displayName", "/objectAttributeDetails/*/attributeSources/*/ruleRef/name", "/objectAttributeDetails/*/attributeSources/*/ruleRef/type"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("copy-" + strconv.Itoa(i)), Selector: v2.Selector{Type: "IDENTITY_OBJECT_CONFIG", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "IDENTITY_OBJECT_CONFIG", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage[identityAttributesRule] != 1 {
					t.Fatal("ordering caused differences", r.Counts)
				}
				right[1].(map[string]any)["displayName"] = "Changed"
				req.Right = identityAttributeInput(t, "prod", "IDENTITY_OBJECT_CONFIG", right)
				r = compareTest(t, req, p)
				if r.ExitCode() != 1 || r.Counts[status] != 1 {
					t.Fatal("value change hidden", r.Counts)
				}
				found := false
				for _, d := range r.Decisions {
					if d.RuleID == identityAttributesRule && (d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.identity-object-config") {
						t.Fatal("missing provenance", d)
					}
					if d.Status == status {
						found = true
						l, rr := d.Locations[role], d.Locations[other]
						if l.Pointer != "/objectAttributeDetails/0/displayName" || rr.Pointer != "/objectAttributeDetails/1/displayName" || len(l.Context) != 1 || contextFieldValue(l.Context[0], "Name") != "email" {
							t.Fatal("original indices or named context lost", d)
						}
					}
				}
				if !found {
					t.Fatal("missing difference")
				}
			})
		}
	}
}

func TestIdentityObjectConfigAttributeBoundaries(t *testing.T) {
	left, right := identityAttributePair()
	request := func(typ string, items []any) Request {
		return Request{Mode: "drift", Left: identityAttributeInput(t, "dev", typ, left), Right: identityAttributeInput(t, "prod", typ, items)}
	}
	for _, kind := range []string{"missing", "null", "blank", "number", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			_, items := identityAttributePair()
			item := items[0].(map[string]any)
			switch kind {
			case "missing":
				delete(item, "name")
			case "null":
				item["name"] = nil
			case "blank":
				item["name"] = " "
			case "number":
				item["name"] = 1
			case "duplicate":
				item["name"] = "email"
			}
			if r := compareTest(t, request("IDENTITY_OBJECT_CONFIG", items), portable()); r.ExitCode() != 2 || r.Counts["conflict"] != 1 {
				t.Fatal("invalid name accepted", r.Counts)
			}
		})
	}
	p := portable()
	p.DisabledRules = []string{identityAttributesRule}
	if r := compareTest(t, request("IDENTITY_OBJECT_CONFIG", right), p); r.ExitCode() != 1 || r.RuleUsage[identityAttributesRule] != 0 {
		t.Fatal("disable ignored", r.Counts)
	}
	if r := compareTest(t, request("ROLE", right), portable()); r.ExitCode() != 1 || r.RuleUsage[identityAttributesRule] != 0 {
		t.Fatal("rule escaped type scope", r.Counts)
	}
	for _, name := range []string{"EMAIL", "new-name"} {
		_, items := identityAttributePair()
		items[1].(map[string]any)["name"] = name
		if r := compareTest(t, request("IDENTITY_OBJECT_CONFIG", items), portable()); r.ExitCode() != 1 {
			t.Fatal("name change hidden", r.Counts)
		}
	}
	_, right = identityAttributePair()
	sources := right[1].(map[string]any)["attributeSources"].([]any)
	sources[0], sources[1] = sources[1], sources[0]
	if r := compareTest(t, request("IDENTITY_OBJECT_CONFIG", right), portable()); r.ExitCode() != 1 {
		t.Fatal("nested source ordering hidden", r.Counts)
	}
	req := request("IDENTITY_OBJECT_CONFIG", left)
	req.Mode = "deployment"
	req.Operations = []v2.Operation{{Type: "IDENTITY_OBJECT_CONFIG", Key: "Invented", Action: "update"}}
	if r := compareTest(t, req, portable()); r.ExitCode() != 2 || r.Counts["unverified"] == 0 {
		t.Fatal("alignment granted deployment authority", r.Counts)
	}
}
