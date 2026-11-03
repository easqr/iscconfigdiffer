# Implementation and acceptance record

Development completed locally on 2026-11-02. Implementation and validation involved no pushes, live exports, tenant mutation, or remote service. Pre-existing exporter `.vscode/` and `config-dev.json` were not read or edited.

| Slice | Implemented | Acceptance evidence |
| --- | --- | --- |
| 1. Shared contracts | SDK-free exporter `snapshot` models/parser/path helpers/reader; compatible root aliases and wrappers | Existing exporter external consumer, exporter CLI tests, all standalone builds |
| 2. Input readers | Explicit snapshot/tree modes; arbitrary independent roots; pinned root handle; memory byte input; current inventory hashes and historical provenance | Synthetic scope/zero types, checksum/count, JSON/UTF-8, schema/version/path, own-ID conflict, nested-link, root replacement, tree deletion/addition, unmanaged/admin files tests |
| 3. Matching | Exact named and composite keys, dependency correspondence, singleton cardinality, reserved explicit pairings | Different IDs, repeated lifecycle names under two parents, parent/source correspondence, renames, duplicate/unknown identities, unresolved dependencies, missing object counted once |
| 4. Structure | Recursive comparison, unknown fields, absent/null/type/value, exact numbers, ordered arrays, scoped ignores | Synthetic structural, large integer/decimal equivalence, nested-ID preservation and array-order tests |
| 5. Directional expectations | Typed per-environment values; independent applicability; explicit bindings; conflict and required-unused checks | Full 4-environment-pair × 6-value × 6-value matrix, including incorrectly equal values, reversed sides, same environments, missing/null/empty; null expectation and strict policy shape tests |
| 6. References/collections | Declared references via correspondence, explicit external mappings, keyed arrays with unique scalar/composite keys | Equivalent/wrong/unresolved targets, explicit external targets, reordered keyed items, duplicate collection keys, ordinary arrays remain ordered |
| 7. Reports/CLI/Hub | Versioned deterministic JSON/text, reconciled counts, output omission/redaction, all exit codes, offline mapping import | Matching presentation independence, repeatability, rule-order independence, sensitive identity provenance, CLI 0/1/2, supported/rejected Hub paths, enabled/disabled/context/ambiguity/string-type tests |
| 8. Documentation/manual checks | Reader/policy/report contracts, CLI/library usage, synthetic policy example, release caveat | Synthetic exporter Organize → FileStore → snapshot/tree readers → differ end-to-end test; read-only manual validation of the two reference snapshots |

Final verification commands for **each module**: formatting check, `go test ./...`, `go vet ./...`, `go test -race ./...`, and `go build ./...`. Standalone binaries built to `/private/tmp/isc-configdiffer` and `/private/tmp/isc-exporter`. The differ imports only the SDK-free shared package in production; its synthetic compatibility test imports exporter root. No Go workspace is required.

Manual development validation read the authorized reference snapshots only. Both validated against schema/envelope version 1, their selected scope, checksum and count inventories. Self-comparison was complete without drift; cross-comparison was complete with drift and counts reconciling to both inventories. These observations are not automated assertions. No tenant payloads, tenant-derived policies, or tenant-specific report artifacts were copied into either repository. Every automated fixture is invented and uses temporary directories or memory.

## Decisions and limits

* Reports omit all raw payload/expected values in both formats; sensitive identity selectors also hide logical/physical identity. This is intentionally more conservative than selective value printing. Names and IDs remain visible as provenance unless a corresponding sensitive identity selector masks them.
* Literal selectors can assert missing values; wildcard selectors require concrete structural expansion. Optional unused rules remain visible; required unused selectors make the report incomplete. Objects missing entirely are reported once, rather than treated as empty payloads.
* Exact matching is intentionally conservative. New object types or missing named keys require explicit identity policy. Explicit pairing does not waive logical-key validation or ignore name drift. Identity dependencies require corresponding parents available in the effective scope.
* Edited trees must be quiescent while read. Directory handles pin roots, and rereads/inventory checks detect changes, but the reader does not claim a cryptographic or adversarial filesystem transaction. Use immutable exporter generations for strong coherence.
* Keyed-array pointers use the left physical item index where available, otherwise the right physical index. Use wildcard policy paths for per-item rules below keyed collections. Collection selectors cannot overlap each other.
* Configuration Hub import supports a documented JSONPath subset and string values only. Unsupported active expressions fail; unsupported disabled mappings remain inert with provenance. No proprietary comparison parity or access to built-in substitutions is assumed.
* The relative exporter module replacement is a **local development dependency**, not a publishable release configuration. Publish/tag the exporter shared package first, pin a real version, remove the replace, and verify independent checkout builds before release.

## Subsequent slice: optional mapping retrieval (implemented)

Initially deferred from the differ release, then implemented in configexporter under the user's separate retrieval request. The pinned Go SDK v2.7.106 `api_v2026.ConfigurationHubAPIService.GetObjectMappings(ctx, sourceOrg).Execute()` performs the optional read. An explicit `-retrieve-mappings` action reuses native SDK configuration/authentication, the existing V2026 client, adapter gate and GET retries. The SDK retains per-version token caches.

Original mapping bytes and checksum-bound source/target, API origin/endpoint, retrieval ID/time, count and optional operator-supplied export-job correlation are saved together in a new separate bundle directory. Ordinary exports and their permissions/payloads are unchanged. Retrieval does not run an export, backup action, cleanup or tenant mutation. Synthetic exporter tests cover the SDK/CLI/storage path, and `TestRetrievedMappingBundleImportsOffline` verifies retrieval → publication → strict offline import, preserving both enabled and disabled status. No live retrieval was performed. See [exporter mapping retrieval](../../configexporter/docs/MAPPINGS.md) for usage, permissions, provenance and limitations.
