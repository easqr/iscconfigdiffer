# Comparison policy v2

V2 is an offline, vendor-neutral compiler and comparison engine in `v2/`. The root
package adapts validated ISC snapshots and Configuration Hub artifacts. Production
comparison code imports neither exporter authentication nor a generated SDK.

## Library entry points

* `v2.Decode`, `v2.Compile`, `v2.Run`: neutral policy/documents.
* `differ.AdaptSnapshot`: validates and detaches `snapshot.Input`, preserving
  provenance and unknown fields. Payload becomes `content`; envelope fields other
  than `object` become `metadata`.
* `differ.Compile` and `differ.Compare`: ISC profiles and validated storage.
* `differ.ImportHub` / `ReadHub`: offline array/bundle adapters.

The CLI and library use one comparison engine and policy/report schema `2`.
The CLI defaults to drift mode; select `--mode deployment` for draft validation.
Policies use payload-relative selectors. `DecodePolicy`, `Compile`, and `Compare`
are the public ISC entry points; the `v2` package provides the neutral core.

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

## Schema

The root has `version:2`, `id`, optional `modes`, `profile`, `bindings`,
`null_handling`, and rule arrays.
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

Selectors contain `type`, `path`, optional exact logical `key`, and optional
`space` (`content`, the default, or `metadata`). Paths are JSON Pointers relative
to the selected section. `""` selects the section root. Escape `~` as `~0` and
`/` as `~1`. A segment exactly `*` selects one object member or array item; no
recursive wildcard, regex, filter or expression language exists. A literal member
named `*` cannot be addressed by a selector. Literal missing paths are evaluated;
wildcards expand only from available structures.

| Array | Additional fields |
| --- | --- |
| `identities` | `type`, `strategy`: `fields`, `singleton`, or `dependent`; `paths`; optional `component_type`: `scalar` (default) or `string`; for dependent identity, `parent_type`, `parent_path` |
| `pairings` | `type`, `left_id`, `right_id`, optional `baseline_id` |
| `ignores` | Selector; explicit subtree exclusion/normalization |
| `expectations` | Selector; `operation`, `authority`; operation-specific fields below |
| `equivalences` | Selector; `left_env`, `right_env`; `pairs` or `groups`; optional `left_tenant`, `right_tenant` |
| `references` | Selector; `target_type`, optional one-to-one `external` ID map; drift-only |
| `collections` | Selector; `mode`: `ordered`, `keyed`, or `multiset`; `keys`, optional `component_type`, and `key_cases` only for keyed; optional `item_label` and `display_fields` for report context |
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

`pairs` is an array of typed `{left,right}` values, applicable in the declared
direction and environment pair. `groups` is an array of environment-to-value
objects; each group explicitly establishes symmetric equivalence among its
environments. Do not combine pairs/groups. Cases must be unambiguous. There is no
substring replacement, substitution chaining, inferred transitivity or coercion.
Several disjoint cases belong in one rule; overlapping rule ownership is rejected.

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

* `isc-identity-v2`: existing named, composite, parent-qualified and singleton
  identity strategies plus SOURCE provisioning alignment and generated
  provisioning policy/field metadata exclusions. Other fields remain comparable.
* `isc-envelope-v2`: identities plus envelope JWS header/signature exclusions.
* `isc-portable-v2`: additionally excludes envelope `/self/id`, content `/id`,
  `/created`, `/modified`, SOURCE `/schemas/*/id`, `/schemas/*/created`,
  `/schemas/*/modified`, and lifecycle `/identityCount`.

All three contain explicit deployment structural expectations for envelope
`/version` and `/self/type`. Unknown envelope/payload fields remain comparable in
drift and unverified in deployment without authority. These profiles do not strip
other nested IDs or delta tokens and do not infer reference paths or array sorting.
The three SOURCE schema metadata exclusions apply to every schema array item in
both comparison modes and preserve exclusion provenance. They work with ordered
or keyed arrays and leave schema attributes, nested reference IDs, and schema
additions/removals comparable.
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
 `component_type` accepts `scalar`
(the default) or `string` (nonblank strings only). All key components remain
present and nonnull regardless of null/absent-property normalization.

The neutral engine interprets these declarations without SOURCE-specific code.
ISC profile assembly lives in `internal/iscprofile`; SOURCE schema and
provisioning providers are separate files. Existing public `ISCProfile` and
`Compile` entry points remain unchanged.

Sources: [provisioning policy creation](https://developer.sailpoint.com/docs/api/create-provisioning-policy-v-2/)
and [subtype provisioning policies](https://developer.sailpoint.com/docs/api/create-source-subtype-provisioning-policy-v-2/).

## Requests, operations and CLI

Drift roles are `left` and `right`. Deployment roles are `source`, `actual`, and
optional `baseline`. Reading mode is independently `snapshot` or `tree`. An
explicit common type scope must be available in every supplied input. Default
scope requires equal selected type sets in the two primary inputs; selected types
with zero objects remain declared scope.

For a worked explanation of intent, action checks, and partial-draft handling,
see the [operations walkthrough](../README.md#deployment-operations-file).

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
files need a separately inspected adapter; no format or substitution semantics
are guessed. Full three-way destination-impact and change-authorization analysis
are outside this implementation.

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
not rewrite the draft. A multi-case mapping group is rejected for automatic
deployment promotion because choosing among conditional destinations needs an
explicit authored policy. It remains supported for drift. Successful retrieval
does not establish applicability or approval, and no undocumented global Hub
substitution parity is claimed.

## Reports and verification

V2 JSON includes input provenance/content hashes, effective type/operation scope,
request/policy/profile digests, matching decisions, replacement history, rule
usage, adapter evidence, diagnostics, decision counts and object coverage.
Each input also has a measured `document_hash` covering detached documents and
declared types, independently of an upstream artifact `content_hash` supplied by
a neutral caller. Request digests bind both.
Decision statuses are `equal`, `equivalent`, `excluded`, `verified`, `different`,
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

Automated verification uses invented data only. Both module test suites, race
tests, vet, build and formatting checks are required before marking the seven
slices complete. See [implementation evidence](implementation.md).
