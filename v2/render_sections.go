package v2

import (
	"strconv"
	"strings"
)

func markCollapsedValues(d *humanDecision) {
	d.HasCollapsedValues = d.WholeSchema
	for _, loc := range d.Locations {
		d.HasCollapsedValues = d.HasCollapsedValues || loc.CollapseValue
	}
	for i := range d.Children {
		markCollapsedValues(&d.Children[i])
	}
}

// Sections follow captured content paths. Classification never reads input
// documents, configuration values, or matching-rule IDs.
func decisionCategory(d Decision) string {
	if space(d.Space) == "metadata" {
		return "metadata"
	}
	category := ""
	for _, loc := range d.Locations {
		parts, err := pointer(loc.Pointer)
		if err != nil || len(parts) == 0 {
			return "other"
		}
		current := "other"
		switch parts[0] {
		case "connectorAttributes":
			current = "connection"
		case "schemas":
			current = "schemas"
		case "provisioningPolicies":
			current = "provisioning"
		case "identityAttributeConfig":
			if len(parts) > 1 && parts[1] == "attributeTransforms" {
				current = "identity-transforms"
			}
		}
		if category != "" && category != current {
			return "other"
		}
		category = current
	}
	if category == "" {
		return "other"
	}
	return category
}

func decisionStatuses(d humanDecision) string {
	statuses := map[string]bool{d.Status: true}
	var add func(humanDecision)
	add = func(child humanDecision) {
		statuses[child.Status] = true
		for _, nested := range child.Children {
			add(nested)
		}
	}
	for _, child := range d.Children {
		add(child)
	}
	return strings.Join(keys(statuses), " ")
}

func decisionSections(g humanGroup) []humanSection {
	out := []humanSection{}
	for _, section := range []struct{ id, title string }{{"connection", "Connection settings"}, {"schemas", "Schemas"}, {"provisioning", "Provisioning policies"}, {"identity-transforms", "Identity attribute transforms"}, {"other", "Other settings"}, {"metadata", "Export metadata"}} {
		if section.id == "identity-transforms" {
			out = append(out, identityTransformSections(g)...)
			continue
		}
		s := humanSection{ID: g.ID + "-" + section.id, Category: section.id, Title: section.title}
		counts := map[string]int{}
		for _, d := range g.Decisions {
			if d.Category == section.id {
				s.Decisions = append(s.Decisions, d)
				s.DecisionCount += d.DecisionCount
				counts[d.Status]++
			}
		}
		if len(s.Decisions) > 0 {
			s.Counts = countView(counts)
			out = append(out, s)
		}
	}
	return out
}

// Group by captured item context, never by configuration values or input rereads.
// A missing, redacted, or inconsistent name falls back to the general section.
func identityTransformSections(g humanGroup) []humanSection {
	byName := map[string][]humanDecision{}
	for _, d := range g.Decisions {
		if d.Category != "identity-transforms" {
			continue
		}
		name := ""
		ambiguous := false
		for _, loc := range d.Locations {
			for _, c := range loc.Context {
				parts, err := pointer(c.Pointer)
				if err != nil || len(parts) != 3 || parts[0] != "identityAttributeConfig" || parts[1] != "attributeTransforms" {
					continue
				}
				for _, f := range c.Fields {
					if f.Label == "Identity attribute" {
						if name != "" && name != f.Value {
							ambiguous = true
						}
						name = f.Value
					}
				}
			}
		}
		if ambiguous {
			name = ""
		}
		byName[name] = append(byName[name], d)
	}
	out := []humanSection{}
	for i, name := range keys(byName) {
		s := humanSection{ID: g.ID + "-identity-transforms-" + strconv.Itoa(i), Category: "identity-transforms", Title: "Identity attribute transforms", Decisions: byName[name]}
		if name != "" {
			s.Title = "Identity attribute transform: " + name
		}
		counts := map[string]int{}
		for _, d := range s.Decisions {
			s.DecisionCount += d.DecisionCount
			counts[d.Status]++
		}
		s.Counts = countView(counts)
		out = append(out, s)
	}
	return out
}
