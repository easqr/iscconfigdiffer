# Contracts, version 1

## Input schema and inventory

The SDK-free exporter `snapshot` package owns `Artifact`, `Changes`, `Manifest`, and `Snapshot`. Exporter root types remain compatible aliases. Existing `Organize`, `FileStore`, `CurrentPath`, library consumers, and exporter CLI remain available. Exporter JSON/path helpers delegate to the shared package.

Manifest schema 1 has `schema:1`, nonempty portable `tenant`, nonempty `job_id`, nonzero RFC3339 `completed`, distinct portable selected `types`, nonnegative per-type `counts`, `artifacts` (`path`, lowercase SHA-256), and publication `changes`. Unknown manifest metadata is represented in the manifest hash. Required contract property spellings are exact. Schema 1 has **no success-status field**; completion metadata does not prove remote job success or cryptographic authenticity.

Envelope version 1 requires `version:1`, `self.type`, `self.id`, and an object-valued `object` payload. If payload `id` exists it must equal `self.id`. Self ID and type determine the canonical path `TYPE/id-<lowercase hex UTF-8 ID>.json`. Type components use the exporter portable component contract; IDs are nonempty, at most 100 bytes, and exclude separators, controls, `.` and `..`. Case-colliding paths, duplicate JSON keys at any depth, invalid UTF-8, trailing JSON, unsupported versions, conflicting identities, and malformed envelopes fail validation. Unknown envelope/payload fields remain intact and comparable. Numbers are decoded as `json.Number`.

Snapshot mode reads only manifest-listed files. Every listed path, byte checksum, envelope, selected type, count, and identity is validated. Unmanaged files are ignored. A snapshot integrity failure never changes the mode automatically.

Tree mode inventories current canonical exporter-format `.json` artifacts, incorporating edits, additions, and deletions. Manifest types are the selected scope; old counts and checksums are historical provenance, not assertions about current files. A type can therefore contain zero current objects after deletion. A manifest-free tree must explicitly declare selected types, even for an empty inventory; scope is never inferred from object presence. A supplied manifest must still be structurally valid schema 1 metadata. Type overrides alongside a manifest are rejected. To expand manifest scope, intentionally edit its types and historical count metadata, or explicitly create a manifest-free tree.

Root `.git`, `.hg`, and `.svn` administration entries are excluded from tree traversal. Ordinary non-JSON files are unmanaged and ignored. Other `.json` files must be canonical configuration artifacts; put policies, saved mappings, and unrelated JSON outside the configuration root. Nested symlinks and special files encountered in the inventory, including administration entry links, are rejected. Snapshot mode ignores unlisted entries by design. In-memory `FromFiles` applies the same inventory and schema rules to bytes; files have no filesystem link semantics.

Disk roots are resolved once and opened with `os.OpenRoot`; a replacement tenant symlink cannot redirect later files. Root containment is not authenticity. Byte/metadata rereads and a tree inventory recheck detect concurrent changes; an intentionally mutable tree must be quiescent while read. These checks are not an adversarial filesystem transaction. Immutable exporter generations provide the strongest coherence. Payloads, source values, and filesystem exception text are not included in validation diagnostics.

## Scope and identities

Default selected type sets must be equal, regardless of tenant or environment equality. Explicit comparison scope must be declared by both sides. Reports record effective scope and excluded types; unavailable export scope never means an empty inventory.

Matching uses type and an exact, case-sensitive logical key, with IDs and paths retained as physical provenance. Automatic named types are: ACCESS_PROFILE, CAMPAIGN_FILTER, CONNECTOR_RULE, FORM_DEFINITION, GOVERNANCE_GROUP, IDENTITY_OBJECT_CONFIG, IDENTITY_PROFILE, PASSWORD_POLICY, PASSWORD_SYNC_GROUP, PUBLIC_IDENTITIES_CONFIG, ROLE, RULE, SEGMENT, SERVICE_DESK_INTEGRATION, SOD_POLICY, SOURCE, SPS_PARAMETER, TAG, TRANSFORM, TRIGGER_SUBSCRIPTION, WORKFLOW. Their key is payload `/object/name`.

Special identities:

| Type | Logical key |
| --- | --- |
| LIFECYCLE_STATE | Corresponding IDENTITY_PROFILE logical key via `/object/identityProfileRef/id`, plus `/object/technicalName` |
| ATTR_SYNC_SOURCE_CONFIG | Corresponding SOURCE logical key via `/object/source/id` |
| NOTIFICATION_TEMPLATE | `/object/key`, `/object/locale`, `/object/medium` |
| ACCESS_REQUEST_CONFIG, AUTH_ORG | Explicit built-in singleton; at most one per side |

