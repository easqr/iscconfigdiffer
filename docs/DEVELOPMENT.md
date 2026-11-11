# Contributor guide

For installation, comparisons, and policy authoring, start with the
[README](../README.md) and [documentation index](README.md).

## Local checks

Use Go 1.27.3 or newer. From the configdiffer module directory, run:

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./...
git diff --check
```

Format changed Go files with `gofmt` before running the checks.
Automated fixtures use invented data and temporary directories, without tenant
credentials or live services. Documented examples are exercised by the test suite.

Builds use `github.com/easqr/iscconfigexporter` v0.8.0 through `go.mod` and
`go.sum`. An exporter checkout is not required. When changing shared input
contracts, also run the exporter's checks in its own checkout and verify
compatibility with the published dependency.

## Local files

The repository ignores `policy.json`, `policy-rules/`, `reports/`, and `bin/`.
Use these locations for local policies, generated reports, and compiled commands.
Keep reusable examples under `examples/`, using invented names and values.

## Local module overrides

Consumers normally use `github.com/easqr/iscconfigdiffer` v0.7.0. To test changes
from a checkout before publishing, add a replacement in the consuming application's
`go.mod`:

```gomod
replace github.com/easqr/iscconfigdiffer => /absolute/path/to/iscconfigdiffer
```

Run `go mod tidy` after adding imports. This override is for local testing;
release consumers do not need it. The exporter stays pinned to its published
v0.8.0 dependency.

## Releases

Use matching source and module tags such as `v0.7.0`. Before tagging, run the
checks above, verify the module path and pinned dependency, and review the
staged files. Commit the release source and documentation, then create an
annotated tag at that commit. Push the branch and tag together. Verify the
published CLI with `go install github.com/easqr/iscconfigdiffer/cmd/isc-configdiffer@v0.7.0`
and check `isc-configdiffer --help` returns success.

## Contributor references

- [Rule-pack authoring](development/rule-packs.md): embedded format, validation, and changing defaults.
- [Implementation and verification record](development/implementation.md): architecture, test coverage, historical checks, and compatibility notes.
- [Correctness review record](development/review.md): previous corrections and regression coverage.
