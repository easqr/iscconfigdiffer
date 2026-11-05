package v2

import (
	"errors"
	"strings"
)

func validateCollectionKeys(r Collection) error {
	if r.Mode != "keyed" {
		if len(r.KeyCases) > 0 || r.ComponentType != "" {
			return errors.New("collection key options require keyed mode")
		}
		return nil
	}
	if !contains([]string{"", "scalar", "string"}, r.ComponentType) {
		return errors.New("invalid collection component type")
	}
	checkPaths := func(paths []string) error {
		if len(paths) == 0 {
			return errors.New("collection key paths required")
		}
		for _, path := range paths {
			if !concrete(path) {
				return errors.New("concrete collection key required")
			}
		}
		return nil
	}
	if err := checkPaths(r.Keys); err != nil {
		return err
	}
	seen := map[string]bool{}
	whenPath := ""
	for i, c := range r.KeyCases {
		if !concrete(c.WhenPath) {
			return errors.New("concrete collection key case path required")
		}
		if i == 0 {
			whenPath = c.WhenPath
		} else if c.WhenPath != whenPath {
			return errors.New("collection key cases must use the same discriminator path")
		}
		value, err := JSON(c.WhenValue)
		if err != nil || !scalar(value) {
			return errors.New("collection key case requires a nonnull scalar when_value")
		}
		token := valueKey(value)
		if seen[token] {
			return errors.New("duplicate collection key case")
		}
		seen[token] = true
		if err := checkPaths(c.Keys); err != nil {
			return err
		}
	}
	return nil
}

func collectionKey(item any, r Collection) (string, error) {
	paths := r.Keys
	for _, c := range r.KeyCases {
		value, present := get(item, c.WhenPath)
		want, _ := JSON(c.WhenValue) // Cases are strictly validated during compilation.
		if present && equal(value, want) {
			paths = c.Keys
			break
		}
	}
	parts := make([]any, 0, len(paths))
	for _, path := range paths {
		key, present := get(item, path)
		if !present || !scalar(key) {
			return "", errors.New("collection key unavailable")
		}
		if r.ComponentType == "string" {
			s, ok := key.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return "", errors.New("collection key must be a nonblank string")
			}
		}
		parts = append(parts, key)
	}
	return valueKey(parts), nil
}
