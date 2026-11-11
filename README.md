# ISC Configuration Differ

Compare SailPoint Identity Security Cloud (ISC) configuration exports and produce
a report of differences. Configdiffer matches corresponding objects across
environments and applies your comparison policy, including any differences you
have explicitly approved.

Comparisons run locally. You do not need ISC credentials or a connection to a
tenant, and the tool does not modify your exports or deploy changes.

Follow the steps below for your first comparison. If you do not have exports
available, use the [sample-data walkthrough](docs/quick-start.md). To call the
module from an application, see [Go library usage](#use-as-a-go-library).

## Before you start

You will need:

- **Go 1.27.3 or newer** and Git to build the tool. Building requires network
  access to download dependencies, including `iscconfigexporter v0.8.0`.
- Two configuration exports in the
  [iscconfigexporter format](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/docs/SNAPSHOTS.md).
  Each untouched export should have a `manifest.json` and object directories
  such as `SOURCE` and `ROLE`.
- Read access to the exports and a writable directory for reports. Reports can
  contain configuration values; keep them in storage appropriate for your data.

A comparison policy is a JSON file that tells the tool how to match objects and
which differences to allow or report. The repository includes a minimal policy
for a first comparison and larger examples you can adapt later.

## Build and install

Get the source and build the command-line tool:

```sh
git clone --branch v0.7.0 https://github.com/easqr/iscconfigdiffer.git
cd iscconfigdiffer
mkdir -p bin
go build -o ./bin/isc-configdiffer ./cmd/isc-configdiffer
```

If you already have a checkout, run the build commands from its root directory.
Go downloads the exporter dependency automatically; you do not need a sibling
exporter checkout. Run the examples below from the configdiffer repository root.

To install the published command without a source checkout, run:

```sh
go install github.com/easqr/iscconfigdiffer/cmd/isc-configdiffer@v0.7.0
```

Go installs it in `GOBIN` or the default `GOPATH/bin`. Add that directory to your
`PATH` and run `isc-configdiffer --help` to check the installation; help exits
successfully without needing inputs. The examples below use the source checkout's
`./bin/isc-configdiffer` and included policy files.

## Compare two exports

Use `drift` mode to compare a development export with a production export:

```sh
./bin/isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --scope SOURCE \
  --policy examples/policy.json
```

Replace `/data/dev` and `/data/prod` with your export directories. Select the
roots containing `manifest.json`, rather than an individual object file or a
`SOURCE` subdirectory.

| Option | What it means here |
| --- | --- |
| `--left` / `--right` | The two export roots to compare |
| `--left-mode snapshot` / `--right-mode snapshot` | Validate each untouched export against its manifest |
| `--left-env dev` / `--right-env prod` | Labels used by environment-specific policy rules; they do not connect to tenants |
| `--scope SOURCE` | Compare source configurations; both exports must declare this type |
| `--policy examples/policy.json` | Use the included minimal portable policy |

The portable policy matches objects by their logical identities, normally their
exact names, and excludes selected generated IDs and metadata. It also supplies
built-in rules for supported configuration types and connectors. It does **not**
approve different environment values such as development and production
hostnames. See [profiles and policy syntax](docs/configuration.md#policy-syntax-and-profiles)
for the defaults used by this policy.

For a broader comparison, use `--scope SOURCE,ROLE` if both exports declare
those types. If you omit `--scope`, both exports must declare the same type set.
For edited copies or inputs without a manifest, follow the
[input-mode guide](docs/inputs.md); those inputs need `tree` mode and may need
explicit type declarations.

## Find and read the report

The command above saves **`reports/portable-comparison.html`**, relative to the
directory where you ran it. Open that file in a browser. It is self-contained
and needs no network connection. The report itself is saved to a file rather
than printed to the terminal.

Start with the result and input summary to confirm the inputs, policy, and scope.
Then use the object index to find a configuration and inspect its differences.
Reports retain the original input paths and identify the policy rules that
influenced each result. Grouped findings and underlying decision counts can
differ; see the [report guide](docs/reports.md) for details.

The process exit code tells you how to interpret the run:

| Exit code | Meaning | Next step |
| --- | --- | --- |
| `0` | Comparison completed with no unexpected differences or violations | Review the report for the selected scope |
| `1` | Comparison completed and found differences or violations | Inspect findings and decide which changes are expected |
| `2` | Invalid input/settings, an incomplete comparison, or a report-writing failure | Read the error and any saved diagnostic report before retrying |

Exit `1` is a normal result when drift is found. In scripts, handle it separately
from exit `2`. A comparison that exits `2` may still save a diagnostic report;
requests rejected before comparison do not create one.

Add `--format md` for Markdown or `--format json` for structured output. With
this policy, those files are `reports/portable-comparison.md` and
`reports/portable-comparison.json`. `--output-dir` changes the destination;
`--output-file` selects the filename. Existing reports with the same name are
replaced. See [all CLI options](docs/cli.md).

Reports include observed values by default. Use `--exclude-values` to omit
observed and expected values; names, paths, and evidence metadata can still be
visible. See [report privacy options](docs/reports.md#values-and-privacy) before sharing a report.

## Allow expected environment differences

A hostname change from development to production may be intentional. To allow
that pair of values, add an `equivalences` rule to your own policy. To require a
particular value in each environment, use an `expectations` rule instead.

For an accepted known difference with a documented reason, use `exceptions`.
Value-matching exceptions use the same syntax as equivalences. Use
`operation: "exclude"` to exclude a complete selected subtree, such as a named
schema. Both forms require a nonblank text `rationale`. Reports label matches `exception` and show the rationale separately
from environment equivalences. Missing or invalid rationales return exit `2`.
See [the exception example](examples/policy-exceptions.json) and
[policy syntax](docs/configuration.md#exceptions).

Follow the [environment-mapping walkthrough](docs/environment-mappings.md) to
identify an object and field, save a complete policy, assign environment labels,
and rerun the comparison. It includes hostname, Active Directory, service-account,
and provisioning-field examples.

Keep your policy files outside the export directories. For local work in this
checkout, `policy.json`, `policy-rules/`, and `reports/` are ignored by Git. For
larger policies, [split rules into explicit fragments](docs/policy-files.md).

## Other comparison workflows

| I want to… | Read |
| --- | --- |
| Try the tool without tenant exports | [Sample-data walkthrough](docs/quick-start.md) |
| Compare one object or include/exclude named objects | [Input scope and selection](docs/inputs.md) |
| Choose profiles, exclusions, or custom rules | [Configuration reference](docs/configuration.md) |
| Validate a proposed deployment draft | [Deployment validation](docs/deployment.md) |
| Use mappings already saved from Configuration Hub | [Hub mapping import](docs/hub-mappings.md) |
| Resolve an error or incomplete comparison | [Troubleshooting](docs/troubleshooting.md) |

`deployment` mode checks a projected draft against explicit expectations. It
uses source, actual draft, and optional baseline inputs, plus an operation list.
It does not apply a deployment or authorize changes. Start with `drift` for an
ordinary comparison between environments.

## Use as a Go library

The module path is **`github.com/easqr/iscconfigdiffer`**; its root package is named
`differ`. Add the published release to your application's Go module:

```sh
go get github.com/easqr/iscconfigdiffer@v0.7.0
```

The [Go library guide](docs/library.md) shows how to load a policy, read validated
snapshots, run a comparison, and render a report.
The `v2` package also supports policies and documents beyond ISC exports.

## Further documentation

Use the [documentation index](docs/README.md) to find task guides, detailed format
and policy contracts, and contributor references.

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
