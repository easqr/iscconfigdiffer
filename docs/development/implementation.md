# Implementation and verification record

Contributor reference for architecture, test coverage, and historical verification.
For usage, see the [README](../../README.md).

The module has one comparison implementation: the neutral `v2` engine, with ISC
snapshot and optional Configuration Hub adapters in the root `differ` package.
ISC domain rules are shipped as embedded JSON under `internal/iscprofile/packs`.
The loader assembles common, SOURCE, and connector layers with version/digest
provenance. The ISC adapter classifies SOURCE connectors from pack-declared
metadata; the neutral engine uses generic document variants. It contains no ISC
connector names or SOURCE-specific matching logic. Implementation policies attach
source-scoped collection mapping extensions, add expectations/equivalences, or
explicitly replace/disable defaults. They do not need to duplicate shipped keys
or labels. See [the rule-pack format](rule-packs.md).
Policies and reports use schema `2`. Exporter manifests/envelopes retain their
independent storage-format numbers.

| Area | Implemented behavior | Verification |
| --- | --- | --- |
| Policy compiler | Strict JSON, typed states, profile expansion/replacement, conflicts, deterministic digests | `v2/compile_test.go`, `v2/review_test.go` |
| ISC adapters | Validated detached snapshot/tree inputs, ISC profiles, string identity components | `adapter_test.go`, `compare_test.go`, `review_test.go` |
| Matching and arrays | Logical identities, explicit pairings, dependencies, nested keyed arrays, ordered/multiset semantics | Neutral engine and root adapter tests |
| Drift | Typed equivalence and assertions, explicit exclusions, references, retained decisions | `v2/run_test.go`, `v2/regression_test.go` |
| Deployment | Independent expectations, projected draft validation, explicit operations and baseline evidence | Neutral and root deployment tests |
| Hub adapter | Offline arrays/bundles, supported JSONPath, ambiguity/context/integrity checks, explicit promotion | `hub_test.go`, `adapter_test.go`, CLI integration tests |
| CLI/reporting | One parser, drift default, explicit deployment, report schema `2`, redaction and provenance | `cmd/isc-configdiffer/main_test.go`, review regression tests |

Synthetic tests cover exporter organization/publication to validated readers and
comparison, plus mapping retrieval/publication to offline import. No tenant
exports, credentials or network services are fixtures. Current examples are
compiled and executed in `TestDocumentedExamples`,
`TestDocumentedForestMappingAndReportContext`, and
`TestDocumentedDomainMappingAndContext`. `rule_packs_test.go` and
`v2/collection_extensions_test.go` cover connector isolation, ambiguous identities,
override/disable rules, detached definitions, scoped extensions, and provenance. Domain tests also cover composite-key
uniqueness, per-input context, deployment authority, and server ordering.
Collection mapping validation,
typed fallback, original pointers, baseline alignment, deterministic digests, and
compiled-plan detachment are covered in `v2/collection_mappings_test.go`.
`v2/item_selectors_test.go` and `source_ad_domain_users_test.go` cover complete
logical item scopes, per-domain approvals, reordered inputs, cross-domain
rejection, scope conflicts, coverage, redaction, and concurrent compiled reuse.
`v2/items_selectors_test.go` and `provisioning_nested_mapping_test.go` verify
conjunctive ancestor scopes, independent policy/field ordering, non-leakage to
other policies or fields, condition-order digests, and nested report provenance.

## Policy file composition

`policy_files.go` implements `differ.LoadPolicy` in the root adapter; the CLI uses
it for `--policy`. The neutral `v2.Decode`, `Compile`, and `Run` remain independent
of the filesystem. A root manifest can contain shared settings and rules; its
explicit, contained includes accept only rule arrays, disabled IDs, and attribution.
File digests and rule ownership travel through existing evidence records; human
renderers display the recorded index without reopening files.

`policy_files_test.go` and `TestCLIIncludedPolicy` cover composition equivalence,
strict shapes, path containment and aliases, cross-file ID/scope conflicts,
authority isolation, relocatable provenance, report origins, and the modular
SOURCE/Entra/AD/TRANSFORM examples. `policy_files_review_test.go` adds normalized
path/provenance consistency, root-symlink compatibility, filesystem error causes,
and exact semantic error ownership across rule categories and files. The neutral
`v2.RuleError` carries IDs only; file attribution stays in the adapter. See the
[review findings](review.md#root-policy-includes-review).
Tenant exports are development checks only.

## Missing-subtree presentation

`v2/render_subtrees.go` groups redundant missing-subtree decisions only in the
human view. `v2/render_subtrees_test.go` covers nested and empty containers,
JSON Pointer escaping, keyed/ordered arrays, reordered physical indices,
explicit checks, role/section/status boundaries, deployment baselines, value
omission, redacted combined objects, rendering, and structured-report immutability.
`v2/render_implementations.go` first groups changed `transform/type` discriminators
and their related attribute differences. `v2/render_implementations_test.go` covers
ordinary drift, nested implementations, explicit checks, approved type mappings,
same-type changes, ordering, conservative boundaries, value omission, redaction,
and JSON immutability. `source_provisioning_implementation_test.go` verifies
separate ENABLE/DISABLE findings with independently reordered policies and fields
and preserved per-input context. All fixtures are invented.
The comparison engine and JSON report schema are unchanged.

## Checks

Run the [contributor checks](../DEVELOPMENT.md#local-checks). See the
[review record](review.md) for correctness findings.

The single-engine consolidation was verified on 2026-11-04: tests, race tests,
vet and builds passed in both modules. The README quick start, default drift
report schema, JSON examples, documentation links and library example were also
checked. All example policies use schema `2`.

## Boundaries

* Inputs and policies are detached; compiled plans support concurrent reuse.
* Only exporter-format projected drafts are supported. Native proprietary draft
  formats require a separately inspected adapter.
* Baseline-derived references and explicit delete validation require baseline
  evidence; the actual draft never supplies its own expected destination ID.
* Hub multi-case groups work for drift and require authored deployment
  expectations instead of automatic promotion.
* Counts distinguish decision units from object groups and do not claim leaf
  coverage percentages for whole-subtree assertions or exclusions.
* No deployment, remediation, destination-impact analysis or authorization gate
  is implemented. SDK/authentication stay in the exporter.
* Builds download `github.com/easqr/iscconfigexporter` v0.8.0 without a local
  replacement. Standalone versioned installation is unavailable while configdiffer
  remains unpublished.

## Entra collection compatibility

The subscribed-SKU exclusion replaced the former `isc.SOURCE.entra-skus` and
`isc.SOURCE.entra-service-plans` collections. Those IDs are unavailable for
extensions or replacements. Policies using them must remove those references.
To compare this inventory, disable `isc.SOURCE.entra-subscribed-skus` and declare
custom collection rules if keyed SKU/service-plan matching is required.
