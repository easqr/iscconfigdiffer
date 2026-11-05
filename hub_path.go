package differ

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
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
	out := ""
	for s != "" {
		switch s[0] {
		case '.':
			s = s[1:]
			part := member.FindString(s)
			if part == "" {
				return "", errors.New("unsupported Hub JSONPath")
			}
			out += "/" + escapePointer(part)
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
			out += "/" + escapePointer(part)
		default:
			return "", errors.New("unsupported Hub JSONPath")
		}
	}
	return out, nil
}
