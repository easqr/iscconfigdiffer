package differ

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/easqr/iscconfigdiffer/v2"
)

var fragmentFields = map[string]bool{
	"identities": true, "pairings": true, "ignores": true, "expectations": true,
	"equivalences": true, "references": true, "collections": true, "collection_mappings": true,
	"sensitive": true, "disabled_rules": true, "evidence": true,
}
var policyRuleFields = []string{"Identities", "Pairings", "Ignores", "Expectations", "Equivalences", "References", "Collections", "CollectionMappings", "Sensitive"}

// LoadPolicy reads one root policy and its explicitly included rule fragments.
// Includes are relative to, and contained within, the root policy directory.
// Fragments cannot contain policy settings or additional includes. Returned file
// evidence is attribution only: it never supplies rule evidence or authority.
func LoadPolicy(path string) (Policy, error) {
	var out Policy
	absolute, err := filepath.Abs(path)
	if err != nil {
		return out, fmt.Errorf("cannot locate policy %q: %w", path, err)
	}
	// A root symlink selects the target bundle. Includes are contained within
	// that policy's directory, independently of where the link was placed.
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return out, fmt.Errorf("cannot locate policy %q: %w", path, err)
	}
	root, err := os.OpenRoot(filepath.Dir(absolute))
	if err != nil {
		return out, fmt.Errorf("cannot read policy %q: %w", path, err)
	}
	defer root.Close()
	type source struct {
		name     string
		info     os.FileInfo
		data     []byte
		body     map[string]any
		fragment bool
	}
	sources := []source{}
	read := func(name string, fragment bool) (source, error) {
		s := source{name: filepath.ToSlash(filepath.Clean(name)), fragment: fragment}
		if !filepath.IsLocal(name) {
			return s, fmt.Errorf("policy include %q must be relative and stay within the root directory", name)
		}
		// Read the same normalized path that is recorded in provenance. Cleaning
		// only the record would misidentify files when symlinks precede "..".
		name = filepath.Clean(name)
		// Reject special files before opening, so a named pipe cannot block loading.
		info, err := root.Stat(name)
		if err != nil {
			return s, fmt.Errorf("cannot read policy file %q: %w", s.name, err)
		}
		if !info.Mode().IsRegular() {
			return s, fmt.Errorf("policy file %q must be a regular file", s.name)
		}
		f, err := root.Open(name)
		if err != nil {
			return s, fmt.Errorf("cannot read policy file %q: %w", s.name, err)
		}
		defer f.Close()
		s.info, err = f.Stat()
		if err != nil {
			return s, fmt.Errorf("cannot stat policy file %q: %w", s.name, err)
		}
		if !s.info.Mode().IsRegular() {
			return s, fmt.Errorf("policy file %q must be a regular file", s.name)
		}
		for _, old := range sources {
			if os.SameFile(old.info, s.info) {
				return s, fmt.Errorf("duplicate policy file %q (already loaded as %q)", s.name, old.name)
			}
		}
		s.data, err = io.ReadAll(f)
		if err != nil {
			return s, fmt.Errorf("cannot read policy file %q: %w", s.name, err)
		}
		value, err := v2.JSON(s.data)
		if err != nil {
			return s, fmt.Errorf("policy file %q: %w", s.name, err)
		}
		var ok bool
		s.body, ok = value.(map[string]any)
		if !ok {
			return s, fmt.Errorf("policy file %q must contain a JSON object", s.name)
		}
		return s, nil
	}
	first, err := read(filepath.Base(absolute), false)
	if err != nil {
		return out, err
	}
	sources = append(sources, first)
	includes := []string{}
	if value, present := first.body["includes"]; present {
		values, ok := value.([]any)
		if !ok {
			return out, fmt.Errorf("policy file %q: includes must be an array of relative filenames", first.name)
		}
		for _, value := range values {
			name, ok := value.(string)
			if !ok || strings.TrimSpace(name) == "" {
				return out, fmt.Errorf("policy file %q: includes requires nonblank filenames", first.name)
			}
			includes = append(includes, name)
		}
		delete(first.body, "includes")
	}
	rootBytes, _ := json.Marshal(first.body)
	out, err = v2.Decode(rootBytes)
	if err != nil {
		return out, fmt.Errorf("policy file %q: %w", first.name, err)
	}
	policies := []Policy{out}
	for _, name := range includes {
		s, err := read(name, true)
		if err != nil {
			return Policy{}, err
		}
		keys := make([]string, 0, len(s.body))
		for key := range s.body {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if !fragmentFields[key] {
				return Policy{}, fmt.Errorf("policy fragment %q: property %q is not a rule field; settings and includes belong in the root policy", s.name, key)
			}
		}
		// Promote only for strict shape decoding; these headers are not authored or merged.
		s.body["version"] = 2
		s.body["id"] = "fragment"
		data, _ := json.Marshal(s.body)
		p, err := v2.Decode(data)
		if err != nil {
			return Policy{}, fmt.Errorf("policy fragment %q: %w", s.name, err)
		}
		sources = append(sources, s)
		policies = append(policies, p)
	}
	seenRules, seenDisabled := map[string]string{}, map[string]string{}
	out = policies[0]
	for index, p := range policies {
		source := sources[index]
		ids := []string{}
		for _, evidence := range p.Evidence {
			if evidence.Kind == "" {
				return Policy{}, fmt.Errorf("policy file %q: evidence kind required", source.name)
			}
		}
		for _, field := range policyRuleFields {
			cat := reflect.ValueOf(p).FieldByName(field)
			for i := 0; i < cat.Len(); i++ {
				id := cat.Index(i).FieldByName("Rule").Interface().(v2.Rule).ID
				if !v2.ValidRuleID(id) {
					return Policy{}, fmt.Errorf("policy file %q: invalid rule ID %q", source.name, id)
				}
				if previous, exists := seenRules[id]; exists {
					return Policy{}, fmt.Errorf("duplicate rule ID %q in policy files %q and %q", id, previous, source.name)
				}
				seenRules[id] = source.name
				ids = append(ids, id)
			}
			if index > 0 {
				dst := reflect.ValueOf(&out).Elem().FieldByName(field)
				dst.Set(reflect.AppendSlice(dst, cat))
			}
		}
		for _, id := range p.DisabledRules {
			if !v2.ValidRuleID(id) {
				return Policy{}, fmt.Errorf("policy file %q: invalid disabled rule ID %q", source.name, id)
			}
			if previous, exists := seenDisabled[id]; exists {
				return Policy{}, fmt.Errorf("duplicate disabled rule ID %q in policy files %q and %q", id, previous, source.name)
			}
			seenDisabled[id] = source.name
		}
		if index > 0 {
			out.DisabledRules = append(out.DisabledRules, p.DisabledRules...)
			out.Evidence = append(out.Evidence, p.Evidence...)
		}
		digest := sha256.Sum256(source.data)
		role := "root"
		if source.fragment {
			role = "fragment"
		}
		sort.Strings(ids)
		disabled := append([]string{}, p.DisabledRules...)
		sort.Strings(disabled)
		out.Evidence = append(out.Evidence, v2.Evidence{Kind: "policy-file", RecordID: source.name, Digest: hex.EncodeToString(digest[:]), Attributes: map[string]string{"role": role, "rule_ids": strings.Join(ids, " "), "disabled_rules": strings.Join(disabled, " ")}})
	}
	return out, nil
}

// Add file context to compiler errors without putting filesystem behavior into
// the core. Rule IDs remain the ownership keys; filenames never imply scope.
func policyFileError(p Policy, err error) error {
	if err == nil {
		return nil
	}
	ids := map[string]bool{}
	var ruleErr *v2.RuleError
	if errors.As(err, &ruleErr) {
		for _, id := range ruleErr.RuleIDs {
			ids[id] = true
		}
	}
	fileSet := map[string]bool{}
	root := ""
	for _, e := range p.Evidence {
		if e.Kind != "policy-file" {
			continue
		}
		if e.Attributes["role"] == "root" {
			root = e.RecordID
		}
		for _, id := range strings.Fields(e.Attributes["rule_ids"] + " " + e.Attributes["disabled_rules"]) {
			if ids[id] {
				fileSet[e.RecordID] = true
				break
			}
		}
	}
	if len(fileSet) == 0 && root != "" {
		fileSet[root] = true
	}
	files := make([]string, 0, len(fileSet))
	for file := range fileSet {
		files = append(files, file)
	}
	if len(files) == 0 {
		return err
	}
	sort.Strings(files)
	return fmt.Errorf("%w (policy files: %s)", err, strings.Join(files, ", "))
}
