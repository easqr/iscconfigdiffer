package differ

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

const syncAttributesRule = "isc.ATTR_SYNC_SOURCE_CONFIG.attributes"

func attributeSyncInput(t *testing.T, env string, attributes []any, sources ...string) snapshot.Input {
	t.Helper()
	if len(sources) == 0 {
		sources = []string{"Invented Source"}
	}
	objects := []map[string]any{}
	for i, name := range sources {
		id := fmt.Sprintf("source-%d", i)
		objects = append(objects, obj("SOURCE", id, name, nil), obj("ATTR_SYNC_SOURCE_CONFIG", fmt.Sprintf("sync-%d", i), "", map[string]any{"source": map[string]any{"id": id, "name": name, "type": "SOURCE"}, "attributes": attributes}))
	}
	return input(t, env, []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}, objects...)
}
func syncMapping(target, name, label string, enabled bool) map[string]any {
	return map[string]any{"target": target, "name": name, "displayName": label, "enabled": enabled}
}
func syncAttributePair() ([]any, []any) {
	return []any{syncMapping("mail", "email", "Email", true), syncMapping("phone", "phone", "Phone", false)}, []any{syncMapping("phone", "phone", "Phone", false), syncMapping("mail", "email", "Email", true)}
}

func TestAttributeSyncKeyedTargets(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		for _, mode := range []string{"drift", "deployment"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				left, right := syncAttributePair()
				p := v2.Policy{Version: 2, ID: "attribute-sync", Profile: profile}
				req := Request{Mode: mode, Left: attributeSyncInput(t, "dev", left), Right: attributeSyncInput(t, "prod", right), Scope: []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}}
				status, role := "different", "left"
				if mode == "deployment" {
					status, role = "violation", "source"
					req.Operations = []v2.Operation{{Type: "SOURCE", Key: "Invented Source", Action: "update"}, {Type: "ATTR_SYNC_SOURCE_CONFIG", Key: "Invented Source", Action: "update"}}
					p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("copy-parent-name"), Selector: v2.Selector{Type: "SOURCE", Path: "/name"}, Operation: "copy_source", Authority: "invented-release"})
					for i, path := range []string{"/source/name", "/source/type", "/attributes/*/target", "/attributes/*/name", "/attributes/*/displayName", "/attributes/*/enabled"} {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule(fmt.Sprintf("copy-%d", i)), Selector: v2.Selector{Type: "ATTR_SYNC_SOURCE_CONFIG", Path: path}, Operation: "copy_source", Authority: "invented-release"})
					}
					if profile != "isc-portable-v2" {
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("parent-envelope-id"), Selector: v2.Selector{Type: "SOURCE", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
						p.Expectations = append(p.Expectations, v2.Expectation{Rule: rule("envelope-id"), Selector: v2.Selector{Type: "ATTR_SYNC_SOURCE_CONFIG", Space: "metadata", Path: "/self/id"}, Operation: "copy_source", Authority: "invented-release"})
					}
				}
				r := compareTest(t, req, p)
				if r.ExitCode() != 0 || r.RuleUsage[syncAttributesRule] != 1 {
					t.Fatal("ordering caused drift", r.Counts)
				}
				found := false
				for _, d := range r.Decisions {
					if d.RuleID == syncAttributesRule {
						found = true
						if d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.attribute-sync" || d.Evidence.Attributes["layer"] != "type" {
							t.Fatal("pack provenance absent", d)
						}
					}
				}
				if !found {
					t.Fatal("collection alignment absent")
				}
				for _, field := range []string{"name", "displayName", "enabled"} {
					_, right = syncAttributePair()
					if field == "enabled" {
						right[1].(map[string]any)[field] = false
					} else {
						right[1].(map[string]any)[field] = "changed"
					}
					req.Right = attributeSyncInput(t, "prod", right)
					r = compareTest(t, req, p)
					if r.ExitCode() != 1 || r.Counts[status] != 1 {
						t.Fatal("aligned field mismatch hidden", field, r.Counts)
					}
					found = false
					for _, d := range r.Decisions {
						if d.Status != status {
							continue
						}
						found = true
						if d.Locations[role].Pointer != "/attributes/0/"+field {
							t.Fatal("left/source index lost", d)
						}
						other := "right"
						if mode == "deployment" {
							other = "actual"
						}
						if d.Locations[other].Pointer != "/attributes/1/"+field || len(d.Locations[role].Context) != 1 || contextFieldValue(d.Locations[role].Context[0], "Target") != "mail" {
							t.Fatal("per-input target context lost", d)
						}
					}
					if !found {
						t.Fatal("changed field decision absent")
					}
				}
			})
		}
	}
}

