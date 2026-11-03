package differ

import (
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

type Options struct {
	Scope           []string
	IncludeMatching bool
}
type comparison struct {
	p           Policy
	report      *Report
	m           *matcher
	used        map[string]bool
	entry       *Entry
	left, right snapshot.Input
	key, typ    string
	asserted    map[string]bool
}

func Compare(left, right snapshot.Input, p Policy, o Options) (Report, error) {
	report := Report{Version: 1, Complete: false, Entries: []Entry{}, Diagnostics: []Diagnostic{}, Scope: []string{}, ExcludedLeft: []string{}, ExcludedRight: []string{}}
	if e := p.Validate(); e != nil {
		return report, e
	}
	b, e := json.Marshal(p)
	if e != nil {
		return report, errors.New("policy contains invalid JSON values")
	}
	p, e = DecodePolicy(b)
	if e != nil {
		return report, e
	}
	left, e = snapshot.Clone(left)
	if e != nil {
		return report, e
	}
	right, e = snapshot.Clone(right)
	if e != nil {
		return report, e
	}
	for side, env := range p.Bindings {
		in := &left
		if side == "right" {
			in = &right
		}
		if in.Environment != "" && in.Environment != env {
			return report, errors.New("conflicting environment binding")
		}
		in.Environment = env
	}
	report.Left, report.Right = summary(left), summary(right)
	report.PolicyID, report.PolicyDigest, report.Profile = p.ID, p.Digest(), p.Profile
	ls, rs := map[string]bool{}, map[string]bool{}
	for _, t := range left.Manifest.Types {
		ls[t] = true
	}
	for _, t := range right.Manifest.Types {
		rs[t] = true
	}
	scope := map[string]bool{}
	if len(o.Scope) == 0 {
		if !reflect.DeepEqual(ls, rs) {
			return report, errors.New("selected scopes differ; explicit common scope required")
		}
		for t := range ls {
			scope[t] = true
		}
	} else {
		if e := snapshot.ValidateTypes(o.Scope); e != nil {
			return report, e
		}
		for _, t := range o.Scope {
			if !ls[t] || !rs[t] {
				return report, errors.New("comparison scope absent from an input")
			}
			scope[t] = true
		}
	}
	report.Complete = true
	p = p.canonical()
	report.Scope = sortedKeys(scope)
	for _, t := range sortedKeys(ls) {
		if !scope[t] {
			report.ExcludedLeft = append(report.ExcludedLeft, t)
		}
	}
	for _, t := range sortedKeys(rs) {
		if !scope[t] {
			report.ExcludedRight = append(report.ExcludedRight, t)
		}
	}
	diagnose := func(d Diagnostic) {
		report.Diagnostics = append(report.Diagnostics, d)
		if d.Code != "unused rule" || d.Required {
			report.Complete = false
		}
	}
	m := match(left, right, p, scope, diagnose)
	c := comparison{p: p, report: &report, m: m, used: map[string]bool{}, left: left, right: right}
	for _, r := range p.Identities {
		for _, in := range []snapshot.Input{left, right} {
			for _, obj := range in.Objects {
				if scope[obj.Type] && obj.Type == r.Type {
					c.used[r.ID] = true
				}
			}
		}
	}

	for _, pair := range m.pairs {
		ent := Entry{PairingRule: pair.Rule}
		if pair.left != nil {
			ent.Type = pair.left.Type
			ent.Key = pair.left.Key
			ent.Left = &Provenance{pair.left.ID, pair.left.Path}
		}
		if pair.right != nil {
			ent.Type = pair.right.Type
			if ent.Key == "" {
				ent.Key = pair.right.Key
			}
			ent.Right = &Provenance{pair.right.ID, pair.right.Path}
		}
		if pair.Rule != "" {
			c.used[pair.Rule] = true
		}
		for _, r := range p.Sensitive {
			if r.Type != ent.Type || (r.Key != "" && r.Key != ent.Key) {
				continue
			}
			var a, b any
			if pair.left != nil {
				a = pair.left.Envelope
			}
			if pair.right != nil {
				b = pair.right.Envelope
			}
			for _, path := range expand(r.Path, a, b) {
				_, aok := lookup(a, path)
				_, bok := lookup(b, path)
				if aok || bok {
					c.used[r.ID] = true
				}
			}
		}
		switch {
		case pair.left == nil:
			ent.Status = "missing_left"
			report.Counts.MissingLeft++
		case pair.right == nil:
			ent.Status = "missing_right"
			report.Counts.MissingRight++
		default:
			c.entry = &ent
			c.key = ent.Key
			c.typ = ent.Type
			c.asserted = map[string]bool{}
			for _, r := range p.Expectations {
				if r.Type != c.typ || (r.Key != "" && r.Key != c.key) {
					continue
				}
				for _, path := range expand(r.Path, pair.left.Envelope, pair.right.Envelope) {
					c.used[r.ID] = true
					c.asserted[path] = true
					av, aok := lookup(pair.left.Envelope, path)
					bv, bok := lookup(pair.right.Envelope, path)
					failed := []string{}
					for i, in := range []snapshot.Input{left, right} {
						want, ok := r.Values[in.Environment]
						if !ok {
							diagnose(Diagnostic{Code: "expectation environment unavailable", Side: []string{"left", "right"}[i], Type: c.typ, RuleID: r.ID})
							continue
						}
						actual, present := av, aok
						if i == 1 {
							actual, present = bv, bok
						}
						if !present || !equal(actual, want) {
							failed = append(failed, []string{"left", "right"}[i])
						}
					}
					if len(failed) > 0 {
						ent.Differences = append(ent.Differences, Difference{path, "expectation", r.ID, failed})
					}
				}
			}
			c.walk(pair.left.Envelope, true, pair.right.Envelope, true, "")
			sort.Slice(ent.Differences, func(i, j int) bool {
				a, b := ent.Differences[i], ent.Differences[j]
				if a.Path != b.Path {
					return a.Path < b.Path
				}
				return a.Kind+a.RuleID < b.Kind+b.RuleID
			})
			if len(ent.Differences) > 0 {
				ent.Status = "different"
				report.Counts.Different++
			} else {
				ent.Status = "matching"
				report.Counts.Matching++
			}
		}
		c.redact(&ent)
		if ent.Status != "matching" || o.IncludeMatching {
			report.Entries = append(report.Entries, ent)
		}
	}
	unused := func(id string, required bool) {
		if !c.used[id] {
			diagnose(Diagnostic{Code: "unused rule", RuleID: id, Required: required})
		}
	}
	for _, r := range p.Identities {
		unused(r.ID, false)
	}
	for _, r := range p.Pairings {
		unused(r.ID, false)
	}
	for _, r := range p.Sensitive {
		unused(r.ID, false)
	}
	for _, r := range p.Ignores {
		unused(r.ID, r.Required)
	}
	for _, r := range p.Expectations {
		unused(r.ID, r.Required)
	}
	for _, r := range p.References {
		unused(r.ID, r.Required)
	}
	for _, r := range p.Collections {
		unused(r.ID, r.Required)
	}
	for _, r := range p.Hub {
		if r.Enabled {
			unused(r.ID, false)
		}
	}
	sort.Slice(report.Entries, func(i, j int) bool {
		a, b := report.Entries[i], report.Entries[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		return a.Status < b.Status
	})
	for i, d := range report.Diagnostics {
		for _, r := range p.Sensitive {
			if r.Type == d.Type && p.sensitiveIdentity(r) && d.Path != "" && !strings.HasPrefix(d.Path, "/") {
				report.Diagnostics[i].Path = "[redacted]"
			}
		}
	}
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		a, _ := json.Marshal(report.Diagnostics[i])
		b, _ := json.Marshal(report.Diagnostics[j])
		return string(a) < string(b)
	})
	return report, nil
}
func equal(a, b any) bool {
	if x, ok := a.(json.Number); ok {
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		xr, xok := new(big.Rat).SetString(x.String())
		yr, yok := new(big.Rat).SetString(y.String())
		return xok && yok && xr.Cmp(yr) == 0
	}
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equal(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i, v := range x {
			if !equal(v, y[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}
func expand(selector string, a, b any) []string {
	parts, _ := segments(selector)
	paths := []string{""}
	for _, part := range parts {
		next := []string{}
		for _, p := range paths {
			if part != "*" {
				next = append(next, p+"/"+escape(part))
				continue
			}
			keys := map[string]bool{}
			for _, root := range []any{a, b} {
				v, _ := lookup(root, p)
				switch v := v.(type) {
				case map[string]any:
					for k := range v {
						keys[k] = true
					}
				case []any:
					for i := range v {
						keys[strconv.Itoa(i)] = true
					}
				}
			}
			for _, k := range sortedKeys(keys) {
				next = append(next, p+"/"+escape(k))
			}
		}
		paths = next
	}
	return paths
}
func (c *comparison) diff(path, kind, rule string) {
	c.entry.Differences = append(c.entry.Differences, Difference{Path: path, Kind: kind, RuleID: rule})
}
func (c *comparison) diagnostic(code, path, rule string) {
	c.report.Complete = false
	c.report.Diagnostics = append(c.report.Diagnostics, Diagnostic{Code: code, Type: c.typ, Path: path, RuleID: rule})
}
func (c *comparison) walk(a any, aok bool, b any, bok bool, path string) {
	if c.asserted[path] {
		return
	}
	for _, r := range c.p.Ignores {
		if selectorMatch(r.Selector, c.typ, c.key, path) {
			c.used[r.ID] = true
			return
		}
	}
	if c.p.Profile != "" && (path == "/jwsHeader" || path == "/jwsSignature") {
		return
	}
	if c.p.Profile == "isc-portable-v1" {
		if path == "/self/id" || path == "/object/id" || path == "/object/created" || path == "/object/modified" || (c.typ == "LIFECYCLE_STATE" && path == "/object/identityCount") {
			return
		}
	}
	for _, r := range c.p.References {
		if !selectorMatch(r.Selector, c.typ, c.key, path) {
			continue
		}
		c.used[r.ID] = true
		as, aValid := a.(string)
		bs, bValid := b.(string)
		if !aok || !bok || !aValid || !bValid {
			c.diagnostic("unresolved reference", path, r.ID)
			return
		}
		lp, rp := physical(r.TargetType, as), physical(r.TargetType, bs)
		la, rb := c.m.left[lp], c.m.right[rp]
		// An explicit out-of-snapshot mapping cannot replace correspondence that
		// was established from the actual inventories, even if the supplied right
		// ID does not exist in that inventory.
		if target := c.m.correspondence[lp]; target != "" {
			if rb == nil {
				c.diagnostic("unresolved reference", path, r.ID)
			} else if target != rp {
				c.diff(path, "reference", r.ID)
			}
			return
		}
		if la != nil && rb != nil {
			target := c.m.correspondence[lp]
			if target == "" {
				c.diagnostic("unresolved reference correspondence", path, r.ID)
			} else if target != rp {
				c.diff(path, "reference", r.ID)
			}
			return
		}
		if v, ok := r.External[as]; ok {
			if v != bs {
				c.diff(path, "reference", r.ID)
			}
			return
		}
		c.diagnostic("unresolved reference", path, r.ID)
		return
	}
	for _, r := range c.p.Hub {
		if !r.Enabled || !selectorMatch(r.Selector, c.typ, c.key, path) {
			continue
		}
		if c.left.Manifest.Tenant != r.SourceTenant || c.right.Manifest.Tenant != r.TargetTenant {
			c.diagnostic("Hub tenant context mismatch", path, r.ID)
			return
		}
		if as, ok := a.(string); ok && as == r.Source && equal(b, r.Target) {
			c.used[r.ID] = true
			return
		}
	}
	if !aok || !bok {
		if aok != bok {
			kind := "missing_left"
			if aok {
				kind = "missing_right"
			}
			c.diff(path, kind, "")
		}
		return
	}
	am, amok := a.(map[string]any)
	bm, bmok := b.(map[string]any)
	if amok && bmok {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		for _, k := range sortedKeys(keys) {
			av, ao := am[k]
			bv, bo := bm[k]
			c.walk(av, ao, bv, bo, path+"/"+escape(k))
		}
		return
	}
	aa, aaok := a.([]any)
	ba, baok := b.([]any)
	if aaok && baok {
		for _, r := range c.p.Collections {
			if selectorMatch(r.Selector, c.typ, c.key, path) {
				c.used[r.ID] = true
				c.keyed(aa, ba, path, r)
				return
			}
		}
		n := len(aa)
		if len(ba) > n {
			n = len(ba)
		}
		for i := 0; i < n; i++ {
			var av, bv any
			if i < len(aa) {
				av = aa[i]
			}
			if i < len(ba) {
				bv = ba[i]
			}
			c.walk(av, i < len(aa), bv, i < len(ba), path+"/"+strconv.Itoa(i))
		}
		return
	}
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		c.diff(path, "type", "")
	} else if !equal(a, b) {
		c.diff(path, "value", "")
	}
}
func (c *comparison) keyed(a, b []any, path string, r Collection) {
	indices := []map[string]int{{}, {}}
	for side, arr := range [][]any{a, b} {
		for i, item := range arr {
			parts := []any{}
			for _, k := range r.Keys {
				v, ok := lookup(item, k)
				if !ok || v == nil {
					c.diagnostic("missing collection key", path, r.ID)
					return
				}
				switch v.(type) {
				case string, bool, json.Number:
				default:
					c.diagnostic("non-scalar collection key", path, r.ID)
					return
				}
				valueType := reflect.TypeOf(v).String()
				if n, ok := v.(json.Number); ok {
					rat, _ := new(big.Rat).SetString(n.String())
					if rat == nil {
						c.diagnostic("invalid numeric key", path, r.ID)
						return
					}
					v = rat.RatString()
				}
				parts = append(parts, struct {
					Type  string
					Value any
				}{valueType, v})
			}
			raw, _ := json.Marshal(parts)
			key := string(raw)
			if _, ok := indices[side][key]; ok {
				c.diagnostic("duplicate collection key", path, r.ID)
				return
			}
			indices[side][key] = i
		}
	}
	keys := map[string]bool{}
	for _, idx := range indices {
		for k := range idx {
			keys[k] = true
		}
	}
	for _, k := range sortedKeys(keys) {
		ai, ao := indices[0][k]
		bi, bo := indices[1][k]
		var av, bv any
		if ao {
			av = a[ai]
		}
		if bo {
			bv = b[bi]
		}
		i := ai
		if !ao {
			i = bi
		}
		c.walk(av, ao, bv, bo, path+"/"+strconv.Itoa(i))
	}
}

// Reports omit all payload values. Sensitive identity selectors additionally hide
// logical keys and physical provenance, which can themselves contain values.
func (p Policy) sensitiveIdentity(r Sensitive) bool {
	if identityPath(r.Selector) {
		return true
	}
	for _, i := range p.Identities {
		if i.Type == r.Type {
			for _, path := range i.Paths {
				if overlaps(r.Selector, Selector{Type: r.Type, Path: path}) {
					return true
				}
			}
		}
	}
	return false
}
func (c *comparison) redact(e *Entry) {
	for _, r := range c.p.Sensitive {
		direct := r.Type == e.Type && (r.Key == "" || r.Key == e.Key)
		derived := (r.Type == "IDENTITY_PROFILE" && e.Type == "LIFECYCLE_STATE") || (r.Type == "SOURCE" && e.Type == "ATTR_SYNC_SOURCE_CONFIG")
		if (direct || derived) && c.p.sensitiveIdentity(r) {
			e.Key = "[redacted]"
			if e.Left != nil {
				e.Left = &Provenance{ID: "[redacted]", Path: "[redacted]"}
			}
			if e.Right != nil {
				e.Right = &Provenance{ID: "[redacted]", Path: "[redacted]"}
			}
		}
	}
}