Composite keys are JSON string tuples. Both parents must have established correspondence; unresolved dependencies make the comparison incomplete. Explicit physical ID pairings are reserved before automatic matching, including parent pairings, and recorded by rule ID. Pairings do not erase name/value drift. Every object still needs a valid logical identity. Unknown types and missing default keys require an explicit `identities` strategy; no fuzzy matching or ID fallback exists. Duplicate logical keys, conflicting pairings, and unavailable pairing endpoints make results invalid/incomplete. Missing objects appear once and are never counted again as differing objects.

## Policy schema

The JSON policy has exact, case-sensitive properties, unknown-property rejection at every rule level, duplicate-key rejection, `version:1`, and a stable `id`. Rule IDs are unique across all categories and use 1–200 ASCII letters/digits plus `_ . : -` (first character alphanumeric). Optional root properties:

| Property | Rule fields |
| --- | --- |
| `profile` | `""`, `isc-envelope-v1`, or `isc-portable-v1` |
| `bindings` | Optional `left` and `right` environment strings; conflict with input environment is invalid |
| `identities` | `id`, `type`, `strategy` (`fields` or `singleton`), `paths` for fields |
| `pairings` | `id`, `type`, `left_id`, `right_id` |
| `ignores` | `id`, selector, optional `required` |
| `expectations` | `id`, selector, `values` object keyed by environment, optional `required` |
| `references` | `id`, selector, `target_type`, optional one-to-one `external` left-ID-to-right-ID map and `required` |
| `collections` | `id`, selector, nonempty `keys` array of concrete item-relative pointers, optional `required` |
| `sensitive` | `id`, selector |
| `hub` | Imported rules described below |

A selector contains `type`, `path`, and optional exact object logical `key`. All policy paths are envelope-relative JSON Pointer: payload paths start `/object`. Empty pointer selects the envelope root. Escape `~` as `~0` and `/` as `~1`. A segment exactly `*` matches one map member or array index. There is no recursive wildcard, filter, substring, or regex syntax; a literal member named `*` is not addressable in selectors. Report difference paths are concrete JSON Pointers, with no wildcards. Identity and collection key paths must be concrete. Identity components must be nonempty strings. Arrays use canonical decimal indices.

Conflicting comparison rules, ancestor/descendant ignore/mapping overlaps, duplicate IDs/strategies/pair endpoints, malformed selectors, ambiguous Hub conversions, and rules overlapping active profile exclusions are rejected. Nested scalar rules under one keyed collection are allowed; overlapping collection selectors are rejected. Use wildcard item selectors under keyed collections. Sensitive rules affect output, not matching or comparison. Rule arrays are canonicalized by ID for evaluation and policy digest, so reordering rules cannot change reports.

Environment expectations assert a typed whole value on **each side**, before ordinary equality. Applicability depends on object type, optional key, and selector structure, independently of current values. A literal path is applicable even if missing on both sides. Wildcards expand from the union of present containers. Missing environment expectations are diagnostics/incomplete. Production/production and sandbox/sandbox assert their respective same value on both sides. Reversing sides uses each input's explicit environment binding. Incorrectly equal values still drift. Missing, null, and empty are distinct; a null expectation requires a present null. An empty wildcard selection is unused; required unused selectors prevent completeness. Optional unused rules are visible diagnostics without blocking completeness. Unmatched objects are reported as missing rather than evaluated as synthetic payloads.

Profiles are visible and versioned:

* Empty profile compares all envelope fields.
* `isc-envelope-v1` (default) excludes only envelope `/jwsHeader` and `/jwsSignature`.
* `isc-portable-v1` additionally excludes `/self/id`, payload root `/object/id`, `/object/created`, `/object/modified`, and LIFECYCLE_STATE `/object/identityCount`. This is an explicit own-identity and metadata exclusion profile, not a global ID stripper.

Delta tokens require an explicit ignore at their actual payload path. Unknown fields and nested IDs remain comparable. Identity fields cannot be ignored before matching; ignore selectors overlapping built-in or configured logical identity paths are rejected. Profiles run after identity matching and never establish correspondence.

