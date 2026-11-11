// Package iscprofile loads shipped ISC rules for the vendor-neutral differ.
package iscprofile

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/easqr/iscconfigdiffer/v2"
)

//go:embed packs/*.json
var shipped embed.FS

type connectorIdentifier struct {
	Path   string   `json:"path"`
	Values []string `json:"values"`
}
type connectorDefinition struct {
	Variant     string                `json:"variant"`
	Identifiers []connectorIdentifier `json:"identifiers"`
}
type rulePack struct {
	ID        string               `json:"id"`
	Version   int                  `json:"version"`
	Layer     string               `json:"layer"`
	Profiles  []string             `json:"profiles"`
	Connector *connectorDefinition `json:"connector,omitempty"`
	Rules     json.RawMessage      `json:"rules"`
	digest    string
}

var packsOnce sync.Once
var packs []rulePack
var packsError error

func loadPacks() ([]rulePack, error) {
	packsOnce.Do(func() {
		entries, err := shipped.ReadDir("packs")
		if err != nil {
			packsError = err
			return
		}
		seen := map[string]bool{}
		for _, entry := range entries {
			data, err := shipped.ReadFile("packs/" + entry.Name())
			if err != nil {
				packsError = err
				return
			}
			pack, err := decodePack(data)
			if err != nil {
				packsError = fmt.Errorf("invalid shipped rule pack %s: %w", entry.Name(), err)
				return
			}
			if seen[pack.ID] {
				packsError = errors.New("duplicate shipped rule pack")
				return
			}
			seen[pack.ID] = true
			packs = append(packs, pack)
		}
	})
	return packs, packsError
}
func decodePack(data []byte) (rulePack, error) {
	var p rulePack
	value, err := v2.JSON(data)
	if err != nil {
		return p, err
	}
	if err := packShape(value); err != nil {
		return p, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, err
	}
	if p.ID == "" || p.Version != 1 || !contains([]string{"common", "type", "connector"}, p.Layer) || len(p.Profiles) == 0 {
		return p, errors.New("invalid rule pack metadata")
	}
	rules, err := v2.Decode(p.Rules)
	if err != nil {
		return p, err
	}
	if rules.ID != p.ID || rules.Profile != "" || len(rules.Modes) > 0 || len(rules.Bindings) > 0 || len(rules.CollectionMappings) > 0 || len(rules.DisabledRules) > 0 || len(rules.Evidence) > 0 || len(rules.ObjectFilters) > 0 {
		return p, errors.New("invalid rule pack policy header")
	}
	for _, cat := range ruleCategories(&rules) {
		for i := 0; i < cat.Len(); i++ {
			if cat.Index(i).FieldByName("Rule").Interface().(v2.Rule).Evidence != nil {
				return p, errors.New("rule pack evidence is loader-owned")
			}
		}
	}
	if (p.Layer == "connector") != (p.Connector != nil) {
		return p, errors.New("connector pack requires connector definition")
	}
	if p.Connector != nil {
		if p.Connector.Variant == "" || len(p.Connector.Identifiers) == 0 {
			return p, errors.New("invalid connector definition")
		}
		for _, id := range p.Connector.Identifiers {
			if !v2.LiteralPointer(id.Path) || len(id.Values) == 0 {
				return p, errors.New("invalid connector identifier")
			}
			for _, v := range id.Values {
				if strings.TrimSpace(v) == "" {
					return p, errors.New("blank connector identifier")
				}
			}
		}
		for _, cat := range ruleCategories(&rules) {
			for i := 0; i < cat.Len(); i++ {
				s := cat.Index(i).FieldByName("Selector")
				if !s.IsValid() {
					return p, errors.New("connector rules require selectors")
				}
				selector := s.Interface().(v2.Selector)
				if selector.Type != "SOURCE" || selector.Variant != p.Connector.Variant {
					return p, errors.New("connector rule escapes connector scope")
				}
			}
		}
	}
	digest := sha256.Sum256(data)
	p.digest = hex.EncodeToString(digest[:])
	return p, nil
}

