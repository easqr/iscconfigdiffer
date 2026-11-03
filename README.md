# ISC configuration differ

Policy v2 adds explicit **drift** and **deployment** comparison modes, a neutral
policy compiler, independent deployment expectations, aligned nested collections,
decision provenance, and optional offline Hub adapters. See the
[v2 contract, CLI and migration guide](docs/policy-v2.md) and
[seven-slice acceptance record](docs/v2-implementation.md). Existing v1 API and
CLI invocations without `--mode` retain their behavior.

Offline Go library and thin CLI for comparing two exporter-format inputs. No authentication, live tenant calls, mutation, deployment, remediation, or Git automation. Inputs are validated before comparison; ambiguous or unresolved comparisons cannot return a clean exit status.

## Local development

Go 1.27.2 is required, matching the exporter. Keep `configdiffer` and `configexporter` as siblings:

```sh
go test ./...
go vet ./...
go test -race ./...
go build ./...
go build -o /tmp/isc-configdiffer ./cmd/isc-configdiffer
```

The module uses `replace github.com/elianderson/iscutilities/configexporter => ../configexporter` for local development. No Go workspace file is required. **Publication requires publishing/tagging the exporter with its new `snapshot` package, replacing `v0.0.0` with that version, and removing the local replace.** The exporter currently has no configured remote; these changes are local and have not been published.

## CLI

Each side must explicitly choose `snapshot` or `tree`. Any two readable directories work independently. Root symlinks are a convenience; a shared parent or tenant-named directory is unnecessary.

```sh
isc-configdiffer --left /data/agreed --left-mode tree \
  --right /data/fresh --right-mode snapshot \
  --left-label agreed --right-label fresh \
  --left-env production --right-env production \
  --policy examples/policy.json --format json
```

A tree without a manifest requires `--left-types ROLE,SOURCE` or `--right-types ROLE,SOURCE`. With a manifest, its selected types define tree scope, including types with zero current objects. An explicit `--scope ROLE,SOURCE` must exist on both sides. Otherwise selected type sets must match exactly. `--matching` includes matching entries without changing counts or exit status.

Exit codes: **0** complete/no drift; **1** complete/drift; **2** invalid input/policy or incomplete comparison. Parse and read failures go to standard error; completed comparisons, including incomplete diagnostic reports, go to standard output. Report write failures exit 2.

Saved Configuration Hub mappings can be added using `--hub mappings.json --hub-source-tenant invented-prod --hub-target-tenant invented-sb`. Tenant context must agree with manifest provenance. A manifest-free tree cannot supply verified Hub tenant context. Mappings remain directional; reversing sides requires an explicitly reversed mapping set.

## Library

```go
left, err := snapshot.Read(leftDir, snapshot.Options{
    Mode: snapshot.TreeMode, Label: "agreed", Environment: "production",
})
// Handle err, then read right with its own Options.
report, err := differ.Compare(left, right, differ.DefaultPolicy(), differ.Options{})
// Handle err before using the report. report.ExitCode() is 2 on validation failure.
```

Import `github.com/elianderson/iscutilities/configexporter/snapshot` and `github.com/elianderson/iscutilities/configdiffer` (package name `differ`). For memory inputs, call `snapshot.FromFiles(map[string][]byte, options)` with exporter-format bytes and optional `manifest.json`. This shares the disk reader's validation contract. Arbitrarily constructing or mutating an `Input` is rejected by `Compare`; inputs are detached and never normalized in place. Labels and environments can be assigned independently of tenant provenance.

Read [contracts](docs/contracts.md) for matching, selectors, exact policy fields, metadata profiles, references, keyed arrays, numeric semantics, and Hub compatibility. [Implementation and validation](docs/implementation.md) records acceptance work, limitations, and the separate optional retrieval slice.
