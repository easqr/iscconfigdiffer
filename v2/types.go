// Package v2 compiles vendor-neutral comparison policies and compares detached
// documents offline. Vendor envelope formats and mapping languages live in adapters.
package v2

import "encoding/json"

type Selector struct {
	Type    string `json:"type"`
	Variant string `json:"variant,omitempty"`
	Key     string `json:"key,omitempty"`
	Space   string `json:"space,omitempty"`
	Path    string `json:"path"`
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
type CollectionKeyCase struct {
	WhenPath  string          `json:"when_path"`
	WhenValue json.RawMessage `json:"when_value"`
	Keys      []string        `json:"keys"`
}

// CollectionKeyMapping aligns one key component through explicit environment
// values. It does not change observed values or establish value equivalence.
type CollectionKeyMapping struct {
	Path   string                      `json:"path"`
	Groups []CollectionKeyMappingGroup `json:"groups"`
}
type CollectionKeyMappingGroup struct {
	Key    string         `json:"key"`
	Values map[string]any `json:"values"`
}

// DisplayField declares a scalar collection-item identifier for report context.
type DisplayField struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}
type Collection struct {
	Rule
	Selector
	Mode          string                 `json:"mode"`
	Keys          []string               `json:"keys,omitempty"`
	ComponentType string                 `json:"component_type,omitempty"`
	KeyCases      []CollectionKeyCase    `json:"key_cases,omitempty"`
	KeyMappings   []CollectionKeyMapping `json:"key_mappings,omitempty"`
	ItemLabel     string                 `json:"item_label,omitempty"`
	DisplayFields []DisplayField         `json:"display_fields,omitempty"`
	PriorityOrder *PriorityOrder         `json:"priority_order,omitempty"`
}

// PriorityOrder derives collection precedence from referenced context documents.
// Lower numeric priorities precede higher ones; observed documents are immutable.
type PriorityOrder struct {
	Type          string `json:"type"`
	ReferencePath string `json:"reference_path"`
	MatchPath     string `json:"match_path"`
	PriorityPath  string `json:"priority_path"`
}

// CollectionMapping adds implementation-specific mappings to an existing
// collection without taking ownership of its structural definition.
type CollectionMapping struct {
	Rule
	Collection  string                 `json:"collection"`
	Key         string                 `json:"key,omitempty"`
	KeyMappings []CollectionKeyMapping `json:"key_mappings"`
}
type CollectionMappingUsage struct {
	RuleID     string    `json:"rule_id"`
	Collection string    `json:"collection"`
	Evidence   *Evidence `json:"evidence,omitempty"`
}
type ValuePair struct {
	Left  any `json:"left"`
	Right any `json:"right"`
}

// StatePair explicitly distinguishes absent properties from present JSON values.
type StatePair struct {
	Left  State `json:"left"`
	Right State `json:"right"`
}
type Equivalence struct {
	Rule
	Selector
	Operation   string                   `json:"operation,omitempty"`
	Item        *CollectionItemSelector  `json:"item,omitempty"`
	Items       []CollectionItemSelector `json:"items,omitempty"`
	LeftEnv     string                   `json:"left_env"`
	RightEnv    string                   `json:"right_env"`
	LeftTenant  string                   `json:"left_tenant,omitempty"`
	RightTenant string                   `json:"right_tenant,omitempty"`
	Pairs       []ValuePair              `json:"pairs,omitempty"`
	StatePairs  []StatePair              `json:"state_pairs,omitempty"`
	// Each environment contributes exactly one value to a group. Groups and
	// pairs cannot be combined in one rule. Groups are explicitly symmetric.
	Groups []map[string]any `json:"groups,omitempty"`
}

// CollectionItemSelector selects an aligned item by its complete logical key.
// Mapped keys reference declared mapping-group names; values match raw keys.
type CollectionItemSelector struct {
	Collection string                       `json:"collection"`
	Keys       map[string]CollectionItemKey `json:"keys"`
}
type CollectionItemKey struct {
	Mapped string          `json:"mapped,omitempty"`
	Value  json.RawMessage `json:"value,omitempty"`
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
	ObjectFilters map[string]ObjectFilter `json:"object_filters,omitempty"`
	DisabledRules []string                `json:"disabled_rules,omitempty"`
	// NullHandling controls content-property comparison; neutral default is strict.
	NullHandling       string              `json:"null_handling,omitempty"`
	Version            int                 `json:"version"`
	ID                 string              `json:"id"`
	Modes              []string            `json:"modes,omitempty"`
	Profile            string              `json:"profile,omitempty"`
	Bindings           map[string]string   `json:"bindings,omitempty"`
	Identities         []Identity          `json:"identities,omitempty"`
	Pairings           []Pairing           `json:"pairings,omitempty"`
	Ignores            []Ignore            `json:"ignores,omitempty"`
	Expectations       []Expectation       `json:"expectations,omitempty"`
	Equivalences       []Equivalence       `json:"equivalences,omitempty"`
	References         []Reference         `json:"references,omitempty"`
	Collections        []Collection        `json:"collections,omitempty"`
	CollectionMappings []CollectionMapping `json:"collection_mappings,omitempty"`
	Sensitive          []Sensitive         `json:"sensitive,omitempty"`
	Evidence           []Evidence          `json:"evidence,omitempty"`
}

