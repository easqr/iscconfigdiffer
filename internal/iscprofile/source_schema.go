package iscprofile

import "github.com/elianderson/iscutilities/configdiffer/v2"

func addSourceSchemaRules(profile *v2.Profile, rule func(string) v2.Rule) {
	if profile.ID != "isc-portable-v2" {
		return
	}
	for _, field := range []string{"id", "created", "modified"} {
		profile.Rules.Ignores = append(profile.Rules.Ignores, v2.Ignore{Rule: rule("isc.SOURCE.schema-" + field), Selector: v2.Selector{Type: "SOURCE", Path: "/schemas/*/" + field}})
	}
}
