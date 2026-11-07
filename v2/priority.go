package v2

import (
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func priorityNumber(value any) (*big.Rat, error) {
	n, ok := value.(json.Number)
	if !ok {
		return nil, errors.New("numeric priority required")
	}
	r, ok := new(big.Rat).SetString(string(n))
	if !ok {
		return nil, errors.New("numeric priority required")
	}
	return r, nil
}

type priorityEntry struct {
	key, label string
	number     *big.Rat
}

func sortPriorities(entries []priorityEntry) ([]string, []ContextField, error) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].number.Cmp(entries[j].number) < 0 })
	order := []string{}
	context := []ContextField{}
	for i, e := range entries {
		if i > 0 && e.number.Cmp(entries[i-1].number) == 0 {
			return nil, nil, errors.New("duplicate priorities make precedence ambiguous")
		}
		order = append(order, e.key)
		context = append(context, ContextField{Label: strconv.Itoa(i + 1), Value: e.label})
	}
	return order, context, nil
}

// Compare relations to common corresponding peers, not absolute ranks. An
// unmatched extra object must not shift every other object's relative priority.
func (x *execution) relativeOrder(f frame, r Equivalence) {
	defer func() { x.orderContext = [2][]ContextField{}; x.normalization = "" }()
	entries := [2][]priorityEntry{}
	for _, g := range x.inventory.groups {
		if g.typ != r.Type || g.nodes[0] == nil || g.nodes[1] == nil {
			continue
		}
		for side := 0; side < 2; side++ {
			value, present := get(g.nodes[side].doc.Content, r.Path)
			n, err := priorityNumber(value)
			if !present || err != nil {
				x.record(f, "unverified", "relative priority context requires numeric priorities", r.Rule, "")
				return
			}
			entries[side] = append(entries[side], priorityEntry{key: g.key, label: g.key, number: n})
		}
	}
	orders := [2][]string{}
	for side := 0; side < 2; side++ {
		var err error
		orders[side], x.orderContext[side], err = sortPriorities(entries[side])
		if err != nil {
			x.record(f, "conflict", err.Error(), r.Rule, "")
			return
		}
	}
	predecessors := func(order []string) map[string]bool {
		out := map[string]bool{}
		for _, key := range order {
			if key == x.current.key {
				break
			}
			out[key] = true
		}
		return out
	}
	if !reflect.DeepEqual(predecessors(orders[0]), predecessors(orders[1])) {
		x.record(f, "different", "relative priority order differs", r.Rule, "")
	} else if equal(f.nodes[0].value, f.nodes[1].value) {
		x.record(f, "equal", "relative priority order unchanged", r.Rule, "")
	} else {
		x.normalization = "relative_priority_order"
		x.record(f, "equivalent", "relative priority order unchanged", r.Rule, "")
	}
}

// No precedence is derivable or needed for a single entry, or for collections
// with no contextual references at all. Preserve ordinary positional semantics
// for these values, including global/unscoped rules.
func (x *execution) positionalPriorityCollection(a [3][]any, r Collection) bool {
	if len(a[0]) <= 1 && len(a[1]) <= 1 && len(a[2]) <= 1 {
		return true
	}
	for _, arr := range a {
		for _, item := range arr {
			value, ok := get(item, r.PriorityOrder.ReferencePath)
			if ok && value != nil {
				return false
			}
		}
	}
	return true
}

func (x *execution) collectionPriority(f frame, r Collection, a [3][]any, idx [3]map[string]int) {
	defer func() { x.orderContext = [2][]ContextField{}; x.normalization = "" }()
	o := r.PriorityOrder
	entries := [2][]priorityEntry{}
	for side := 0; side < 2; side++ {
		if !contains(x.inputs[side].Types, o.Type) {
			x.record(f, "unverified", "priority context type unavailable: "+o.Type, r.Rule, "")
			return
		}
		for _, key := range keys(idx[side]) {
			item := a[side][idx[side][key]]
			ref, _ := get(item, o.ReferencePath)
			id, ok := ref.(string)
			if !ok || strings.TrimSpace(id) == "" {
				x.record(f, "unverified", "collection priority reference unavailable", r.Rule, "")
				return
			}
			var context *Document
			for i := range x.inputs[side].Documents {
				doc := &x.inputs[side].Documents[i]
				if doc.Type != o.Type {
					continue
				}
				value, present := get(doc.Content, o.MatchPath)
				if present && equal(value, ref) {
					if context != nil {
						x.record(f, "conflict", "ambiguous collection priority reference", r.Rule, "")
						return
					}
					context = doc
				}
			}
			if context == nil {
				x.record(f, "unverified", "collection priority reference unresolved", r.Rule, "")
				return
			}
			value, present := get(context.Content, o.PriorityPath)
			n, err := priorityNumber(value)
			if !present || err != nil {
				x.record(f, "unverified", "collection priority context requires numeric priorities", r.Rule, "")
				return
			}
			labels := []string{}
			for _, path := range r.Keys {
				v, _ := get(item, path)
				if s, ok := v.(string); ok {
					labels = append(labels, s)
				} else {
					labels = append(labels, valueKey(v))
				}
			}
			entries[side] = append(entries[side], priorityEntry{key: key, label: strings.Join(labels, " / "), number: n})
		}
	}
	orders := [2][]string{}
	for side := 0; side < 2; side++ {
		order, context, err := sortPriorities(entries[side])
		if err != nil {
			x.record(f, "conflict", err.Error(), r.Rule, "")
			return
		}
		x.orderContext[side] = context
		// Added/removed members are compared by the keyed traversal. Only shared
		// members can establish a reversal of relative precedence.
		for _, key := range order {
			if _, ok := idx[1-side][key]; ok {
				orders[side] = append(orders[side], key)
			}
		}
	}
	if x.request.Mode == "deployment" {
		x.record(f, "unverified", "priority ordering requires an authoritative whole-value expectation", r.Rule, "")
	} else if reflect.DeepEqual(orders[0], orders[1]) {
		x.normalization = "relative_priority_order"
		x.record(f, "equivalent", "relative priority order unchanged", r.Rule, "")
	} else {
		x.record(f, "different", "relative priority order differs", r.Rule, "")
	}
}
