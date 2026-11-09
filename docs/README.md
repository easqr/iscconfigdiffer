# Documentation

Start with the [README](../README.md) to build the tool, compare two exports,
and open your first report. For a run without tenant exports, use the
[sample-data walkthrough](quick-start.md).

## Guides by task

| I want to… | Read |
| --- | --- |
| Understand snapshot/tree modes, scope, and object selection | [Input format and scope](inputs.md) |
| Understand findings, filters, values, or exit codes | [Reports and exit codes](reports.md) |
| Allow approved environment differences | [Environment mappings](environment-mappings.md) |
| Organize a large policy into files | [Policy files and fragments](policy-files.md) |
| Check a proposed deployment draft | [Deployment validation](deployment.md) |
| Import saved Configuration Hub mappings | [Hub mappings](hub-mappings.md) |
| Use the module from Go | [Go library guide](library.md) |
| Resolve an error | [Troubleshooting](troubleshooting.md) |

## Reference

- [CLI options](cli.md): flags, defaults, and input roles.
- [Configuration reference](configuration.md): policy syntax, profiles, built-in rules, selectors, and examples.
- [Policy and report contract](policy.md): exact schemas, comparison semantics, provenance, and library entry points.
- [Export input contract](contracts.md): validation rules, identities, and storage-format requirements.
- [Shipped rules and custom policies](rule-packs.md): rule-pack behavior, connector classification, and overrides.
- [Example policies](../examples): complete policies, fragments, and deployment operations using invented data.
- [Exporter snapshot format](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/docs/SNAPSHOTS.md): exported files, manifests, and readers.
- [Exporter mapping retrieval](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/docs/MAPPINGS.md): saved mapping arrays and bundles.

## Contributors

- [Contributor guide](DEVELOPMENT.md): local checks and repository conventions.
- [Rule-pack authoring](development/rule-packs.md): changing embedded defaults.
- [Implementation and verification record](development/implementation.md): architecture and test coverage.
- [Correctness review record](development/review.md): previous corrections and regression coverage.
