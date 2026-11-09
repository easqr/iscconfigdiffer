package differ

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

const sourcePriorityRule = "isc.IDENTITY_OBJECT_CONFIG.attribute-source-priority"
const profilePriorityRule = "isc.IDENTITY_PROFILE.priority-order"

func priorityInput(t *testing.T, env string, priorities []any, order []string, alter func([]map[string]any)) snapshot.Input {
	t.Helper()
	objects := []map[string]any{}
	for i, name := range []string{"A", "B", "C"} {
		if i >= len(priorities) {
			continue
		}
		objects = append(objects, obj("IDENTITY_PROFILE", "profile-"+name, name, map[string]any{"priority": priorities[i], "authoritativeSource": map[string]any{"id": "source-" + name, "name": name, "type": "SOURCE"}}))
	}
	sources := []any{}
	for _, name := range order {
		sources = append(sources, map[string]any{"applicationRef": map[string]any{"id": "source-" + name, "name": name, "type": "SOURCE"}, "ruleRef": map[string]any{"id": env + "-rule", "name": "Promote", "type": "RULE"}})
	}
	objects = append(objects, obj("IDENTITY_OBJECT_CONFIG", "same-id", "Identity", map[string]any{"objectAttributeDetails": []any{map[string]any{"name": "manager", "attributeSources": sources}}}))
	if alter != nil {
		alter(objects)
	}
	return input(t, env, []string{"IDENTITY_OBJECT_CONFIG", "IDENTITY_PROFILE"}, objects...)
}

func TestISCPriorityOrder(t *testing.T) {
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		t.Run(profile, func(t *testing.T) {
			p := v2.Policy{Version: 2, ID: "priority", Profile: profile}
			req := Request{Mode: "drift", Left: priorityInput(t, "dev", []any{5, 10, 40}, []string{"C", "A", "B"}, nil), Right: priorityInput(t, "prod", []any{10, 20, 30}, []string{"B", "C", "A"}, nil), Scope: []string{"IDENTITY_OBJECT_CONFIG"}}
			r := compareTest(t, req, p)
			if r.ExitCode() != 0 || r.RuleUsage[sourcePriorityRule] == 0 {
				t.Fatal("numeric spacing or export order caused drift", r.Counts)
			}
			if r.ObjectCoverage["compared_groups"] != 1 {
				t.Fatal("context profiles leaked into scope", r.ObjectCoverage)
			}
			// A changed name/type/value still compares at the original input indices.
			req.Right = priorityInput(t, "prod", []any{10, 20, 30}, []string{"B", "C", "A"}, func(objects []map[string]any) {
				objects[3]["object"].(map[string]any)["objectAttributeDetails"].([]any)[0].(map[string]any)["attributeSources"].([]any)[2].(map[string]any)["ruleRef"].(map[string]any)["name"] = "Changed"
			})
			r = compareTest(t, req, p)
			if r.ExitCode() != 1 || r.Counts["different"] != 1 {
				t.Fatal("aligned value change hidden", r.Counts)
			}
			for _, d := range r.Decisions {
				if d.Status == "different" && (d.Locations["left"].Pointer != "/objectAttributeDetails/0/attributeSources/1/ruleRef/name" || d.Locations["right"].Pointer != "/objectAttributeDetails/0/attributeSources/2/ruleRef/name") {
					t.Fatal("original pointers lost", d)
				}
			}
			req.Right = priorityInput(t, "prod", []any{20, 10, 30}, []string{"C", "A", "B"}, nil)
			req.ExcludeValues = true
			r = compareTest(t, req, p)
			if r.ExitCode() != 1 || r.Counts["different"] != 1 {
				t.Fatal("priority reversal not highlighted once", r.Counts)
			}
			found := false
			for _, d := range r.Decisions {
				if d.Code == "relative priority order differs" {
					found = true
					if d.Evidence == nil || d.Evidence.Attributes["rule_pack"] != "isc.identity-object-config" || len(d.Locations["left"].Context) < 2 || d.Locations["left"].Value != nil {
						t.Fatal("context/provenance/omission lost", d)
					}
				}
			}
			if !found {
				t.Fatal("missing ordering finding")
			}
			for _, render := range []func(io.Writer) error{r.HTML, r.Markdown} {
				var out bytes.Buffer
				if err := render(&out); err != nil {
					t.Fatal(err)
				}
				for _, s := range []string{"Priority order", "Relative precedence changed", "manager"} {
					if !strings.Contains(out.String(), s) {
						t.Fatal("order report context absent", s)
					}
				}
			}
			req.ExcludeValues = false
			req.Scope = []string{"IDENTITY_PROFILE"}
			req.Right = priorityInput(t, "prod", []any{10, 20, 30}, []string{"A", "B", "C"}, nil)
			r = compareTest(t, req, p)
			if r.ExitCode() != 0 || r.Counts["equivalent"] != 3 {
				t.Fatal("profile priority spacing caused drift", r.Counts)
			}
			req.Right = priorityInput(t, "prod", []any{20, 10, 30}, []string{"A", "B", "C"}, nil)
			r = compareTest(t, req, p)
			if r.ExitCode() != 1 || r.Counts["different"] != 2 {
				t.Fatal("profile precedence reversal hidden", r.Counts)
			}
			req.ObjectKey = "C"
			if r = compareTest(t, req, p); r.ExitCode() != 0 {
				t.Fatal("unaffected peer reported as reordered", r.Counts)
			}
			req.ObjectKey = "A"
			if r = compareTest(t, req, p); r.ExitCode() != 1 {
				t.Fatal("name filter discarded peer context", r.Counts)
			}
		})
	}
}

