// Package v2 compiles vendor-neutral comparison policies and compares detached
// documents offline. Vendor envelope formats and mapping languages live in adapters.
package v2

import "encoding/json"

type Selector struct {
	Type  string `json:"type"`
	Key   string `json:"key,omitempty"`
	Space string `json:"space,omitempty"`
	Path  string `json:"path"`
}
type Rule struct {
	ID       string    `json:"id"`
	Replaces string    `json:"replaces,omitempty"`
	Required bool      `json:"required,omitempty"`
	Modes    []string  `json:"modes,omitempty"`
	Evidence *Evidence `json:"evidence,omitempty"`
}

// Evidence records attribution, never permission or inferred authority.
type Evidence struct {
	Kind       string            `json:"kind"`
	Digest     string            `json:"digest,omitempty"`
	RecordID   string            `json:"record_id,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}
type Identity struct {
	Rule
	Type          string   `json:"type"`
	Strategy      string   `json:"strategy"`
	Paths         []string `json:"paths,omitempty"`
	ComponentType string   `json:"component_type,omitempty"`
	ParentType    string   `json:"parent_type,omitempty"`
	ParentPath    string   `json:"parent_path,omitempty"`
}
type Pairing struct {
	Rule
	Type    string `json:"type"`
	LeftID  string `json:"left_id"`
	RightID string `json:"right_id"`
	// BaselineID explicitly relates the same object to an optional third input.
	BaselineID string `json:"baseline_id,omitempty"`
}
type Ignore struct {
	Rule
	Selector
}
type Sensitive struct {
	Rule
	Selector
}
type Collection struct {
	Rule
	Selector
	Mode string   `json:"mode"`
	Keys []string `json:"keys,omitempty"`
}
type ValuePair struct {
	Left  any `json:"left"`
	Right any `json:"right"`
}
type Equivalence struct {
	Rule
	Selector
	LeftEnv     string      `json:"left_env"`
	RightEnv    string      `json:"right_env"`
	LeftTenant  string      `json:"left_tenant,omitempty"`
	RightTenant string      `json:"right_tenant,omitempty"`
	Pairs       []ValuePair `json:"pairs,omitempty"`
	// Each environment contributes exactly one value to a group. Groups and
	// pairs cannot be combined in one rule. Groups are explicitly symmetric.
	Groups []map[string]any `json:"groups,omitempty"`
}
type State struct {
	State string          `json:"state"`
	Value json.RawMessage `json:"value,omitempty"`
}
type Expectation struct {
	Rule
	Selector
	Operation         string           `json:"operation"`
	Expected          *State           `json:"expected,omitempty"`
	Values            map[string]State `json:"values,omitempty"`
	From              *string          `json:"from,omitempty"`
	TargetType        string           `json:"target_type,omitempty"`
	Authority         string           `json:"authority"`
	SourceEquals      *State           `json:"source_equals,omitempty"`
	SourceEnvironment string           `json:"source_environment,omitempty"`
	SourceTenant      string           `json:"source_tenant,omitempty"`
	TargetTenant      string           `json:"target_tenant,omitempty"`
}
type Reference struct {
	Rule
	Selector
	TargetType string            `json:"target_type"`
	External   map[string]string `json:"external,omitempty"`
}
type Policy struct {
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Modes        []string          `json:"modes,omitempty"`
	Profile      string            `json:"profile,omitempty"`
	Bindings     map[string]string `json:"bindings,omitempty"`
	Identities   []Identity        `json:"identities,omitempty"`
	Pairings     []Pairing         `json:"pairings,omitempty"`
	Ignores      []Ignore          `json:"ignores,omitempty"`
	Expectations []Expectation     `json:"expectations,omitempty"`
	Equivalences []Equivalence     `json:"equivalences,omitempty"`
	References   []Reference       `json:"references,omitempty"`
	Collections  []Collection      `json:"collections,omitempty"`
	Sensitive    []Sensitive       `json:"sensitive,omitempty"`
	Evidence     []Evidence        `json:"evidence,omitempty"`
}

// Profile is supplied by an adapter; the compiler has no built-in vendor types.
type Profile struct {
	ID    string
	Rules Policy
}
type Compiled struct {
	policy        Policy
	digest        string
	replacements  map[string]string
	profileDigest string
}

func (c *Compiled) Digest() string { return c.digest }

type Document struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Path     string `json:"path"`
	Content  any    `json:"content"`
	Metadata any    `json:"metadata"`
}
type Input struct {
	Label        string     `json:"label"`
	Environment  string     `json:"environment,omitempty"`
	Tenant       string     `json:"tenant,omitempty"`
	ContentHash  string     `json:"content_hash"`
	DocumentHash string     `json:"document_hash"`
	ManifestHash string     `json:"manifest_hash,omitempty"`
	JobID        string     `json:"job_id,omitempty"`
	Completed    string     `json:"completed,omitempty"`
	ReadMode     string     `json:"read_mode,omitempty"`
	Types        []string   `json:"types"`
	Documents    []Document `json:"-"`
}
type Operation struct {
	Type   string `json:"type"`
	Key    string `json:"key"`
	Action string `json:"action"`
}
type Request struct {
	Mode     string
	Left     Input
	Right    Input
	Baseline *Input
	Scope    []string
	// ObjectKey selects an exact logical key; Scope must contain one type.
	ObjectKey       string
	Operations      []Operation
	IncludeMatching bool
}
type Location struct {
	ID        string `json:"id"`
	Artifact  string `json:"artifact"`
	Pointer   string `json:"pointer"`
	Present   bool   `json:"present"`
	ValueKind string `json:"value_kind,omitempty"`
}
type Decision struct {
	Type       string              `json:"type"`
	Key        string              `json:"key"`
	Status     string              `json:"status"`
	Code       string              `json:"code,omitempty"`
	Space      string              `json:"space,omitempty"`
	RuleID     string              `json:"rule_id,omitempty"`
	Replaces   string              `json:"replaces,omitempty"`
	Authority  string              `json:"authority,omitempty"`
	Evidence   *Evidence           `json:"evidence,omitempty"`
	ItemKeys   []string            `json:"item_keys,omitempty"`
	Locations  map[string]Location `json:"locations,omitempty"`
	Derivation *Derivation         `json:"derivation,omitempty"`
}
type Derivation struct {
	Role     string   `json:"role"`
	Type     string   `json:"type"`
	Location Location `json:"location"`
}
type Diagnostic struct {
	Code     string `json:"code"`
	RuleID   string `json:"rule_id,omitempty"`
	Type     string `json:"type,omitempty"`
	Side     string `json:"side,omitempty"`
	Required bool   `json:"required,omitempty"`
}
type Match struct {
	Type      string              `json:"type"`
	Key       string              `json:"key"`
	RuleID    string              `json:"rule_id"`
	Locations map[string]Location `json:"locations"`
}
type Report struct {
	Version        int                 `json:"version"`
	Mode           string              `json:"mode"`
	Complete       bool                `json:"complete"`
	Correct        bool                `json:"correct"`
	PolicyID       string              `json:"policy_id"`
	PolicyDigest   string              `json:"policy_digest"`
	RequestDigest  string              `json:"request_digest"`
	ProfileDigest  string              `json:"profile_digest,omitempty"`
	Profile        string              `json:"profile,omitempty"`
	Inputs         map[string]Input    `json:"inputs"`
	Scope          []string            `json:"scope"`
	ObjectKey      string              `json:"object_key,omitempty"`
	Operations     []Operation         `json:"operations,omitempty"`
	ObjectCoverage map[string]int      `json:"object_coverage"`
	Excluded       map[string][]string `json:"excluded"`
	Counts         map[string]int      `json:"counts"`
	RuleUsage      map[string]int      `json:"rule_usage"`
	Replacements   map[string]string   `json:"replacements"`
	Evidence       []Evidence          `json:"evidence,omitempty"`
	Matches        []Match             `json:"matches"`
	Decisions      []Decision          `json:"decisions"`
	Diagnostics    []Diagnostic        `json:"diagnostics"`
}

func (r Report) ExitCode() int {
	if !r.Complete {
		return 2
	}
	if !r.Correct {
		return 1
	}
	return 0
}
