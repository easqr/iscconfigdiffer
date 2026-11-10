package v2

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"sort"
	"strconv"
)

type slot struct {
	value       any
	present     bool
	pointer     string
	unavailable bool
}
type frame struct {
	items       map[string]map[string]CollectionItemKey
	itemScope   *CollectionItemSelector
	itemScopes  []CollectionItemSelector
	extensions  []CollectionMappingUsage
	property    bool
	nodes       [3]slot
	path, space string
	itemKeys    []string
}
type execution struct {
	c             *Compiled
	request       Request
	report        Report
	inputs        [3]*Input
	inventory     *inventory
	current       *group
	roles         [3]string
	derivation    *Derivation
	expected      *State
	expectedRole  string
	normalization string
	orderContext  [2][]ContextField
	operations    map[string]Operation
}

func role(side int, _ bool, deployment bool) string {
	if deployment {
		return [3]string{"source", "actual", "baseline"}[side]
	}
	return [3]string{"left", "right", "baseline"}[side]
}
func Run(request Request, c *Compiled) (Report, error) {
	x := execution{c: c, request: request, roles: [3]string{"left", "right", "baseline"}}
	x.report = Report{NullHandling: cNullHandling(c), ValuesExcluded: request.ExcludeValues, Version: 2, Mode: request.Mode, Complete: false, Correct: false, Inputs: map[string]Input{}, Excluded: map[string][]string{}, Counts: map[string]int{}, RuleUsage: map[string]int{}, Replacements: map[string]string{}, Matches: []Match{}, Decisions: []Decision{}, Diagnostics: []Diagnostic{}}
	x.report.ObjectCoverage = map[string]int{}
	if c == nil || c.policy.Version != 2 || c.digest == "" {
		return x.report, errors.New("compiled policy required")
	}
	if request.Mode != "drift" && request.Mode != "deployment" {
		return x.report, errors.New("explicit drift/deployment mode required")
	}
	if len(c.policy.Modes) > 0 && !contains(c.policy.Modes, request.Mode) {
		return x.report, errors.New("policy does not permit requested mode")
	}
	if request.Mode == "drift" && (request.Baseline != nil || len(request.Operations) > 0) {
		return x.report, errors.New("drift cannot take deployment inputs")
	}
	if request.Mode == "deployment" {
		x.roles = [3]string{"source", "actual", "baseline"}
		if len(request.Operations) == 0 {
			return x.report, errors.New("explicit deployment operations required")
		}
	}
	left, e := detached(request.Left)
	if e != nil {
		return x.report, e
	}
	right, e := detached(request.Right)
	if e != nil {
		return x.report, e
	}
	x.inputs = [3]*Input{&left, &right, nil}
	if request.Baseline != nil {
		b, e := detached(*request.Baseline)
		if e != nil {
			return x.report, e
		}
		x.inputs[2] = &b
	}
	for role, env := range c.policy.Bindings {
		found := false
		for i, in := range x.inputs {
			if x.roles[i] != role {
				continue
			}
			found = true
			if in == nil {
				return x.report, errors.New("binding input unavailable")
			}
			if in.Environment != "" && in.Environment != env {
				return x.report, errors.New("conflicting environment binding")
			}
			in.Environment = env
		}
		if !found {
			return x.report, errors.New("binding role conflicts with mode")
		}
	}
	if request.ObjectKey != "" && len(request.Scope) != 1 {
		return x.report, errors.New("object key requires one explicit scope type")
	}
	scope := map[string]bool{}
	if len(request.Scope) == 0 {
		if !reflect.DeepEqual(left.Types, right.Types) {
			return x.report, errors.New("selected scopes differ; explicit scope required")
		}
		for _, t := range left.Types {
			scope[t] = true
		}
	} else {
		for _, t := range request.Scope {
			if scope[t] || !contains(left.Types, t) || !contains(right.Types, t) {
				return x.report, errors.New("invalid or unavailable scope")
			}
			scope[t] = true
		}
	}
	for _, in := range x.inputs {
		if in == nil {
			continue
		}
		for t := range scope {
			if !contains(in.Types, t) {
				return x.report, errors.New("baseline scope unavailable")
			}
		}
	}
	x.report.PolicyID = c.policy.ID
	x.report.PolicyDigest = c.digest
	x.report.Profile = c.policy.Profile
	x.report.DisabledRules = append([]string{}, c.policy.DisabledRules...)
	x.report.ProfileDigest = c.profileDigest
	x.report.Scope = keys(scope)
	x.report.ObjectKey = request.ObjectKey
	x.report.ObjectFilters = cloneObjectFilters(c.policy.ObjectFilters)
	x.report.Complete = true
	x.report.Correct = true
	x.report.Evidence = []Evidence{}
	for _, e := range c.policy.Evidence {
		e.Attributes = maps.Clone(e.Attributes)
		x.report.Evidence = append(x.report.Evidence, e)
	}
	for a, b := range c.replacements {
		x.report.Replacements[a] = b
	}
	for i, in := range x.inputs {
		if in == nil {
			continue
		}
		copy := *in
		copy.Documents = nil
		copy.Types = append([]string{}, in.Types...)
		x.report.Inputs[x.roles[i]] = copy
		x.report.Excluded[x.roles[i]] = []string{}
		for _, t := range in.Types {
			if !scope[t] {
				x.report.Excluded[x.roles[i]] = append(x.report.Excluded[x.roles[i]], t)
			}
		}
		for _, doc := range in.Documents {
			if scope[doc.Type] {
				x.report.ObjectCoverage[x.roles[i]+"_in_scope"]++
			}
		}
	}
	p := c.policy
	for _, cat := range categories(&p) {
		for i := 0; i < cat.Len(); i++ {
			x.report.RuleUsage[ruleOf(cat.Index(i)).ID] = 0
		}
	}
	x.inventory = matchInputs(x.inputs, c.policy, scope, request.Mode, x.diagnose, func(id string) { x.report.RuleUsage[id]++ })
	ops := map[string]Operation{}
	for _, op := range request.Operations {
		k := physical(op.Type, op.Key)
		if !scope[op.Type] || op.Key == "" || ops[k].Action != "" || !contains([]string{"create", "update", "delete"}, op.Action) {
			x.report.Complete = false
			x.report.Correct = false
			return x.report, errors.New("invalid deployment operation")
		}
		ops[k] = op
	}
	x.operations = ops
	x.report.Operations = append([]Operation{}, request.Operations...)
	sort.Slice(x.report.Operations, func(i, j int) bool { return stable(x.report.Operations[i]) < stable(x.report.Operations[j]) })
	x.report.ObjectCoverage["selected_operations"] = 0
	for _, op := range ops {
		if (request.ObjectKey == "" || op.Key == request.ObjectKey) && objectFilterReason(c.policy.ObjectFilters, op.Type, op.Key) == "" {
			x.report.ObjectCoverage["selected_operations"]++
		}
	}
	if request.ObjectKey != "" && request.Mode == "deployment" && x.report.ObjectCoverage["selected_operations"] == 0 && objectFilterReason(c.policy.ObjectFilters, request.Scope[0], request.ObjectKey) == "" {
		x.report.Complete = false
		x.report.Correct = false
		return x.report, errors.New("selected object requires a deployment operation")
	}
	requestBytes, _ := json.Marshal(struct {
		Mode, Policy string
		Inputs       map[string]Input
		Scope        []string
		Operations   []Operation
		ObjectKey    string `json:",omitempty"`
	}{request.Mode, c.digest, x.report.Inputs, x.report.Scope, x.report.Operations, request.ObjectKey})
	x.report.RequestDigest = hash(requestBytes)
	seenOps := map[string]bool{}
	foundObject := false
	filterKeys := map[string]bool{}
	for _, g := range x.inventory.groups {
		filterKeys[physical(g.typ, g.key)] = true
		if request.ObjectKey != "" && g.key != request.ObjectKey {
			for i, n := range g.nodes {
				if n != nil {
					x.report.ObjectCoverage[x.roles[i]+"_outside_object_filter"]++
				}
			}
			continue
		}
		if reason := objectFilterReason(c.policy.ObjectFilters, g.typ, g.key); reason != "" {
			x.excludeObject(g, reason)
			continue
		}
		foundObject = true
		x.current = g
		op, selected := ops[physical(g.typ, g.key)]
		if request.Mode == "deployment" {
			if !selected {
				for i, n := range g.nodes {
					if n != nil {
						x.report.ObjectCoverage[x.roles[i]+"_outside_operations"]++
					}
				}
				if g.nodes[1] != nil {
					x.record(frame{}, "unverified", "draft object outside operation scope", Rule{}, "")
				}
				continue
			}
			seenOps[physical(g.typ, g.key)] = true
		}
		x.report.ObjectCoverage["compared_groups"]++
		locations := map[string]Location{}
		for i, o := range g.nodes {
			if o != nil {
				locations[x.roles[i]] = Location{ID: o.doc.ID, Artifact: o.doc.Path, Present: true}
			}
		}
		x.report.Matches = append(x.report.Matches, Match{Type: g.typ, Key: g.key, RuleID: g.rule, Locations: locations})
		variants := map[string]bool{}
		for _, node := range g.nodes {
			if node != nil {
				variants[node.doc.Variant] = true
			}
		}
		if len(variants) > 1 {
			x.record(frame{}, "conflict", "document variant mismatch", Rule{}, "")
			continue
		}
		if request.Mode == "deployment" && op.Action == "delete" {
			if g.nodes[1] != nil {
				x.record(frame{}, "violation", "deleted object present in draft", Rule{}, "")
			} else if g.nodes[2] == nil {
				x.record(frame{}, "unverified", "delete baseline unavailable", Rule{}, "")
			} else {
				x.record(frame{}, "verified", "explicit delete operation", Rule{}, "request")
			}
			continue
		}
		if request.Mode == "deployment" && op.Action == "create" && g.nodes[2] != nil {
			x.record(frame{}, "violation", "create target already exists", Rule{}, "")
		}
		if request.Mode == "deployment" && op.Action == "update" && x.inputs[2] != nil && g.nodes[2] == nil {
			x.record(frame{}, "violation", "update target absent from baseline", Rule{}, "")
		}
		if g.nodes[0] == nil || g.nodes[1] == nil {
			if request.Mode == "drift" && g.nodes[0] == nil && g.nodes[1] == nil {
				continue
			}
			status := "different"
			if request.Mode == "deployment" {
				status = "violation"
			}
			x.record(frame{}, status, "object missing", Rule{}, "")
			continue
		}
		for _, sp := range []string{"content", "metadata"} {
			f := frame{space: sp}
			for i, o := range g.nodes {
				if o != nil {
					v := o.doc.Content
					if sp == "metadata" {
						v = o.doc.Metadata
					}
					f.nodes[i] = slot{value: v, present: true}
				}
			}
			x.walk(f)
		}
	}
	if request.ObjectKey != "" && !foundObject {
		code := "selected object unavailable"
		if objectFilterReason(c.policy.ObjectFilters, request.Scope[0], request.ObjectKey) != "" {
			code = "selected object excluded by policy"
		}
		x.diagnose(Diagnostic{Code: code, Type: request.Scope[0], Key: request.ObjectKey})
	}
	x.checkObjectFilters(scope, filterKeys)
	for k, op := range ops {
		if request.ObjectKey != "" && op.Key != request.ObjectKey {
			continue
		}
		if objectFilterReason(c.policy.ObjectFilters, op.Type, op.Key) != "" {
			continue
		}
		if !seenOps[k] {
			x.current = &group{typ: op.Type, key: op.Key}
			x.record(frame{}, "unverified", "operation object unavailable", Rule{}, "")
		}
	}
	unverifiedGroups := map[string]bool{}
	for _, d := range x.report.Decisions {
		if d.Status == "unverified" || d.Status == "conflict" {
			unverifiedGroups[physical(d.Type, d.Key)] = true
		}
	}
	x.report.ObjectCoverage["unverified_groups"] = len(unverifiedGroups)
	x.redact()
	for _, cat := range categories(&p) {
		for i := 0; i < cat.Len(); i++ {
			r := ruleOf(cat.Index(i))
			if !active(r, request.Mode) {
				continue
			}
			if x.report.RuleUsage[r.ID] == 0 {
				x.report.Diagnostics = append(x.report.Diagnostics, Diagnostic{Code: "unused rule", RuleID: r.ID, Required: r.Required})
				if r.Required {
					x.report.Complete = false
				}
			}
		}
	}
	if !x.report.Complete {
		x.report.Correct = false
	}
	sort.Slice(x.report.Decisions, func(i, j int) bool { return stable(x.report.Decisions[i]) < stable(x.report.Decisions[j]) })
	sort.Slice(x.report.Diagnostics, func(i, j int) bool { return stable(x.report.Diagnostics[i]) < stable(x.report.Diagnostics[j]) })
	sort.Slice(x.report.Matches, func(i, j int) bool { return stable(x.report.Matches[i]) < stable(x.report.Matches[j]) })
	// Reports may be edited by consumers; detach evidence from the compiled plan.
	b, _ := json.Marshal(x.report)
	var result Report
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&result); err != nil {
		x.report.Complete = false
		x.report.Correct = false
		return x.report, errors.New("cannot encode report")
	}
	return result, nil
}
func (x *execution) slotValue(n slot) *State {
	if n.unavailable && !x.request.ExcludeValues {
		return &State{State: "unavailable"}
	}
	return x.reportValue(n.value, n.present)
}
func (x *execution) reportValue(value any, present bool) *State {
	if x.request.ExcludeValues {
		return nil
	}
	if !present {
		return &State{State: "absent"}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return &State{State: "unavailable"}
	}
	return &State{State: "present", Value: data}
}
func stable(v any) string { b, _ := json.Marshal(v); return string(b) }
func (x *execution) diagnose(d Diagnostic) {
	for i, r := range []string{"left", "right"} {
		if d.Side == r {
			d.Side = x.roles[i]
		}
	}
	x.report.Diagnostics = append(x.report.Diagnostics, d)
	x.report.Complete = false
}
func (x *execution) record(f frame, status, code string, r Rule, authority string) {
	d := Decision{Type: x.current.typ, Key: x.current.key, Status: status, Code: code, Space: f.space, RuleID: r.ID, Replaces: r.Replaces, Authority: authority, Evidence: r.Evidence, ItemKeys: append([]string{}, f.itemKeys...), Locations: map[string]Location{}}
	d.Normalization = x.normalization
	d.Item = f.itemScope
	d.Items = f.itemScopes
	d.Extensions = f.extensions
	if code == "null and absent properties equivalent" {
		d.Normalization = "null_equals_absent"
	}
	d.Derivation = x.derivation
	d.Expected = x.expected
	d.ExpectedRole = x.expectedRole
	for i, o := range x.current.nodes {
		if o != nil {
			n := f.nodes[i]
			present := n.present
			if f.space == "" {
				present = true
			}
			d.Locations[x.roles[i]] = Location{ID: o.doc.ID, Artifact: o.doc.Path, Pointer: n.pointer, Present: present, ValueKind: valueKind(n.value, present), Value: x.slotValue(n), Context: x.locationContext(o.doc, x.current.key, f.space, n.pointer)}
			if i < 2 && len(x.orderContext[i]) > 0 {
				loc := d.Locations[x.roles[i]]
				loc.Context = append(loc.Context, ItemContext{Label: "Priority order", Pointer: n.pointer, Fields: x.orderContext[i]})
				d.Locations[x.roles[i]] = loc
			}
			if f.space == "" {
				loc := d.Locations[x.roles[i]]
				loc.ValueKind = "document"
				loc.Value = x.reportValue(o.doc.Content, true)
				d.Locations[x.roles[i]] = loc
			}
		}
	}
	x.report.Counts[status]++
	if r.ID != "" {
		x.report.RuleUsage[r.ID]++
	}
	if status == "unverified" || status == "conflict" {
		x.report.Complete = false
	}
	if status == "different" || status == "violation" {
		x.report.Correct = false
	}
	// Retain all policy decisions, even when literal equal leaves are omitted.
	if status != "equal" || r.ID != "" || x.request.IncludeMatching {
		x.report.Decisions = append(x.report.Decisions, d)
	}
}
func (x *execution) selected(s Selector, f frame) bool {
	return x.variantSelected(s) && selects(s, x.current.typ, x.current.key, f.space, f.path)
}
func (x *execution) variantSelected(s Selector) bool {
	if s.Variant == "" {
		return true
	}
	for _, node := range x.current.nodes {
		if node != nil && node.doc.Variant != s.Variant {
			return false
		}
	}
	return true
}
func (x *execution) walk(f frame) {
	for _, r := range x.c.policy.Ignores {
		if active(r.Rule, x.request.Mode) && x.selected(r.Selector, f) {
			x.record(f, "excluded", "explicit normalization", r.Rule, "")
			return
		}
	}
	for _, r := range x.c.policy.Expectations {
		if active(r.Rule, x.request.Mode) && x.selected(r.Selector, f) {
			if r.SourceEnvironment != "" && r.SourceEnvironment != x.inputs[0].Environment || r.SourceTenant != "" && r.SourceTenant != x.inputs[0].Tenant || r.TargetTenant != "" && r.TargetTenant != x.inputs[1].Tenant {
				x.record(f, "unverified", "expectation input context mismatch", r.Rule, r.Authority)
				return
			}
			if r.SourceEquals != nil {
				wantPresent := r.SourceEquals.State == "present"
				want, _ := JSON(r.SourceEquals.Value)
				if !x.statesEqual(f, f.nodes[0], slot{value: want, present: wantPresent}) {
					continue
				}
			}
			x.expect(f, r)
			return
		}
	}
	if x.request.Mode == "drift" {
		for _, r := range x.c.policy.References {
			if active(r.Rule, "drift") && x.selected(r.Selector, f) {
				x.reference(f, r)
				return
			}
		}
		for _, r := range x.c.policy.Equivalences {
			if !active(r.Rule, "drift") || !x.selected(r.Selector, f) || !equivalenceItemsSelected(r, f.items) {
				continue
			}
			nf := f
			nf.itemScope = r.Item
			nf.itemScopes = r.Items
			if r.Operation == "relative_order" {
				x.relativeOrder(nf, r)
				return
			}
			l, rr := x.inputs[0], x.inputs[1]
			if (r.LeftTenant != "" && r.LeftTenant != l.Tenant) || (r.RightTenant != "" && r.RightTenant != rr.Tenant) {
				x.record(nf, "unverified", "equivalence tenant context mismatch", r.Rule, "")
				return
			}
			if l.Environment == r.LeftEnv && rr.Environment == r.RightEnv {
				for _, pair := range r.StatePairs {
					if matchesEquivalenceState(f.nodes[0], pair.Left) && matchesEquivalenceState(f.nodes[1], pair.Right) {
						x.record(nf, "equivalent", "approved value relationship", r.Rule, "")
						return
					}
				}
			}
			if !f.nodes[0].present || !f.nodes[1].present {
				continue
			}
			a, b := f.nodes[0].value, f.nodes[1].value
			matched := false
			if l.Environment == r.LeftEnv && rr.Environment == r.RightEnv {
				for _, p := range r.Pairs {
					matched = matched || (equal(a, p.Left) && equal(b, p.Right))
				}
			}
			for _, g := range r.Groups {
				av, ao := g[l.Environment]
				bv, bo := g[rr.Environment]
				matched = matched || (ao && bo && equal(a, av) && equal(b, bv))
			}
			if matched {
				x.record(nf, "equivalent", "approved value relationship", r.Rule, "")
				return
			}
		}
	}
	// Expand explicit literal descendants even when both containers are absent.
	children := x.literalChildren(f)
	hasMap := false
	hasArray := false
	badType := false
	presentKind := ""
	for i, n := range f.nodes {
		if i == 2 && x.request.Mode == "drift" {
			continue
		}
		if !n.present {
			continue
		}
		kind := "scalar"
		switch n.value.(type) {
		case map[string]any:
			kind = "map"
			if i < 2 {
				hasMap = true
			}
		case []any:
			kind = "array"
			if i < 2 {
				hasArray = true
			}
		}
		if i < 2 {
			if presentKind != "" && presentKind != kind {
				badType = true
			}
			presentKind = kind
		}
	}
	if badType {
		x.leaf(f, "type mismatch")
		return
	}
	// Baseline-only containers still expand preservation/assertion wildcards.
	if !f.nodes[0].present && !f.nodes[1].present && f.nodes[2].present {
		switch f.nodes[2].value.(type) {
		case map[string]any:
			hasMap = true
		case []any:
			hasArray = true
		}
	}
	if (hasMap || hasArray) && f.nodes[0].present != f.nodes[1].present {
		status := "different"
		if x.request.Mode == "deployment" {
			status = "unverified"
		}
		x.record(f, status, "container presence differs", Rule{}, "")
	}
	if hasArray {
		for _, r := range x.c.policy.Collections {
			if active(r.Rule, x.request.Mode) && x.selected(r.Selector, f) {
				x.arrays(f, r)
				return
			}
		}
		x.arrays(f, Collection{Mode: "ordered"})
		return
	}
	if hasMap || len(children) > 0 {
		if !hasMap && (f.nodes[0].present || f.nodes[1].present) {
			x.leaf(f, "non-container with descendant assertion")
		}
		for _, n := range f.nodes {
			if m, ok := n.value.(map[string]any); ok {
				for k := range m {
					children[k] = true
				}
			}
		}
		if len(children) == 0 {
			x.leaf(f, "")
			return
		}
		for _, k := range keys(children) {
			nf := f
			nf.path = f.path + "/" + esc(k)
			nf.property = true
			for i, n := range f.nodes {
				v, ok := n.value.(map[string]any)
				value, present := v[k]
				nf.nodes[i] = slot{value: value, present: n.present && ok && present, pointer: n.pointer + "/" + esc(k), unavailable: n.unavailable || n.present && !ok}
			}
			x.walk(nf)
		}
		return
	}
	x.leaf(f, "")
}
func (x *execution) literalChildren(f frame) map[string]bool {
	out := map[string]bool{}
	add := func(s Selector, r Rule) {
		if !active(r, x.request.Mode) || !x.variantSelected(s) || s.Type != x.current.typ || space(s.Space) != f.space || (s.Key != "" && s.Key != x.current.key) {
			return
		}
		a, _ := pointer(s.Path)
		b, _ := pointer(f.path)
		if len(a) <= len(b) {
			return
		}
		for i := range b {
			if a[i] != "*" && a[i] != b[i] {
				return
			}
		}
		if a[len(b)] != "*" {
			out[a[len(b)]] = true
		}
	}
	for _, r := range x.c.policy.Expectations {
		add(r.Selector, r.Rule)
	}
	for _, r := range x.c.policy.Equivalences {
		if len(r.StatePairs) > 0 && x.request.Mode == "drift" {
			add(r.Selector, r.Rule)
		}
	}
	for _, r := range x.c.policy.References {
		if x.request.Mode == "drift" {
			add(r.Selector, r.Rule)
		}
	}
	return out
}

