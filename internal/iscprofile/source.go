package iscprofile

import "github.com/elianderson/iscutilities/configdiffer/v2"

func addSourceRules(profile *v2.Profile, rule func(string) v2.Rule) {
	addSourceSchemaRules(profile, rule)
	addSourceProvisioningRules(profile, rule)
}
