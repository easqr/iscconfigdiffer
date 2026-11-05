package v2

import (
	"fmt"
	"sort"
)

func validateCollectionExtensions(p Policy, collections map[string]Collection, coactive func(Rule, Rule) bool) error {
	for i, extension := range p.CollectionMappings {
		r, ok := collections[extension.Collection]
		if !ok || r.Mode != "keyed" {
			return ruleError(fmt.Errorf("collection mapping %s requires an effective keyed collection", extension.ID), extension.ID, extension.Collection)
		}
		if extension.Replaces != "" || len(extension.KeyMappings) == 0 || r.Key != "" && extension.Key != "" && r.Key != extension.Key {
			return ruleError(fmt.Errorf("invalid collection mapping extension %s", extension.ID), extension.ID, extension.Collection)
		}
		// Extension modes may restrict the collection, never reactivate it.
		for _, mode := range []string{"drift", "deployment"} {
			if (len(p.Modes) == 0 || contains(p.Modes, mode)) && active(extension.Rule, mode) && !active(r.Rule, mode) {
				return ruleError(fmt.Errorf("collection mapping %s exceeds collection modes", extension.ID), extension.ID, extension.Collection)
			}
		}
		for _, previous := range p.CollectionMappings[:i] {
			if previous.Collection == extension.Collection && (previous.Key == "" || extension.Key == "" || previous.Key == extension.Key) && coactive(previous.Rule, extension.Rule) {
				return ruleError(fmt.Errorf("overlapping collection mapping extensions: %s %s", previous.ID, extension.ID), previous.ID, extension.ID, extension.Collection)
			}
		}
		r.KeyMappings = append(append([]CollectionKeyMapping{}, r.KeyMappings...), extension.KeyMappings...)
		if err := validateCollectionKeys(r); err != nil {
			return ruleError(fmt.Errorf("collection mapping %s: %w", extension.ID, err), extension.ID, extension.Collection)
		}
		sort.Slice(extension.KeyMappings, func(i, j int) bool { return extension.KeyMappings[i].Path < extension.KeyMappings[j].Path })
	}
	return nil
}

func (x *execution) extendCollection(f frame, r Collection) (frame, Collection) {
	for _, extension := range x.c.policy.CollectionMappings {
		if extension.Collection != r.ID || !active(extension.Rule, x.request.Mode) || extension.Key != "" && extension.Key != x.current.key {
			continue
		}
		r.KeyMappings = append(append([]CollectionKeyMapping{}, r.KeyMappings...), extension.KeyMappings...)
		f.extensions = append(append([]CollectionMappingUsage{}, f.extensions...), CollectionMappingUsage{RuleID: extension.ID, Collection: r.ID, Evidence: extension.Evidence})
		x.report.RuleUsage[extension.ID]++
	}
	return f, r
}
