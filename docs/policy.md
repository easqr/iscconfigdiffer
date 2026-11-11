# Comparison policy v2

Comparison policies use schema `2` for offline comparison of validated ISC
exports and saved Configuration Hub mappings. The `v2` library package also
supports vendor-neutral documents and policies.

This is the detailed contract reference. For a first comparison, start with the
[README](../README.md). For policy authoring examples, use the
[configuration reference](configuration.md); for application setup, use the
[Go library guide](library.md).

## Library entry points

* `v2.Decode`, `v2.Compile`, `v2.Run`: neutral policy/documents.
* `differ.AdaptSnapshot`: validates and detaches `snapshot.Input`, preserving
  provenance and unknown fields. Payload becomes `content`; envelope fields other
  than `object` become `metadata`.
* `differ.LoadPolicy(path)`: strict root-policy/fragment file composition with provenance.
* `differ.DecodePolicy(bytes)`: strict self-contained policy decoding, without file access.
* `differ.Compile` and `differ.Compare`: ISC profiles and validated storage.
* `differ.ImportHub` / `ReadHub`: offline array/bundle adapters.

The CLI and library use one comparison engine and policy/report schema `2`.
The CLI defaults to drift mode; select `--mode deployment` for draft validation.
Policies use payload-relative selectors. `DecodePolicy`, `Compile`, and `Compare`
are the public ISC entry points; the `v2` package provides the neutral core.

## Root includes and rule-only fragments

Use `differ.LoadPolicy(path)` or CLI `--policy FILE` for a root policy containing
`"includes": ["sources/active-directory.json", "transforms.json"]`.
Includes are an optional array of explicit, nonblank, root-relative filenames.
Root-policy symlinks resolve to the target bundle before reading: includes use
the target policy's directory and provenance uses its basename. Include paths
are lexically normalized before reading, so the recorded path identifies the
file actually opened, including when a symlink component precedes `..` in the
authored name. All includes must remain inside the root directory, including
symlink targets.
Absolute/escaping paths, repeated files (including hard-link/symlink aliases),
nonregular files, invalid/duplicate JSON properties, and unknown properties fail.
There are no globs, recursive includes, directory scanning, or implicit overrides.
The root itself may contain rules. No `includes` means a normal single-file policy.
`DecodePolicy` and `v2.Decode` do no I/O and reject `includes`.

Root settings (`version`, `id`, `profile`, `bindings`, `null_handling`, `modes`,
`object_filters`)
are never merged from fragments. Fragments accept only these fields:

```text
identities, pairings, ignores, expectations, equivalences, exceptions, references,
collections, collection_mappings, sensitive, disabled_rules, evidence
```

All except `evidence` and `disabled_rules` are ordinary rule arrays with the same
syntax and validation as root rules. `disabled_rules` is an array of shipped rule
IDs; `evidence` is an array of attribution records. Rule-level modes and evidence
remain part of each rule. A fragment has no `version`, `id`, or `includes` header;
`{}` is a valid empty fragment. All files use strict JSON without comments.

Composition appends arrays and then compiles once. Rule IDs are globally unique
across categories and files. Repeated disabled IDs also fail. Include order does
not confer precedence. Conflict checks, explicit replacement, and disabling
rules apply across all files; a more specific selector does not override a broader rule. File names do not confer scope; explicit selectors do.
Rule validation errors carry structured `*v2.RuleError` ownership, available via
`errors.As`; file attribution uses those exact IDs rather than words in an error
message. Cross-file conflicts identify both owners where known, and filenames
are deduplicated. Policy-level errors identify the root. Parsing, invalid rule-ID,
invalid attribution-kind, and duplicate-ID errors name their files during loading.
Filesystem failures preserve their underlying causes for `errors.Is` and
`errors.As`. `LoadPolicy` validates file structure and ownership metadata;
`Compile` or `Compare` validates the full assembled policy.

`LoadPolicy` adds attribution-only `Policy.Evidence` records, one per physical
file, with `kind: "policy-file"`, a root-relative `record_id`, SHA-256 of actual
file bytes, and string attributes `role` (`root`/`fragment`), `rule_ids`, and
`disabled_rules` (sorted, space-separated IDs). The root name is its basename.
Fragment evidence is retained. File evidence never populates a rule's evidence
or authority. JSON reports retain the index; human reports inventory files and
show origins for applied rules/extensions. Evidence is public metadata and is
not hidden by value exclusion.

Because file provenance participates in the effective policy digest, splitting,
reformatting, or reordering a manifest changes policy/request digests without
necessarily changing decisions. Relocating an unchanged bundle with the same
root basename and relative layout does not change file provenance. The same comparison rules apply to single-file and assembled policies.

