package iscprofile

import "github.com/elianderson/iscutilities/configdiffer/v2"

func addSourceRules(profile *v2.Profile, rule func(string) v2.Rule) {
	profile.Rules.Ignores = append(profile.Rules.Ignores, v2.Ignore{
		Rule:     rule("isc.SOURCE.before-provisioning-rule-id"),
		Selector: v2.Selector{Type: "SOURCE", Path: "/beforeProvisioningRule/id"},
	})
	addSourceSchemaRules(profile, rule)
	addSourceProvisioningRules(profile, rule)
	addSourceActiveDirectoryRules(profile, rule)
}
