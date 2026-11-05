# Correctness review record

The current engine's regression tests protect these behavior boundaries:

| Area | Correct behavior | Evidence |
| --- | --- | --- |
| Policy compilation | Normalize content-space aliases, reject ambiguous active ownership, honor root mode restrictions and profile rules | `v2/compile_test.go`, `v2/review_test.go` |
| Object matching | Count identity use for explicit pairings, preserve typed key distinctions, require explicit baseline pairing when ambiguous | Neutral matching and root profile tests |
| Arrays | Evaluate literal absent indices and baseline-only preservation wildcards; keep concrete per-input pointers after keyed alignment | `v2/review_test.go`, `v2/run_test.go` |
| References | Inventory correspondence takes precedence over external maps; references to explicitly deleted targets fail | `compare_test.go`, neutral reference tests |
| Expectations | Distinguish omitted `from` from an explicit root pointer; never derive expected destination IDs from the draft | Neutral deployment and root adapter tests |
| Provenance | Measure document hashes independently, detach evidence even on error paths, reject malformed mapping bundles | Review and adapter tests |
| Privacy | Mask missing operation keys when sensitive and count sensitive selectors only when data exists | `v2/review_test.go` |
| CLI | Use a single policy/parser path, resolve environment bindings consistently, reject unsupported schemas and unknown flags | CLI tests |

All automated inputs are invented. Tests never copy tenant examples or use live
services. Source and draft values are immutable throughout comparison.

No complete success is returned for unverified expectations, ambiguous identities,
collection conflicts, or unused required rules. Known violations may be complete
failures; completeness and correctness are distinct.

## Intended limits

Exporter-format projected drafts are the supported input boundary. Destination
existence cannot be established without a baseline. Delete validation and
baseline-derived references need that evidence. Full impact analysis, change
authorization, remediation, and proprietary draft formats remain outside scope.

Run the checks in [implementation.md](implementation.md) after code changes.
