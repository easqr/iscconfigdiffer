// Package differ compares validated offline ISC configuration inputs.
package differ

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

type Selector struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Key  string `json:"key,omitempty"`
}
type Ignore struct {
	ID string `json:"id"`
	Selector
	Required bool `json:"required,omitempty"`
}
type Expectation struct {
	ID string `json:"id"`
	Selector
	Values   map[string]any `json:"values"`
	Required bool           `json:"required,omitempty"`
}
type Pairing struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	LeftID  string `json:"left_id"`
	RightID string `json:"right_id"`
}
type Identity struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Strategy string   `json:"strategy"`
	Paths    []string `json:"paths,omitempty"`
}
type Reference struct {
	ID string `json:"id"`
	Selector
	TargetType string            `json:"target_type"`
	External   map[string]string `json:"external,omitempty"`
	Required   bool              `json:"required,omitempty"`
}
type Collection struct {
	ID string `json:"id"`
	Selector
	Keys     []string `json:"keys"`
	Required bool     `json:"required,omitempty"`
}
type Sensitive struct {
	ID string `json:"id"`
	Selector
}
type HubRule struct {
	JSONPath string `json:"json_path"`
	Created  string `json:"created,omitempty"`
	Modified string `json:"modified,omitempty"`
	ID       string `json:"id"`
	Selector
	Source       string `json:"source"`
	Target       string `json:"target"`
	Enabled      bool   `json:"enabled"`
	SourceTenant string `json:"source_tenant"`
	TargetTenant string `json:"target_tenant"`
	Provenance   string `json:"provenance"`
}
type Policy struct {
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Profile      string            `json:"profile,omitempty"`
	Bindings     map[string]string `json:"bindings,omitempty"`
	Identities   []Identity        `json:"identities,omitempty"`
	Pairings     []Pairing         `json:"pairings,omitempty"`
	Ignores      []Ignore          `json:"ignores,omitempty"`
	Expectations []Expectation     `json:"expectations,omitempty"`
	References   []Reference       `json:"references,omitempty"`
	Collections  []Collection      `json:"collections,omitempty"`
	Sensitive    []Sensitive       `json:"sensitive,omitempty"`
	Hub          []HubRule         `json:"hub,omitempty"`
}

