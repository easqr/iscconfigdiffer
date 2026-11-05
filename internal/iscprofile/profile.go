// Package iscprofile declares ISC comparison semantics without implementing a differ.
package iscprofile

import (
	"errors"
	"sort"
	"strings"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

func Build(p v2.Policy) (v2.Profile, error) {
	if p.Profile != "isc-identity-v2" && p.Profile != "isc-envelope-v2" && p.Profile != "isc-portable-v2" {
		return v2.Profile{}, errors.New("unknown ISC v2 profile")
	}
	profile := v2.Profile{ID: p.Profile, Rules: v2.Policy{NullHandling: "null_equals_absent"}}
	rule := func(id string) v2.Rule {
		return v2.Rule{ID: id, Evidence: &v2.Evidence{Kind: "isc-profile", RecordID: p.Profile}}
	}
	types := map[string]bool{}
	addIdentity := func(typ, strategy string, paths ...string) {
		types[typ] = true
		componentType := ""
		if len(paths) > 0 {
			componentType = "string"
		}
		profile.Rules.Identities = append(profile.Rules.Identities, v2.Identity{Rule: rule("isc.identity." + typ), Type: typ, Strategy: strategy, Paths: paths, ComponentType: componentType})
	}
	for _, typ := range namedTypes {
		addIdentity(typ, "fields", "/name")
	}
	for _, typ := range []string{"ACCESS_REQUEST_CONFIG", "AUTH_ORG"} {
		addIdentity(typ, "singleton")
	}
	addIdentity("NOTIFICATION_TEMPLATE", "fields", "/key", "/locale", "/medium")
	for _, r := range []v2.Identity{{Rule: rule("isc.identity.LIFECYCLE_STATE"), Type: "LIFECYCLE_STATE", Strategy: "dependent", Paths: []string{"/technicalName"}, ComponentType: "string", ParentType: "IDENTITY_PROFILE", ParentPath: "/identityProfileRef/id"}, {Rule: rule("isc.identity.ATTR_SYNC_SOURCE_CONFIG"), Type: "ATTR_SYNC_SOURCE_CONFIG", Strategy: "dependent", ParentType: "SOURCE", ParentPath: "/source/id"}} {
		types[r.Type] = true
		profile.Rules.Identities = append(profile.Rules.Identities, r)
	}
	for _, r := range p.Identities {
		types[r.Type] = true
	}
	ignore := func(typ, sp, path, label string) {
		profile.Rules.Ignores = append(profile.Rules.Ignores, v2.Ignore{Rule: rule("isc." + typ + "." + label), Selector: v2.Selector{Type: typ, Space: sp, Path: path}})
	}
	for _, typ := range sortedKeys(types) {
		if p.Profile != "isc-identity-v2" {
			ignore(typ, "metadata", "/jwsHeader", "jws-header")
			ignore(typ, "metadata", "/jwsSignature", "jws-signature")
		}
		if p.Profile == "isc-portable-v2" {
			ignore(typ, "metadata", "/self/id", "physical-id")
			for _, field := range []string{"id", "created", "modified"} {
				ignore(typ, "content", "/"+field, field)
			}
			if typ == "LIFECYCLE_STATE" {
				ignore(typ, "content", "/identityCount", "identity-count")
			}
		}
		// Structural envelope expectations are explicit and deployment-only.
		for _, path := range []string{"/version", "/self/type"} {
			r := rule("isc." + typ + ".envelope" + strings.ReplaceAll(path, "/", "."))
			r.Modes = []string{"deployment"}
			profile.Rules.Expectations = append(profile.Rules.Expectations, v2.Expectation{Rule: r, Selector: v2.Selector{Type: typ, Space: "metadata", Path: path}, Operation: "copy_source", Authority: "isc-envelope-structure"})
		}
	}
	addSourceRules(&profile, rule)
	return profile, nil
}

var namedTypes = strings.Fields("ACCESS_PROFILE CAMPAIGN_FILTER CONNECTOR_RULE FORM_DEFINITION GOVERNANCE_GROUP IDENTITY_OBJECT_CONFIG IDENTITY_PROFILE PASSWORD_POLICY PASSWORD_SYNC_GROUP PUBLIC_IDENTITIES_CONFIG ROLE RULE SEGMENT SERVICE_DESK_INTEGRATION SOD_POLICY SOURCE SPS_PARAMETER TAG TRANSFORM TRIGGER_SUBSCRIPTION WORKFLOW")

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
