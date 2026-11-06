package v2

// missingSubtreeSide identifies unruled presence differences. Rule-driven checks
// remain independent findings even when they sit inside a missing subtree.
func missingSubtreeSide(d Decision, roles []string) (string, bool) {
	if d.Key == "[redacted]" || d.RuleID != "" || d.Authority != "" || d.Evidence != nil || d.ExpectedRole != "" || d.Derivation != nil || d.Normalization != "" {
		return "", false
	}
	switch d.Status {
	case "different":
		if d.Code != "container presence differs" && d.Code != "missing "+roles[0] && d.Code != "missing "+roles[1] {
			return "", false
		}
	case "unverified":
		if d.Code != "container presence differs" && d.Code != "no authoritative expectation" {
			return "", false
		}
	default:
		return "", false
	}
	a, ao := d.Locations[roles[0]]
	b, bo := d.Locations[roles[1]]
	if !ao || !bo || a.Present == b.Present {
		return "", false
	}
	if a.Present {
		return roles[0], true
	}
	return roles[1], true
}

func subtreeContains(parent, child Decision, roles []string, presentRole string) bool {
	if space(parent.Space) != space(child.Space) || parent.Status != child.Status {
		return false
	}
	for _, role := range roles {
		p, po := parent.Locations[role]
		c, co := child.Locations[role]
		if !po || !co || p.ID != c.ID || p.Artifact != c.Artifact {
			return false
		}
		if role == roles[0] || role == roles[1] {
			if p.Present != c.Present {
				return false
			}
		} else if !p.Present && c.Present {
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
		// A missing keyed item's absent-side pointer may be its collection path.
		// The present side must establish ancestry; an empty-container leaf at the
		// very same path can also duplicate the container's presence decision.
		if role == presentRole && child.Code == "container presence differs" && len(cp) == len(pp) {
			return false
		}
	}
	return true
}

// groupMissingSubtrees folds only redundant presence decisions beneath their
// outermost known missing container. It uses physical paths on every role, never
// configuration values or rendered text. The input report is left untouched.
func groupMissingSubtrees(decisions []Decision, views []humanDecision, roles []string) []humanDecision {
	type parentKey struct{ space, status, role, id, artifact, path string }
	parents := map[parentKey][]int{}
	for i, d := range decisions {
		role, ok := missingSubtreeSide(d, roles)
		if !ok || d.Code != "container presence differs" {
			continue
		}
		loc := d.Locations[role]
		if loc.ValueKind != "object" && loc.ValueKind != "array" {
			continue
		}
		parts, err := pointer(loc.Pointer)
		if err != nil {
			continue
		}
		canonical := ""
		for _, part := range parts {
			canonical += "/" + esc(part)
		}
		key := parentKey{space(d.Space), d.Status, role, loc.ID, loc.Artifact, canonical}
		parents[key] = append(parents[key], i)
	}
	owner := make([]int, len(decisions))
	for i, d := range decisions {
		owner[i] = -1
		role, ok := missingSubtreeSide(d, roles)
		if !ok {
			continue
		}
		loc := d.Locations[role]
		parts, err := pointer(loc.Pointer)
		if err != nil {
			continue
		}
		key := parentKey{space(d.Space), d.Status, role, loc.ID, loc.Artifact, ""}
		for depth := 0; depth <= len(parts); depth++ {
			found := -1
			ambiguous := false
			for _, candidate := range parents[key] {
				if candidate == i || !subtreeContains(decisions[candidate], d, roles, role) {
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
			} // Do not infer ownership from ambiguous/redacted paths.
			if found != -1 {
				owner[i] = found
				break
			}
			if depth < len(parts) {
				key.path += "/" + esc(parts[depth])
			}
		}
	}
	// All descendants are flat children of the outermost container, retaining
	// their original report order and complete per-decision provenance.
	for i, parent := range owner {
		if parent < 0 {
			continue
		}
		child := views[i]
		child.Nested = true
		child.DecisionCount = 1
		views[parent].Children = append(views[parent].Children, child)
	}
	out := make([]humanDecision, 0, len(views))
	for i, view := range views {
		if owner[i] >= 0 {
			continue
		}
		view.DecisionCount = 1 + len(view.Children)
		if len(view.Children) > 0 && view.FindingTitle == "" {
			view.FindingTitle = "Missing subtree"
			view.ChildrenNotice = "These details describe the same missing subtree."
		}
		out = append(out, view)
	}
	return out
}
