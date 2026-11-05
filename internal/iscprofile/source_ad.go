package iscprofile

import "github.com/elianderson/iscutilities/configdiffer/v2"

// Active Directory connector configuration lives under SOURCE connectorAttributes.
// No server, credential, forest-name, or domain-DN equivalence is inferred here.
func addSourceActiveDirectoryRules(profile *v2.Profile, rule func(string) v2.Rule) {
	profile.Rules.Collections = append(profile.Rules.Collections, v2.Collection{
		Rule:          rule("isc.SOURCE.ad-forests"),
		Selector:      v2.Selector{Type: "SOURCE", Path: "/connectorAttributes/forestSettings"},
		Mode:          "keyed",
		Keys:          []string{"/forestName"},
		ComponentType: "string",
		ItemLabel:     "Forest",
		DisplayFields: []v2.DisplayField{{Label: "Name", Path: "/forestName"}},
	}, v2.Collection{
		Rule:          rule("isc.SOURCE.ad-domains"),
		Selector:      v2.Selector{Type: "SOURCE", Path: "/connectorAttributes/domainSettings"},
		Mode:          "keyed",
		Keys:          []string{"/forestName", "/domainDN"},
		ComponentType: "string",
		ItemLabel:     "Domain",
		DisplayFields: []v2.DisplayField{
			{Label: "Forest name", Path: "/forestName"},
			{Label: "Domain DN", Path: "/domainDN"},
		},
	})
}
