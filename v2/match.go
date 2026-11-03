package v2

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
)

type member struct {
	doc   Document
	key   string
	token string
}
type group struct {
	typ, key, rule string
	nodes          [3]*member
}
type inventory struct {
	groups   []*group
	physical [3]map[string]*group
}

func physical(typ, id string) string { b, _ := json.Marshal([]string{typ, id}); return string(b) }
func detached(in Input) (Input, error) {
	out := in
	out.Types = append([]string{}, in.Types...)
	sort.Strings(out.Types)
	seen := map[string]bool{}
	for _, t := range out.Types {
		if t == "" || seen[t] {
			return out, errors.New("invalid input type scope")
		}
		seen[t] = true
	}
	out.Documents = []Document{}
	seenID := map[string]bool{}
	for _, doc := range in.Documents {
		if doc.Type == "" || doc.ID == "" || !seen[doc.Type] || seenID[physical(doc.Type, doc.ID)] {
			return out, errors.New("invalid document identity/scope")
		}
		seenID[physical(doc.Type, doc.ID)] = true
		b, e := json.Marshal(doc)
		if e != nil {
			return out, errors.New("invalid document JSON")
		}
		var d Document
		v, e := JSON(b)
		if e != nil {
			return out, e
		}
		b, _ = json.Marshal(v)
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		if e = dec.Decode(&d); e != nil {
			return out, e
		}
		out.Documents = append(out.Documents, d)
	}
	sort.Slice(out.Documents, func(i, j int) bool {
		return physical(out.Documents[i].Type, out.Documents[i].ID) < physical(out.Documents[j].Type, out.Documents[j].ID)
	})
	b, _ := json.Marshal(struct {
		Types     []string
		Documents []Document
	}{out.Types, out.Documents})
	out.DocumentHash = hash(b)
	if out.ContentHash == "" {
		out.ContentHash = out.DocumentHash
	}
	return out, nil
}
func matchInputs(inputs [3]*Input, p Policy, scope map[string]bool, mode string, diagnose func(Diagnostic), use func(string)) *inventory {
	m := &inventory{}
	for i := range m.physical {
		m.physical[i] = map[string]*group{}
	}
	strategies := map[string]Identity{}
	for _, r := range p.Identities {
		strategies[r.Type] = r
	}
	order := []string{}
	seen := map[string]bool{}
	var add func(string)
	add = func(t string) {
		if seen[t] {
			return
		}
		seen[t] = true
		r := strategies[t]
		if r.Strategy == "dependent" {
			add(r.ParentType)
		}
		order = append(order, t)
	}
	for _, t := range keys(scope) {
		add(t)
	}
	for _, typ := range order {
		if !scope[typ] {
			continue
		}
		r, ok := strategies[typ]
		if !ok {
			diagnose(Diagnostic{Code: "identity strategy unavailable", Type: typ})
			continue
		}
		byKey := [3]map[string]*member{}
		byID := [3]map[string]*member{}
		bad := [3]map[string]bool{}
		for side, in := range inputs {
			byKey[side] = map[string]*member{}
			byID[side] = map[string]*member{}
			bad[side] = map[string]bool{}
			if in == nil {
				continue
			}
			for _, doc := range in.Documents {
				if doc.Type != typ {
					continue
				}
				parts := []any{}
				valid := true
				if r.Strategy == "dependent" {
					v, ok := get(doc.Content, r.ParentPath)
					id, s := v.(string)
					parent := m.physical[side][physical(r.ParentType, id)]
					if !ok || !s || parent == nil || mode == "drift" && (parent.nodes[0] == nil || parent.nodes[1] == nil) {
						valid = false
					} else {
						parts = append(parts, parent.key)
					}
				}
				for _, path := range r.Paths {
					v, ok := get(doc.Content, path)
					_, isString := v.(string)
					if !ok || !scalar(v) || v == "" || r.ComponentType == "string" && !isString {
						valid = false
						break
					}
					parts = append(parts, v)
				}
				if !valid {
					diagnose(Diagnostic{Code: "identity dependency/component unavailable", Type: typ, Side: role(side, inputs[2] != nil, false)})
					continue
				}
				k := "singleton"
				if r.Strategy != "singleton" {
					if len(parts) == 1 {
						if s, ok := parts[0].(string); ok {
							k = s
						} else {
							k = valueKey(parts)
						}
					} else {
						b, _ := json.Marshal(parts)
						k = string(b)
					}
				}
				token := valueKey(parts)
				use(r.ID)
				o := &member{doc: doc, key: k, token: token}
				byID[side][doc.ID] = o
				if byKey[side][token] != nil {
					bad[side][token] = true
					diagnose(Diagnostic{Code: "duplicate logical identity", Type: typ})
				}
				byKey[side][token] = o
			}
			for k := range bad[side] {
				delete(byKey[side], k)
			}
			for id, o := range byID[side] {
				if bad[side][o.token] {
					delete(byID[side], id)
				}
			}
		}
		used := [3]map[string]bool{{}, {}, {}}
		appendGroup := func(g *group) {
			for i, o := range g.nodes {
				if o != nil && used[i][o.doc.ID] {
					diagnose(Diagnostic{Code: "ambiguous correspondence endpoint", Type: typ, RuleID: g.rule})
					return
				}
			}
			m.groups = append(m.groups, g)
			if g.rule != r.ID {
				use(g.rule)
			}
			for i, o := range g.nodes {
				if o != nil {
					used[i][o.doc.ID] = true
					m.physical[i][physical(typ, o.doc.ID)] = g
				}
			}
		}
		for _, pair := range p.Pairings {
			if pair.Type != typ {
				continue
			}
			a, b := byID[0][pair.LeftID], byID[1][pair.RightID]
			if a == nil || b == nil {
				diagnose(Diagnostic{Code: "pairing endpoint unavailable", RuleID: pair.ID, Type: typ})
				continue
			}
			g := &group{typ: typ, key: a.key, rule: pair.ID, nodes: [3]*member{a, b, nil}}
			if inputs[2] != nil {
				if pair.BaselineID != "" {
					g.nodes[2] = byID[2][pair.BaselineID]
					if g.nodes[2] == nil {
						diagnose(Diagnostic{Code: "baseline pairing endpoint unavailable", RuleID: pair.ID, Type: typ})
					}
				} else {
					left, right := byKey[2][a.token], byKey[2][b.token]
					if left != nil && right != nil && left != right {
						diagnose(Diagnostic{Code: "ambiguous baseline pairing; explicit baseline_id required", Type: typ, RuleID: pair.ID})
					} else if left != nil {
						g.nodes[2] = left
					} else {
						g.nodes[2] = right
					}
				}
			} else if pair.BaselineID != "" {
				diagnose(Diagnostic{Code: "baseline pairing input unavailable", RuleID: pair.ID, Type: typ})
			}
			appendGroup(g)
		}
		all := map[string]bool{}
		for _, idx := range byKey {
			for k := range idx {
				all[k] = true
			}
		}
		for _, k := range keys(all) {
			g := &group{typ: typ, rule: r.ID}
			for side, idx := range byKey {
				if o := idx[k]; o != nil && !used[side][o.doc.ID] {
					g.nodes[side] = o
					if g.key == "" {
						g.key = o.key
					}
				}
			}
			if g.nodes[0] != nil || g.nodes[1] != nil || g.nodes[2] != nil {
				appendGroup(g)
			}
		}
	}
	display := map[string]bool{}
	for _, g := range m.groups {
		k := physical(g.typ, g.key)
		if display[k] {
			diagnose(Diagnostic{Code: "ambiguous logical key presentation", Type: g.typ})
		}
		display[k] = true
	}
	sort.Slice(m.groups, func(i, j int) bool {
		a, b := m.groups[i], m.groups[j]
		if a.typ != b.typ {
			return a.typ < b.typ
		}
		return a.key < b.key
	})
	return m
}
