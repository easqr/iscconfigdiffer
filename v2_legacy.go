package differ

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

// TranslateV1 produces a drift-only migration policy. Original Compare/DecodePolicy
// remain the compatibility executor and retain the exact v1 report semantics.
// Envelope-root collection/identity semantics require the legacy executor.
func TranslateV1(p Policy) (v2.Policy, error) {
	if e := p.Validate(); e != nil {
		return v2.Policy{}, e
	}
	b, e := json.Marshal(p)
	if e != nil {
		return v2.Policy{}, e
	}
	p, e = DecodePolicy(b)
	if e != nil {
		return v2.Policy{}, e
	}
	out := v2.Policy{Version: 2, ID: p.ID, Profile: "isc-identity-v2", Bindings: p.Bindings, Evidence: []v2.Evidence{{Kind: "legacy-policy", Digest: p.Digest()}}}
	out.Modes = []string{"drift"}
	switch p.Profile {
	case "isc-envelope-v1":
		out.Profile = "isc-envelope-v2"
	case "isc-portable-v1":
		out.Profile = "isc-portable-v2"
	}
	rule := func(id string, required bool) v2.Rule {
		return v2.Rule{ID: id, Required: required, Modes: []string{"drift"}, Evidence: &v2.Evidence{Kind: "legacy-policy", Digest: p.Digest(), RecordID: id}}
	}
	sel := func(s Selector) v2.Selector {
		v := v2.Selector{Type: s.Type, Key: s.Key, Space: "metadata", Path: s.Path}
		if s.Path == "/object" || strings.HasPrefix(s.Path, "/object/") {
			v.Space = "content"
			v.Path = strings.TrimPrefix(s.Path, "/object")
		}
		return v
	}
	for _, r := range p.Identities {
		paths := []string{}
		for _, path := range r.Paths {
			if path != "/object" && !strings.HasPrefix(path, "/object/") {
				return out, errors.New("metadata identities require legacy execution")
			}
			paths = append(paths, strings.TrimPrefix(path, "/object"))
		}
		rr := rule(r.ID, false)
		rr.Modes = nil
		if namedType(r.Type) || containsISCIdentity(r.Type) {
			rr.Replaces = "isc.identity." + r.Type
		}
		componentType := ""
		if len(paths) > 0 {
			componentType = "string"
		}
		out.Identities = append(out.Identities, v2.Identity{Rule: rr, Type: r.Type, Strategy: r.Strategy, Paths: paths, ComponentType: componentType})
	}
	for _, r := range p.Pairings {
		rr := rule(r.ID, false)
		rr.Modes = nil
		out.Pairings = append(out.Pairings, v2.Pairing{Rule: rr, Type: r.Type, LeftID: r.LeftID, RightID: r.RightID})
	}
	for _, r := range p.Ignores {
		out.Ignores = append(out.Ignores, v2.Ignore{Rule: rule(r.ID, r.Required), Selector: sel(r.Selector)})
	}
	for _, r := range p.Expectations {
		if r.Path == "" {
			for _, sp := range []string{"content", "metadata"} {
				values := map[string]v2.State{}
				for env, v := range r.Values {
					if m, ok := v.(map[string]any); ok {
						var want any
						if sp == "content" {
							want = m["object"]
						} else {
							meta := map[string]any{}
							for k, v := range m {
								if k != "object" {
									meta[k] = v
								}
							}
							want = meta
						}
						b, _ := json.Marshal(want)
						values[env] = v2.State{State: "present", Value: b}
					} else {
						b, _ := json.Marshal(v)
						values[env] = v2.State{State: "present", Value: b}
					}
				}
				out.Expectations = append(out.Expectations, v2.Expectation{Rule: rule(r.ID+"."+sp, r.Required), Selector: v2.Selector{Type: r.Type, Key: r.Key, Space: sp, Path: ""}, Operation: "literal", Values: values, Authority: "legacy-environment-assertion"})
			}
			continue
		}
		values := map[string]v2.State{}
		for env, v := range r.Values {
			b, _ := json.Marshal(v)
			values[env] = v2.State{State: "present", Value: b}
		}
		out.Expectations = append(out.Expectations, v2.Expectation{Rule: rule(r.ID, r.Required), Selector: sel(r.Selector), Operation: "literal", Values: values, Authority: "legacy-environment-assertion"})
	}
	for _, r := range p.References {
		if r.Path == "" {
			return out, errors.New("envelope-root reference requires legacy execution")
		}
		out.References = append(out.References, v2.Reference{Rule: rule(r.ID, r.Required), Selector: sel(r.Selector), TargetType: r.TargetType, External: r.External})
	}
	for _, r := range p.Collections {
		if r.Path == "" {
			return out, errors.New("envelope-root collection requires legacy execution")
		}
		out.Collections = append(out.Collections, v2.Collection{Rule: rule(r.ID, r.Required), Selector: sel(r.Selector), Mode: "keyed", Keys: r.Keys})
	}
	for _, r := range p.Sensitive {
		out.Sensitive = append(out.Sensitive, v2.Sensitive{Rule: rule(r.ID, false), Selector: sel(r.Selector)})
		if r.Path == "" {
			out.Sensitive = append(out.Sensitive, v2.Sensitive{Rule: rule(r.ID+".content", false), Selector: v2.Selector{Type: r.Type, Key: r.Key, Path: ""}})
		}
	}
	// Legacy Hub rules are bound to tenant context; explicit environment bindings
	// are required for a neutral equivalence policy rather than guessed.
	if len(p.Hub) > 0 {
		if p.Bindings["left"] == "" || p.Bindings["right"] == "" {
			return out, errors.New("bind left/right environments before translating Hub rules")
		}
		appendHubRules(&out, p.Hub, p.Bindings["left"], p.Bindings["right"])
	}
	_, e = CompileV2(out)
	return out, e
}
func containsISCIdentity(t string) bool {
	return t == "ACCESS_REQUEST_CONFIG" || t == "AUTH_ORG" || t == "NOTIFICATION_TEMPLATE" || t == "LIFECYCLE_STATE" || t == "ATTR_SYNC_SOURCE_CONFIG"
}
