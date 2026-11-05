package v2

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

func itemKeyEqual(a, b CollectionItemKey) bool {
	if a.Mapped != "" || b.Mapped != "" {
		return a.Mapped != "" && a.Mapped == b.Mapped
	}
	av, ae := JSON(a.Value)
	bv, be := JSON(b.Value)
	return ae == nil && be == nil && equal(av, bv)
}
func sameItemPaths(a, b map[string]CollectionItemKey) bool {
	if len(a) != len(b) {
		return false
	}
	for path := range a {
		if _, ok := b[path]; !ok {
			return false
		}
	}
	return true
}
func itemSelected(scope *CollectionItemSelector, items map[string]map[string]CollectionItemKey) bool {
	if scope == nil {
		return true
	}
	actual, ok := items[scope.Collection]
	return ok && sameItemIdentity(scope.Keys, actual)
}
func sameItemIdentity(want, actual map[string]CollectionItemKey) bool {
	if !sameItemPaths(want, actual) {
		return false
	}
	for path, key := range want {
		if !itemKeyEqual(key, actual[path]) {
			return false
		}
	}
	return true
}

// Complete tuples in the same collection are disjoint when any component differs.
// Scopes on different ancestor collections can coexist and are not assumed disjoint.
func disjointItems(a, b *CollectionItemSelector) bool {
	if a == nil || b == nil || a.Collection != b.Collection {
		return false
	}
	if !sameItemPaths(a.Keys, b.Keys) {
		return true
	}
	for path, key := range a.Keys {
		if !itemKeyEqual(key, b.Keys[path]) {
			return true
		}
	}
	return false
}
func validateItemForms(p Policy) error {
	for _, r := range p.Equivalences {
		if r.Item != nil && r.Items != nil {
			return ruleError(errors.New("equivalence must use item or items, not both"), r.ID)
		}
		if r.Items != nil && len(r.Items) == 0 {
			return ruleError(errors.New("equivalence items must be nonempty"), r.ID)
		}
	}
	return nil
}
func equivalenceItems(r Equivalence) []CollectionItemSelector {
	if r.Item != nil {
		return []CollectionItemSelector{*r.Item}
	}
	return r.Items
}
func equivalenceItemsSelected(r Equivalence, items map[string]map[string]CollectionItemKey) bool {
	for _, scope := range equivalenceItems(r) {
		if !itemSelected(&scope, items) {
			return false
		}
	}
	return true
}
func disjointEquivalenceItems(a, b Equivalence) bool {
	for _, first := range equivalenceItems(a) {
		for _, second := range equivalenceItems(b) {
			if disjointItems(&first, &second) {
				return true
			}
		}
	}
	return false
}
func validateEquivalenceItems(p Policy, collections map[string]Collection) error {
	for _, r := range p.Equivalences {
		scopes := equivalenceItems(r)
		seenCollections := map[string]bool{}
		for i := range scopes {
			scope := &scopes[i]
			if seenCollections[scope.Collection] {
				return ruleError(fmt.Errorf("equivalence %s repeats a collection in items", r.ID), r.ID)
			}
			seenCollections[scope.Collection] = true
			c, ok := collections[scope.Collection]
			if !ok || c.Mode != "keyed" || !active(c.Rule, "drift") || !collectionContains(c.Selector, r.Selector, c.Mode) {
				return ruleError(fmt.Errorf("equivalence %s requires an ancestor effective keyed collection", r.ID), r.ID)
			}
			for _, previous := range scopes[:i] {
				parent := collections[previous.Collection]
				if !collectionContains(parent.Selector, c.Selector, parent.Mode) && !collectionContains(c.Selector, parent.Selector, c.Mode) {
					return ruleError(fmt.Errorf("equivalence %s items must form an ancestor chain", r.ID), r.ID)
				}
			}
			complete := func(paths []string) bool {
				unique := map[string]CollectionItemKey{}
				for _, path := range paths {
					unique[path] = CollectionItemKey{}
				}
				return sameItemPaths(scope.Keys, unique)
			}
			choices := [][]string{c.Keys}
			for _, keyCase := range c.KeyCases {
				choices = append(choices, keyCase.Keys)
			}
			for i, paths := range choices {
				seen := map[string]CollectionItemKey{}
				for _, path := range paths {
					seen[path] = CollectionItemKey{}
				}
				if len(seen) != len(paths) {
					return ruleError(errors.New("item scopes require distinct key component paths"), r.ID)
				}
				for _, previous := range choices[:i] {
					other := map[string]CollectionItemKey{}
					for _, path := range previous {
						other[path] = CollectionItemKey{}
					}
					if sameItemPaths(seen, other) && !slices.Equal(paths, previous) {
						return ruleError(errors.New("item scope key tuple order is ambiguous"), r.ID)
					}
				}
			}
			validTuple := complete(c.Keys)
			for _, keyCase := range c.KeyCases {
				validTuple = validTuple || complete(keyCase.Keys)
			}
			if len(scope.Keys) == 0 || !validTuple {
				return ruleError(fmt.Errorf("equivalence %s item requires a complete collection key tuple", r.ID), r.ID)
			}
			mappings := append([]CollectionKeyMapping{}, c.KeyMappings...)
			for _, extension := range p.CollectionMappings {
				if extension.Collection == c.ID && active(extension.Rule, "drift") && (extension.Key == "" || r.Key == "" || extension.Key == r.Key) {
					mappings = append(mappings, extension.KeyMappings...)
				}
			}
			for path, key := range scope.Keys {
				if !concrete(path) || (key.Mapped != "") == (len(key.Value) > 0) {
					return ruleError(errors.New("item key requires exactly one mapped group or raw value"), r.ID)
				}
				if key.Mapped != "" {
					if strings.TrimSpace(key.Mapped) == "" {
						return ruleError(errors.New("item mapped group must be nonblank"), r.ID)
					}
					found := false
					for _, mapping := range mappings {
						if mapping.Path != path {
							continue
						}
						for _, group := range mapping.Groups {
							found = found || group.Key == key.Mapped
						}
					}
					if !found {
						return ruleError(fmt.Errorf("equivalence %s references an unavailable mapping group", r.ID), r.ID)
					}
				} else {
					value, err := JSON(key.Value)
					if err != nil || !scalar(value) {
						return ruleError(errors.New("item raw key requires a nonnull scalar value"), r.ID)
					}
					if c.ComponentType == "string" {
						s, ok := value.(string)
						if !ok || strings.TrimSpace(s) == "" {
							return ruleError(errors.New("item raw key must be a nonblank string"), r.ID)
						}
					}
				}
			}
		}
		sort.Slice(r.Items, func(i, j int) bool {
			a, _ := pointer(collections[r.Items[i].Collection].Path)
			b, _ := pointer(collections[r.Items[j].Collection].Path)
			if len(a) != len(b) {
				return len(a) < len(b)
			}
			return r.Items[i].Collection < r.Items[j].Collection
		})
	}
	return nil
}