func TestPriorityContextFailuresAndOverrides(t *testing.T) {
	left := priorityInput(t, "dev", []any{5, 10, 40}, []string{"A", "B", "C"}, nil)
	for _, kind := range []string{"missing-profiles", "null", "missing-priority", "string", "ties", "duplicate-ref", "unresolved", "missing-item-ref"} {
		t.Run(kind, func(t *testing.T) {
			right := priorityInput(t, "prod", []any{10, 20, 30}, []string{"A", "B", "C"}, func(objects []map[string]any) {
				first := objects[0]["object"].(map[string]any)
				switch kind {
				case "null":
					first["priority"] = nil
				case "missing-priority":
					delete(first, "priority")
				case "string":
					first["priority"] = "10"
				case "ties":
					first["priority"] = 20
				case "duplicate-ref":
					objects[1]["object"].(map[string]any)["authoritativeSource"] = first["authoritativeSource"]
				case "unresolved":
					first["authoritativeSource"].(map[string]any)["id"] = "unknown"
				case "missing-item-ref":
					delete(objects[3]["object"].(map[string]any)["objectAttributeDetails"].([]any)[0].(map[string]any)["attributeSources"].([]any)[0].(map[string]any)["applicationRef"].(map[string]any), "id")
				}
			})
			if kind == "missing-profiles" {
				right = input(t, "prod", []string{"IDENTITY_OBJECT_CONFIG"}, obj("IDENTITY_OBJECT_CONFIG", "same-id", "Identity", map[string]any{"objectAttributeDetails": []any{map[string]any{"name": "manager", "attributeSources": []any{map[string]any{"applicationRef": map[string]any{"id": "source-A", "name": "A"}}, map[string]any{"applicationRef": map[string]any{"id": "source-B", "name": "B"}}}}}}))
			}
			r := compareTest(t, Request{Mode: "drift", Left: left, Right: right, Scope: []string{"IDENTITY_OBJECT_CONFIG"}}, portable())
			if r.ExitCode() != 2 {
				t.Fatal("incomplete context accepted", kind, r.Counts)
			}
		})
	}
	req := Request{Mode: "drift", Left: left, Right: priorityInput(t, "prod", []any{10, 20, 30}, []string{"B", "A", "C"}, nil), Scope: []string{"IDENTITY_OBJECT_CONFIG"}}
	p := portable()
	p.DisabledRules = []string{sourcePriorityRule}
	if r := compareTest(t, req, p); r.ExitCode() != 1 {
		t.Fatal("disable did not restore export ordering", r.Counts)
	}
	req.Scope = []string{"IDENTITY_PROFILE"}
	p.DisabledRules = []string{profilePriorityRule}
	if r := compareTest(t, req, p); r.ExitCode() != 1 || r.Counts["different"] != 3 {
		t.Fatal("disable did not restore numeric comparison", r.Counts)
	}
	// Deployment still requires authority; ordering equivalence never supplies it.
	req.Scope = []string{"IDENTITY_OBJECT_CONFIG"}
	req.Mode = "deployment"
	req.Operations = []v2.Operation{{Type: "IDENTITY_OBJECT_CONFIG", Key: "Identity", Action: "update"}}
	if r := compareTest(t, req, portable()); r.ExitCode() != 2 || r.Counts["unverified"] == 0 {
		t.Fatal("derived order authorized deployment", r.Counts)
	}
}

