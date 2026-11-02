package differ

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

type HubContext struct{ SourceTenant, TargetTenant, Provenance string }
type hubMapping struct {
	ID       string  `json:"objectMappingId"`
	Type     string  `json:"objectType"`
	Path     string  `json:"jsonPath"`
	Source   *string `json:"sourceValue"`
	Target   *string `json:"targetValue"`
	Enabled  *bool   `json:"enabled"`
	Created  string  `json:"created"`
	Modified string  `json:"modified"`
}

var member = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*`)
var index = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

// HubPointer supports $, .member, ['member'], ["member"], [index] and [*].
// Recursive descent, unions, slices, scripts and filters are rejected.
func HubPointer(path string) (string, error) {
	if !strings.HasPrefix(path, "$") {
		return "", errors.New("unsupported Hub JSONPath")
	}
	s := path[1:]
	out := "/object"
	for s != "" {
		switch s[0] {
		case '.':
			s = s[1:]
			part := member.FindString(s)
			if part == "" {
				return "", errors.New("unsupported Hub JSONPath")
			}
			out += "/" + escape(part)
			s = s[len(part):]
		case '[':
			end := -1
			quoted := byte(0)
			escaped := false
			for i := 1; i < len(s); i++ {
				ch := s[i]
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' && quoted != 0 {
					escaped = true
					continue
				}
				if quoted != 0 {
					if ch == quoted {
						quoted = 0
					}
					continue
				}
				if ch == '\'' || ch == '"' {
					quoted = ch
					continue
				}
				if ch == ']' {
					end = i
					break
				}
			}
			if end < 0 {
				return "", errors.New("unsupported Hub JSONPath")
			}
			part := s[1:end]
			s = s[end+1:]
			if part == "*" || index.MatchString(part) {
				out += "/" + part
				continue
			}
			if len(part) < 2 {
				return "", errors.New("unsupported Hub JSONPath")
			}
			if part[0] == '\'' && part[len(part)-1] == '\'' {
				part = part[1 : len(part)-1]
				if strings.ContainsAny(part, "\\'") {
					return "", errors.New("unsupported Hub quoted escape")
				}
			} else {
				var decoded string
				if e := json.Unmarshal([]byte(part), &decoded); e != nil {
					return "", errors.New("unsupported Hub JSONPath")
				}
				part = decoded
			}
			if part == "*" {
				return "", errors.New("literal wildcard member unsupported")
			}
			out += "/" + escape(part)
		default:
			return "", errors.New("unsupported Hub JSONPath")
		}
	}
	return out, nil
}
func ImportHub(b []byte, ctx HubContext) ([]HubRule, error) {
	if ctx.SourceTenant == "" || ctx.TargetTenant == "" || ctx.Provenance == "" {
		return nil, errors.New("Hub context and provenance required")
	}
	if v, e := snapshot.DecodeJSON(b); e != nil {
		return nil, e
	} else if e := exactShape(v, reflect.TypeFor[[]hubMapping]()); e != nil {
		return nil, e
	}
	var mappings []hubMapping
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&mappings); e != nil || mappings == nil {
		return nil, errors.New("Hub mappings must be a saved JSON array with known fields")
	}
	rules := []HubRule{}
	for _, m := range mappings {
		if m.ID == "" || m.Type == "" || m.Path == "" || m.Source == nil || m.Target == nil || m.Enabled == nil {
			return nil, errors.New("incomplete Hub mapping")
		}
		path, e := HubPointer(m.Path)
		if e != nil && *m.Enabled {
			return nil, e
		}
		rules = append(rules, HubRule{ID: m.ID, Selector: Selector{Type: m.Type, Path: path}, JSONPath: m.Path, Created: m.Created, Modified: m.Modified, Source: *m.Source, Target: *m.Target, Enabled: *m.Enabled, SourceTenant: ctx.SourceTenant, TargetTenant: ctx.TargetTenant, Provenance: ctx.Provenance})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	p := DefaultPolicy()
	p.Hub = rules
	if e := p.Validate(); e != nil {
		return nil, e
	}
	return rules, nil
}
