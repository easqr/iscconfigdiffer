package v2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// JSON rejects duplicate keys at every depth and preserves exact numbers.
func JSON(b []byte) (any, error) {
	if !utf8.Valid(b) {
		return nil, errors.New("invalid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	v, e := jsonValue(d)
	if e != nil {
		return nil, errors.New("invalid JSON")
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return v, nil
}
func jsonValue(d *json.Decoder) (any, error) {
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, e
				}
				s, ok := k.(string)
				if !ok {
					return nil, errors.New("key")
				}
				if _, ok = m[s]; ok {
					return nil, errors.New("duplicate key")
				}
				v, e := jsonValue(d)
				if e != nil {
					return nil, e
				}
				m[s] = v
			}
			t, e = d.Token()
			if e != nil || t != json.Delim('}') {
				return nil, errors.New("object")
			}
			return m, nil
		case '[':
			a := []any{}
			for d.More() {
				v, e := jsonValue(d)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			t, e = d.Token()
			if e != nil || t != json.Delim(']') {
				return nil, errors.New("array")
			}
			return a, nil
		}
		return nil, errors.New("delimiter")
	}
	return t, nil
}
func strict(b []byte, out any) error {
	v, e := JSON(b)
	if e != nil {
		return e
	}
	if e = shape(v, reflect.TypeOf(out).Elem()); e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e = d.Decode(out); e != nil {
		return errors.New("invalid policy shape")
	}
	return nil
}

var rawType = reflect.TypeFor[json.RawMessage]()

func shape(v any, t reflect.Type) error {
	if t == rawType || t.Kind() == reflect.Interface {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		if v == nil {
			return errors.New("null policy object")
		}
		return shape(v, t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("policy object required")
		}
		fields := map[string]reflect.StructField{}
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
					fields[tag] = f
				}
			}
		}
		collect(t)
		for k, x := range m {
			f, ok := fields[k]
			if !ok {
				return errors.New("unknown policy property")
			}
			if e := shape(x, f.Type); e != nil {
				return e
			}
		}
		for k, f := range fields {
			if t == reflect.TypeFor[Equivalence]() && m["operation"] == "relative_order" && (k == "left_env" || k == "right_env") {
				continue
			}
			if !strings.Contains(f.Tag.Get("json"), "omitempty") {
				if _, ok := m[k]; !ok {
					return errors.New("missing policy property: " + k)
				}
			}
		}
	case reflect.Slice:
		a, ok := v.([]any)
		if !ok {
			return errors.New("policy array required")
		}
		for _, x := range a {
			if e := shape(x, t.Elem()); e != nil {
				return e
			}
		}
	case reflect.Map:
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("policy map required")
		}
		for _, x := range m {
			if e := shape(x, t.Elem()); e != nil {
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
			return errors.New("policy number required")
		}
	}
	return nil
}
func pointer(p string) ([]string, error) {
	if p == "" {
		return nil, nil
	}
	if !strings.HasPrefix(p, "/") {
		return nil, errors.New("JSON Pointer required")
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
func esc(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
func get(v any, p string) (any, bool) {
	a, e := pointer(p)
	if e != nil {
		return nil, false
	}
	for _, k := range a {
		switch x := v.(type) {
		case map[string]any:
			var ok bool
			v, ok = x[k]
			if !ok {
				return nil, false
			}
		case []any:
			i, e := strconv.Atoi(k)
			if e != nil || i < 0 || i >= len(x) || strconv.Itoa(i) != k {
				return nil, false
			}
			v = x[i]
		default:
			return nil, false
		}
	}
	return v, true
}
func arrayIndex(s string) (int, bool) {
	i, e := strconv.Atoi(s)
	return i, e == nil && i >= 0 && strconv.Itoa(i) == s
}
func keys[V any](m map[string]V) []string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
func canonical(v any) any {
	switch x := v.(type) {
	case json.Number:
		r, ok := new(big.Rat).SetString(x.String())
		if ok {
			return []any{"number", r.RatString()}
		}
	case map[string]any:
		m := map[string]any{}
		for k, v := range x {
			m[k] = canonical(v)
		}
		return []any{"object", m}
	case []any:
		a := []any{}
		for _, v := range x {
			a = append(a, canonical(v))
		}
		return []any{"array", a}
	}
	return v
}
func valueKey(v any) string { b, _ := json.Marshal(canonical(v)); return string(b) }
func equal(a, b any) bool   { return valueKey(a) == valueKey(b) }
func scalar(v any) bool {
	switch v.(type) {
	case string:
		return true
	case bool, json.Number:
		return true
	}
	return false
}
func concrete(p string) bool {
	a, e := pointer(p)
	if e != nil {
		return false
	}
	for _, s := range a {
		if s == "*" {
			return false
		}
	}
	return true
}
func space(s string) string {
	if s == "" {
		return "content"
	}
	return s
}
func overlap(a, b Selector) bool {
	if a.Type != b.Type || (a.Variant != "" && b.Variant != "" && a.Variant != b.Variant) || space(a.Space) != space(b.Space) || (a.Key != "" && b.Key != "" && a.Key != b.Key) {
		return false
	}
	x, _ := pointer(a.Path)
	y, _ := pointer(b.Path)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] && x[i] != "*" && y[i] != "*" {
			return false
		}
	}
	return true
}
func selects(s Selector, typ, key, sp, path string) bool {
	if s.Type != typ || space(s.Space) != sp || (s.Key != "" && s.Key != key) {
		return false
	}
	x, _ := pointer(s.Path)
	y, _ := pointer(path)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != "*" && x[i] != y[i] {
			return false
		}
	}
	return true
}
