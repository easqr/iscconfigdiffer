package v2

import "strconv"

// A schema is summarized only when its entire concrete collection item is
// absent. A missing attribute inside a matched schema is not a missing schema.
func missingSchema(d Decision, roles []string) (string, string, bool) {
	role, ok := missingSubtreeSide(d, roles)
	if !ok || d.Code != "container presence differs" || space(d.Space) != "content" {
		return "", "", false
	}
	loc := d.Locations[role]
	parts, err := pointer(loc.Pointer)
	if err != nil || len(parts) != 2 || parts[0] != "schemas" || loc.ValueKind != "object" {
		return "", "", false
	}
	if _, ok := arrayIndex(parts[1]); !ok {
		return "", "", false
	}
	name := ""
	for _, c := range loc.Context {
		if c.Pointer == loc.Pointer && c.Label == "Schema" {
			for _, f := range c.Fields {
				if f.Label == "Name" {
					name = f.Value
				}
			}
		}
	}
	return role, name, true
}

func schemaContains(parent, child Decision, roles []string, present string) bool {
	if parent.Type != child.Type || parent.Key != child.Key || space(parent.Space) != space(child.Space) {
		return false
	}
	for _, role := range roles {
		p, po := parent.Locations[role]
		c, co := child.Locations[role]
		if !po || !co || p.ID != c.ID || p.Artifact != c.Artifact || !p.Present && c.Present {
			return false
		}
		pp, err := pointer(p.Pointer)
		if err != nil {
			return false
		}
		cp, err := pointer(c.Pointer)
		if err != nil || len(cp) < len(pp) {
			return false
		}
		for i := range pp {
			if pp[i] != cp[i] {
				return false
			}
		}
		if role == present && len(cp) == len(pp) {
			return false
		}
	}
	return true
}

// Preserve explicit assertions, violations, conflicts, and unverified ruled
// checks as independent findings. Informational rule decisions can be nested,
// keeping their full original values, paths and provenance in schema details.
func schemaDetail(d Decision, roles []string) bool {
	if _, ok := missingSubtreeSide(d, roles); ok {
		return true
	}
	if d.Key == "[redacted]" || d.Authority != "" || d.ExpectedRole != "" || d.Derivation != nil {
		return false
	}
	return d.Status == "equal" || d.Status == "equivalent" || d.Status == "excluded"
}

func groupMissingSchemas(decisions []Decision, views []humanDecision, roles []string, inputs map[string]Input) ([]Decision, []humanDecision) {
	candidates := []int{}
	type schemaKey struct{ role, space, id, artifact, path string }
	byPath := map[schemaKey][]int{}
	present := map[int]string{}
	names := map[int]string{}
	for i, d := range decisions {
		if role, name, ok := missingSchema(d, roles); ok {
			candidates = append(candidates, i)
			present[i] = role
			names[i] = name
			loc := d.Locations[role]
			k := schemaKey{role, space(d.Space), loc.ID, loc.Artifact, loc.Pointer}
			byPath[k] = append(byPath[k], i)
		}
	}
	owner := make([]int, len(decisions))
	for i := range owner {
		owner[i] = -1
	}
	for i, d := range decisions {
		if !schemaDetail(d, roles) {
			continue
		}
		found := -1
		ambiguous := false
		for _, role := range roles[:2] {
			loc, ok := d.Locations[role]
			if !ok {
				continue
			}
			parts, err := pointer(loc.Pointer)
			if err != nil || len(parts) < 3 {
				continue
			}
			k := schemaKey{role, space(d.Space), loc.ID, loc.Artifact, contextPointer(parts[:2])}
			for _, p := range byPath[k] {
				if p == i || !schemaContains(decisions[p], d, roles, present[p]) {
					continue
				}
				if found != -1 {
					ambiguous = true
					break
				}
				found = p
			}
			if ambiguous {
				break
			}
		}
		if ambiguous {
			found = -1
		}
		owner[i] = found
	}
	for i, p := range owner {
		if p >= 0 {
			child := views[i]
			child.Nested = true
			child.DecisionCount = 1
			views[p].Children = append(views[p].Children, child)
		}
	}
	label := func(role string) string {
		if env := inputs[role].Environment; env != "" {
			return role + " (" + env + ")"
		}
		return role
	}
	for _, p := range candidates {
		missing := roles[0]
		if present[p] == roles[0] {
			missing = roles[1]
		}
		view := &views[p]
		view.WholeSchema = true
		view.FindingTitle = "Schema missing from " + label(missing)
		if names[p] != "" {
			view.FindingTitle = "Schema " + strconv.Quote(names[p]) + " missing from " + label(missing)
		}
		view.Explanation = "This entire schema is present only in " + label(present[p]) + ". Its attributes and settings are details of this missing schema, rather than separate schema changes."
		view.ChildrenNotice = "Schema attributes, settings, and informational policy outcomes are retained here. Explicit assertions and actionable rule checks remain separate findings."
		view.DecisionCount = 1 + len(view.Children)
	}
	remaining := []Decision{}
	out := []humanDecision{}
	for i, d := range decisions {
		if owner[i] < 0 {
			remaining = append(remaining, d)
			out = append(out, views[i])
		}
	}
	return remaining, out
}
