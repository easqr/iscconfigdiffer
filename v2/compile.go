package v2

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
)

var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$`)

func Decode(b []byte) (Policy, error) {
	var p Policy
	e := strict(b, &p)
	if e != nil {
		return p, e
	}
	if p.Version != 2 {
		return p, errors.New("policy version 2 required")
	}
	return p, nil
}

func DecodeOperations(b []byte) ([]Operation, error) {
	var ops []Operation
	err := strict(b, &ops)
	return ops, err
}
func DecodeEvidence(b []byte) (Evidence, error) {
	var ev Evidence
	err := strict(b, &ev)
	if err == nil && ev.Kind == "" {
		err = errors.New("evidence kind required")
	}
	return ev, err
}
func ruleOf(v reflect.Value) Rule { return v.FieldByName("Rule").Interface().(Rule) }
func categories(p *Policy) []reflect.Value {
	v := reflect.ValueOf(p).Elem()
	a := []reflect.Value{}
	for _, n := range []string{"Identities", "Pairings", "Ignores", "Expectations", "Equivalences", "References", "Collections", "Sensitive"} {
		a = append(a, v.FieldByName(n))
	}
	return a
}
func Compile(p Policy, profiles ...Profile) (*Compiled, error) {
	b, e := json.Marshal(p)
	if e != nil {
		return nil, errors.New("policy contains non-JSON values")
	}
	p, e = Decode(b)
	if e != nil {
		return nil, e
	}
	if !validID.MatchString(p.ID) {
		return nil, errors.New("invalid policy ID")
	}
	seenModes := map[string]bool{}
	for _, mode := range p.Modes {
		if !contains([]string{"drift", "deployment"}, mode) || seenModes[mode] {
			return nil, errors.New("invalid policy modes")
		}
		seenModes[mode] = true
	}
	sort.Strings(p.Modes)
	for role, env := range p.Bindings {
		if env == "" || !contains([]string{"left", "right", "source", "actual", "baseline"}, role) {
			return nil, errors.New("invalid binding")
		}
	}
	base := Policy{}
	found := p.Profile == ""
	for _, profile := range profiles {
		if profile.ID == p.Profile {
			if found {
				return nil, errors.New("duplicate profile")
			}
			base = profile.Rules
			found = true
		}
	}
	if !found {
		return nil, errors.New("unknown profile")
	}
	// Profiles are caller-owned too; compiled plans never retain their maps/slices.
	profileBytes, err := json.Marshal(base)
	if err != nil {
		return nil, errors.New("invalid profile JSON")
	}
	var detachedProfile Policy
	if err = strict(profileBytes, &detachedProfile); err != nil {
		return nil, err
	}
	base = detachedProfile
	if base.Version != 0 || base.ID != "" || base.Profile != "" || len(base.Modes) > 0 || len(base.Bindings) > 0 {
		return nil, errors.New("profile rules may supply comparison defaults, rule arrays and evidence only")
	}
	for _, setting := range []string{p.NullHandling, base.NullHandling} {
		if !contains([]string{"", "strict", "null_equals_absent"}, setting) {
			return nil, errors.New("invalid null_handling setting")
		}
	}
	if p.NullHandling == "" {
		p.NullHandling = base.NullHandling
	}
	if p.NullHandling == "" {
		p.NullHandling = "strict"
	}
	p.Evidence = append(p.Evidence, base.Evidence...)
	// Empty selector space is the content alias, including replacement ownership.
	for _, policy := range []*Policy{&p, &base} {
		for _, cat := range categories(policy) {
			for i := 0; i < cat.Len(); i++ {
				s := cat.Index(i).FieldByName("Selector")
				if s.IsValid() {
					v := s.Interface().(Selector)
					v.Space = space(v.Space)
					s.Set(reflect.ValueOf(v))
				}
			}
		}
	}
	replaced := map[string]string{}
	profileIDs := map[string]int{}
	allIDs := map[string]bool{}
	bc, pc := categories(&base), categories(&p)
	for _, cat := range bc {
		for i := 0; i < cat.Len(); i++ {
			r := ruleOf(cat.Index(i))
			if r.Replaces != "" {
				return nil, errors.New("profile rule cannot declare an unresolved replacement")
			}
			sort.Strings(r.Modes)
		}
		sort.Slice(cat.Interface(), func(i, j int) bool { return ruleOf(cat.Index(i)).ID < ruleOf(cat.Index(j)).ID })
	}
	profileJSON, _ := json.Marshal(base)
	profileValue, _ := JSON(profileJSON)
	profileJSON, _ = json.Marshal(canonical(profileValue))
	profileDigest := ""
	if p.Profile != "" {
		profileDigest = hash(profileJSON)
	}
	for i, cat := range bc {
		for j := 0; j < cat.Len(); j++ {
			r := ruleOf(cat.Index(j))
			if allIDs[r.ID] {
				return nil, errors.New("duplicate profile rule")
			}
			allIDs[r.ID] = true
			profileIDs[r.ID] = i
		}
	}
	for i, cat := range pc {
		for j := 0; j < cat.Len(); j++ {
			r := ruleOf(cat.Index(j))
			if !validID.MatchString(r.ID) || allIDs[r.ID] {
				return nil, errors.New("invalid or duplicate rule ID")
			}
			allIDs[r.ID] = true
			if r.Replaces != "" {
				ci, ok := profileIDs[r.Replaces]
				if !ok || ci != i || replaced[r.Replaces] != "" {
					return nil, fmt.Errorf("invalid replacement for rule %s", r.ID)
				}
				replaced[r.Replaces] = r.ID
			}
		}
	}
	for i, cat := range bc {
		for j := 0; j < cat.Len(); j++ {
			r := ruleOf(cat.Index(j))
			if replaced[r.ID] == "" {
				pc[i].Set(reflect.Append(pc[i], cat.Index(j)))
			}
		}
	}
	for _, cat := range pc {
		sort.Slice(cat.Interface(), func(i, j int) bool { return ruleOf(cat.Index(i)).ID < ruleOf(cat.Index(j)).ID })
	}
	for i := range p.Equivalences {
		if len(p.Equivalences[i].Modes) == 0 {
			p.Equivalences[i].Modes = []string{"drift"}
		}
	}
	for i := range p.References {
		if len(p.References[i].Modes) == 0 {
			p.References[i].Modes = []string{"drift"}
		}
		if contains(p.References[i].Modes, "deployment") {
			return nil, errors.New("references are drift-only; use reference_target expectations for deployment")
		}
	}
	var claims []struct {
		s        Selector
		kind, id string
	}
	rulesByID := map[string]Rule{}
	coactive := func(a, b Rule) bool {
		for _, mode := range []string{"drift", "deployment"} {
			if (len(p.Modes) == 0 || contains(p.Modes, mode)) && active(a, mode) && active(b, mode) {
				return true
			}
		}
		return false
	}
	collectionsByID := map[string]Collection{}
	for _, r := range p.Collections {
		collectionsByID[r.ID] = r
	}
	for _, cat := range pc {
		for j := 0; j < cat.Len(); j++ {
			r := ruleOf(cat.Index(j))
			rulesByID[r.ID] = r
		}
	}
	claim := func(s Selector, kind, id string) error {
		if s.Type == "" || (space(s.Space) != "content" && space(s.Space) != "metadata") {
			return fmt.Errorf("invalid selector for rule %s", id)
		}
		if _, e := pointer(s.Path); e != nil {
			return fmt.Errorf("invalid selector for rule %s", id)
		}
		for _, old := range claims {
			if !overlap(old.s, s) {
				continue
			}
			if !coactive(rulesByID[id], rulesByID[old.id]) {
				continue
			}
			if kind == "collection" && old.kind == "collection" {
				if !collectionContains(old.s, s, collectionsByID[old.id].Mode) && !collectionContains(s, old.s, collectionsByID[id].Mode) {
					return fmt.Errorf("collection conflict: %s %s", old.id, id)
				}
				continue
			}
			if kind == "collection" || old.kind == "collection" {
				a, b := old.s, s
				parentID := old.id
				if old.kind != "collection" {
					a, b = s, old.s
					parentID = id
				}
				if !collectionContains(a, b, collectionsByID[parentID].Mode) {
					return fmt.Errorf("collection/value conflict: %s %s", old.id, id)
				}
				continue
			}
			return fmt.Errorf("rule conflict: %s %s", old.id, id)
		}
		claims = append(claims, struct {
			s        Selector
			kind, id string
		}{s, kind, id})
		return nil
	}
	for _, cat := range pc {
		for j := 0; j < cat.Len(); j++ {
			r := ruleOf(cat.Index(j))
			if !validID.MatchString(r.ID) {
				return nil, errors.New("invalid profile rule ID")
			}
			seen := map[string]bool{}
			for _, m := range r.Modes {
				if (m != "drift" && m != "deployment") || seen[m] {
					return nil, fmt.Errorf("invalid modes: %s", r.ID)
				}
				seen[m] = true
			}
			sort.Strings(r.Modes)
			if r.Evidence != nil && r.Evidence.Kind == "" {
				return nil, fmt.Errorf("invalid evidence: %s", r.ID)
			}
		}
	}
	idents := map[string]Identity{}
	for _, r := range p.Identities {
		if r.ComponentType != "" && r.ComponentType != "scalar" && r.ComponentType != "string" {
			return nil, errors.New("invalid identity component type")
		}
		if r.Type == "" || idents[r.Type].ID != "" || len(r.Modes) > 0 {
			return nil, errors.New("duplicate/conditional identity")
		}
		if r.Strategy != "fields" && r.Strategy != "singleton" && r.Strategy != "dependent" {
			return nil, errors.New("invalid identity strategy")
		}
		if r.Strategy == "fields" && len(r.Paths) == 0 {
			return nil, errors.New("identity paths required")
		}
		if r.Strategy == "singleton" && (len(r.Paths) > 0 || r.ParentType != "" || r.ParentPath != "" || r.ComponentType != "") {
			return nil, errors.New("invalid singleton")
		}
		if r.Strategy == "dependent" && (r.ParentType == "" || !concrete(r.ParentPath) || r.ParentPath == "") {
			return nil, errors.New("invalid identity dependency")
		}
		if r.Strategy != "dependent" && (r.ParentType != "" || r.ParentPath != "") {
			return nil, errors.New("unexpected identity dependency")
		}
		for _, path := range r.Paths {
			if !concrete(path) {
				return nil, errors.New("concrete identity path required")
			}
		}
		idents[r.Type] = r
	}
	vis := map[string]int{}
	var visit func(string) error
	visit = func(t string) error {
		if vis[t] == 1 {
			return errors.New("identity dependency cycle")
		}
		if vis[t] == 2 {
			return nil
		}
		r, ok := idents[t]
		if !ok {
			return errors.New("identity parent strategy unavailable")
		}
		vis[t] = 1
		if r.Strategy == "dependent" {
			if e := visit(r.ParentType); e != nil {
				return e
			}
		}
		vis[t] = 2
		return nil
	}
	for t := range idents {
		if e := visit(t); e != nil {
			return nil, e
		}
	}
	endpoints := map[string]bool{}
	for _, r := range p.Pairings {
		if r.Type == "" || r.LeftID == "" || r.RightID == "" || len(r.Modes) > 0 {
			return nil, errors.New("invalid pairing")
		}
		for i, id := range []string{r.LeftID, r.RightID, r.BaselineID} {
			if id == "" {
				continue
			}
			k := fmt.Sprint(i) + physical(r.Type, id)
			if endpoints[k] {
				return nil, errors.New("conflicting pairing endpoint")
			}
			endpoints[k] = true
		}
	}
	for _, r := range p.Ignores {
		if e := claim(r.Selector, "ignore", r.ID); e != nil {
			return nil, e
		}
		if identityOverlap(r.Selector, idents) {
			return nil, errors.New("ignore overlaps identity")
		}
	}
	for _, r := range p.Expectations {
		if r.SourceEquals != nil {
			if err := validateState(*r.SourceEquals); err != nil {
				return nil, err
			}
			if !reflect.DeepEqual(r.Modes, []string{"deployment"}) {
				return nil, errors.New("source guard must be deployment-only")
			}
		}
		if e := claim(r.Selector, "expectation", r.ID); e != nil {
			return nil, e
		}
		switch r.Operation {
		case "literal":
			if (r.Expected == nil) == (len(r.Values) == 0) {
				return nil, errors.New("literal requires exactly one expected/values source")
			}
		case "absent":
			if r.Expected != nil || len(r.Values) > 0 {
				return nil, errors.New("absent cannot carry values")
			}
		case "copy_source", "preserve_target", "reference_target":
			if r.Expected != nil || len(r.Values) > 0 || r.From != nil && !concrete(*r.From) {
				return nil, errors.New("invalid derived expectation")
			}
		default:
			return nil, errors.New("invalid expectation operation")
		}
		if r.Operation == "reference_target" && r.TargetType == "" {
			return nil, errors.New("target type required")
		}
		if (r.Operation == "literal" || r.Operation == "absent") && (r.From != nil || r.TargetType != "") {
			return nil, errors.New("unexpected derivation fields")
		}
		if r.Operation != "reference_target" && r.TargetType != "" {
			return nil, errors.New("unexpected target type")
		}
		if r.Expected != nil {
			if e := validateState(*r.Expected); e != nil {
				return nil, e
			}
		}
		for env, s := range r.Values {
			if env == "" {
				return nil, errors.New("empty expectation environment")
			}
			if e := validateState(s); e != nil {
				return nil, e
			}
		}
	}
	for i := range p.Equivalences {
		r := &p.Equivalences[i]
		if e := claim(r.Selector, "equivalence", r.ID); e != nil {
			return nil, e
		}
		if identityOverlap(r.Selector, idents) {
			return nil, errors.New("equivalence overlaps identity")
		}
		if contains(r.Modes, "deployment") {
			return nil, errors.New("equivalence is drift-only")
		}
		if r.LeftEnv == "" || r.RightEnv == "" || (len(r.Pairs) == 0) == (len(r.Groups) == 0) {
			return nil, errors.New("equivalence context/cases required")
		}
		seenL, seenR := map[string]bool{}, map[string]bool{}
		for _, pair := range r.Pairs {
			l, rr := valueKey(pair.Left), valueKey(pair.Right)
			if seenL[l] || seenR[rr] || seenR[l] || seenL[rr] {
				return nil, errors.New("ambiguous equivalence cases")
			}
			seenL[l] = true
			seenR[rr] = true
		}
		sort.Slice(r.Pairs, func(i, j int) bool { return valueKey(r.Pairs[i].Left) < valueKey(r.Pairs[j].Left) })
		seen := map[string]bool{}
		for _, g := range r.Groups {
			if len(g) < 2 {
				return nil, errors.New("equivalence group needs environments")
			}
			if _, ok := g[r.LeftEnv]; !ok {
				return nil, errors.New("group environment missing")
			}
			if _, ok := g[r.RightEnv]; !ok {
				return nil, errors.New("group environment missing")
			}
			for env, v := range g {
				k := env + "/" + valueKey(v)
				if env == "" || seen[k] {
					return nil, errors.New("ambiguous equivalence group")
				}
				seen[k] = true
			}
		}
		sort.Slice(r.Groups, func(i, j int) bool { return valueKey(r.Groups[i]) < valueKey(r.Groups[j]) })
	}
	for _, r := range p.References {
		if e := claim(r.Selector, "reference", r.ID); e != nil {
			return nil, e
		}
		if r.TargetType == "" {
			return nil, errors.New("reference target required")
		}
		seen := map[string]bool{}
		for l, rr := range r.External {
			if l == "" || rr == "" || seen[rr] {
				return nil, errors.New("external references must be one-to-one")
			}
			seen[rr] = true
		}
	}
	for _, r := range p.Collections {
		if e := claim(r.Selector, "collection", r.ID); e != nil {
			return nil, e
		}
		if r.Mode != "ordered" && r.Mode != "keyed" && r.Mode != "multiset" {
			return nil, errors.New("invalid collection mode")
		}
		if (r.Mode == "keyed") != (len(r.Keys) > 0) {
			return nil, errors.New("keyed collection requires keys only")
		}
		if err := validateCollectionKeys(r); err != nil {
			return nil, err
		}
		if err := validateCollectionDisplay(r); err != nil {
			return nil, err
		}
	}
	for _, r := range p.Sensitive {
		if r.Type == "" || (space(r.Space) != "content" && space(r.Space) != "metadata") {
			return nil, errors.New("invalid sensitive selector")
		}
		if _, e := pointer(r.Path); e != nil {
			return nil, e
		}
	}
	for _, r := range p.Collections {
		if r.Mode != "multiset" {
			continue
		}
		for _, other := range claims {
			if other.id != r.ID && overlap(r.Selector, other.s) && coactive(r.Rule, rulesByID[other.id]) {
				return nil, errors.New("multiset cannot contain field or collection rules")
			}
		}
	}
	// Replacements must retain ownership; changing a path requires a new policy/profile.
	for ci, cat := range bc {
		for j := 0; j < cat.Len(); j++ {
			old := cat.Index(j)
			id := ruleOf(old).ID
			next := replaced[id]
			if next == "" {
				continue
			}
			for k := 0; k < pc[ci].Len(); k++ {
				nv := pc[ci].Index(k)
				if ruleOf(nv).ID != next {
					continue
				}
				for _, name := range []string{"Type", "Selector"} {
					a, b := old.FieldByName(name), nv.FieldByName(name)
					if a.IsValid() && b.IsValid() && !reflect.DeepEqual(a.Interface(), b.Interface()) {
						return nil, errors.New("replacement changes ownership")
					}
				}
			}
		}
	}
	for _, e := range p.Evidence {
		if e.Kind == "" {
			return nil, errors.New("evidence kind required")
		}
	}
	sort.Slice(p.Evidence, func(i, j int) bool { return valueKey(p.Evidence[i]) < valueKey(p.Evidence[j]) })
	b, _ = json.Marshal(p)
	v, _ := JSON(b)
	b, _ = json.Marshal(canonical(v))
	return &Compiled{policy: p, digest: hash(b), replacements: replaced, profileDigest: profileDigest}, nil
}
func nested(parent, child Selector) bool {
	if !overlap(parent, child) {
		return false
	}
	a, _ := pointer(parent.Path)
	b, _ := pointer(child.Path)
	return len(b) > len(a) && b[len(a)] == "*"
}
func collectionContains(parent, child Selector, mode string) bool {
	if !overlap(parent, child) {
		return false
	}
	a, _ := pointer(parent.Path)
	b, _ := pointer(child.Path)
	if len(b) <= len(a) {
		return false
	}
	if b[len(a)] == "*" {
		return true
	}
	if mode == "ordered" {
		_, ok := arrayIndex(b[len(a)])
		return ok
	}
	return false
}
func identityOverlap(s Selector, ids map[string]Identity) bool {
	if space(s.Space) != "content" {
		return false
	}
	r := ids[s.Type]
	paths := append([]string{}, r.Paths...)
	if r.ParentPath != "" {
		paths = append(paths, r.ParentPath)
	}
	for _, p := range paths {
		if overlap(s, Selector{Type: s.Type, Path: p}) {
			return true
		}
	}
	return false
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func active(r Rule, mode string) bool { return len(r.Modes) == 0 || contains(r.Modes, mode) }
func validateState(s State) error {
	switch s.State {
	case "absent":
		if len(s.Value) > 0 {
			return errors.New("absent has no value")
		}
	case "present":
		if len(s.Value) == 0 {
			return errors.New("present needs value, including explicit null")
		}
		if _, e := JSON(s.Value); e != nil {
			return e
		}
	default:
		return errors.New("invalid state")
	}
	return nil
}

// literalSegments is used by adapters to reject unsupported wildcard members.
func LiteralPointer(p string) bool { return concrete(p) }

// RuleIDs returns a detached, sorted list for coverage tooling.
func (c *Compiled) RuleIDs() []string {
	p := c.policy
	out := []string{}
	for _, cat := range categories(&p) {
		for i := 0; i < cat.Len(); i++ {
			out = append(out, ruleOf(cat.Index(i)).ID)
		}
	}
	sort.Strings(out)
	return out
}
