package v2

import "strings"

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
	for _, section := range []struct{ id, title string }{{"connection", "Connection settings"}, {"schemas", "Schemas"}, {"provisioning", "Provisioning policies"}, {"other", "Other settings"}, {"metadata", "Export metadata"}} {
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
