package v2

// implementationDifference selects ordinary drift comparisons only. Assertions,
// normalization, and unverified deployment checks keep independent findings.
func implementationDifference(d Decision) bool {
	if d.Key == "[redacted]" || d.Status != "different" || d.RuleID != "" || d.Authority != "" || d.Evidence != nil || d.ExpectedRole != "" || d.Derivation != nil || d.Normalization != "" {
		return false
	}
	switch d.Code {
	case "value mismatch", "type mismatch", "container presence differs", "missing left", "missing right":
		return true
	}
	return false
}

// transformAttributes identifies the conventional transform/type discriminator.
// This is a presentation convention, independent of vendor types and policies.
func transformAttributes(d Decision, roles []string) (map[string]string, bool) {
	if !implementationDifference(d) || d.Code != "value mismatch" {
		return nil, false
	}
	paths := map[string]string{}
	for i, role := range roles {
		loc, ok := d.Locations[role]
		if !ok {
			return nil, false
		}
		parts, err := pointer(loc.Pointer)
		if err != nil || len(parts) < 2 || parts[len(parts)-2] != "transform" || parts[len(parts)-1] != "type" {
			return nil, false
		}
		if i < 2 && (!loc.Present || loc.ValueKind != "string") {
			return nil, false
		}
		paths[role] = contextPointer(append(append([]string{}, parts[:len(parts)-1]...), "attributes"))
	}
	return paths, true
}

func implementationContains(parent, child Decision, attributes map[string]string, roles []string) bool {
	if parent.Type != child.Type || parent.Key != child.Key || space(parent.Space) != space(child.Space) {
		return false
	}
	for _, role := range roles {
		p, po := parent.Locations[role]
		c, co := child.Locations[role]
		if !po || !co || p.ID != c.ID || p.Artifact != c.Artifact {
			return false
		}
		prefix, err := pointer(attributes[role])
		if err != nil {
			return false
		}
		path, err := pointer(c.Pointer)
		if err != nil || len(path) < len(prefix) {
			return false
		}
		for i := range prefix {
			if prefix[i] != path[i] {
				return false
			}
		}
	}
	return true
}

// groupTransformImplementations groups changed discriminators and their related
// unruled attribute differences before missing-subtree grouping. The original
// discriminator locations/values provide the summary's type comparison, and
// every attribute decision keeps its original locations and provenance below it.
func groupTransformImplementations(decisions []Decision, views []humanDecision, roles []string) ([]Decision, []humanDecision) {
	type key struct{ space, id, artifact, path string }
	candidates := map[key][]int{}
	attributes := map[int]map[string]string{}
	for i, d := range decisions {
		paths, ok := transformAttributes(d, roles)
		if !ok {
			continue
		}
		attributes[i] = paths
		loc := d.Locations[roles[0]]
		k := key{space(d.Space), loc.ID, loc.Artifact, paths[roles[0]]}
		candidates[k] = append(candidates[k], i)
	}
	owner := make([]int, len(decisions))
	for i, d := range decisions {
		owner[i] = -1
		if !implementationDifference(d) {
			continue
		}
		loc, ok := d.Locations[roles[0]]
		if !ok {
			continue
		}
		parts, err := pointer(loc.Pointer)
		if err != nil {
			continue
		}
		k := key{space(d.Space), loc.ID, loc.Artifact, ""}
		// Prefix order selects the outermost changed implementation, including
		// nested transform objects, without comparing all decision pairs.
		for depth := 0; depth <= len(parts); depth++ {
			found := -1
			ambiguous := false
			for _, candidate := range candidates[k] {
				if candidate == i || !implementationContains(decisions[candidate], d, attributes[candidate], roles) {
					continue
				}
				if found != -1 {
					ambiguous = true
					break
				}
				found = candidate
			}
			if ambiguous {
				break
			}
			if found != -1 {
				owner[i] = found
				break
			}
			if depth < len(parts) {
				k.path += "/" + esc(parts[depth])
			}
		}
	}
	for i, parent := range owner {
		if parent < 0 {
			continue
		}
		child := views[i]
		child.Nested = true
		child.DecisionCount = 1
		views[parent].Children = append(views[parent].Children, child)
	}
	remaining := make([]Decision, 0, len(decisions))
	out := make([]humanDecision, 0, len(views))
	for i, view := range views {
		if owner[i] >= 0 {
			continue
		}
		if _, ok := attributes[i]; ok {
			view.FindingTitle = "Transform implementation differs"
			view.Explanation = "The transform types differ."
			if len(view.Children) > 0 {
				view.Explanation += " The type comparison is shown here; related attribute differences are included in the nested decisions."
			}
			view.ChildrenNotice = "These details compare attributes of the transform implementations shown above."
		}
		remaining = append(remaining, decisions[i])
		out = append(out, view)
	}
	return remaining, out
}