See the [policy-file walkthrough](policy-files.md#organize-policies-across-files) and the
[modular example](../examples/policy-includes.json). These are custom
policy fragments, distinct from the adapter's embedded [rule packs](rule-packs.md).

## CLI report files

The CLI saves every produced report under `--output-dir` (default `reports` in
the working directory) as `<policy-id>.html` by default. Select `--format md` for
`<policy-id>.md` or `--format json` for `<policy-id>.json`. HTML documents are
self-contained and require no network access. Markdown and HTML carry the same
human report content, grouped by object with per-role locations, matching, rule
evidence, derivation and input/policy provenance. HTML uses local JavaScript for
search and status filters; static content remains readable without it. Counts
distinguish missing-object, container-presence and other difference decisions.
Displayed group counts may omit ordinary equal leaves counted by the engine.
Redacted logical identities are combined and labelled rather than presented as
individual objects. Rendering uses only the structured report, preserving
redaction and never reopening configuration artifacts. The library exposes `Report.HTML` and `Report.Markdown`;
JSON serialization is unchanged. The built-in policy ID is `default`.
`--output-file` overrides the
exact filename, without directory components; it does not alter the format.
Directories are created as needed. Existing reports are replaced after the new
report has been fully written. Report contents are written only to files;
standard output remains empty.
Output failures return exit `2`. Incomplete comparison reports are saved too;
invalid requests that never produce a report do not create one. The library's
report API does not write files automatically.

## Object selection

`differ.Request.ObjectKey` and `v2.Request.ObjectKey` optionally select one exact,
case-sensitive logical key. Set `Scope` to exactly one explicit type. The CLI
exposes this as `--scope SOURCE --name "Active Directory"`. Selection uses the
matched group's source/left key (including explicit pairings), not a physical ID.

The full inventory remains validated and matched for identity/reference context;
only the selected group produces comparison decisions. A one-sided object remains
a drift difference. No matching group produces `selected object unavailable` and
exit `2`. Deployment requires an operation for the selected key; unselected
operations remain reference context and do not produce comparison decisions.
Global identity diagnostics and required-rule checks still apply.

Reports include optional `object_key` and bind it into `request_digest`.
`*_in_scope` coverage remains the full type inventory; `*_outside_object_filter`
counts skipped inventory objects, and `compared_groups` counts selected groups.
Sensitive selectors redact the filter key along with other object identities.

## Named object selection

Keep using `--scope` to choose types. For reusable name lists across those types,
add `object_filters` to the **root policy**:

```json
{
  "version": 2,
  "id": "filtered-drift",
  "profile": "isc-portable-v2",
  "object_filters": {
    "SOURCE": {
      "exclude": ["Dev - Active Directory"]
    },
    "ROLE": {
      "include": ["Employee Access", "Finance Analyst"]
    }
  }
}
```

With `--scope SOURCE,ROLE`, this compares every source except
`Dev - Active Directory`, and only the two listed roles. Other types are selected
only when included in the command's type scope. A type without an `object_filters`
entry keeps its ordinary scope behavior. See the complete
[filtered policy example](../examples/policy-filtered-drift.json).

- `include` is a whitelist: when supplied, only those logical names are validated
  for that type. Omit it to permit all names not excluded.
- `exclude` is a blacklist. It wins when a name occurs in both lists.
- Names are exact and case-sensitive. There are no wildcards, regular expressions,
  substrings, or physical-ID matching. Composite identities use the logical key
  shown in reports; explicit pairings use the left/source key, just like `--name`.
- Omit unused lists instead of writing empty arrays. Blank or duplicate names,
  empty filter entries, and types without an identity strategy fail compilation.
- A listed include name missing from both primary inputs and any baseline makes
  the active comparison incomplete. A name present on one side still produces
  an object-presence finding. An unused exclusion is reported as a diagnostic
  without failing the result. Checks apply only to types in scope and names not
  narrowed away by `--name`.
- Filters that select no configurations in the active scope produce an incomplete
  result (exit `2`). `--name` narrows the policy lists further; it cannot re-enable
  a configuration excluded by policy.

These lists work in drift and deployment. Excluded configurations and operations
are not validated, but remain context for references, dependent identities, and
relative priority comparisons. A referenced target scheduled for deletion is
still treated as such, even when its own validation is excluded. Input integrity,
identity matching, policy compilation, and required-rule checks remain enforced;
a filter is not permission to accept malformed exports or ambiguous identities.
A filtered result makes no claim about the excluded configurations.

Reports list **Configurations excluded by policy**, including each name and reason.
JSON includes `object_filters` and `excluded_objects`; `*_excluded_by_policy` and
`excluded_groups` distinguish policy exclusions from `--name` filtering and value
normalization. Filters contribute to policy/request digests. Sensitive type rules
also redact filter names, skipped-object identities, and named diagnostics.

`object_filters` is a root-policy setting, not a rule array. Keep it out of
rule-only include fragments; the same names apply consistently to both inputs.
Your existing local policy need not change unless you want to enable filtering.

## Schema

The root has `version:2`, `id`, optional `modes`, `profile`, `bindings`,
`null_handling`, `object_filters`, `disabled_rules`, `collection_mappings`, and rule arrays.
Root `modes` restricts permitted comparison modes; an omitted restriction permits both execution modes.
JSON is strict: exact property spelling, no unknown properties or duplicate keys,
valid UTF-8, no trailing JSON, and exact numbers. Policy/rule IDs use 1–200 ASCII
letters/digits and `_ . : -`, beginning with a letter/digit. Required JSON
properties must be supplied even when their valid value is an empty pointer.

All rule categories include `id` and optional `replaces`, `required`, `modes`, and
`evidence`. Evidence contains `kind`, optional `digest`, `record_id`, and string
`attributes`. Evidence is attribution; authority is a separate expectation field.
Treat labels, authority and evidence attributes as visible report metadata and
do not put secrets in them.

Selectors contain `type`, `path`, optional exact logical `key`, optional
adapter-classified `variant`, and optional `space` (`content`, the default, or `metadata`). Paths are JSON Pointers relative
to the selected section. `""` selects the section root. Escape `~` as `~0` and
`/` as `~1`. A segment exactly `*` selects one object member or array item.
Literal missing paths are evaluated; wildcards expand only from available
structures.

| Array | Additional fields |
| --- | --- |
| `identities` | `type`, `strategy`: `fields`, `singleton`, or `dependent`; `paths`; optional `component_type`: `scalar` (default) or `string`; for dependent identity, `parent_type`, `parent_path` |
| `pairings` | `type`, `left_id`, `right_id`, optional `baseline_id` |
| `ignores` | Selector; explicit subtree exclusion/normalization |
| `expectations` | Selector; `operation`, `authority`; operation-specific fields below |
| `exceptions` | Value matching uses the same syntax and drift-only restrictions as `equivalences`, plus required nonblank string `rationale`; matched decisions have status `exception`. `operation: "exclude"` skips a selected subtree without value cases and with optional environment restrictions. Exact container/absence `state_pairs` may enclose nested collection and ignore rules; see [subtree exceptions](configuration.md#exceptions). |
| `equivalences` | Selector; `left_env`, `right_env`; exactly one of `pairs`, `groups`, or `state_pairs`; optional `left_tenant`, `right_tenant`, and keyed-ancestor `item` or `items` scope |
| `references` | Selector; `target_type`, optional one-to-one `external` ID map; drift-only |
| `collection_mappings` | `collection` effective keyed collection ID, optional object `key`, and nonempty `key_mappings`; common rule fields except `replaces` |
| `collections` | Selector; `mode`: `ordered`, `keyed`, or `multiset`; `keys`, optional `component_type`, `key_cases`, and `key_mappings` only for keyed; optional `item_label` and `display_fields` for report context |
| `sensitive` | Selector; conservative report redaction |

Identity and pairing rules are unconditional: `modes` is not allowed. Other rules
default to both modes, except equivalence and references, which default to drift
and cannot enable deployment. Use `modes:["deployment"]` for deployment-only
expectations and normalization. Binding names must match the request roles.

Exact, typed identity tuples distinguish numbers, strings and booleans. String
identity components are nonempty. Parent-qualified identity uses established
parent identities. Drift requires parent correspondence; deployment can identify
baseline/source-only parents for explicit deletions. Physical pairings reserve endpoints before automatic
matching; duplicate logical keys remain ambiguous even with pairings. Unknown
types need explicit strategies. Dependency cycles fail compilation; unavailable
parents or endpoints prevent runtime completeness. A rename remains value drift.

An ignore may select exactly a dependent identity's physical `parent_path` when
that path is not also a logical component in `paths`. Matching still uses the
original reference and establishes parent correspondence before exclusions run.
Logical identity components, broader parent objects, wildcard overlaps, and
parent-reference descendants remain protected. Equivalence rules retain the
stricter identity-overlap prohibition.

`pairs` is an array of typed `{left,right}` values, applicable in the declared
direction and environment pair. `groups` is an array of environment-to-value
objects; each group explicitly establishes symmetric equivalence among its
environments. Do not combine case forms. Cases must be unambiguous. There is no
substring replacement, substitution chaining, inferred transitivity or coercion.
Several disjoint cases belong in one rule; overlapping rule ownership is rejected.

Use `state_pairs` when an approved relationship includes an absent property.
Each `left` and `right` is a tagged state: `{"state":"absent"}` or
`{"state":"present","value":...}`. For example, a policy can explicitly approve
an optional setting's disabled value and absence in either direction:

```json
{
  "id": "optional-setting-disabled",
  "type": "SOURCE",
  "key": "Invented Source",
  "path": "/connectorAttributes/optionalSetting",
  "left_env": "dev",
  "right_env": "prod",
  "state_pairs": [
    {"left": {"state": "present", "value": false}, "right": {"state": "absent"}},
    {"left": {"state": "absent"}, "right": {"state": "present", "value": false}}
  ]
}
```

State pairs apply only in the declared left/right environment direction, just
like `pairs`; list both cases to approve both value orientations. Each left state
and each right state must be unique within its respective side. An absent state
cannot contain `value`; a present state requires it, including explicit `null`.
Matching uses exact presence and typed JSON equality, so `false`, `null`, and
absence remain distinct within these cases even with `null_equals_absent` enabled.
Existing `pairs` and `groups` continue to require both properties to be present.
These are drift equivalences, not exclusions or deployment assertions. Reports
retain the original values, presence, pointers, rule attribution, and item scope.

### Equivalence item scopes

An equivalence may include `item:{"collection":RULE_ID,"keys":{...}}` to select
one aligned item, or `items:[...]` to require several ancestor identities together.
Use exactly one form; `items` must be nonempty and each collection may appear only
once. All listed conditions must match, and their collections must form a compatible
nested ancestor chain of the selected field. Condition order is immaterial;
compilation canonicalizes `items` from outer to inner for stable policy digests.
The field selector still uses wildcard array segments. Scoping occurs after collection key resolution, including
environment mapping extensions; it never uses numeric item positions.

Each `keys` entry is an item-relative key pointer with exactly one of:

* `{"mapped":"group-name"}`: the logical group `key` declared for that key
  component in an applicable collection mapping.
* `{"value":SCALAR}`: an exact raw, unmapped key component.

The complete key tuple is required. This includes every component of a composite
key, or the complete default/conditional tuple of a `key_cases` collection.
Component paths must be distinct; conditional tuples cannot ambiguously reorder
the same set of component paths.
Group names are validated against inline mappings and drift-active extensions
compatible with the equivalence's object key. A raw literal is not resolved
through an environment mapping; it does not match a mapped alias even when its
text equals an alias name. Raw numeric keys retain exact mathematical equality;
strings, numbers, and booleans do not coerce. Null, structured, blank string
components under `component_type:"string"`, missing components, and ambiguous
mapped/value forms are rejected.

Unknown, disabled, replaced, non-keyed, inactive, or non-ancestor collection
references fail compilation. Reference the effective replacement ID when
appropriate. Scope does not expand the collection's type, variant, key, section,
or path ownership. Two rules are disjoint when at least one collection constrained
by both rules has different complete tuples. This allows independent CREATE/UPN,
CREATE/email, and UPDATE/UPN approvals at the same field path. Without a disjoint
shared constraint, ownership may overlap: identical/reordered conditions,
broad/scoped competitors, and partially overlapping ancestor constraints are
rejected. A narrower rule does not take precedence over a broader one. Scope never adds precedence over ignores,
expectations, or other rule categories.

The selector requires consistent resolved component identities across the inputs
that contain the aligned item; it never borrows a conditional key from only one
side. The selector also works on a
field nested under additional child collections while retaining that ancestor
identity. Missing counterparts do not receive equivalence, duplicate keys still
conflict, and unavailable mapping environments cannot be bypassed. An unused
scoped rule follows existing optional/required coverage behavior. Unapproved
values fall back to ordinary comparison; declaring a scope is not an assertion
that an item must exist or that both environments must have a specified value.

Reports retain `Decision.item` for the single form or `Decision.items` for the
joint form on applied scoped equivalence decisions, together
with original locations, containing-item context, and mapping extension evidence.
HTML and Markdown show Collection item scope. Scope identifiers, like other
identity metadata, remain visible with `--exclude-values`; explicit sensitive
redaction removes them for affected types. Scope definitions are detached and
included in the effective policy digest. Equivalences remain drift-only;
expectations do not accept `item` or `items`.

For example, these two rules approve service accounts for two domains in the
same mapped forest. The complete executable policy and both domains' DN mapping
groups are in [policy-drift.json](../examples/policy-drift.json).

```json
{
  "equivalences": [
    {
      "id": "directory-corporate-domain-user",
      "type": "SOURCE",
      "key": "Invented Directory",
      "path": "/connectorAttributes/domainSettings/*/user",
      "item": {
        "collection": "isc.SOURCE.ad-domains",
        "keys": {
          "/forestName": {
            "mapped": "corporate-forest"
          },
          "/domainDN": {
            "mapped": "corporate-domain"
          }
        }
      },
      "left_env": "dev",
      "right_env": "prod",
      "groups": [
        {
          "dev": "svc-corporate@dev.example.invalid",
          "prod": "svc-corporate@prod.example.invalid"
        }
      ],
      "evidence": {
        "kind": "invented-policy",
        "record_id": "corporate-domain-service-account"
      }
    },
    {
      "id": "directory-regional-domain-user",
      "type": "SOURCE",
      "key": "Invented Directory",
      "path": "/connectorAttributes/domainSettings/*/user",
      "item": {
        "collection": "isc.SOURCE.ad-domains",
        "keys": {
          "/forestName": {
            "mapped": "corporate-forest"
          },
          "/domainDN": {
            "mapped": "regional-domain"
          }
        }
      },
      "left_env": "dev",
      "right_env": "prod",
      "groups": [
        {
          "dev": "svc-regional@regional.dev.example.invalid",
          "prod": "svc-regional@regional.prod.example.invalid"
        }
      ],
      "evidence": {
        "kind": "invented-policy",
        "record_id": "regional-domain-service-account"
      }
    }
  ]
}
```

Both rules reference `corporate-forest`, but their `corporate-domain` and
`regional-domain` identities differ. Moving a valid account pair to the other
domain produces drift, even though that pair is approved elsewhere.

### Nested provisioning example

The CREATE policy's UPN template requires two simultaneous constraints: the
`isc.SOURCE.provisioning-policies` tuple `/usageType = CREATE` and the
`isc.SOURCE.provisioning-fields` tuple `/name = userPrincipalName`. Both are
inherited structural keys; no numeric array indices or copied collection rules
are needed in the custom policy.

```json
{
  "id": "directory-create-user-principal-name",
  "type": "SOURCE",
  "key": "Invented Directory",
  "path": "/provisioningPolicies/*/fields/*/transform/attributes/value",
  "items": [
    {
      "collection": "isc.SOURCE.provisioning-policies",
      "keys": {
        "/usageType": {
          "value": "CREATE"
        }
      }
    },
    {
      "collection": "isc.SOURCE.provisioning-fields",
      "keys": {
        "/name": {
          "value": "userPrincipalName"
        }
      }
    }
  ],
  "left_env": "dev",
  "right_env": "prod",
  "groups": [
    {
      "dev": "$uid@dev.example.invalid",
      "prod": "$uid@prod.example.invalid"
    }
  ],
  "evidence": {
    "kind": "invented-policy",
    "record_id": "create-upn-template"
  }
}
```

This accepts only the indicated literal template pair in that policy and field.
Other policy usages, field names, transform types/attributes, and field flags
remain comparable. Identical account templates elsewhere do not inherit approval.
The full runnable declaration is also in
[policy-drift.json](../examples/policy-drift.json). Single `item` rules and existing
mapped/raw key semantics remain supported. No deployment behavior is changed.

### Collection key mappings

A keyed collection may declare `key_mappings`, an array of:

```json
{
  "path": "/forestName",
  "groups": [
    {
      "key": "corporate-forest",
      "values": {"dev":"dev.example.invalid","prod":"prod.example.invalid"}
    }
  ]
}
```

`path` is a concrete item-relative pointer present in the collection's default
`keys` or a `key_cases` tuple. Each path has at most one mapping. Each mapping
requires at least one group; group keys are unique nonblank strings within that
mapping. A group requires at least two named environments, each contributing one
nonnull scalar value. `component_type:"string"` also requires every mapped value
to be a nonblank string. Within one path/environment, a typed value may belong
to only one group. Invalid or ambiguous policies fail compilation. Mapping and
group order have no meaning; effective policy digests cover sorted definitions.

After selecting the item's complete key tuple, each mapped component resolves
using the input's explicit Environment (including policy bindings). This applies
to source, actual, and optional baseline in deployment. A mapping for an inactive
conditional key component does not apply. When an active component has mappings,
a missing or unlisted environment produces a conflict; the tool does not infer
environments from tenant names, directory names, role, or observed values.

A listed value becomes a tagged logical group identity for alignment only.
Unlisted values use a separately tagged raw value and exact typed matching. There
is no coercion, case folding, chaining, or inferred transitivity. Group keys cannot
collide with raw values. An environment's declared value must match exactly;
a value mapped in a different environment is not implicitly accepted. Duplicate
resolved item tuples produce conflicts; mapping never merges duplicate entries.
Mappings are local to the selected collection and each parent object/array; they
do not rename root objects, affect references elsewhere, or establish value
comparison rules.

Original documents, values, paths, display context, and expectation derivations
are retained. The collection decision records key mapping/order alignment with
its rule ID, replacement and evidence; child item identities use hashes of the
resolved typed tuples. Explicit sensitive redaction and value omission retain
their existing behavior. Mapping changes are bound into policy/request digests.

Collection alignment supplies neither drift equivalence nor deployment authority.
Use a separate drift `equivalences` rule at the aligned value path to accept an
intentional difference. Deployment needs independent authoritative expectations;
`copy_source` still requires the raw source value and `preserve_target` still
requires the aligned baseline value. Profile replacement ownership, selector
conflict handling, and mode restrictions are unchanged.

### Collection mapping extensions

`collection_mappings` attaches environment mappings to an existing effective
keyed collection. Each entry has its own `id`, a `collection` rule ID, optional
exact object `key`, required `key_mappings`, and optional `modes`, `required`, and
`evidence`. It inherits type, variant, section, path, key structure, and display
labels. It does not replace the collection or claim value authority.

Extensions may restrict, but not expand, collection modes or object scope.
Unknown, non-keyed, disabled, or replaced targets are errors; reference the
effective replacement's ID if a structural replacement is also intentional.
Overlapping active extensions for the same collection/object scope are errors,
as are mapping paths overlapping the collection's own inline mappings. Use one
extension with multiple groups/components for a scope rather than competing
extensions. Independent object scopes or disjoint modes can have their own
extensions. Existing key mapping validation applies after extension composition.

Extension IDs have independent usage/required checks and evidence. Collection
and descendant decisions retain `extensions` with extension rule ID, collection
rule ID, and attribution. Ordinary collection evidence still identifies the
shipped rule pack. Definitions remain separate in the effective policy digest;
input documents and expected values are never rewritten.

### Explicitly disabling shipped rules

Root `disabled_rules` lists unique shipped/profile rule IDs to omit. It cannot
name unknown IDs, custom rules, or a rule also being replaced. Disabling
an ignore restores ordinary comparison and deployment coverage requirements;
it does not verify the field. Disabling an identity requires a usable replacement
strategy, otherwise matching remains incomplete. Disabled collection targets
cannot receive mapping extensions. File order never supplies precedence.
Reports retain `disabled_rules` and both the original profile digest and the
effective policy digest.

### Expectations

| Operation | Expected source |
| --- | --- |
| `literal` | Exactly one of `expected` or environment-indexed `values` |
| `absent` | Missing state, with no supplied value |
| `copy_source` | Source value/state at the aligned field, or concrete `from` path |
| `preserve_target` | Baseline value/state at the aligned field, or concrete `from` path |
| `reference_target` | Corresponding baseline object's physical ID for `target_type`; source reference at the aligned field or `from` |

Omitted `from` uses the aligned field. Explicit `"from":""` selects the section
root. In Go this uses `From *string`: nil versus a pointer to an empty string.
References to explicitly deleted targets are violations, even when a retained
draft field still contains the old baseline ID.

State is `{"state":"absent"}` or
`{"state":"present","value":null}` (or another typed JSON value). Missing,
null, empty string, empty object and empty array are different under strict
comparison. Numbers compare
by exact mathematical value; strings/numbers/booleans never coerce.

`authority` must be nonempty and `evidence.kind` present before an expectation can
verify anything. Missing authority/evidence, environment values, baseline or
reference correspondence yields `unverified`, never success. `copy_source` is
authoritative only because the policy explicitly declares that preservation
requirement. A missing baseline field can authoritatively require absence if the
baseline object is established. A missing baseline object cannot.

Only literal/absence assertions run in drift. They independently assert both
environments, so two equal-but-wrong values violate the policy. Derivation
operations require deployment; if enabled in drift they are unverified.

Reference expectations require independent destination evidence: the baseline ID
or an explicit literal assertion. The draft's own ID is never used as an expected
destination ID. New-object references without independent evidence remain
unverified. Use a literal for an independently governed expected new-object ID.

Hub promotion can add `source_equals`, `source_environment`, `source_tenant`, and
`target_tenant`. A source guard is a tagged state and must be deployment-only.
It preserves mapping applicability; a nonmatching source leaves the field for
ordinary coverage evaluation. Context mismatch is unverified. These guards do
not change the draft or grant authority.

### Conflicts and normalization

There is no first/last-match or specificity precedence. Profiles expand to named
rules; `replaces` explicitly replaces a profile rule of the same category and
ownership. Replacement history is retained. Ordinary custom-rule replacement
chains are unsupported: compose a new versioned profile instead.

Competing active value rules and ancestor/descendant ownership overlap fail
compilation. Rules with disjoint modes can own the same field. Ignores and
equivalence cannot overlap logical identity fields. Expectations may assert an
identity after matching but cannot change correspondence.

Collection rules establish alignment and may contain field rules. Nested keyed
collections are supported; child rules under keyed collections require wildcard
item selectors. Ordered collections also permit canonical literal item indices,
including missing indices selected by assertions. Array
order is significant unless explicitly normalized. Keyed keys must be present,
nonnull scalar values, unique per side; empty string keys are valid. Composite
keys retain types and exact numeric equality. Multisets preserve multiplicity and
cannot contain field/collection rules. An ignore of a child does not erase the
existence difference of its parent container or array item.

Deployment equivalence is inactive. A declared keyed collection allows order
normalization but does not verify its member values; those still need
expectations. Multisets alone cannot establish deployment expectations; a whole
array assertion can instead validate a required value. Exclusion rules cannot own
the same active field as an expectation or hide a failed substitution. The
`null_handling` comparison setting applies within expectations without claiming
field ownership or granting authority.

## Null and absent payload properties

`null_handling` accepts `strict` or `null_equals_absent`. Neutral policies default
to `strict`. All ISC profiles supply `null_equals_absent`; an explicit policy
setting overrides the profile default. The effective setting is part of the
compiled policy digest and appears as `Report.null_handling`. The profile's
default is covered by its digest too.

With equivalence enabled, a null-valued content property and an absent content
property compare as equivalent. It applies recursively to object properties in
whole-value expectations, source guards and multiset members, as well as ordinary
field comparisons. Drift decisions using it are `equivalent`; expectation checks
remain `verified` when satisfied. Applied normalization is recorded in the
decision's `normalization` field. Original locations, presence, values and expected
states remain unchanged in reports.

Envelope metadata remains strict. Empty strings, false, zero, empty containers,
array lengths/membership, and required nonnull identity/collection keys are not
normalized to absence. Null array elements remain elements. A null object is not
an empty object. Optional drift reference fields with null/absent values have no
reference to resolve; nonempty references still require correspondence. Deployment
still needs explicit expectations/authority, and unavailable derivations remain
unverified. Exclusions retain their existing precedence.

This default follows the null/absent-property convention in the
[SailPoint API guidelines](https://sailpoint-oss.github.io/sailpoint-api-guidelines/).

## ISC profiles

Profiles are assembled from validated embedded [JSON rule packs](rule-packs.md):
common defaults, SOURCE defaults, and connector defaults. Your custom policy
extends or explicitly replaces/disables those defaults.

`Selector.variant` optionally restricts a rule to adapter-classified document
metadata. `Document.variant` is independent of the display name and participates
in the detached document hash. Empty selector variants cover all variants;
different nonempty variants have disjoint ownership. Replacements retain variant
ownership along with the rest of their selector. Matched documents with different
variants produce a conflict; rules never normalize across incompatible connectors.
Unrecognized connectors remain unclassified and use ordinary comparison plus
common/SOURCE defaults. Unsupported arrays do not receive inferred semantics.


* `isc-identity-v2`: existing named, composite, parent-qualified and singleton
  identity strategies plus SOURCE schema/attribute, AD forest/domain, and provisioning
  alignment, and generated provisioning policy/field metadata exclusions. Other fields remain comparable.
* `isc-envelope-v2`: identities plus envelope JWS header/signature exclusions.
* `isc-portable-v2`: additionally excludes envelope `/self/id`, content `/id`,
  `/created`, `/modified`, `/owner/id`, SOURCE `/schemas/*/id`, `/schemas/*/created`,
  `/schemas/*/modified`, SOURCE `/connectorAttributes/cloudCacheUpdate`,
  `/connectorAttributes/since`, and
  lifecycle `/identityCount`.

All three contain explicit deployment structural expectations for envelope
`/version` and `/self/type`. Unknown envelope/payload fields remain comparable in
drift and unverified in deployment without authority. These profiles do not strip
arbitrary nested IDs or delta tokens and do not infer reference paths or array sorting.
The three SOURCE schema metadata exclusions apply to every schema array item in
both comparison modes and preserve exclusion provenance. They work with ordered
or keyed arrays. Attribute properties and schema additions/removals remain
comparable, except for the explicit reference ID exclusion below.
ISC profile identity components are nonempty strings, following ISC domain
constraints; generic neutral profiles may use typed scalar components.

Rule IDs are `isc.identity.TYPE` for identity, `isc.TYPE.FIELD` for exclusions
(`physical-id`, `jws-header`, `jws-signature`, `identity-count`, `id`, `created`,
`modified`), plus `isc.SOURCE.schema-id`, `isc.SOURCE.schema-created`, and
`isc.SOURCE.schema-modified` for schema metadata. Structural expectations use
`isc.TYPE.envelope.version` / `isc.TYPE.envelope.self.type`.
Unknown types with custom identities receive the same explicit metadata profile
expansion. Profile digest covers that expansion; policy digest covers effective
rules, replacements, profile selection and evidence.

## SOURCE cache-update metadata

`isc-portable-v2` supplies `isc.SOURCE.cloud-cache-update` from the
`isc.source-portable` pack, excluding only content
`/connectorAttributes/cloudCacheUpdate` for SOURCE objects in drift and
deployment. It applies independently of connector classification, including
unclassified sources. Identity/envelope profiles and other object types retain
ordinary comparison. Adjacent connector attributes are not excluded.

This is a portable comparison choice for operational cache-update metadata;
it does not assert a documented cache refresh trigger or interval. Set
`disabled_rules:["isc.SOURCE.cloud-cache-update"]` to restore normal comparison
and deployment coverage requirements. Exclusion decisions retain pack evidence
and the original pointer.

## SOURCE health-check timing metadata

`isc-portable-v2` supplies `isc.SOURCE.health-since` from `isc.source-portable`,
excluding only content `/connectorAttributes/since` in drift and deployment for
all SOURCE connectors, including unclassified sources. Identity/envelope profiles
and other object types retain ordinary comparison. Top-level `/since`, health
and status values, aggregation cursors, and unrelated nested fields are unchanged.

The field is treated as operational health/status timing metadata. This policy
choice does not assert a refresh trigger for the nested timestamp. Exclusion
provenance retains the pack and original pointer. Listing the rule ID in
`disabled_rules` restores normal comparison and deployment authority requirements.

## Common portable owner references

`isc.portable` declares `isc.$TYPE.owner-id` at content `/owner/id`, expanding
for all registered configuration types and custom identity types.
It applies in drift and deployment only under `isc-portable-v2`. Identity and
envelope profiles retain owner ID comparison. Owner name, type, and other fields
within the exported reference remain ordinary comparable values; no other nested IDs are
excluded by this rule. Reports attribute it to the common portable pack.
Disable the concrete expanded rule ID (for example `isc.ROLE.owner-id`) to restore
normal comparison/deployment coverage for that type.

## IDENTITY_OBJECT_CONFIG attribute matching

All ISC profiles match `/objectAttributeDetails` entries by their exact,
case-sensitive `/name`, independently of array order. Names must be nonblank
strings and unique within the array; missing, null, invalid, or duplicate names
produce a conflict rather than positional matching. Added, removed, or renamed
attributes remain differences.

The outer collection rule aligns attribute definitions. Nested `attributeSources`
use identity-profile precedence for multi-application entries, as described below.
All ISC profiles also exclude
these nested reference IDs through the same type pack:

| Path | Rule ID |
| --- | --- |
| `/objectAttributeDetails/*/attributeSources/*/ruleRef/id` | `isc.IDENTITY_OBJECT_CONFIG.attribute-source-rule-id` |
| `/objectAttributeDetails/*/attributeSources/*/applicationRef/id` | `isc.IDENTITY_OBJECT_CONFIG.attribute-source-application-id` |

Reference names, types, and other fields remain comparable. These exclusions apply
to every attribute and source entry in drift and deployment. List either rule ID
in `disabled_rules` to restore its ID comparison and deployment coverage
requirements. Matching and exclusions do not approve differing values or supply
deployment expectations. Reports show the identity attribute name alongside the
original input paths.

The shipped rule is `isc.IDENTITY_OBJECT_CONFIG.object-attribute-details`.
No local policy entry is needed. List its ID in `disabled_rules` to restore ordered comparison, or use an
explicit same-selector replacement for a structural exception.

## Relative priority and attribute-source precedence

All ISC profiles compare `IDENTITY_PROFILE` `/priority` by relative precedence,
using `isc.IDENTITY_PROFILE.priority-order`. Lower numbers precede higher numbers.
For corresponding profiles, priorities `5, 10, 40` and `10, 20, 30` are equivalent
when they produce the same order. A reversal is reported even if a profile's own
number did not change. Profiles present on only one side remain object-presence
findings; they do not shift the relative order of the shared profiles. An exact
`--name` filter still uses the other matching profiles as priority context.
Original priority values remain in the profile findings unless values are omitted.

For `IDENTITY_OBJECT_CONFIG`, `isc.IDENTITY_OBJECT_CONFIG.attribute-source-priority`
aligns multi-application `attributeSources` entries by `/applicationRef/name` and
compares the application order derived from identity-profile priorities. It resolves
each original `/applicationRef/id` against a profile's `/authoritativeSource/id`
in the same input, then reads `/priority`. Exported array positions and numeric
spacing do not determine equivalence. Other entry fields remain comparable, and
reports retain their original paths. A precedence reversal is one finding with a
named **Priority order** list for each side, rather than mismatched application
names at different array positions.

Both exports must include the relevant `IDENTITY_PROFILE` documents. They can be
used as context with `--scope IDENTITY_OBJECT_CONFIG`; they are not added to the
comparison scope. Context comes only from the supplied exports, without fetching
live tenant data. Missing, null, or nonnumeric required priorities, unresolved
references, duplicate reference matches, and tied priorities prevent a conclusive
result (exit 2). Source IDs remain available for lookup despite their exclusion
from value comparison. Collections with at most one item on every input, or with
no application references at all, retain ordinary positional comparison. Mixed
multi-entry collections with unavailable references remain incomplete.

Priority ordering compares shared entries; added or removed applications still
produce ordinary membership findings. Source names must be unique in each keyed
collection. Pure global-rule arrays retain their existing ordering semantics.

The scalar relative-order equivalence is drift-only. In deployment, priority values
still need explicit expectations; the contextual collection order needs an
authoritative whole-value expectation. Alignment never supplies deployment approval.
Use `disabled_rules` with either shipped rule ID to restore ordinary numeric or
positional comparison.

The neutral policy mechanisms can also be authored explicitly:

```json
{
  "equivalences": [
    {
      "id": "profile-priority-order",
      "type": "IDENTITY_PROFILE",
      "path": "/priority",
      "operation": "relative_order",
      "replaces": "isc.IDENTITY_PROFILE.priority-order"
    }
  ],
  "collections": [
    {
      "id": "attribute-source-precedence",
      "replaces": "isc.IDENTITY_OBJECT_CONFIG.attribute-source-priority",
      "type": "IDENTITY_OBJECT_CONFIG",
      "path": "/objectAttributeDetails/*/attributeSources",
      "mode": "keyed",
      "keys": ["/applicationRef/name"],
      "component_type": "string",
      "priority_order": {
        "type": "IDENTITY_PROFILE",
        "reference_path": "/applicationRef/id",
        "match_path": "/authoritativeSource/id",
        "priority_path": "/priority"
      }
    }
  ]
}
```

This is a rule fragment illustrating explicit replacements; the shipped defaults
need no local policy entries. `relative_order` takes a concrete content path and
uses the type's matched objects; it cannot also declare value pairs, groups,
environment restrictions, or item selectors. `priority_order` requires a keyed
collection with concrete reference, match, and priority paths and no key cases.
The context type need not be selected for comparison. Existing collection key
mappings can still align environment-specific application names; value equivalence
for those names remains a separate policy decision.

## ATTR_SYNC_SOURCE_CONFIG attribute matching

All ISC profiles compare `/attributes` as a keyed collection using each complex
object's `/target` string. Entry order has no meaning for this comparison.
Targets must be nonblank strings and unique within each configuration's array;
missing, null, structured, non-string, blank, or duplicate targets produce a
conflict. The same target can be used by different source configurations.

Matching by target aligns entries; it does not approve differences in their
values. `name`, `displayName`, `enabled`, and other fields remain comparable.
Added or removed targets still produce findings. Reports retain original input
indices and show Target/Name context. Deployment still requires explicit field
expectations. `/source/id` is excluded from value comparison by
`isc.ATTR_SYNC_SOURCE_CONFIG.source-id`; source names, types, and other fields
remain comparable. The original ID is still required to resolve the parent SOURCE
before comparison. Missing, invalid, or unresolved parent references remain
incomplete; changing the logical parent still produces differences. The exclusion
does not substitute name-based lookup.
Disable this rule to restore parent-reference ID comparison and deployment coverage.

The shipped collection is `isc.ATTR_SYNC_SOURCE_CONFIG.attributes`. It applies to all parent source
connectors. Use `disabled_rules` to restore ordinary ordered comparison, or an
explicit same-selector replacement for a structural exception.

Attribute-sync identity depends on SOURCE matching. Include both types with
`--scope SOURCE,ATTR_SYNC_SOURCE_CONFIG`; this also compares SOURCE
objects under that scope. Selecting only ATTR_SYNC_SOURCE_CONFIG still leaves
its parent matching context unavailable and returns an incomplete result.

## ACCESS_REQUEST_CONFIG approval scheme references

All ISC profiles exclude
`/entitlementRequestConfig/grantRequestApprovalSchemes/*/id` for
ACCESS_REQUEST_CONFIG in drift and deployment. The wildcard covers every scheme
array member, including `/entitlementRequestConfig/grantRequestApprovalSchemes/0/id`.
Names, types, and other scheme/configuration fields remain comparable. Missing
schemes and array ordering still produce findings. Other nested IDs, other
approval-scheme paths, and other configuration types retain normal comparison.

The rule ID is `isc.ACCESS_REQUEST_CONFIG.grant-approval-scheme-id`. No local mapping is needed.
List the rule ID in `disabled_rules` to restore ID comparison and deployment
coverage requirements.

## ROLE access profile and entitlement references

All ISC profiles exclude these ROLE reference IDs in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/accessProfiles/*/id` | `isc.ROLE.access-profile-id` |
| `/entitlements/*/id` | `isc.ROLE.entitlement-id` |
| `/entitlements/*/sourceId` | `isc.ROLE.entitlement-source-id` |

The wildcards apply to every array member, including `/accessProfiles/0/id`,
`/entitlements/0/id`, and `/entitlements/0/sourceId`. Names, types, entitlement
`sourceName`, and other fields remain comparable. Missing members and array
ordering still produce findings. Other nested IDs and ROLE `/source/id` retain
normal comparison; these rules do not apply to other configuration types.

Add an individual rule ID to `disabled_rules` to restore that ID's comparison and
deployment coverage requirements. No custom-policy mappings are needed.

## ACCESS_PROFILE entitlement and source references

All ISC profiles exclude these ACCESS_PROFILE reference IDs in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/entitlements/*/id` | `isc.ACCESS_PROFILE.entitlement-id` |
| `/source/id` | `isc.ACCESS_PROFILE.source-id` |

The entitlement wildcard applies to every array member, for example
`/entitlements/0/id` and `/entitlements/2/id`. Entitlement and source names, types,
and other fields remain comparable. Missing entitlements and source references
still produce findings; entitlement array ordering is unchanged. Other nested IDs
and identical paths on other configuration types retain normal comparison.

No local policy mapping is needed. Add either rule ID to `disabled_rules` to
restore its ID comparison and deployment coverage requirements.

## WORKFLOW creator and modifier references

All ISC profiles exclude these WORKFLOW reference IDs in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/creator/id` | `isc.WORKFLOW.creator-id` |
| `/modifiedBy/id` | `isc.WORKFLOW.modified-by-id` |

The exported field is `creator`, not `create`. Reference names, types, and other fields
remain comparable, as do identical paths on other configuration types. List either
rule ID in `disabled_rules` to restore its ID comparison and deployment coverage
requirements. No custom-policy mapping is needed.

## SOURCE correlation reference IDs

All ISC profiles supply `isc.SOURCE.correlation-config-details-id` at
`/correlationConfigDetails/id` and `isc.SOURCE.manager-correlation-rule-id` at
`/managerCorrelationRule/id`. Both are declared in the SOURCE pack, independent
of connector classification, and apply in drift and deployment. Only the direct
IDs are excluded; reference name/type and remaining correlation configuration
retain normal comparison and authority requirements. Other object types are
unchanged. Each rule can be explicitly disabled through `disabled_rules`, and
exclusion provenance retains the rule, pack, and original pointer.

## SOURCE before-provisioning rule references

All ISC profiles supply `isc.SOURCE.before-provisioning-rule-id`, excluding only
content `/beforeProvisioningRule/id` on SOURCE objects in drift and deployment.
The reference's `name`, `type`, and other fields retain ordinary comparison and
expectation requirements. Missing or changed names/types remain findings; the
exclusion does not establish equivalence or deployment authority for them.
Exclusion decisions retain the profile rule ID, evidence, and original pointer.
Other object types and unrelated nested IDs remain comparable.

## SOURCE schema rules

All ISC profiles declare `isc.SOURCE.schemas` at `/schemas` and
`isc.SOURCE.schema-attributes` at `/schemas/*/attributes`, both keyed by `/name`
with nonblank string components. Schema names are unique within a source for
comparison; attribute names are unique within a schema. Duplicates, missing,
null, blank or non-string names fail with conflicts. These are explicit differ
validation requirements, not assumptions about undocumented platform constraints.

`nativeObjectType` is not an identity component. Repeated native types are valid
for comparison, and changes to native types remain data differences. Per-role
`Location.context` identifies Schema (name/native type) and Attribute (name), with
original pointers and no inferred labels for absent items. Context applies to
expectation derivations and follows existing omission/redaction behavior.

Portable schema metadata exclusions are unchanged. All ISC profiles exclude
`/schemas/*/attributes/*/schema/id` with rule
`isc.SOURCE.schema-attribute-reference-id` in drift and deployment modes.
Reference `name` and `type`, schema/attribute membership, and other properties
remain comparable. No new exclusions or matching changes apply to other object types.
Deployment member values require explicit expectations beneath the keyed arrays;
parent collection rules do not grant authority.

Custom collection rules at these paths require explicit same-category,
same-selector `replaces`. Replacements supply their own display annotations.

## SOURCE Active Directory forest and domain rules

All ISC profiles include connector-scoped `isc.SOURCE.ad-forests` at
`/connectorAttributes/forestSettings`, keyed by the nonblank string `/forestName`.
Names must be unique within a source for comparison. This is an explicit differ
requirement, not a documented guarantee of ISC validation. Entries are unordered;
other settings remain comparable. The rule displays Forest / Name context using
each input's original forest name and pointer. Missing counterparts receive no
inferred forest labels.

`isc.SOURCE.ad-domains` matches `/connectorAttributes/domainSettings` by the
complete `[/forestName, /domainDN]` tuple, with nonblank string components.
Uniqueness is required for the tuple within each source, not either component
alone. This is a differ requirement rather than a documented ISC validation
guarantee. Domain entries are unordered; context shows Domain / Forest name /
Domain DN at each input's original location, including expectation derivations.
Invalid components and duplicate tuples produce conflicts. Each component can
use the existing generic `key_mappings`; both resolved components must match.
No DN normalization is implied. Server lists remain ordered, and credentials,
connection settings, unknown fields, and other values remain comparable.
Domain-to-forest referential integrity is not validated.

These rules use `variant:"active-directory"`. Only matching connector metadata enables them;
SOURCE type and array shape alone are insufficient. Server ordering is unchanged.
Environment mappings belong in `collection_mappings`, targeting the shipped rule
ID and optional source key. Forest and domain collections receive independent
extensions; definitions are not automatically reused. Separate drift equivalences
accept the mapped field values. Structural exceptions require full same-selector
replacements, including the connector variant.

Active Directory sources exclude `/connectorAttributes/recommendationStatus`
under all ISC profiles in drift and deployment. The shipped connector rule
`isc.SOURCE.ad-recommendation-status` applies to every recognized Active Directory
source, regardless of its name. It excludes the field even when missing, null,
or differently shaped on the other side. Other connector types and adjacent
attributes remain comparable. This is a shipped comparison choice; no local policy
entry is needed. Add the rule ID to `disabled_rules` to restore comparison and
normal deployment coverage requirements.

The Entra pack excludes the entire `/connectorAttributes/subscribedSkus`
subtree with `isc.SOURCE.entra-subscribed-skus`, scoped to
`variant:"microsoft-entra"`, in all built-in profiles and both modes. Nested
service plans are included in the exclusion; adjacent settings and other
connector variants remain comparable. The exclusion applies even when the
inventory is missing on one side or has a different shape.

Disable `isc.SOURCE.entra-subscribed-skus`
to compare inventory, and declare custom collection rules if keyed
SKU/service-plan matching is needed. Disabling restores ordinary comparison;
it does not infer deployment expectations.

See the [complete drift example](../examples/policy-drift.json),
[forest walkthrough](environment-mappings.md#forest-names-that-differ-between-environments),
and [domain walkthrough](environment-mappings.md#domain-settings-between-environments).

## SOURCE provisioning policy rules

All ISC profiles declare keyed collections for `/provisioningPolicies` and
`/provisioningPolicies/*/fields`. Default policy keys are `/usageType`; a
`CREATE_MACHINE_ACCOUNT` policy instead uses `/usageType` and `/subtypeId`.
Fields use `/name`. Components must be nonblank strings. Duplicate or missing
keys fail with a conflict; policy display names and policy/field physical IDs do
not determine alignment. Subtype IDs are used as exported; matching different
subtype IDs across tenants requires an explicitly configured logical key.

The built-in collection IDs are `isc.SOURCE.provisioning-policies` and
`isc.SOURCE.provisioning-fields`. Metadata exclusions are
`isc.SOURCE.provisioning-policy-{id,created,modified}` and
`isc.SOURCE.provisioning-field-{id,created,modified}` at their direct item paths.
These operate in both modes and all ISC profiles. Other nested IDs and transform
array ordering remain comparable. Keyed alignment does not grant deployment
member-value authority. Explicit replacements retain the existing same-category,
same-selector ownership requirements.

Generic keyed collection rules can define `key_cases`, each containing a concrete
item-relative `when_path`, a nonnull scalar `when_value`, and a nonempty list of
concrete `keys` pointers that replaces the whole default tuple for matching items.
Cases must share one discriminator path and have distinct typed values. Missing
or unmatched discriminators use default keys.

Collection annotations `item_label` and `display_fields:[{"label":...,"path":...}]`
capture readable identifiers from each real ancestor array item into
`Location.context`. Labels must be nonblank, display labels unique within a rule,
and paths concrete and item-relative. Missing/null/blank/structured display values
are skipped; annotations never change comparison validity or correspondence.
Context entries contain a label, original item pointer and labelled scalar
identifiers, ordered outermost to innermost. Values are resolved independently
for each role, including explicit derivation `from` pointers. Absent counterparts
are never assigned the other side's metadata.

SOURCE provisioning policies declare name/usage/subtype context and their fields
declare name context. HTML displays it beside each original JSON Pointer and
supports searching it; Markdown provides a per-role context table. JSON retains
structured context. Identifiers remain available with `--exclude-values`;
`sensitive` redaction removes affected location and derivation context. As with
object names and evidence, do not select secrets as display metadata.

`component_type` accepts `scalar` (the default) or `string` (nonblank strings only).
All key components remain present and nonnull regardless of null/absent-property
normalization.

Sources: [provisioning policy creation](https://developer.sailpoint.com/docs/api/create-provisioning-policy-v-2/)
and [subtype provisioning policies](https://developer.sailpoint.com/docs/api/create-source-subtype-provisioning-policy-v-2/).

## Requests, operations and CLI

Drift roles are `left` and `right`. Deployment roles are `source`, `actual`, and
optional `baseline`. Reading mode is independently `snapshot` or `tree`. An
explicit common type scope must be available in every supplied input. Default
scope requires equal selected type sets in the two primary inputs; selected types
with zero objects remain declared scope.

For a worked explanation of intent, action checks, and partial-draft handling,
see the [operations walkthrough](deployment.md#deployment-operations-file).

Deployment takes a strict operations JSON array:

```json
[{"type":"SOURCE","key":"Invented Directory","action":"update"}]
```

Actions are `create`, `update`, or `delete`. Keys are logical source keys, including
the source key for an explicit renamed pairing. Composite string keys use a JSON
tuple string. Ambiguous key presentation cannot pass. Every operation must be
available in type scope and unique. Unexpected draft objects are unverified.
Unselected source/baseline objects are outside the operation comparison, although
they may establish reference correspondence. Omission never implies deletion.

An explicit delete validates absence in the projected draft and requires a
baseline object; it does not attest that an uninspected proprietary deployment
format contains a remote delete instruction. A supplied baseline also establishes
create/update existence checks. Without a baseline, ordinary update content can
still be verified by independent expectations.

```sh
isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --policy examples/policy-drift.json --scope SOURCE --format json

isc-configdiffer --mode deployment \
  --source /data/dev --source-mode snapshot --source-env dev \
  --actual /data/draft --actual-mode tree --actual-types SOURCE --actual-env prod \
  --baseline /data/prod --baseline-mode snapshot --baseline-env prod \
  --operations examples/operations.json \
  --policy examples/policy-deployment.json --scope SOURCE --format json
```

Tree mode without a manifest needs the role's `--*-types`. Role-specific labels
use `--*-label`. `--matching` includes ordinary equal leaves; it does not change
counts, completeness, request digest, or policy decisions. The same policy schema is used in both comparison modes.

Only exporter-format projected drafts are supported. Native proprietary draft
files, full three-way destination-impact analysis, and change authorization are
not supported.

## Optional Hub adapter

`--hub` accepts an array file or a bundle directory containing `mappings.json`
and `provenance.json`. Bundle checks validate byte digest, count, schema, pinned
API version/endpoint, retrieval metadata and caller context. Retrieval context is
recorded evidence, not remote authenticity or a transaction with the exports.

Array files require `--hub-source-tenant` and `--hub-target-tenant`. Bundles supply
these from provenance; conflicting supplied context fails. The `default` source
group requires `--hub-default-source-tenant REAL-SOURCE`. Import also requires the
two resolved input environments (from policy bindings or CLI labels); tenant identity is checked against snapshot
provenance during execution. Manifest-free trees cannot invent verified tenant
context from their labels.

Enabled mappings support the existing documented JSONPath subset and string
values only. Unsupported active expressions or properties fail. Disabled records,
including unknown semantics, remain inert evidence with diagnostics. Mapping IDs,
original path, enabled status, timestamps and hashes remain attributable; values
are not emitted. Multiple cases at one path are grouped into one equivalence rule
and checked for collisions/chains. Neither IDs nor names are used to infer an
object pairing.

Deployment import requires both `--hub-deployment-authority APPROVAL` and
`--hub-deployment-evidence /policies/evidence.json`. Evidence is a strict object
with nonempty `kind`. Promotion creates guarded destination expectations; it does
not rewrite the draft. Successful retrieval
does not establish applicability or approval, and no undocumented global Hub
substitution parity is claimed.

## Reports and verification

V2 JSON includes input provenance/content hashes, effective type/operation scope,
request/policy/profile digests, matching decisions, replacement/disable history, collection extension provenance, rule
usage, adapter evidence, diagnostics, decision counts and object coverage.
Each input also has a measured `document_hash` covering detached documents and
declared types, independently of an upstream artifact `content_hash` supplied by
a neutral caller. Request digests bind both.
Decision statuses are `equal`, `equivalent`, `exception`, `excluded`, `verified`, `different`,
`violation`, `unverified`, and `conflict`. Locations retain each role's physical
pointer, presence and JSON kind. Keyed item identities are opaque hashes rather
than raw item-key values. Derived expectations retain the independent origin's
role/artifact/pointer. Observed values are captured by default in optional tagged
`Location.value` states (`present` with a JSON value, including `null`, `absent`,
`unavailable`, or `redacted`). Decisions from expectation checks additionally
carry `expected` and `expected_role` when the expected value can be determined.
Derivation locations can also carry their observed value.

Set `Request.ExcludeValues` or CLI `--exclude-values` to omit all observed,
expected, and derivation values. `values_excluded` records this presentation
choice. Values are absent from the report when excluded, rather than hidden only
by a renderer. This option does not alter counts, decisions, matching, policy
usage, exit status or request digest. Rendering uses captured, redacted report
values and does not reopen configuration artifacts.

Human reports group repetitive, unruled missing-container decisions and their
missing descendants into one finding. HTML and Markdown retain all child paths,
values, and provenance in expandable `Nested decisions` sections. Parent values
come from the captured report; value exclusion and sensitive-rule redaction are
preserved. Explicit rule checks stay separate. Grouping requires consistent
physical ancestry, object identity, section, status, and presence direction across
the input roles. Reordered keyed items use each side's own pointers; redacted
combined identities stay ungrouped. The renderer indexes missing-container
ancestors rather than comparing every pair of decisions.

A changed, unruled string discriminator at `.../transform/type` produces a
`Transform implementation differs` finding. Its captured type values and original
discriminator pointers form the summary; ordinary `different` decisions below
that transform's `attributes` paths become its flat child details. This display
convention uses no vendor, SOURCE, connector, policy-name, or field-name checks.
Each role must establish physical ancestry within the same object and section;
reordered keyed items keep their own indices. Outer changed implementations own
the related differences of nested transforms. Ambiguous ownership stays ungrouped.

Explicit rule checks, equivalent/normalized results, and unverified deployment
checks stay independent. Metadata such as a transform's name or description also
stays outside the attribute group. The convention applies to nested objects named
`transform`; other objects with a `type` property retain existing presentation.
Values excluded from the JSON report are not reread to establish or display type
changes: the captured `value mismatch`, presence, kind, and pointers identify them.
This is presentation grouping only, with no new comparison or equivalence rule.

`Displayed findings` and object status badges count grouped entries; object rows
also show their underlying displayed decision count. `Decision counts` retain the
engine's full counts, including omitted equal leaves. HTML filters count findings
and their represented decisions, and a search matching a child path opens that
subtree's details. Markdown uses `<details>` blocks for compatible viewers.
Grouping leaves JSON, policy/request digests, rule usage, completeness, correctness,
and exit codes unchanged.

Counts are evaluated decision units, not object counts or percentage leaf
coverage. A whole-subtree expectation/exclusion counts once. Object coverage
separately records scoped inputs, compared groups, selected operations, objects
outside operation scope, and groups with unverified/conflict decisions. Matching
and policy decisions remain available when ordinary equal leaves are omitted.
Required unused rules prevent completeness. Unused optional rules are visible
without blocking it. Sensitive selectors conservatively hide all identity and
physical provenance and values for their object type, dependent types and types
referencing them through active declared reference/reference-target rules,
including operation keys, expected values and derivation origins. This can redact
more than the selected object. Labels and evidence remain visible metadata.

Exit 0: complete and correct. Exit 1: complete with known differences/violations.
Exit 2: invalid input/policy/request, incomplete/unverified comparison, or output
failure. `correct` is false whenever incomplete. No unverified result can pass.
Inputs, policies, profiles, and reports are detached; compiled plans can be reused
concurrently. Policy order and permitted collection reordering do not change
comparison semantics. Omitting `--mode` selects drift and still produces report schema `2`.

## Human report sections and missing-schema summaries

HTML and Markdown divide findings by captured content paths: `connectorAttributes`
(Connection settings), `schemas` (Schemas), `provisioningPolicies` (Provisioning
policies), other content (Other settings), and the metadata envelope (Export
metadata). Sections retain separate grouped-finding and underlying-decision counts.
The HTML Section filter composes with status and text search. A decision whose
roles do not agree on a section remains in Other settings.

A missing concrete `/schemas/<index>` object is a single named finding with
expandable descendants and schema values. Only physical ancestry consistent across
all roles establishes ownership. Redacted, malformed, or ambiguous locations are
not used to infer a schema relationship. Missing attributes inside a shared schema
remain separate from whole-schema absence. Informational equal/equivalent/excluded
outcomes can be nested with their original provenance; authoritative assertions,
violations, conflicts, and ruled unverified checks remain independent.

HTML filters also match nested statuses and text. A matching summary can therefore
contain details with other statuses; counts and outcomes are never recomputed from
filters. Container values and long text values are collapsed
in both human formats. This is presentation only: JSON decisions, locations,
values, policy usage, and completeness/correctness remain unchanged. Value omission
and sensitive-data redaction apply before rendering.
