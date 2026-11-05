package iscprofile

import (
	"encoding/json"

	"github.com/elianderson/iscutilities/configdiffer/v2"
)

// Ordinary policies are addressed by usage. Machine creation policies are scoped
// to a subtype; their display names and tenant-generated policy IDs are not keys.
func addSourceProvisioningRules(profile *v2.Profile, rule func(string) v2.Rule) {
	policies := v2.Collection{
		Rule:          rule("isc.SOURCE.provisioning-policies"),
		Selector:      v2.Selector{Type: "SOURCE", Path: "/provisioningPolicies"},
		Mode:          "keyed",
		Keys:          []string{"/usageType"},
		ComponentType: "string",
		ItemLabel:     "Provisioning policy",
		DisplayFields: []v2.DisplayField{
			{Label: "Name", Path: "/name"},
			{Label: "Usage type", Path: "/usageType"},
			{Label: "Subtype ID", Path: "/subtypeId"},
		},
		KeyCases: []v2.CollectionKeyCase{{
			WhenPath:  "/usageType",
			WhenValue: json.RawMessage(`"CREATE_MACHINE_ACCOUNT"`),
			Keys:      []string{"/usageType", "/subtypeId"},
		}},
	}
	fields := v2.Collection{
		Rule:          rule("isc.SOURCE.provisioning-fields"),
		Selector:      v2.Selector{Type: "SOURCE", Path: "/provisioningPolicies/*/fields"},
		Mode:          "keyed",
		Keys:          []string{"/name"},
		ComponentType: "string",
		ItemLabel:     "Field",
		DisplayFields: []v2.DisplayField{{Label: "Name", Path: "/name"}},
	}
	profile.Rules.Collections = append(profile.Rules.Collections, policies, fields)
	for _, field := range []string{"id", "created", "modified"} {
		profile.Rules.Ignores = append(profile.Rules.Ignores,
			v2.Ignore{Rule: rule("isc.SOURCE.provisioning-policy-" + field), Selector: v2.Selector{Type: "SOURCE", Path: "/provisioningPolicies/*/" + field}},
			v2.Ignore{Rule: rule("isc.SOURCE.provisioning-field-" + field), Selector: v2.Selector{Type: "SOURCE", Path: "/provisioningPolicies/*/fields/*/" + field}},
		)
	}
}
