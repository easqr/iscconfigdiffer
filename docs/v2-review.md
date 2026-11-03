# Seven-slice correctness and compatibility review

Review date: 2026-11-02. Scope: the agreed contract, neutral engine, ISC/v1
adapters, offline Hub adapter, and CLI/reporting. All reproductions use invented
data. Unrelated working-tree changes were preserved; no exporter code changes,
live calls, deployment, commits or publication were needed.

## Findings and fixes

| Slice | Finding before correction | Correction and regression evidence |
| --- | --- | --- |
| 1 Compiler | Ordered collections rejected literal item selectors; omitted `space` rejected otherwise equivalent replacement ownership | Normalize selector aliases; permit canonical positional selectors for ordered arrays while retaining wildcard requirements for keyed arrays. `TestReviewOrderedCollectionAllowsLiteralItemSelector`, `TestReviewEquivalentReplacementSelectorSpace` |
| 1 Compiler | Profiles silently discarded root bindings/mode restrictions; conflicts included modes excluded by the policy | Reject unsupported profile root semantics, retain profile evidence, normalize profile modes, and intersect conflict checks with permitted root modes. `TestReviewProfileCannotSilentlyIgnoreBindings`, `TestReviewRootModesLimitConflicts` |
| 2 ISC/compatibility | Typed identities relaxed ISC/v1 string-only identity requirements | ISC profiles and translated v1 identities explicitly require string components; neutral identities retain typed scalar support. `TestReviewISCIdentityTypesPreserveV1Constraints` |
| 2 ISC/compatibility | V1 external mappings could override established reference correspondence, including an unresolved actual ID | Known correspondence takes precedence; unresolved actual references retain exit 2. `TestReviewLegacyExternalCannotOverrideKnownCorrespondence` and existing reference tests |
| 2 ISC/compatibility | V1 positional assertions under keyed arrays could suppress a different item after reordering | Reject positional rules beneath keyed collections according to the documented wildcard requirement. `TestReviewLegacyPositionalAssertionUnderKeyedArrayRejected` |
| 3 Matching/arrays | Source/draft renames could leave the baseline unmatched; dependent deletions required an absent draft parent | Consider both explicitly paired logical identities for baseline matching, require `baseline_id` on ambiguity, and support baseline/source-only parents in deployment. Drift retains its correspondence requirement. `TestReviewBaselineRenamePairing`, `TestReviewDependentDeleteWithoutDraftParent` |
| 3 Matching/arrays | Pairings left required identity rules marked unused; delimiter keys were ambiguous for unrestricted neutral identifiers | Count actual identity applications and encode internal physical keys as string tuples. `TestReviewRequiredIdentityIsUsedForExplicitPair` |
| 4 Drift | A literal assertion beyond an array's current extent could be skipped and return a clean result | Evaluate sparse addressed indices without traversing all intermediate indices; malformed indices cannot silently pass. `TestReviewLiteralPathsBeyondArrayInventory` |
| 5 Deployment | Preservation wildcards skipped baseline-only arrays, returning a clean result | Expand baseline-only containers and assert values against missing actual fields; incompatible baseline shapes do not invalidate unrelated source-copy rules. `TestReviewBaselineOnlyWildcardPreservation`, `TestReviewBaselineShapeDoesNotInvalidateIndependentCopy` |
| 5 Deployment | References to explicitly deleted targets could verify against the old baseline ID | Reject such reference expectations. `TestReviewCannotReferenceExplicitlyDeletedTarget` |
| 5 Deployment | Explicit empty `from` selected the aligned field instead of the section root | Distinguish omitted pointers from explicit empty pointers. `TestReviewExplicitRootDerivation` |
| 6 Hub adapter | Empty bundles could omit the required count or carry a zero retrieval time | Require all sidecar contract fields and a nonzero timestamp. `TestReviewHubSidecarRequiredCountAndRetrievalTime` |
| 6/7 Hub CLI | Import ignored policy environment bindings when CLI labels were absent | Use the same resolved bindings as execution; preserve conflict rejection. Embedded legacy mappings can use CLI environments during drift migration. `TestReviewHubUsesPolicyEnvironmentBindings` |
| 7 Provenance | Request digests trusted stale upstream hashes; error reports shared mutable evidence with compiled plans | Always measure `document_hash` and detach evidence on early error paths. `TestReviewRequestDigestMeasuresDocuments`, `TestReviewErrorReportCannotMutateCompiledEvidence` |
| 7 Privacy/reporting | Missing operations disclosed sensitive logical keys; nonexistent required sensitive selectors counted as used | Redact keys without inventory objects; count selector use only where data exists. Correct document location kinds and collection usage counts. `TestReviewSensitiveMissingOperationKey`, `TestReviewRequiredSensitiveSelectorMustExist` |

Highest priority: false clean results from skipped array/preservation assertions,
references to deleted targets, and v1 external-reference overrides. Privacy and
provenance gaps were corrected too. Fixed behaviors have regression coverage;
existing compatibility and synthetic integration tests remain in acceptance.

## Compatibility notes

* V1 public signatures, report schema, default CLI routing and valid supported
  policies remain available. Formerly accepted unsafe external overrides and
  positional rules beneath keyed arrays now fail instead of masking differences.
  Use wildcard item selectors beneath keyed arrays.
* V2 `Expectation.From` is an optional `*string` in Go: nil means the aligned field,
  a pointer to `""` means the section root, and a nonempty pointer means a concrete
  path. V1 signatures are unchanged. JSON explicit empty `from` now follows JSON
  Pointer semantics. Go callers assigning a source path must use a string pointer.
* V2 reports add measured `document_hash` alongside upstream `content_hash`.
  Digests and usage counts may change to reflect normalized selectors, actual
  documents and corrected rule application accounting.
* If source and draft names match different baseline objects, specify
  `baseline_id`; no destination is guessed.

## Retained boundaries

Only validated exporter-format projected drafts are supported. No proprietary
draft adapter, live access, remediation, full destination-impact analysis or
authorization gate was added. Without a baseline, create/update content can be
checked against independent expectations but destination existence is not
established. Deletes and baseline-derived reference expectations require baseline
evidence. Multi-case Hub groups work for drift and require authored expectations
for deployment promotion.

## Verification

All 21 new review regression tests pass, together with the existing suites.
Final checks passed in both `configdiffer` and `configexporter`:

* `go test ./...`
* `go test -race ./...`
* `go vet ./...`
* `go build ./...`
* `gofmt -l` and `git diff --check` (no findings).

No unresolved in-scope findings remain from this review. The retained boundaries
above are intentional scope limits, not claims of support for uninspected formats
or destination-impact/authorization analysis.
