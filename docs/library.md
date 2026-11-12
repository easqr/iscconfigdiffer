# Use as a Go library

Use Go 1.27.4 or newer. The module path is
`github.com/easqr/iscconfigdiffer`, and its root package is named `differ`.

## Add the module to your application

For a new application, create a directory and run
`go mod init example.com/config-comparison` in it. For an existing application,
use its current Go module directory. Then add the published release:

```sh
go get github.com/easqr/iscconfigdiffer@v0.7.0
```

The existing `v0.7.0` release downloads exporter `v0.8.0` and predates the new
filename formats. This development checkout pins exporter `v0.10.0`, supporting
manifest schemas 1-5. Use this checkout through a local differ replacement until
a new differ release is published; no exporter replacement is required. For contributor testing against a local
checkout, see the [contributor guide](DEVELOPMENT.md#local-module-overrides).

## Compare two snapshots

The root package is named `differ`. Add this function to your application to
compare two validated snapshots using a policy file. Its environment labels match
the included drift example:

```go
package example

import (
    differ "github.com/easqr/iscconfigdiffer"
    "github.com/easqr/iscconfigdiffer/v2"
    "github.com/easqr/iscconfigexporter/snapshot"
)

func CompareSnapshots(leftDir, rightDir, policyPath string) (v2.Report, error) {
    var empty v2.Report
    policy, err := differ.LoadPolicy(policyPath)
    if err != nil {
        return empty, err
    }
    left, err := snapshot.Read(leftDir, snapshot.Options{
        Mode: snapshot.SnapshotMode, Environment: "dev",
    })
    if err != nil {
        return empty, err
    }
    right, err := snapshot.Read(rightDir, snapshot.Options{
        Mode: snapshot.SnapshotMode, Environment: "prod",
    })
    if err != nil {
        return empty, err
    }
    return differ.Compare(differ.Request{
        Mode: "drift", Left: left, Right: right,
    }, policy)
}
```

For a manifest-free tree, Go callers explicitly set naming options:

```go
input, err := snapshot.Read(treeDir, snapshot.Options{
    Mode: snapshot.TreeMode,
    Types: []string{"ROLE", "LIFECYCLE_STATE"},
    FilenameMode: snapshot.NameFilename,
    FilenameEscape: true,
    FilenameAppendID: false,
})
```

For snapshots, retain the manifest and let the reader apply its naming policy.
Different filename conventions do not change policy-based object matching.

## Handle results and write a report

Handle the returned error before consuming the report. Then use
`report.ExitCode()`, `report.HTML(writer)`, `report.Markdown(writer)`, or
`json.NewEncoder(writer).Encode(report)`.
For deployment, `Request.Left` is the source, `Right` is the actual draft,
`Baseline` is an optional `*snapshot.Input`, and `Operations` is a
`[]v2.Operation`.

`snapshot.FromFiles` accepts exporter-format bytes in memory and applies the same
validation as the disk reader. Construct validated inputs with `Read` or
`FromFiles`; the ISC adapter rejects arbitrary or mutated `snapshot.Input`
structures. Comparison operates on detached representations. For neutral document
formats, use `v2.Compile` and `v2.Run` with your own profiles and adapters; use
`differ.Compile` or `Compare` to expand built-in ISC profiles. The root package also exposes `DecodePolicy` and aliases for the `Policy`
and `Report` types. Use `LoadPolicy(path)` to read a root policy and its fragments.
`DecodePolicy(bytes)` and `v2.Decode(bytes)` decode a self-contained policy without
filesystem access and reject `includes`. File composition belongs to the adapter;
the neutral compiler receives one assembled policy.
