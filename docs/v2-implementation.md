# Policy v2 implementation record

Follow-up correctness and compatibility review: [findings and fixes](v2-review.md).

Scope: the seven approved slices, local/offline only. Existing uncommitted
`docs/implementation.md`, `hub_test.go`, and exporter mapping retrieval changes
are preserved. No changes to real exports or credentials are required.

Baseline (2026-11-02): `go test ./...` passes in both sibling modules.

| Slice | Status | Evidence |
| --- | --- | --- |
| 1 Compiler | Complete | Strict schema/property/state checks, deterministic semantic digests, explicit replacement and conflict tests in `v2/compile_test.go` |
| 2 Compatibility/ISC | Complete | Snapshot validation and detached metadata/content; v1 migration/exit parity, profile replacement, legacy CLI-label routing tests |
| 3 Matching/arrays | Complete | Generic dependency graph, reserved pairings, exact typed identities, nested keyed physical pointers, multiset multiplicity, container-presence regressions |
| 4 Drift | Complete | Approved pairs/groups, typed assertions, equally-wrong-value failures, reference correspondence, immutable inputs, retained normalization evidence |
| 5 Deployment | Complete | Five expectation operations, authoritative independent derivation, source guards, explicit create/update/delete scope, partial-draft and missing-baseline tests |
| 6 Hub adapter | Complete | Synthetic exporter retrieval/publication to v2 import, checksums/context/default binding, enabled/disabled/unsupported semantics, guarded explicit deployment promotion |
| 7 Public interface | Complete | V2 CLI/report, migration, redaction, request/profile/policy digests, coverage, executable invented examples, final verification |

## Implementation choices

* The new `v2` package owns neutral policy compilation and execution; no SDK or
  exporter imports belong there. Root-package adapters reuse validated snapshots.
* V1 public API and CLI remain operational. An explicit translator supports
  migration; legacy execution is retained for compatibility, rather than silently
  changing historical report/selector semantics.
* V2 policy selectors address `content` (default) or `metadata`. ISC metadata is
  the export envelope without `object`; unknown metadata remains comparable.
* Drift roles are left/right. Deployment roles are source/actual/baseline.
* Profiles supply identifiable rules, not hidden execution branches. Explicit
  replacement is restricted to the same rule category and ownership scope.
* Deployment operations are explicitly scoped by type/logical key: create,
  update, or delete. An omitted draft object is not an implied deletion.
* Unasserted deployment content is unverified, including equal source/draft values.
  An authoritative whole-subtree expectation can cover that subtree. Coverage
  counts evaluated decisions, not estimated leaf counts of excluded subtrees.

## Conflict matrix

| Pair | Rule |
| --- | --- |
| Value/value (expectation, equivalence, ignore, reference) | Reject overlapping ownership, except disjoint exact equivalence cases |
| Collection/field | Allow descendants addressed through wildcard items |
| Collection/collection | Allow strict nesting; reject equal or wildcard-overlapping ownership |
| Identity/ignore or equivalence | Reject overlap; matching cannot be suppressed or rewritten |
| Identity/expectation | Allow assertion after matching |
| Sensitive/any | Allow; affects presentation only |
| Profile/custom | Explicit same-category replacement required for competing ownership |

Runtime ambiguity produces an incomplete report. Invalid policy/request returns
an error and cannot pass. Known value violations are complete failures; missing
authority/evidence is unverified and incomplete. No full destination-impact or
change-authorization analysis is included.

## Final acceptance

Completed locally on 2026-11-02. In each sibling module the following passed:

* `go test ./...`
* `go test -race ./...`
* `go vet ./...`
* `go build ./...`
* Formatting inventory (`gofmt -l`) and `git diff --check` (no findings).

Final differ race runs included root adapter tests, the legacy/v2 CLI tests, and
neutral engine/compiler tests. The concurrency test reuses one compiled plan
across goroutines. The profile/report mutation regression verifies detached
caller-owned profile maps/slices and returned evidence. Invalid-request and
zero-value compiled-plan tests verify that errors never leave a passing report.

Both checked-in v2 policy examples are loaded, compiled and executed against
invented inputs by `TestV2DocumentedExamples`; operations JSON is strictly decoded.
Hub tests use the synthetic exporter API and real local bundle publisher, never
network services. No real export contents were added to tests/examples/reports.
No credentials or live services were used. No commits, pushes or publication were
performed. No exporter files were changed by this implementation; its existing
uncommitted retrieval work was reused and its full checks passed.

## Boundaries retained

* Native proprietary draft formats are not guessed. Only validated exporter-format
  projected drafts are supported. Explicit delete checks projected absence, not
  an uninspected remote action encoding.
* `reference_target` requires the corresponding baseline object's physical ID.
  The actual draft cannot establish its own expected reference ID. Without a
  baseline or an independently authored literal expectation, it is unverified.
* Hub multi-case groups work for drift. Automatic deployment promotion rejects
  multiple conditional destination cases; author an explicit v2 policy instead.
* Migrated policies declare root `modes:["drift"]`; using them for deployment is
  rejected. Existing v1 execution remains available for explicitly documented
  nontranslatable envelope-root/metadata-identity cases.
* V2 coverage counts decision units and object groups separately. It does not
  claim percentage leaf coverage for whole-subtree assertions/exclusions.
* Local sibling-module replacement remains the existing development setup;
  publishing/version pinning is outside scope.

Usage, schema, profile rule IDs, migration details and examples are in
[the v2 guide](policy-v2.md).
