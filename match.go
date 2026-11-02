package differ

import (
	"encoding/json"
	"errors"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"strings"
)

type object struct {
	snapshot.Object
	Key string
}
type pair struct {
	left, right *object
	Rule        string
}
type matcher struct {
	left, right    map[string]*object
	pairs          []pair
	correspondence map[string]string
}

func physical(t, id string) string { return t + "\x00" + id }
func tuple(a ...string) string     { b, _ := json.Marshal(a); return string(b) }

var named = strings.Fields("ACCESS_PROFILE CAMPAIGN_FILTER CONNECTOR_RULE FORM_DEFINITION GOVERNANCE_GROUP IDENTITY_OBJECT_CONFIG IDENTITY_PROFILE PASSWORD_POLICY PASSWORD_SYNC_GROUP PUBLIC_IDENTITIES_CONFIG ROLE RULE SEGMENT SERVICE_DESK_INTEGRATION SOD_POLICY SOURCE SPS_PARAMETER TAG TRANSFORM TRIGGER_SUBSCRIPTION WORKFLOW")

func namedType(t string) bool {
	for _, v := range named {
		if v == t {
			return true
		}
	}
	return false
}
func identity(o *object, p Policy, index map[string]*object, leftIndex map[string]*object, correspondence map[string]string, right bool) (string, error) {
	fields := []string{}
	strategy := ""
	for _, r := range p.Identities {
		if r.Type == o.Type {
			fields = r.Paths
			strategy = r.Strategy
			break
		}
	}
	if strategy == "singleton" {
		return "singleton", nil
	}
	if strategy == "" {
		switch o.Type {
		case "ACCESS_REQUEST_CONFIG", "AUTH_ORG":
			return "singleton", nil
		case "LIFECYCLE_STATE", "ATTR_SYNC_SOURCE_CONFIG":
			parentType, path, keyPath := "IDENTITY_PROFILE", "/object/identityProfileRef/id", "/object/technicalName"
			if o.Type == "ATTR_SYNC_SOURCE_CONFIG" {
				parentType, path, keyPath = "SOURCE", "/object/source/id", ""
			}
			v, ok := lookup(o.Envelope, path)
			id, valid := v.(string)
			if !ok || !valid {
				return "", errors.New("missing identity dependency")
			}
			parent := index[physical(parentType, id)]
			if parent == nil || parent.Key == "" {
				return "", errors.New("unresolved identity dependency")
			}
			canonical := physical(parentType, id)
			if right {
				found := ""
				for l, r := range correspondence {
					if r == canonical {
						found = l
						break
					}
				}
				canonical = found
			} else if correspondence[canonical] == "" {
				canonical = ""
			}
			if canonical == "" {
				return "", errors.New("identity dependency has no correspondence")
			}
			if keyPath == "" {
				return tuple(leftIndex[canonical].Key), nil
			}
			v, ok = lookup(o.Envelope, keyPath)
			s, valid := v.(string)
			if !ok || !valid || s == "" {
				return "", errors.New("missing technical name")
			}
			return tuple(leftIndex[canonical].Key, s), nil
		case "NOTIFICATION_TEMPLATE":
			fields = []string{"/object/key", "/object/locale", "/object/medium"}
		default:
			if !namedType(o.Type) {
				return "", errors.New("unknown type requires identity policy")
			}
			fields = []string{"/object/name"}
		}
	}
	values := []string{}
	for _, path := range fields {
		v, ok := lookup(o.Envelope, path)
		s, valid := v.(string)
		if !ok || !valid || s == "" {
			return "", errors.New("missing logical identity")
		}
		values = append(values, s)
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return tuple(values...), nil
}
func match(l, r snapshot.Input, p Policy, scope map[string]bool, diagnose func(Diagnostic)) *matcher {
	m := &matcher{left: map[string]*object{}, right: map[string]*object{}, correspondence: map[string]string{}}
	for _, v := range l.Objects {
		if scope[v.Type] {
			o := &object{Object: v}
			m.left[physical(v.Type, v.ID)] = o
		}
	}
	for _, v := range r.Objects {
		if scope[v.Type] {
			o := &object{Object: v}
			m.right[physical(v.Type, v.ID)] = o
		}
	}
	usedL, usedR := map[string]bool{}, map[string]bool{}
	for stage := 0; stage < 2; stage++ {
		indices := []map[string]*object{m.left, m.right}
		keys := []map[string]*object{{}, {}}
		for side, index := range indices {
			for _, pid := range sortedKeys(index) {
				o := index[pid]
				dependent := o.Type == "LIFECYCLE_STATE" || o.Type == "ATTR_SYNC_SOURCE_CONFIG"
				if dependent != (stage == 1) {
					continue
				}
				k, e := identity(o, p, index, m.left, m.correspondence, side == 1)
				if e != nil {
					diagnose(Diagnostic{Code: e.Error(), Side: []string{"left", "right"}[side], Type: o.Type, Path: o.Path})
					continue
				}
				o.Key = k
				lk := physical(o.Type, k)
				if keys[side][lk] != nil {
					diagnose(Diagnostic{Code: "duplicate logical identity", Side: []string{"left", "right"}[side], Type: o.Type})
					continue
				}
				keys[side][lk] = o
			}
		}
		for _, rule := range p.Pairings {
			a, b := m.left[physical(rule.Type, rule.LeftID)], m.right[physical(rule.Type, rule.RightID)]
			dep := rule.Type == "LIFECYCLE_STATE" || rule.Type == "ATTR_SYNC_SOURCE_CONFIG"
			if dep != (stage == 1) {
				continue
			}
			if a == nil || b == nil || a.Key == "" || b.Key == "" {
				diagnose(Diagnostic{Code: "pairing endpoint unavailable", RuleID: rule.ID, Type: rule.Type})
				continue
			}
			lp, rp := physical(a.Type, a.ID), physical(b.Type, b.ID)
			usedL[lp] = true
			usedR[rp] = true
			m.correspondence[lp] = rp
			m.pairs = append(m.pairs, pair{a, b, rule.ID})
		}
		for _, k := range sortedKeys(keys[0]) {
			a := keys[0][k]
			lp := physical(a.Type, a.ID)
			if usedL[lp] {
				continue
			}
			b := keys[1][k]
			if b != nil && !usedR[physical(b.Type, b.ID)] {
				rp := physical(b.Type, b.ID)
				usedL[lp] = true
				usedR[rp] = true
				m.correspondence[lp] = rp
				m.pairs = append(m.pairs, pair{a, b, ""})
			}
		}
	}
	for _, pid := range sortedKeys(m.left) {
		if !usedL[pid] {
			m.pairs = append(m.pairs, pair{left: m.left[pid]})
		}
	}
	for _, pid := range sortedKeys(m.right) {
		if !usedR[pid] {
			m.pairs = append(m.pairs, pair{right: m.right[pid]})
		}
	}
	return m
}
