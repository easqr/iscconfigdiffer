# Export input contract

Comparison policies and reports use schema `2`; their contract is documented in
[policy.md](policy.md). This document describes the independent exporter storage
format, whose manifest `schema` and envelope `version` fields are both `1`.
These storage-format versions are independent of the policy and report schema.

## Schema and inventory

The SDK-free exporter `snapshot` package owns `Artifact`, `Changes`, `Manifest`, and `Snapshot`, along with the shared JSON decoding, checksums and path validation. Exporter APIs use these types directly. `exporter.BuildSnapshot` converts a complete download into an in-memory snapshot; `FileStore.Publish` saves it. For exporter filenames, manifest fields and Go reader modes, see the exporter's [Snapshot files and readers](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/docs/SNAPSHOTS.md) reference. This document describes the additional requirements for comparison inputs.

Manifest schema 1 has `schema:1`, nonempty portable `tenant`, nonempty `job_id`, nonzero RFC3339 `completed`, distinct portable selected `types`, nonnegative per-type `counts`, `artifacts` (`path`, lowercase SHA-256), and publication `changes`. Unknown manifest metadata is represented in the manifest hash. Required contract property spellings are exact. Schema 1 has **no success-status field**; completion metadata does not prove remote job success or cryptographic authenticity.

The exporter envelope requires `version:1`, `self.type`, `self.id`, and an object-valued `object` payload. If payload `id` exists it must equal `self.id`. Self ID and type determine the canonical path `TYPE/id-<lowercase hex UTF-8 ID>.json`. Type components use the exporter portable component contract; IDs are nonempty, at most 100 bytes, and exclude separators, controls, `.` and `..`. Case-colliding paths, duplicate JSON keys at any depth, invalid UTF-8, trailing JSON, unsupported versions, conflicting identities, and malformed envelopes fail validation. Unknown envelope/payload fields remain intact and comparable. Numbers are decoded as `json.Number`.

Snapshot mode reads only manifest-listed files. Every listed path, byte checksum, envelope, selected type, count, and identity is validated. Unmanaged files are ignored. A snapshot integrity failure never changes the mode automatically.

Tree mode inventories current canonical exporter-format `.json` artifacts, incorporating edits, additions, and deletions. Manifest types are the selected scope; old counts and checksums are historical provenance, not assertions about current files. A type can therefore contain zero current objects after deletion. A manifest-free tree must explicitly declare selected types, even for an empty inventory; scope is never inferred from object presence. A supplied manifest must still be structurally valid schema 1 metadata. Type overrides alongside a manifest are rejected. To expand manifest scope, intentionally edit its types and historical count metadata, or explicitly create a manifest-free tree.

Root `.git`, `.hg`, and `.svn` administration entries are excluded from tree traversal. Ordinary non-JSON files are unmanaged and ignored. Other `.json` files must be canonical configuration artifacts; put policies, saved mappings, and unrelated JSON outside the configuration root. Nested symlinks and special files encountered in the inventory, including administration entry links, are rejected. Snapshot mode ignores unlisted entries by design. In-memory `FromFiles` applies the same inventory and schema rules to bytes; files have no filesystem link semantics.

Disk roots are resolved once and opened with `os.OpenRoot`; a replacement tenant symlink cannot redirect later files. Root containment is not authenticity. Byte/metadata rereads and a tree inventory recheck detect concurrent changes; an intentionally mutable tree must be quiescent while read. These checks are not an adversarial filesystem transaction. Immutable exporter generations provide the strongest coherence. Payloads, source values, and filesystem exception text are not included in validation diagnostics.

## Scope and identities

Default primary selected type sets must be equal. An explicit comparison scope
must be available in every input, including an optional deployment baseline.
Missing scope is never interpreted as an empty inventory.

ISC profiles match named objects by the exact, case-sensitive payload `/name`.
Supported named types are ACCESS_PROFILE, CAMPAIGN_FILTER, CONNECTOR_RULE,
FORM_DEFINITION, GOVERNANCE_GROUP, IDENTITY_OBJECT_CONFIG, IDENTITY_PROFILE,
PASSWORD_POLICY, PASSWORD_SYNC_GROUP, PUBLIC_IDENTITIES_CONFIG, ROLE, RULE,
SEGMENT, SERVICE_DESK_INTEGRATION, SOD_POLICY, SOURCE, SPS_PARAMETER, TAG,
TRANSFORM, TRIGGER_SUBSCRIPTION, and WORKFLOW.

| Type | Logical key |
| --- | --- |
| LIFECYCLE_STATE | Parent IDENTITY_PROFILE logical key via `/identityProfileRef/id`, plus `/technicalName` |
| ATTR_SYNC_SOURCE_CONFIG | Parent SOURCE logical key via `/source/id` |
| NOTIFICATION_TEMPLATE | `/key`, `/locale`, `/medium` |
| ACCESS_REQUEST_CONFIG, AUTH_ORG | Singleton; at most one per input |

These paths are payload-relative policy pointers. Composite string keys are JSON
string tuples. Named/profile components must be nonempty strings. Unknown types
need explicit identity rules. Physical IDs remain provenance. Explicit pairings
reserve endpoints before automatic matching; renaming a paired object still
produces a value difference unless an expectation independently permits it.

Drift requires corresponding parents for dependent identities. Deployment may
identify baseline-only or source-only parents for explicitly scoped deletion.
Duplicate logical keys, unavailable dependencies, and ambiguous pairings cannot
produce a complete successful report. See [the policy contract](policy.md) for
collection alignment, references, missing states, conflicts, and provenance.