func DefaultPolicy() Policy { return Policy{Version: 1, ID: "default", Profile: "isc-envelope-v1"} }
func DecodePolicy(b []byte) (Policy, error) {
	var p Policy
	if v, e := snapshot.DecodeJSON(b); e != nil {
		return p, errors.New("invalid policy JSON")
	} else if e := exactShape(v, reflect.TypeFor[Policy]()); e != nil {
		return p, e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	d.DisallowUnknownFields()
	if e := d.Decode(&p); e != nil {
		return p, errors.New("unknown or malformed policy property")
	}
	return p, p.Validate()
}
func (p Policy) Digest() string { b, _ := json.Marshal(p.canonical()); return snapshot.Checksum(b) }
func segments(p string) ([]string, error) {
	if p == "" {
		return nil, nil
	}
	if !strings.HasPrefix(p, "/") {
		return nil, errors.New("selector must be JSON Pointer")
	}
	a := strings.Split(p[1:], "/")
	for i, s := range a {
		for j := 0; j < len(s); j++ {
			if s[j] == '~' {
				if j+1 >= len(s) || (s[j+1] != '0' && s[j+1] != '1') {
					return nil, errors.New("invalid pointer escape")
				}
				j++
			}
		}
		a[i] = strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
	}
	return a, nil
}
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
func selectorMatch(s Selector, typ, key, path string) bool {
	if s.Type != typ || (s.Key != "" && s.Key != key) {
		return false
	}
	a, _ := segments(s.Path)
	b, _ := segments(path)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != "*" && a[i] != b[i] {
			return false
		}
	}
	return true
}
func overlaps(a, b Selector) bool {
	if a.Type != b.Type || (a.Key != "" && b.Key != "" && a.Key != b.Key) {
		return false
	}
	x, _ := segments(a.Path)
	y, _ := segments(b.Path)
	n := len(x)
	if len(y) < n {
		n = len(y)
	}
	for i := 0; i < n; i++ {
		if x[i] != "*" && y[i] != "*" && x[i] != y[i] {
			return false
		}
	}
	return true
}
func identityPath(s Selector) bool {
	for _, v := range []string{"/self", "/object/id", "/object/name", "/object/technicalName", "/object/identityProfileId", "/object/sourceId", "/object/key", "/object/locale", "/object/medium"} {
		if overlaps(s, Selector{Type: s.Type, Path: v}) {
			return true
		}
	}
	return false
}
func (p Policy) Validate() error {
	if p.Version != 1 || !ruleID.MatchString(p.ID) {
		return errors.New("unsupported policy version or missing ID")
	}
	if p.Profile != "" && p.Profile != "isc-envelope-v1" && p.Profile != "isc-portable-v1" {
		return errors.New("unknown metadata profile")
	}
	for side, env := range p.Bindings {
		if (side != "left" && side != "right") || env == "" {
			return errors.New("invalid environment binding")
		}
	}
	ids := map[string]bool{}
	add := func(id string) error {
		if !ruleID.MatchString(id) || ids[id] {
			return errors.New("missing or duplicate rule ID")
		}
		ids[id] = true
		return nil
	}
	check := func(id string, s Selector) error {
		if e := add(id); e != nil {
			return e
		}
		if snapshot.ValidateTypes([]string{s.Type}) != nil {
			return errors.New("rule type required")
		}
		_, e := segments(s.Path)
		return e
	}
	occupied := []Selector{}
	claim := func(s Selector) error {
		for _, old := range occupied {
			if overlaps(old, s) {
				return errors.New("overlapping comparison rules")
			}
		}
		occupied = append(occupied, s)
		return nil
	}
	for _, r := range p.Ignores {
		if e := check(r.ID, r.Selector); e != nil {
			return e
		}
		if identityPath(r.Selector) {
			return errors.New("identity fields cannot be ignored")
		}
		for _, i := range p.Identities {
			for _, path := range i.Paths {
				if overlaps(r.Selector, Selector{Type: i.Type, Path: path}) {
					return errors.New("custom identity field cannot be ignored")
				}
			}
		}
		if e := claim(r.Selector); e != nil {
			return e
		}
	}
	for _, r := range p.Expectations {
		if e := check(r.ID, r.Selector); e != nil {
			return e
		}
		if len(r.Values) == 0 {
			return errors.New("expectation values required")
		}
		for env := range r.Values {
			if env == "" {
				return errors.New("empty expectation environment")
			}
		}
		if e := claim(r.Selector); e != nil {
			return e
		}
	}
	for _, r := range p.References {
		if e := check(r.ID, r.Selector); e != nil {
			return e
		}
		if r.TargetType == "" {
			return errors.New("reference target type required")
		}
		if e := claim(r.Selector); e != nil {
			return e
		}
		seen := map[string]bool{}
		for _, v := range r.External {
			if seen[v] {
				return errors.New("external mapping must be one-to-one")
			}
			seen[v] = true
		}
	}
	for _, r := range p.Collections {
		if e := check(r.ID, r.Selector); e != nil {
			return e
		}
		if len(r.Keys) == 0 {
			return errors.New("collection keys required")
		}
		for _, k := range r.Keys {
			if _, e := segments(k); e != nil {
				return e
			}
			if strings.Contains(k, "*") {
				return errors.New("collection key must be concrete")
			}
		}
		for _, old := range occupied {
			a, _ := segments(old.Path)
			b, _ := segments(r.Path)
			if overlaps(old, r.Selector) && len(a) <= len(b) {
				return errors.New("overlapping collection rule")
			}
		}

	}
	for i, a := range p.Collections {
		for _, b := range p.Collections[i+1:] {
			if overlaps(a.Selector, b.Selector) {
				return errors.New("overlapping collection rules")
			}
		}
	}
	types := map[string]bool{}
	for _, r := range p.Identities {
		if e := add(r.ID); e != nil {
			return e
		}
		if r.Type == "" || types[r.Type] {
			return errors.New("duplicate identity strategy")
		}
		types[r.Type] = true
		if r.Strategy != "fields" && r.Strategy != "singleton" {
			return errors.New("unknown identity strategy")
		}
		if (r.Strategy == "singleton" && len(r.Paths) > 0) || (r.Strategy == "fields" && len(r.Paths) == 0) {
			return errors.New("identity paths required")
		}
		for _, path := range r.Paths {
			if _, e := segments(path); e != nil || strings.Contains(path, "*") {
				return errors.New("invalid identity path")
			}
		}
	}
	left, right := map[string]bool{}, map[string]bool{}
	for _, r := range p.Pairings {
		if e := add(r.ID); e != nil {
			return e
		}
		l, rr := r.Type+"\x00"+r.LeftID, r.Type+"\x00"+r.RightID
		if r.Type == "" || r.LeftID == "" || r.RightID == "" || left[l] || right[rr] {
			return errors.New("conflicting pairing")
		}
		left[l] = true
		right[rr] = true
	}
	for _, r := range p.Sensitive {
		if e := check(r.ID, r.Selector); e != nil {
			return e
		}
	}
	hubs := map[string][]HubRule{}
	for _, r := range p.Hub {
		if e := check(r.ID, r.Selector); e != nil {
			return e
		}
		if r.SourceTenant == "" || r.TargetTenant == "" || r.Provenance == "" {
			return errors.New("Hub context and provenance required")
		}
		if !r.Enabled {
			continue
		}
		translated, e := HubPointer(r.JSONPath)
		if e != nil || translated != r.Path {
			return errors.New("Hub JSONPath translation mismatch")
		}
		for _, old := range occupied {
			if overlaps(old, r.Selector) {
				return errors.New("Hub and comparison rule overlap")
			}
		}
		k := r.Type + "\x00" + r.Path
		for _, old := range hubs[k] {
			if old.Source == r.Source || old.Target == r.Target || old.Source == r.Target || old.Target == r.Source {
				return errors.New("ambiguous Hub substitutions")
			}
		}
		for oldKey, group := range hubs {
			if oldKey == k {
				continue
			}
			for _, old := range group {
				if overlaps(old.Selector, r.Selector) {
					return errors.New("overlapping Hub selectors")
				}
			}
		}
		hubs[k] = append(hubs[k], r)
	}
	profileSelectors := append([]Selector{}, occupied...)
	for _, r := range p.Collections {
		profileSelectors = append(profileSelectors, r.Selector)
	}
	for _, r := range p.Hub {
		if r.Enabled {
			profileSelectors = append(profileSelectors, r.Selector)
		}
	}
	for _, sel := range profileSelectors {
		if p.Profile != "" {
			for _, path := range []string{"/jwsHeader", "/jwsSignature"} {
				if overlaps(sel, Selector{Type: sel.Type, Path: path}) {
					return errors.New("rule overlaps metadata profile")
				}
			}
		}
		if p.Profile == "isc-portable-v1" {
			paths := []string{"/self/id", "/object/id", "/object/created", "/object/modified"}
			if sel.Type == "LIFECYCLE_STATE" {
				paths = append(paths, "/object/identityCount")
			}
			for _, path := range paths {
				if overlaps(sel, Selector{Type: sel.Type, Path: path}) {
					return errors.New("rule overlaps metadata profile")
				}
			}
		}
	}
	return nil
}
func lookup(v any, p string) (any, bool) {
	a, e := segments(p)
	if e != nil {
		return nil, false
	}
	for _, s := range a {
		switch x := v.(type) {
		case map[string]any:
			var ok bool
			v, ok = x[s]
			if !ok {
				return nil, false
			}
		case []any:
			i, e := strconv.Atoi(s)
			if e != nil || strconv.Itoa(i) != s || i < 0 || i >= len(x) {
				return nil, false
			}
			v = x[i]
		default:
			return nil, false
		}
	}
	return v, true
}
func sortedKeys[V any](m map[string]V) []string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}