func TestAttributeSyncInvalidTargets(t *testing.T) {
	for _, kind := range []string{"missing", "null", "blank", "number", "boolean", "object", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			left, right := syncAttributePair()
			m := right[0].(map[string]any)
			switch kind {
			case "missing":
				delete(m, "target")
			case "null":
				m["target"] = nil
			case "blank":
				m["target"] = " "
			case "number":
				m["target"] = 1
			case "boolean":
				m["target"] = false
			case "object":
				m["target"] = map[string]any{"name": "phone"}
			case "duplicate":
				m["target"] = "mail"
			}
			r := compareTest(t, Request{Mode: "drift", Left: attributeSyncInput(t, "dev", left), Right: attributeSyncInput(t, "prod", right), Scope: []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}}, portable())
			if r.ExitCode() != 2 || r.Counts["conflict"] != 1 {
				t.Fatal("invalid target accepted", kind, r.Counts)
			}
		})
	}
}

func TestAttributeSyncTargetsStaySourceScoped(t *testing.T) {
	left, right := syncAttributePair()
	req := Request{Mode: "drift", Left: attributeSyncInput(t, "dev", left, "One", "Two"), Right: attributeSyncInput(t, "prod", right, "One", "Two"), Scope: []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}}
	r := compareTest(t, req, portable())
	if r.ExitCode() != 0 || r.RuleUsage[syncAttributesRule] != 2 {
		t.Fatal("target uniqueness escaped parent config", r.Counts)
	}
	for _, target := range []string{"MAIL", "new-target"} {
		_, changed := syncAttributePair()
		changed[1].(map[string]any)["target"] = target
		req.Right = attributeSyncInput(t, "prod", changed, "One", "Two")
		r = compareTest(t, req, portable())
		if r.ExitCode() != 1 || r.Counts["different"] == 0 {
			t.Fatal("target change/absence hidden", r.Counts)
		}
	}
	// Unverified deployment remains incomplete: alignment supplies no authority.
	req.Mode = "deployment"
	req.Operations = []v2.Operation{{Type: "ATTR_SYNC_SOURCE_CONFIG", Key: "One", Action: "update"}}
	if r = compareTest(t, req, portable()); r.ExitCode() != 2 || r.Counts["unverified"] == 0 {
		t.Fatal("alignment granted deployment authority", r.Counts)
	}
	found := false
	for _, d := range r.Decisions {
		if d.Type == "ATTR_SYNC_SOURCE_CONFIG" && d.Key == "One" && d.Status == "unverified" && strings.HasPrefix(d.Locations["source"].Pointer, "/attributes/") {
			found = true
		}
	}
	if !found {
		t.Fatal("attribute checks were not independently unverified")
	}
}

func TestAttributeSyncCollectionScopeAndOverrides(t *testing.T) {
	left, right := syncAttributePair()
	p := portable()
	req := Request{Mode: "drift", Left: attributeSyncInput(t, "dev", left), Right: attributeSyncInput(t, "prod", right), Scope: []string{"SOURCE", "ATTR_SYNC_SOURCE_CONFIG"}}
	p.DisabledRules = []string{syncAttributesRule}
	if r := compareTest(t, req, p); r.ExitCode() != 1 || r.RuleUsage[syncAttributesRule] != 0 {
		t.Fatal("disable did not restore ordering", r.Counts)
	}
	// Identical fields on other object types retain ordered semantics.
	r := compareTest(t, Request{Mode: "drift", Left: input(t, "dev", []string{"ROLE"}, obj("ROLE", "one", "Invented", map[string]any{"attributes": left})), Right: input(t, "prod", []string{"ROLE"}, obj("ROLE", "one", "Invented", map[string]any{"attributes": right}))}, portable())
	if r.ExitCode() != 1 || r.RuleUsage[syncAttributesRule] != 0 {
		t.Fatal("collection escaped its type scope", r.Counts)
	}
	// Render a reordered change from captured context, including with omitted values.
	right[1].(map[string]any)["displayName"] = "changed"
	req.Right = attributeSyncInput(t, "prod", right)
	req.ExcludeValues = true
	r = compareTest(t, req, portable())
	before, _ := json.Marshal(r)
	for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"Attribute sync mapping", "Target", "mail", "/attributes/0/displayName", "/attributes/1/displayName"} {
			if !strings.Contains(b.String(), text) {
				t.Fatal("report context absent", text)
			}
		}
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("rendering changed structured report")
	}
}
