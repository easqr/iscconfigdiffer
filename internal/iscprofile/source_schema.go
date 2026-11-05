package iscprofile

import "github.com/elianderson/iscutilities/configdiffer/v2"

func addSourceSchemaRules(profile *v2.Profile, rule func(string) v2.Rule) {
	profile.Rules.Collections = append(profile.Rules.Collections,
		v2.Collection{
			Rule:          rule("isc.SOURCE.schemas"),
			Selector:      v2.Selector{Type: "SOURCE", Path: "/schemas"},
			Mode:          "keyed",
			Keys:          []string{"/name"},
			ComponentType: "string",
			ItemLabel:     "Schema",
			DisplayFields: []v2.DisplayField{
				{Label: "Name", Path: "/name"},
				{Label: "Native object type", Path: "/nativeObjectType"},
			},
		},
		v2.Collection{
			Rule:          rule("isc.SOURCE.schema-attributes"),
			Selector:      v2.Selector{Type: "SOURCE", Path: "/schemas/*/attributes"},
			Mode:          "keyed",
			Keys:          []string{"/name"},
			ComponentType: "string",
			ItemLabel:     "Attribute",
			DisplayFields: []v2.DisplayField{{Label: "Name", Path: "/name"}},
		},
	)
	profile.Rules.Ignores = append(profile.Rules.Ignores, v2.Ignore{
		Rule:     rule("isc.SOURCE.schema-attribute-reference-id"),
		Selector: v2.Selector{Type: "SOURCE", Path: "/schemas/*/attributes/*/schema/id"},
	})
	if profile.ID != "isc-portable-v2" {
		return
	}
	for _, field := range []string{"id", "created", "modified"} {
		profile.Rules.Ignores = append(profile.Rules.Ignores, v2.Ignore{Rule: rule("isc.SOURCE.schema-" + field), Selector: v2.Selector{Type: "SOURCE", Path: "/schemas/*/" + field}})
	}
}
