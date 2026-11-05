package v2

// Sensitive selectors conservatively hide the identity and physical provenance of
// their object type and dependency-derived types. Values are never in reports.
func (x *execution) redact() {
	hidden := map[string]bool{}
	for _, r := range x.c.policy.Sensitive {
		if !active(r.Rule, x.request.Mode) {
			continue
		}
		if x.report.ObjectKey != "" && x.report.Scope[0] == r.Type && (r.Key == "" || r.Key == x.report.ObjectKey) {
			hidden[r.Type] = true
		}
		for _, g := range x.inventory.groups {
			if g.typ == r.Type && (r.Key == "" || r.Key == g.key) {
				hidden[g.typ] = true
				for _, node := range g.nodes {
					if node == nil {
						continue
					}
					root := node.doc.Content
					if space(r.Space) == "metadata" {
						root = node.doc.Metadata
					}
					parts, _ := pointer(r.Path)
					if selectionExists(root, parts) {
						x.report.RuleUsage[r.ID]++
					}
				}
			}
		}
		// Missing operations still carry logical identities in the report even
		// though no document was available to apply the data selector to.
		for _, op := range x.report.Operations {
			if op.Type == r.Type && (r.Key == "" || r.Key == op.Key) {
				hidden[op.Type] = true
			}
		}
	}
	changed := true
	for changed {
		changed = false
		for _, r := range x.c.policy.Identities {
			if hidden[r.ParentType] && !hidden[r.Type] {
				hidden[r.Type] = true
				changed = true
			}
		}
	}
	if x.report.ObjectKey != "" && hidden[x.report.Scope[0]] {
		x.report.ObjectKey = "[redacted]"
	}
	mask := func(locations map[string]Location) {
		for role := range locations {
			loc := locations[role]
			loc.ID = "[redacted]"
			loc.Artifact = "[redacted]"
			loc.Pointer = "[redacted]"
			locations[role] = loc
		}
	}
	for i := range x.report.Decisions {
		d := &x.report.Decisions[i]
		if d.Derivation != nil && hidden[d.Derivation.Type] {
			d.Derivation.Location.ID = "[redacted]"
			d.Derivation.Location.Artifact = "[redacted]"
			d.Derivation.Location.Pointer = "[redacted]"
		}
		if hidden[d.Type] {
			d.Key = "[redacted]"
			d.ItemKeys = nil
			mask(d.Locations)
		}
	}
	for i := range x.report.Matches {
		m := &x.report.Matches[i]
		if hidden[m.Type] {
			m.Key = "[redacted]"
			mask(m.Locations)
		}
	}
	for i := range x.report.Operations {
		if hidden[x.report.Operations[i].Type] {
			x.report.Operations[i].Key = "[redacted]"
		}
	}
}

func selectionExists(v any, parts []string) bool {
	if len(parts) == 0 {
		return true
	}
	part := parts[0]
	if part == "*" {
		switch x := v.(type) {
		case map[string]any:
			for _, child := range x {
				if selectionExists(child, parts[1:]) {
					return true
				}
			}
		case []any:
			for _, child := range x {
				if selectionExists(child, parts[1:]) {
					return true
				}
			}
		}
		return false
	}
	switch x := v.(type) {
	case map[string]any:
		child, ok := x[part]
		return ok && selectionExists(child, parts[1:])
	case []any:
		i, ok := arrayIndex(part)
		return ok && i < len(x) && selectionExists(x[i], parts[1:])
	}
	return false
}
