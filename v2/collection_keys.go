package v2

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

func validateCollectionKeys(r Collection) error {
	if r.Mode != "keyed" {
		if len(r.KeyCases) > 0 || len(r.KeyMappings) > 0 || r.ComponentType != "" {
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
	return validateCollectionKeyMappings(r)
}

func validateCollectionKeyMappings(r Collection) error {
	keyPaths := map[string]bool{}
	for _, path := range r.Keys {
		keyPaths[path] = true
	}
	for _, c := range r.KeyCases {
		for _, path := range c.Keys {
			keyPaths[path] = true
		}
	}
	paths := map[string]bool{}
	for _, mapping := range r.KeyMappings {
		if !concrete(mapping.Path) || !keyPaths[mapping.Path] || paths[mapping.Path] {
			return errors.New("collection key mapping requires a unique declared key path")
		}
		paths[mapping.Path] = true
		if len(mapping.Groups) == 0 {
			return errors.New("collection key mapping groups required")
		}
		groups := map[string]bool{}
		seen := map[string]map[string]bool{}
		for _, group := range mapping.Groups {
			if strings.TrimSpace(group.Key) == "" || groups[group.Key] || len(group.Values) < 2 {
				return errors.New("collection key mapping groups require unique nonblank keys and at least two environments")
			}
			groups[group.Key] = true
			for env, value := range group.Values {
				if strings.TrimSpace(env) == "" || !scalar(value) {
					return errors.New("collection key mapping requires named environments and nonnull scalar values")
				}
				if r.ComponentType == "string" {
					s, ok := value.(string)
					if !ok || strings.TrimSpace(s) == "" {
						return errors.New("collection key mapping value must be a nonblank string")
					}
				}
				if seen[env] == nil {
					seen[env] = map[string]bool{}
				}
				token := valueKey(value)
				if seen[env][token] {
					return errors.New("ambiguous collection key mapping value")
				}
				seen[env][token] = true
			}
		}
		// Order has no meaning; stable policy digests cover the effective mapping.
		sort.Slice(mapping.Groups, func(i, j int) bool { return mapping.Groups[i].Key < mapping.Groups[j].Key })
	}
	sort.Slice(r.KeyMappings, func(i, j int) bool { return r.KeyMappings[i].Path < r.KeyMappings[j].Path })
	return nil
}

func collectionKey(item any, r Collection, env string) (string, bool, error) {
	key, mapped, _, err := collectionIdentity(item, r, env)
	return key, mapped, err
}

func collectionIdentity(item any, r Collection, env string) (string, bool, map[string]CollectionItemKey, error) {
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
	components := map[string]CollectionItemKey{}
	mapped := false
	for _, path := range paths {
		key, present := get(item, path)
		if !present || !scalar(key) {
			return "", false, nil, errors.New("collection key unavailable")
		}
		if r.ComponentType == "string" {
			s, ok := key.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return "", false, nil, errors.New("collection key must be a nonblank string")
			}
		}
		raw, _ := json.Marshal(key)
		components[path] = CollectionItemKey{Value: raw}
		for _, mapping := range r.KeyMappings {
			if mapping.Path != path {
				continue
			}
			knownEnv := false
			// Tagged namespaces prevent a literal value from colliding with an alias.
			component := any(map[string]any{"raw": key})
			for _, group := range mapping.Groups {
				value, ok := group.Values[env]
				knownEnv = knownEnv || ok
				if ok && equal(value, key) {
					component = map[string]any{"mapped": group.Key}
					components[path] = CollectionItemKey{Mapped: group.Key}
					mapped = true
				}
			}
			if !knownEnv {
				return "", false, nil, errors.New("collection key mapping environment unavailable")
			}
			key = component
		}
		parts = append(parts, key)
	}
	return valueKey(parts), mapped, components, nil
}
