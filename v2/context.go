package v2

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

func validateCollectionDisplay(r Collection) error {
	if r.ItemLabel == "" && len(r.DisplayFields) == 0 {
		return nil
	}
	if strings.TrimSpace(r.ItemLabel) == "" {
		return errors.New("collection item_label must be nonblank when display context is configured")
	}
	seen := map[string]bool{}
	for _, f := range r.DisplayFields {
		if strings.TrimSpace(f.Label) == "" || seen[f.Label] {
			return errors.New("collection display labels must be nonblank and unique")
		}
		seen[f.Label] = true
		if !concrete(f.Path) {
			return errors.New("collection display fields require concrete item-relative paths")
		}
	}
	return nil
}

func contextPointer(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = esc(p)
	}
	return "/" + strings.Join(out, "/")
}

// Capture labels from each input's real ancestor items. Never infer an absent
// item's name or index from its counterpart on another side of the comparison.
func (x *execution) locationContext(doc Document, key, section, path string) []ItemContext {
	parts, err := pointer(path)
	if err != nil {
		return nil
	}
	root := doc.Content
	if section == "metadata" {
		root = doc.Metadata
	}
	type located struct {
		depth   int
		context ItemContext
	}
	found := []located{}
	for _, r := range x.c.policy.Collections {
		if r.ItemLabel == "" || !active(r.Rule, x.request.Mode) || r.Type != doc.Type || space(r.Space) != section || r.Key != "" && r.Key != key {
			continue
		}
		pattern, _ := pointer(r.Path)
		if len(parts) <= len(pattern) {
			continue
		}
		matches := true
		for i, p := range pattern {
			if p != "*" && p != parts[i] {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		array, ok := get(root, contextPointer(parts[:len(pattern)]))
		if !ok {
			continue
		}
		items, ok := array.([]any)
		if !ok {
			continue
		}
		index, ok := arrayIndex(parts[len(pattern)])
		if !ok || index >= len(items) {
			continue
		}
		ctx := ItemContext{Label: r.ItemLabel, Pointer: contextPointer(parts[:len(pattern)+1])}
		for _, f := range r.DisplayFields {
			value, present := get(items[index], f.Path)
			if !present || !scalar(value) {
				continue
			}
			text, ok := value.(string)
			if !ok {
				data, _ := json.Marshal(value)
				text = string(data)
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			ctx.Fields = append(ctx.Fields, ContextField{Label: f.Label, Value: text})
		}
		found = append(found, located{len(pattern), ctx})
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].depth != found[j].depth {
			return found[i].depth < found[j].depth
		}
		return found[i].context.Label < found[j].context.Label
	})
	out := make([]ItemContext, 0, len(found))
	for _, f := range found {
		out = append(out, f.context)
	}
	return out
}