func packShape(value any) error {
	object := func(value any, required, optional []string) (map[string]any, error) {
		m, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("rule pack object required")
		}
		for key := range m {
			if !contains(required, key) && !contains(optional, key) {
				return nil, errors.New("unknown rule pack property")
			}
		}
		for _, key := range required {
			if _, ok := m[key]; !ok {
				return nil, errors.New("missing rule pack property")
			}
		}
		return m, nil
	}
	m, err := object(value, []string{"id", "version", "layer", "profiles", "rules"}, []string{"connector"})
	if err != nil {
		return err
	}
	if connector, ok := m["connector"]; ok {
		c, err := object(connector, []string{"variant", "identifiers"}, nil)
		if err != nil {
			return err
		}
		identifiers, ok := c["identifiers"].([]any)
		if !ok {
			return errors.New("connector identifiers array required")
		}
		for _, id := range identifiers {
			if _, err := object(id, []string{"path", "values"}, nil); err != nil {
				return err
			}
		}
	}
	return nil
}
func ruleCategories(p *v2.Policy) []reflect.Value {
	out := []reflect.Value{}
	v := reflect.ValueOf(p).Elem()
	for _, field := range []string{"Identities", "Pairings", "Ignores", "Expectations", "Equivalences", "Exceptions", "References", "Collections", "Sensitive"} {
		out = append(out, v.FieldByName(field))
	}
	return out
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func Build(p v2.Policy) (v2.Profile, error) {
	loaded, err := loadPacks()
	if err != nil {
		return v2.Profile{}, err
	}
	known := false
	for _, pack := range loaded {
		known = known || contains(pack.Profiles, p.Profile)
	}
	if p.Profile == "" || !known {
		return v2.Profile{}, errors.New("unknown ISC profile")
	}
	profile := v2.Profile{ID: p.Profile}
	types := map[string]bool{}
	for _, pack := range loaded {
		if !contains(pack.Profiles, p.Profile) {
			continue
		}
		rules, err := v2.Decode(pack.Rules)
		if err != nil {
			return profile, err
		}
		for _, r := range rules.Identities {
			types[r.Type] = true
		}
	}
	for _, r := range p.Identities {
		types[r.Type] = true
	}
	ordered := make([]string, 0, len(types))
	for typ := range types {
		ordered = append(ordered, typ)
	}
	sort.Strings(ordered)
	for _, pack := range loaded {
		if !contains(pack.Profiles, p.Profile) {
			continue
		}
		rules, err := v2.Decode(pack.Rules)
		if err != nil {
			return profile, err
		}
		evidence := &v2.Evidence{Kind: "isc-profile", RecordID: p.Profile, Digest: pack.digest, Attributes: map[string]string{"rule_pack": pack.ID, "rule_pack_version": fmt.Sprint(pack.Version), "layer": pack.Layer}}
		if rules.NullHandling != "" {
			if profile.Rules.NullHandling != "" && profile.Rules.NullHandling != rules.NullHandling {
				return profile, errors.New("conflicting rule pack defaults")
			}
			profile.Rules.NullHandling = rules.NullHandling
		}
		dst, src := ruleCategories(&profile.Rules), ruleCategories(&rules)
		for ci, cat := range src {
			for i := 0; i < cat.Len(); i++ {
				item := cat.Index(i)
				selector := item.FieldByName("Selector")
				template := selector.IsValid() && selector.Interface().(v2.Selector).Type == "$TYPE"
				expand := []string{""}
				if template {
					expand = ordered
				}
				for _, typ := range expand {
					copy := reflect.New(item.Type()).Elem()
					copy.Set(item)
					r := copy.FieldByName("Rule").Interface().(v2.Rule)
					r.Evidence = evidence
					if template {
						s := selector.Interface().(v2.Selector)
						s.Type = typ
						copy.FieldByName("Selector").Set(reflect.ValueOf(s))
						r.ID = strings.ReplaceAll(r.ID, "$TYPE", typ)
					}
					copy.FieldByName("Rule").Set(reflect.ValueOf(r))
					dst[ci].Set(reflect.Append(dst[ci], copy))
				}
			}
		}
		profile.Rules.Evidence = append(profile.Rules.Evidence, v2.Evidence{Kind: "isc-rule-pack", RecordID: pack.ID, Digest: pack.digest, Attributes: map[string]string{"version": fmt.Sprint(pack.Version), "layer": pack.Layer}})
	}
	return profile, nil
}

// ConnectorVariant classifies SOURCE data using shipped connector identifiers.
// Display names and the presence of connector-specific arrays are not evidence.
func ConnectorVariant(content any) (string, error) {
	loaded, err := loadPacks()
	if err != nil {
		return "", err
	}
	variant := ""
	for _, pack := range loaded {
		if pack.Connector == nil {
			continue
		}
		for _, id := range pack.Connector.Identifiers {
			value, ok := contentAt(content, id.Path).(string)
			if !ok || !contains(id.Values, value) {
				continue
			}
			if variant != "" && variant != pack.Connector.Variant {
				return "", errors.New("conflicting SOURCE connector identifiers")
			}
			variant = pack.Connector.Variant
		}
	}
	return variant, nil
}
func contentAt(content any, path string) any {
	if path == "" {
		return content
	}
	for _, part := range strings.Split(path[1:], "/") {
		m, ok := content.(map[string]any)
		if !ok {
			return nil
		}
		content = m[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
	}
	return content
}