// ObjectFilter narrows validation by exact logical object key within a type.
// Exclusions win; omitted include lists allow every key not excluded.
type ObjectFilter struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
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
	Type string `json:"type"`
	// Variant is adapter-classified domain metadata, independent of object name.
	Variant  string `json:"variant,omitempty"`
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
	// ExcludeValues omits observed and expected values from report decisions.
	ExcludeValues bool
}
type ContextField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type ItemContext struct {
	Label   string         `json:"label"`
	Pointer string         `json:"pointer"`
	Fields  []ContextField `json:"fields,omitempty"`
}
type Location struct {
	ID        string        `json:"id"`
	Artifact  string        `json:"artifact"`
	Pointer   string        `json:"pointer"`
	Present   bool          `json:"present"`
	ValueKind string        `json:"value_kind,omitempty"`
	Value     *State        `json:"value,omitempty"`
	Context   []ItemContext `json:"context,omitempty"`
}
type Decision struct {
	Items         []CollectionItemSelector `json:"items,omitempty"`
	Item          *CollectionItemSelector  `json:"item,omitempty"`
	Extensions    []CollectionMappingUsage `json:"extensions,omitempty"`
	Normalization string                   `json:"normalization,omitempty"`
	Expected      *State                   `json:"expected,omitempty"`
	ExpectedRole  string                   `json:"expected_role,omitempty"`
	Type          string                   `json:"type"`
	Key           string                   `json:"key"`
	Status        string                   `json:"status"`
	Code          string                   `json:"code,omitempty"`
	Space         string                   `json:"space,omitempty"`
	RuleID        string                   `json:"rule_id,omitempty"`
	Replaces      string                   `json:"replaces,omitempty"`
	Authority     string                   `json:"authority,omitempty"`
	Evidence      *Evidence                `json:"evidence,omitempty"`
	ItemKeys      []string                 `json:"item_keys,omitempty"`
	Locations     map[string]Location      `json:"locations,omitempty"`
	Derivation    *Derivation              `json:"derivation,omitempty"`
}
type Derivation struct {
	Role     string   `json:"role"`
	Type     string   `json:"type"`
	Location Location `json:"location"`
}
type Diagnostic struct {
	Code     string `json:"code"`
	Key      string `json:"key,omitempty"`
	RuleID   string `json:"rule_id,omitempty"`
	Type     string `json:"type,omitempty"`
	Side     string `json:"side,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type ExcludedObject struct {
	Type      string              `json:"type"`
	Key       string              `json:"key"`
	Reason    string              `json:"reason"`
	Locations map[string]Location `json:"locations"`
}
type Match struct {
	Type      string              `json:"type"`
	Key       string              `json:"key"`
	RuleID    string              `json:"rule_id"`
	Locations map[string]Location `json:"locations"`
}
type Report struct {
	ObjectFilters   map[string]ObjectFilter `json:"object_filters,omitempty"`
	ExcludedObjects []ExcludedObject        `json:"excluded_objects,omitempty"`
	DisabledRules   []string                `json:"disabled_rules,omitempty"`
	NullHandling    string                  `json:"null_handling"`
	ValuesExcluded  bool                    `json:"values_excluded"`
	Version         int                     `json:"version"`
	Mode            string                  `json:"mode"`
	Complete        bool                    `json:"complete"`
	Correct         bool                    `json:"correct"`
	PolicyID        string                  `json:"policy_id"`
	PolicyDigest    string                  `json:"policy_digest"`
	RequestDigest   string                  `json:"request_digest"`
	ProfileDigest   string                  `json:"profile_digest,omitempty"`
	Profile         string                  `json:"profile,omitempty"`
	Inputs          map[string]Input        `json:"inputs"`
	Scope           []string                `json:"scope"`
	ObjectKey       string                  `json:"object_key,omitempty"`
	Operations      []Operation             `json:"operations,omitempty"`
	ObjectCoverage  map[string]int          `json:"object_coverage"`
	Excluded        map[string][]string     `json:"excluded"`
	Counts          map[string]int          `json:"counts"`
	RuleUsage       map[string]int          `json:"rule_usage"`
	Replacements    map[string]string       `json:"replacements"`
	Evidence        []Evidence              `json:"evidence,omitempty"`
	Matches         []Match                 `json:"matches"`
	Decisions       []Decision              `json:"decisions"`
	Diagnostics     []Diagnostic            `json:"diagnostics"`
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
