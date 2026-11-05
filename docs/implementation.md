# Implementation and acceptance record

The module has one comparison implementation: the neutral `v2` engine, with ISC
snapshot and optional Configuration Hub adapters in the root `differ` package.
ISC domain rules are assembled under `internal/iscprofile`, with separate SOURCE
schema and provisioning-policy providers. The neutral engine supports generic
conditional collection keys; it contains no SOURCE-specific matching logic.
Policies and reports use schema `2`. Exporter manifests/envelopes retain their
independent storage-format numbers.

| Slice | Implemented behavior | Verification |
| --- | --- | --- |
| 1 Policy compiler | Strict JSON, typed states, profile expansion/replacement, conflicts, deterministic digests | `v2/compile_test.go`, `v2/review_test.go` |
| 2 ISC adapters | Validated detached snapshot/tree inputs, ISC profiles, string identity components | `adapter_test.go`, `compare_test.go`, `review_test.go` |
| 3 Matching and arrays | Logical identities, explicit pairings, dependencies, nested keyed arrays, ordered/multiset semantics | Neutral engine and root adapter tests |
| 4 Drift | Typed equivalence and assertions, explicit exclusions, references, retained decisions | `v2/run_test.go`, `v2/regression_test.go` |
| 5 Deployment | Independent expectations, projected draft validation, explicit operations and baseline evidence | Neutral and root deployment tests |
| 6 Hub adapter | Offline arrays/bundles, supported JSONPath, ambiguity/context/integrity checks, explicit promotion | `hub_test.go`, `adapter_test.go`, CLI integration tests |
| 7 CLI/reporting | One parser, drift default, explicit deployment, report schema `2`, redaction and provenance | `cmd/isc-configdiffer/main_test.go`, review regression tests |

Synthetic tests cover exporter organization/publication to validated readers and
comparison, plus mapping retrieval/publication to offline import. No tenant
exports, credentials or network services are fixtures. Current examples are
compiled and executed in `TestDocumentedExamples`.

## Checks

Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`
in both sibling modules. Check formatting with `gofmt` and whitespace with
`git diff --check`. See [the review record](review.md) for correctness findings.

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
* The sibling-module replacement is a development setup; publication needs real
  dependency versions and independently verified external builds.
