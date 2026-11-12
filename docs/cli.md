# CLI options

For the flag listing, run `./bin/isc-configdiffer --mode drift --help`.
Help (`--help` or `-h`) returns `0` without reading inputs or writing a report.
Invalid options and incomplete comparison requests return `2`. All filenames and directories
are resolved relative to the current working directory unless absolute.

| Option | Default | Meaning |
| --- | --- | --- |
| `--version` | `false` | Print version, commit, modification state, Go version and platform without reading configuration |
| `--validate-policy` | `false` | Require `--policy FILE`; validate includes, profiles and rule conflicts without reading exports or writing reports |
| `--mode` | `drift` | `drift` or `deployment` |
| `--policy FILE` | Built-in `default` / `isc-envelope-v2` | Read a comparison policy JSON file using schema `2` |
| `--scope TYPES` | Full declared type set; primary sets must match | Explicit comma-separated type subset available in all inputs |
| `--name KEY` | All objects | Exact logical name/key within one explicit `--scope` type |
| `--format` | `html` | `html`, `md` (Markdown), or `json` |
| `--output-dir DIRECTORY` | `reports` | Report directory relative to the working directory; created if needed |
| `--output-file NAME` | Policy ID plus `.html`, `.md`, or `.json` | Exact filename within the output directory; existing reports are replaced |
| `--exclude-values` | `false` | Omit observed, expected, and derivation values in every report format |
| `--matching` | `false` | Include ordinary equal leaves in reports; does not affect result counts or exit status |
| `--operations FILE` | None | Required deployment operation list; invalid in drift mode |

For each role, replace `<role>` in the following options:

- Drift: `left`, `right`.
- Deployment: `source`, `actual`, optional `baseline`.

| Option | Default | Meaning |
| --- | --- | --- |
| `--<role> DIRECTORY` | None | Input root; both primary roots are required |
| `--<role>-mode` | None | Required `snapshot` or `tree` for each supplied root |
| `--<role>-types TYPES` | None | Required for a manifest-free tree; prohibited alongside a manifest |
| `--<role>-env NAME` | Unset | Environment label used by policy rules and Hub import |
| `--<role>-label TEXT` | Role name | Display/provenance label |

The `baseline` options are deployment-only. Deployment uses `source`/`actual`,
not `left`/`right`. Environment labels can instead be supplied through policy
`bindings`; conflicting policy and CLI bindings are rejected.

Optional Configuration Hub flags:

| Option | Meaning |
| --- | --- |
| `--hub PATH` | Saved mapping array file or exporter mapping-bundle directory |
| `--hub-source-tenant NAME` | Required for a bare array; optional consistency check for a bundle |
| `--hub-target-tenant NAME` | Required for a bare array; optional consistency check for a bundle |
| `--hub-default-source-tenant NAME` | Explicitly bind the `default` source group to a real source tenant |
| `--hub-deployment-authority TEXT` | Explicit authority declaration for deployment promotion |
| `--hub-deployment-evidence FILE` | Evidence JSON file for deployment promotion |

The other Hub flags require `--hub`. Deployment Hub import requires both authority and
evidence flags. Neither is used to perform a remote approval operation.

`--validate-policy` accepts only `--policy FILE`; comparison and output flags are
rejected. It returns `0` for a valid policy and `2` for invalid configuration or
an output failure. This is a static check: input-dependent rule usage, identity
resolution and scope validation happen during comparison. `--version` bypasses
configuration and input loading, and cannot be combined with `--validate-policy`.

After saving a report, stdout shows the result, completeness, unexpected-decision
count (`different` plus `violation`), and saved path. Counts are decisions, not
grouped findings. Incomplete comparisons are explicitly labelled and return `2`.
Errors and usage go to stderr; the full report remains in the selected file.