func sortRules[T any](rules []T, id func(T) string) []T {
	out := slices.Clone(rules)
	sort.Slice(out, func(i, j int) bool { return id(out[i]) < id(out[j]) })
	return out
}
func (p Policy) canonical() Policy {
	p.Identities = sortRules(p.Identities, func(r Identity) string { return r.ID })
	p.Pairings = sortRules(p.Pairings, func(r Pairing) string { return r.ID })
	p.Ignores = sortRules(p.Ignores, func(r Ignore) string { return r.ID })
	p.Expectations = sortRules(p.Expectations, func(r Expectation) string { return r.ID })
	p.References = sortRules(p.References, func(r Reference) string { return r.ID })
	p.Collections = sortRules(p.Collections, func(r Collection) string { return r.ID })
	p.Sensitive = sortRules(p.Sensitive, func(r Sensitive) string { return r.ID })
	p.Hub = sortRules(p.Hub, func(r HubRule) string { return r.ID })
	return p
}

var ruleID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$`)

// encoding/json accepts case-insensitive property aliases; our versioned policy
// contract requires exact spellings, including inside embedded selectors.
func exactShape(v any, t reflect.Type) error {
	if t.Kind() == reflect.Interface {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		if v == nil {
			return nil
		}
		return exactShape(v, t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("policy or mapping object required")
		}
		fields := map[string]reflect.Type{}
		var collect func(reflect.Type)
		collect = func(t reflect.Type) {
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.Anonymous {
					collect(f.Type)
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")[0]
				if tag != "" && tag != "-" {
					fields[tag] = f.Type
				}
			}
		}
		collect(t)
		for k, value := range m {
			ft, ok := fields[k]
			if !ok {
				return errors.New("unknown policy or mapping property")
			}
			if e := exactShape(value, ft); e != nil {
				return e
			}
		}
	case reflect.Slice:
		a, ok := v.([]any)
		if !ok {
			return errors.New("policy or mapping array required")
		}
		for _, item := range a {
			if e := exactShape(item, t.Elem()); e != nil {
				return e
			}
		}
	case reflect.Map:
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("policy map required")
		}
		for _, item := range m {
			if e := exactShape(item, t.Elem()); e != nil {
				return e
			}
		}
	case reflect.String:
		if _, ok := v.(string); !ok {
			return errors.New("policy string required")
		}
	case reflect.Bool:
		if _, ok := v.(bool); !ok {
			return errors.New("policy boolean required")
		}
	case reflect.Int:
		if _, ok := v.(json.Number); !ok {
			return errors.New("policy integer required")
		}
	}
	return nil
}