func matchesEquivalenceState(node slot, state State) bool {
	if node.unavailable {
		return false
	}
	if state.State == "absent" {
		return !node.present
	}
	value, _ := JSON(state.Value) // States are validated during compilation.
	return node.present && equal(node.value, value)
}
func (x *execution) leaf(f frame, code string) {
	a, b := f.nodes[0], f.nodes[1]
	if x.request.Mode == "deployment" {
		if a.present || b.present {
			x.record(f, "unverified", "no authoritative expectation", Rule{}, "")
		}
		return
	}
	if !a.present && !b.present {
		return
	}
	if code == "" {
		switch {
		case !a.present:
			code = "missing " + x.roles[0]
		case !b.present:
			code = "missing " + x.roles[1]
		case reflect.TypeOf(a.value) != reflect.TypeOf(b.value):
			code = "type mismatch"
		case !equal(a.value, b.value):
			code = "value mismatch"
		}
	}
	if x.statesEqual(f, a, b) {
		if a.present != b.present || !equal(a.value, b.value) {
			x.record(f, "equivalent", "null and absent properties equivalent", Rule{}, "")
		} else {
			x.record(f, "equal", code, Rule{}, "")
		}
	} else {
		x.record(f, "different", code, Rule{}, "")
	}
}
func (x *execution) expect(f frame, r Expectation) {
	defer func() { x.derivation = nil; x.expected = nil; x.expectedRole = ""; x.normalization = "" }()
	if r.Authority == "" || r.Evidence == nil || r.Evidence.Kind == "" {
		x.record(f, "unverified", "expectation authority/evidence unavailable", r.Rule, r.Authority)
		return
	}
	if x.request.Mode == "drift" && r.Operation != "literal" && r.Operation != "absent" {
		x.record(f, "unverified", "derived expectation requires deployment mode", r.Rule, r.Authority)
		return
	}
	sides := []int{1}
	if x.request.Mode == "drift" {
		sides = []int{0, 1}
	}
	for _, side := range sides {
		x.expected = nil
		x.normalization = ""
		x.expectedRole = x.roles[side]
		x.derivation = nil
		var want any
		present := true
		available := true
		switch r.Operation {
		case "literal":
			s := r.Expected
			if len(r.Values) > 0 {
				v, ok := r.Values[x.inputs[side].Environment]
				available = ok
				s = &v
			}
			if available {
				present = s.State == "present"
				if present {
					want, _ = JSON(s.Value)
				}
			}
		case "absent":
			present = false
		case "copy_source", "preserve_target", "reference_target":
			fromSide := 0
			if r.Operation == "preserve_target" {
				fromSide = 2
			}
			n := f.nodes[fromSide]
			if n.unavailable {
				available = false
			}
			if x.current.nodes[fromSide] == nil {
				available = false
			} else if r.From != nil {
				root := x.current.nodes[fromSide].doc.Content
				if f.space == "metadata" {
					root = x.current.nodes[fromSide].doc.Metadata
				}
				n.value, n.present = get(root, *r.From)
				n.pointer = *r.From
				available = true
			}
			if origin := x.current.nodes[fromSide]; origin != nil {
				x.derivation = &Derivation{Role: x.roles[fromSide], Type: origin.doc.Type, Location: Location{ID: origin.doc.ID, Artifact: origin.doc.Path, Pointer: n.pointer, Present: n.present, ValueKind: valueKind(n.value, n.present), Value: x.slotValue(n), Context: x.locationContext(origin.doc, x.current.key, f.space, n.pointer)}}
			}
			want, present = n.value, n.present
			if r.Operation == "reference_target" && available {
				sourceID, ok := want.(string)
				g := x.inventory.physical[0][physical(r.TargetType, sourceID)]
				if g != nil && x.operations[physical(g.typ, g.key)].Action == "delete" {
					x.record(f, "violation", "reference target scheduled for deletion", r.Rule, r.Authority)
					continue
				}
				if !present || !ok || g == nil || g.nodes[2] == nil {
					available = false
				} else {
					want = g.nodes[2].doc.ID
					present = true
					o := g.nodes[2]
					x.derivation = &Derivation{Role: "baseline", Type: o.doc.Type, Location: Location{ID: o.doc.ID, Artifact: o.doc.Path, Present: true, ValueKind: "object-identity", Value: x.reportValue(o.doc.ID, true)}}
				}
			}
		}
		if !available {
			x.record(f, "unverified", "expectation derivation unavailable", r.Rule, r.Authority)
			continue
		}
		x.expected = x.reportValue(want, present)
		actual := f.nodes[side]
		if x.statesEqual(f, slot{value: want, present: present}, actual) {
			if present != actual.present || present && !equal(want, actual.value) {
				x.normalization = "null_equals_absent"
			}
			x.record(f, "verified", "expectation satisfied: "+x.roles[side], r.Rule, r.Authority)
		} else {
			x.record(f, "violation", "expectation failed: "+x.roles[side], r.Rule, r.Authority)
		}
	}
}
func (x *execution) reference(f frame, r Reference) {
	a, b := f.nodes[0], f.nodes[1]
	if x.nullProperties(f) && !a.unavailable && !b.unavailable && (!a.present || a.value == nil) && (!b.present || b.value == nil) {
		if a.present != b.present {
			x.record(f, "equivalent", "null and absent properties equivalent", r.Rule, "")
		} else {
			x.record(f, "equal", "optional reference absent", r.Rule, "")
		}
		return
	}
	as, ao := a.value.(string)
	bs, bo := b.value.(string)
	if !a.present || !b.present || !ao || !bo {
		x.record(f, "unverified", "reference unavailable", r.Rule, "")
		return
	}
	ga := x.inventory.physical[0][physical(r.TargetType, as)]
	gb := x.inventory.physical[1][physical(r.TargetType, bs)]
	if ga != nil {
		if ga.nodes[1] == nil {
			x.record(f, "unverified", "reference correspondence unavailable", r.Rule, "")
		} else if ga == gb {
			x.record(f, "equivalent", "reference correspondence", r.Rule, "")
		} else {
			x.record(f, "different", "wrong reference target", r.Rule, "")
		}
		return
	}
	if gb != nil {
		x.record(f, "unverified", "source reference unavailable", r.Rule, "")
		return
	}
	if target, ok := r.External[as]; ok {
		if target == bs {
			x.record(f, "equivalent", "external reference correspondence", r.Rule, "")
		} else {
			x.record(f, "different", "wrong external reference target", r.Rule, "")
		}
		return
	}
	x.record(f, "unverified", "reference unavailable", r.Rule, "")
}
func (x *execution) arrays(f frame, r Collection) {
	f, r = x.extendCollection(f, r)
	a := [3][]any{}
	for i, n := range f.nodes {
		if !n.present {
			continue
		}
		v, ok := n.value.([]any)
		if !ok {
			if i == 2 {
				f.nodes[i].unavailable = true
				continue
			}
			x.record(f, "conflict", "collection type mismatch", r.Rule, "")
			return
		}
		a[i] = v
	}
	if r.PriorityOrder != nil && x.positionalPriorityCollection(a, r) {
		r.Mode = "ordered"
	}
	if r.Mode == "ordered" && r.ID != "" {
		x.record(f, "equal", "ordered array alignment", r.Rule, "")
	}
	if r.Mode == "multiset" {
		if x.request.Mode == "deployment" {
			x.record(f, "unverified", "multiset requires an authoritative whole-value expectation", r.Rule, "")
			return
		}
		counts := map[string]int{}
		strictCounts := map[string]int{}
		for _, v := range a[0] {
			counts[valueKey(x.comparisonValue(f.space, v))]++
			strictCounts[valueKey(v)]++
		}
		for _, v := range a[1] {
			counts[valueKey(x.comparisonValue(f.space, v))]--
			strictCounts[valueKey(v)]--
		}
		same := f.nodes[0].present == f.nodes[1].present
		for _, n := range counts {
			same = same && n == 0
		}
		if same {
			for _, n := range strictCounts {
				if n != 0 {
					x.normalization = "null_equals_absent"
					break
				}
			}
			x.record(f, "equivalent", "multiset order normalization", r.Rule, "")
			x.normalization = ""
		} else {
			x.record(f, "different", "multiset members or multiplicity", r.Rule, "")
		}
		return
	}
	if r.Mode == "keyed" {
		idx := [3]map[string]int{{}, {}, {}}
		all := map[string]bool{}
		identities := map[string]map[string]CollectionItemKey{}
		inconsistentIdentities := map[string]bool{}
		mapped := false
		for side, arr := range a {
			for i, v := range arr {
				k, used, components, err := collectionIdentity(v, r, x.inputs[side].Environment)
				if err != nil {
					x.record(f, "conflict", err.Error(), r.Rule, "")
					return
				}
				mapped = mapped || used
				if _, ok := idx[side][k]; ok {
					x.record(f, "conflict", "duplicate collection key", r.Rule, "")
					return
				}
				idx[side][k] = i
				all[k] = true
				if previous, exists := identities[k]; exists && !sameItemIdentity(previous, components) {
					inconsistentIdentities[k] = true
				}
				identities[k] = components
			}
		}
		status := "equal"
		if r.PriorityOrder != nil {
			x.collectionPriority(f, r, a, idx)
		}
		if x.request.Mode == "deployment" {
			status = "excluded"
		}
		code := "keyed array ordering normalization"
		if mapped {
			code = "keyed array key mapping and ordering normalization"
		}
		x.record(f, status, code, r.Rule, "")
		for _, k := range keys(all) {
			nf := f
			nf.items = maps.Clone(f.items)
			if nf.items == nil {
				nf.items = map[string]map[string]CollectionItemKey{}
			}
			if !inconsistentIdentities[k] {
				nf.items[r.ID] = identities[k]
			} else {
				delete(nf.items, r.ID)
			}
			nf.path = f.path + "/*"
			nf.property = false
			nf.itemKeys = append(append([]string{}, f.itemKeys...), hash([]byte(k)))
			for side := range a {
				i, ok := idx[side][k]
				nf.nodes[side] = slot{present: ok, pointer: f.nodes[side].pointer, unavailable: f.nodes[side].unavailable}
				if ok {
					nf.nodes[side].value = a[side][i]
					nf.nodes[side].pointer += "/" + strconv.Itoa(i)
				}
			}
			x.walk(nf)
		}
		if len(all) == 0 {
			x.leaf(f, "")
		}
		return
	}
	indices := map[int]bool{}
	for _, arr := range a {
		for i := range arr {
			indices[i] = true
		}
	}
	for child := range x.literalChildren(f) {
		index, ok := arrayIndex(child)
		if !ok {
			x.record(f, "conflict", "array selector requires canonical nonnegative index", r.Rule, "")
			continue
		}
		indices[index] = true
	}
	if len(indices) == 0 {
		x.leaf(f, "")
		return
	}
	ordered := make([]int, 0, len(indices))
	for i := range indices {
		ordered = append(ordered, i)
	}
	sort.Ints(ordered)
	for _, i := range ordered {
		nf := f
		nf.path = f.path + "/" + strconv.Itoa(i)
		nf.property = false
		for side := range a {
			nf.nodes[side] = slot{pointer: f.nodes[side].pointer + "/" + strconv.Itoa(i), unavailable: f.nodes[side].unavailable}
			if i < len(a[side]) {
				nf.nodes[side].value = a[side][i]
				nf.nodes[side].present = true
			}
		}
		x.walk(nf)
	}
}
func valueKind(v any, present bool) string {
	if !present {
		return "absent"
	}
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	}
	return "unknown"
}
