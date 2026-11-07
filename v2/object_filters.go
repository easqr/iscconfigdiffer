package v2

import (
	"fmt"
	"sort"
	"strings"
)

func validateObjectFilters(filters map[string]ObjectFilter, identities map[string]Identity) error {
	if err := validateObjectFilterLists(filters); err != nil {
		return err
	}
	for _, typ := range keys(filters) {
		f := filters[typ]
		if identities[typ].ID == "" {
			return fmt.Errorf("object_filters requires an identity strategy for type %q", typ)
		}
		for _, list := range [][]string{f.Include, f.Exclude} {
			sort.Strings(list)
		}
	}
	return nil
}

func validateObjectFilterLists(filters map[string]ObjectFilter) error {
	for _, typ := range keys(filters) {
		f := filters[typ]
		if len(f.Include) == 0 && len(f.Exclude) == 0 {
			return fmt.Errorf("object_filters for %s requires a nonempty include or exclude list", typ)
		}
		for _, list := range [][]string{f.Include, f.Exclude} {
			if list != nil && len(list) == 0 {
				return fmt.Errorf("object_filters for %s: omit unused lists instead of supplying empty arrays", typ)
			}
			seen := map[string]bool{}
			for _, key := range list {
				if strings.TrimSpace(key) == "" || seen[key] {
					return fmt.Errorf("object_filters for %s requires nonblank, unique exact names in each list", typ)
				}
				seen[key] = true
			}
		}
	}
	return nil
}

func cloneObjectFilters(filters map[string]ObjectFilter) map[string]ObjectFilter {
	if filters == nil {
		return nil
	}
	out := map[string]ObjectFilter{}
	for typ, f := range filters {
		out[typ] = ObjectFilter{Include: append([]string{}, f.Include...), Exclude: append([]string{}, f.Exclude...)}
	}
	return out
}

func objectFilterReason(filters map[string]ObjectFilter, typ, key string) string {
	f, ok := filters[typ]
	if !ok {
		return ""
	}
	if contains(f.Exclude, key) {
		return "explicitly excluded by object filter"
	}
	if len(f.Include) > 0 && !contains(f.Include, key) {
		return "not in object filter include list"
	}
	return ""
}

func (x *execution) excludeObject(g *group, reason string) {
	out := ExcludedObject{Type: g.typ, Key: g.key, Reason: reason, Locations: map[string]Location{}}
	for side, n := range g.nodes {
		if n == nil {
			continue
		}
		x.report.ObjectCoverage[x.roles[side]+"_excluded_by_policy"]++
		out.Locations[x.roles[side]] = Location{ID: n.doc.ID, Artifact: n.doc.Path, Present: true, ValueKind: "document"}
	}
	x.report.ExcludedObjects = append(x.report.ExcludedObjects, out)
	x.report.ObjectCoverage["excluded_groups"]++
}

func (x *execution) checkObjectFilters(scope map[string]bool, seen map[string]bool) {
	active := false
	for _, typ := range keys(x.c.policy.ObjectFilters) {
		if !scope[typ] {
			continue
		}
		active = true
		f := x.c.policy.ObjectFilters[typ]
		for _, name := range f.Include {
			if contains(f.Exclude, name) || x.request.ObjectKey != "" && x.request.ObjectKey != name {
				continue
			}
			if !seen[physical(typ, name)] {
				x.diagnose(Diagnostic{Code: "included object unavailable", Type: typ, Key: name})
			}
		}
		for _, name := range f.Exclude {
			if x.request.ObjectKey != "" && x.request.ObjectKey != name {
				continue
			}
			if !seen[physical(typ, name)] {
				x.report.Diagnostics = append(x.report.Diagnostics, Diagnostic{Code: "unused object exclusion", Type: typ, Key: name})
			}
		}
	}
	if active && x.report.ObjectCoverage["compared_groups"] == 0 {
		x.diagnose(Diagnostic{Code: "object filters selected no configurations"})
	}
}