func TestPriorityDoesNotMutateInput(t *testing.T) {
	in := priorityInput(t, "dev", []any{5, 10, 40}, []string{"C", "A", "B"}, nil)
	before, _ := json.Marshal(in)
	compareTest(t, Request{Mode: "drift", Left: in, Right: priorityInput(t, "prod", []any{10, 20, 30}, []string{"A", "B", "C"}, nil), Scope: []string{"IDENTITY_OBJECT_CONFIG"}}, portable())
	after, _ := json.Marshal(in)
	if !bytes.Equal(before, after) {
		t.Fatal("priority normalization mutated input")
	}
}

func TestSensitivePriorityContextOutsideScope(t *testing.T) {
	p := portable()
	p.Sensitive = []v2.Sensitive{{Rule: rule("private-priority"), Selector: v2.Selector{Type: "IDENTITY_PROFILE", Path: "/priority"}}}
	r := compareTest(t, Request{Mode: "drift", Left: priorityInput(t, "dev", []any{5, 10, 40}, []string{"A", "B", "C"}, nil), Right: priorityInput(t, "prod", []any{20, 10, 30}, []string{"A", "B", "C"}, nil), Scope: []string{"IDENTITY_OBJECT_CONFIG"}}, p)
	if r.RuleUsage["private-priority"] == 0 {
		t.Fatal("context sensitivity unused")
	}
	for _, d := range r.Decisions {
		if d.Key != "[redacted]" {
			t.Fatal("derived context identity leaked", d)
		}
		for _, loc := range d.Locations {
			if loc.Context != nil || loc.Pointer != "[redacted]" || loc.Value != nil && loc.Value.State != "redacted" {
				t.Fatal("derived priority context leaked", loc)
			}
		}
	}
}

func TestPriorityRelationsAndPartialMembership(t *testing.T) {
	left := priorityInput(t, "dev", []any{5, 10, 40}, []string{"A", "B", "C"}, nil)
	req := Request{Mode: "drift", Left: left, Right: priorityInput(t, "prod", []any{5, 1, 40}, []string{"A", "B", "C"}, nil), Scope: []string{"IDENTITY_PROFILE"}, ObjectKey: "A"}
	if r := compareTest(t, req, portable()); r.ExitCode() != 1 {
		t.Fatal("unchanged numeric priority hid a precedence reversal", r.Counts)
	}
	for _, values := range [][]any{{5, 5, 40}, {nil, 10, 40}, {"5", 10, 40}} {
		req.Right = priorityInput(t, "prod", values, []string{"A", "B", "C"}, nil)
		if r := compareTest(t, req, portable()); r.ExitCode() != 2 {
			t.Fatal("invalid scalar priority context accepted", values, r.Counts)
		}
	}
	// C is absent from the right input. Its absence must not generate spurious
	// rank changes for A and B, nor hide the missing-profile finding.
	req.ObjectKey = ""
	req.Right = priorityInput(t, "prod", []any{10, 20}, []string{"A", "B"}, nil)
	r := compareTest(t, req, portable())
	if r.ExitCode() != 1 {
		t.Fatal("missing peer not reported", r.Counts)
	}
	for _, d := range r.Decisions {
		if d.Type == "IDENTITY_PROFILE" && d.Code == "relative priority order differs" {
			t.Fatal("missing peer shifted shared precedence", d)
		}
	}
	req.Scope = []string{"IDENTITY_OBJECT_CONFIG"}
	r = compareTest(t, req, portable())
	if r.ExitCode() != 1 {
		t.Fatal("removed application not reported", r.Counts)
	}
	for _, d := range r.Decisions {
		if d.Code == "relative priority order differs" {
			t.Fatal("removed application shifted shared precedence", d)
		}
	}
}