References resolve only at declared paths, using established object correspondence for `target_type`. Different IDs for corresponding targets match; a wrong known target drifts. Missing targets or missing target correspondence are visible diagnostics and make the comparison incomplete. Out-of-snapshot owners/entitlements require explicit one-to-one external mappings, explicit ignores, or ordinary comparison with no resolver enabled. External mappings never override a known target correspondence.

Maps are unordered. Numbers compare by exact mathematical rational equality (`1`, `1.0`, `1e0` equal; adjacent large integers remain distinct). Absent/null/type/value changes are separate difference kinds. Arrays are ordered unless selected by a keyed collection rule. Keyed collections require present non-null scalar keys unique on each side; composite keys preserve scalar types and precise numeric equality. Duplicate/missing keys make results incomplete. Items are compared recursively in sorted key order. Detailed pointers use the left item's physical index when present, otherwise its right index; `missing_left`/`missing_right` identifies that side. Workflow and transform arrays stay ordered unless explicitly configured.

## Offline Configuration Hub import

`ImportHub` accepts a saved JSON array of exact Object Mapping response fields: `objectMappingId`, `objectType`, `jsonPath`, string `sourceValue`, string `targetValue`, boolean `enabled`, optional `created` and `modified`. Caller supplies source tenant, target tenant, and provenance; the CLI uses the saved file's SHA-256 as provenance. IDs, original JSONPath, status, tenant context, timestamps, and provenance remain in imported policy rules and its digest. Raw mappings and substitution values are not emitted in reports.

Supported JSONPath subset: `$`, `.member`, `['member']`, `["member"]`, `[0]`, and `[*]`, composed in sequence. Dot members use ASCII letters/underscore followed by letters/digits/underscore/hyphen. Double-quoted names use JSON escaping; single-quoted names with backslash/quote escapes are rejected. Translation prepends `/object` because Hub paths are payload-relative. Recursive descent, dot wildcard, filters, unions, slices, scripts, and literal wildcard member names are unsupported. Unsupported **enabled** mappings fail import. Disabled unsupported mappings are retained but never applied. Active policy rules must retain their original JSONPath and its exact supported translation.

Hub values remain strings; numeric/boolean coercion is never inferred. Substitution applies only when the selected source string and selected target string exactly equal the enabled pair under matching source/target tenant context. It does not mutate input or establish object identity. Multiple substitutions at an identical type/path must have disjoint source/target value sets; chains, collisions, and overlapping wildcard paths are rejected. Inapplicable rules are visible as unused. These are substitutions, **not** independent environment assertions: unrelated equal values continue through ordinary comparison. Use strict `expectations` for environment requirements; overlapping substitutions/assertions are invalid.

SailPoint describes payload-relative mapping and enabled status in [Mapping Objects](https://documentation.sailpoint.com/saas/help/confighub/config_hub_mapping.html). The pinned SDK's `api_v2026.ObjectMappingResponse` was inspected for the offline schema. No access to built-in substitution rules or parity with proprietary Configuration Hub comparison is assumed.

## Report schema

Version 1 JSON is deterministic, sorted by type/key/status, with sorted differences and diagnostics. It includes input modes, labels, environment and tenant/job provenance, manifest and current content SHA-256 hashes, scope/exclusions, policy ID/digest, profile ID, four reconciled counts, entries, diagnostics, and `complete`. No volatile report-generation timestamp is included. Content hash covers sorted selected types plus current path/byte-checksum inventory. Manifest hash covers original manifest bytes; a tree can have unchanged historical manifest hash and changed content hash.

Statuses: `missing_left`, `missing_right`, `different`, `matching`. Entries carry type/logical key, physical IDs/paths, optional pairing rule ID, and concrete difference paths/kinds/rule IDs/failed sides. Matching counts remain when matching entries are omitted. Diagnostic messages contain codes and safe structural context, never payload values. All actual/expected payload values are omitted in both JSON and text, a deliberately conservative redaction default. A sensitive selector overlapping identity paths additionally hides the logical key and physical provenance, including derived lifecycle/source-config keys and physical paths in diagnostics. Logical names and physical provenance are otherwise intentionally visible; input labels/tenant metadata are provenance, not payload-sensitive selectors.

`Complete` does not mean no drift. Ambiguous identities, unresolved enabled references, unavailable expectation environments, and unused required selectors make it false and force exit 2. Invalid input/policy returns an error and an incomplete report. No partial result should be treated as authoritative remediation input.
