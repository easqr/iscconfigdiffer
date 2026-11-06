# ISC configuration differ

`configdiffer` is an offline Go library and command-line tool for comparing
SailPoint Identity Security Cloud configuration exports. It matches corresponding
objects across environments, accounts for approved environment differences, and
produces reports that explain which policy rules affected each result.

It supports two comparison modes:

| Mode | Inputs | Purpose |
| --- | --- | --- |
| `drift` | Left and right configurations | Find differences after allowing explicitly approved equivalences and normalization |
| `deployment` | Source, actual projected draft, and optional target baseline | Check the draft against independently declared expectations |

The CLI reads files locally. It does not require credentials, contact tenants,
change exports, or perform deployments. Its comparison policy model is vendor
neutral; ISC exports and saved Configuration Hub mappings are supported through
adapters.

## Contents

- [Build and install](#build-and-install)
- [Quick start](#quick-start)
- [Reports and exit codes](#reports-and-exit-codes)
- [Input format and scope](#input-format-and-scope)
- [Which files do I need?](#which-files-do-i-need)
- [Configure environment-specific mappings](#configure-environment-specific-mappings)
- [Organize policies across files](#organize-policies-across-files)
- [Deployment validation](#deployment-validation)
- [Configuration Hub mapping files and evidence](#configuration-hub-mapping-files-and-evidence)
- [CLI options](#cli-options)
- [Configuration reference](#configuration-reference)
- [Use as a Go library](#use-as-a-go-library)
- [Troubleshooting](#troubleshooting)
- [Development and further documentation](#development-and-further-documentation)

## Build and install

Use **Go 1.27.2 or newer**, as required by `go.mod`. The current development setup
requires the sibling `configexporter` module, which provides the SDK-free snapshot
reader:

```text
iscutilities/
  configdiffer/       # this module
  configexporter/     # includes the snapshot/ package
```

From `configdiffer`:

```sh
mkdir -p bin
go build -o ./bin/isc-configdiffer ./cmd/isc-configdiffer
```

Use `./bin/isc-configdiffer` in the examples below. To install the CLI into
`GOBIN` (or the default `GOPATH/bin`), run:

```sh
go install ./cmd/isc-configdiffer
```

The current `go.mod` requires `configexporter v0.0.0` and replaces it with
`../configexporter`. A `go.work` file is unnecessary. A standalone versioned
installation requires publishing a real exporter version and replacing that local
dependency first; `go install ...@latest` is not the installation procedure for
this checkout.

## Quick start

Run the examples from the `configdiffer` module directory, using the executable
built in the previous section.

### Compare existing exports

If you already have two complete exports from `configexporter`, point the CLI at
their roots. Each root should contain its generated `manifest.json`; `snapshot`
mode validates that exported inventory before comparing it.

```sh
./bin/isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --scope SOURCE \
  --policy examples/policy.json \
  --format json
```

Replace `/data/dev` and `/data/prod` with your export directories. This command
compares all `SOURCE` objects and saves `reports/portable-comparison.json`.
`examples/policy.json` supplies ISC matching and standard metadata exclusions;
it contains no environment-specific value mappings, so different hostnames and
other environment values will appear as differences. Omit `--scope SOURCE` to
compare all declared types when both exports declare the same type set.

An exit code of `1` means the comparison found differences. Read the report as
explained in the next section; then add approved environment mappings using the
walkthrough later in this guide.

### Try with invented sample data

This example creates two invented, manifest-free configuration trees in a
temporary directory. Run these commands from the module directory in the same
shell after building the CLI:

```sh
demo_dir=$(mktemp -d)
mkdir -p "$demo_dir/dev/SOURCE" "$demo_dir/prod/SOURCE"

cat > "$demo_dir/dev/SOURCE/id-61.json" <<'JSON'
{
  "version": 1,
  "self": {"type": "SOURCE", "id": "a"},
  "object": {
    "id": "a",
    "name": "Invented Directory",
    "connectorAttributes": {
      "host": "directory-dev.example.invalid",
      "timeout": 60
    }
  }
}
JSON

cat > "$demo_dir/prod/SOURCE/id-62.json" <<'JSON'
{
  "version": 1,
  "self": {"type": "SOURCE", "id": "b"},
  "object": {
    "id": "b",
    "name": "Invented Directory",
    "connectorAttributes": {
      "host": "directory-prod.example.invalid",
      "timeout": 60
    }
  }
}
JSON

./bin/isc-configdiffer --mode drift \
  --left "$demo_dir/dev" --left-mode tree --left-types SOURCE --left-env dev \
  --right "$demo_dir/prod" --right-mode tree --right-types SOURCE --right-env prod \
  --policy examples/policy-drift.json
```

The comparison completes successfully with exit code `0`. Objects match by their
name, the policy permits the two hostnames, and the portable profile excludes
physical IDs. Change the production `timeout` to `90` and rerun the command to
see an unexpected difference and exit code `1`. Add `--format json` to obtain a
machine-readable report.

## Reports and exit codes

Reports are saved in `./reports`, relative to the directory where you run the
command. The directory is created automatically. The default filename is the
policy's `id` plus `.html` (the default), `.md` for Markdown, or `.json` for JSON.
For example, a policy with
`"id": "source-drift"` produces `reports/source-drift.json` with `--format json`.
Without `--policy`, the built-in policy uses `default.html`, `default.md`, or
`default.json`.

HTML reports are self-contained documents that open locally in a browser without
network access. Use `--format md` for a Markdown document, or `--format json` for
structured processing. Choosing a format does not change comparison results.

Start with the result and input summary: it identifies the compared environments,
export dates, policy, scope, and object inventory. Decision counts describe
comparison decisions, not independent problems. The breakdown separates missing
objects, container-presence differences, and other field or operation differences;
a missing container can also contribute descendant decisions.

The object index groups results by type and logical name. Its status badges count
**grouped findings**; a separate column shows the underlying displayed decisions.
A missing subtree appears once with its per-input path, presence, and value. Open
**Nested decisions** to inspect its repetitive container and leaf decisions, each
with its original path and provenance. For example, one missing branch and its
seven descendants count as one finding representing eight decisions. Other fields
and explicit policy checks remain separate findings.

When a nested transform's `type` changes, **Transform implementation differs**
shows the old and new types at their original `.../transform/type` paths. Its
related attribute differences appear under **Nested decisions**, so a replacement
of one transform implementation appears as one finding. Each provisioning policy
and field keeps its own finding and identifying context. Explicit checks and
changes to other transform properties remain separate.

This grouping recognizes the JSON convention `transform/type` with related
`transform/attributes` paths, independently of the object or connector type.
An unchanged or explicitly approved type comparison keeps attribute differences
in their usual findings; grouping does not declare the implementations equivalent.

The summary distinguishes **Displayed findings** from **Decision counts**. The
latter retain every underlying comparison decision, including ordinary equal
leaves that may be omitted from the details. Grouping changes presentation only:
JSON decisions, comparison totals, rule usage, digests, and exit codes are
unchanged. Redacted identities are combined explicitly; their findings remain
ungrouped because per-object subtree relationships cannot be established. Matched
objects with no displayed findings still appear in the index.

HTML offers expandable object sections, search by object/path/rule, status
filtering, and expand/collapse controls. Filters affect only the displayed details,
not the comparison totals. Everything works locally; with JavaScript disabled,
all details remain accessible through the expandable sections. Markdown provides
the same information in sections and tables, with nested decisions inside
`<details>` blocks for viewers that support expansion. All child content remains
in the Markdown file. Searching for a nested path in HTML opens its child details;
status filters and the visible-result counter operate on grouped findings.

Each decision shows its reason, content/metadata section, per-input JSON Pointer,
presence and JSON type. Artifact paths and IDs identify the source files. Keyed
array item identities explain alignment when input indices differ. Applied rules,
authority, evidence and expectation derivations explain policy decisions. Pointer
`(root)` means the entire content payload or metadata envelope; pointers are not
prefixed with `/object`. Observed values are shown for each input, and expectation
checks also show the independently computed expected value and target role.

Provisioning findings include containing-item context for each input: policy name,
usage type, subtype ID when present, and field name. For example, a long transform
path can be accompanied by `Provisioning policy — Name: Enable Account Policy;
Usage type: ENABLE → Field — Name: AC_NewParent`. The original full JSON Pointer
is retained. Each side uses its own names and indices, including when policies or
fields move or names differ. Missing items are not labelled using the other side's
item. HTML search also finds these names. Markdown and JSON retain the same context.

These names/types are report identity metadata and remain visible with
`--exclude-values`, like object names and IDs. Explicit `sensitive` rules remove
context along with other identity/provenance for affected types. Schema and
attribute findings use the same per-input context model. Renderers use
captured context and do not reopen the configuration files.

Diagnostics distinguish required or blocking findings from unused optional rules.
The latter are listed separately and do not themselves make a comparison fail.
Policy and input hashes are available as provenance for identifying a particular
run; the renderer does not reopen input files.

Use `--output-dir /data/comparison-reports` to choose another directory and
`--output-file ad-only.json` to override the exact filename:

```sh
./bin/isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --scope SOURCE --name "Active Directory" \
  --policy examples/policy-drift.json --format json \
  --output-dir ./reports --output-file ad-only.json
```

`--output-file` accepts a filename, including its desired extension, without
directory components. It does not select the output format; `--format` does.
Existing reports with the same filename are replaced only after the new report
has been completely written. Filtered and full comparisons use the same default
policy filename, so give them different overrides if you want to retain both.
The default `reports/` directory is excluded from Git tracking.

Reports are written only to the output directory, not to standard output.
Read the saved file to inspect or process a report. Input/policy/request and
output errors go to standard error. A completed
comparison saves its diagnostic report even when it is incomplete and exits `2`.
Requests rejected before comparison do not create a new report. An output failure
returns `2`; an older report may remain in place, so always check the exit code.

| Exit code | Meaning |
| --- | --- |
| `0` | Complete and correct; no unexpected drift or violations |
| `1` | Complete with known differences or expectation violations |
| `2` | Invalid input/policy/request, incomplete or unverified comparison, or output failure |

A report's `complete` flag and correctness are separate. `correct` is false
whenever the result is incomplete. Reports include effective scope, input hashes,
policy/profile/request digests, matching decisions, concrete per-input locations,
applied rule IDs, evidence, coverage, and diagnostics. Statuses include `equal`,
`equivalent`, `excluded`, `verified`, `different`, `violation`, `unverified`, and
`conflict`. Decision counts are not object counts or percentage leaf coverage.

Observed and expected configuration values are included by default in HTML,
Markdown, and JSON. Use `--exclude-values` to omit them from all three formats;
this does not change comparison decisions, counts, exit codes, or request digests.
The library exposes the same option as `Request.ExcludeValues`.

JSON locations carry an optional tagged `value`: `{"state":"present","value":...}`
(including explicit `null`), `{"state":"absent"}`, `{"state":"unavailable"}`, or
`{"state":"redacted"}`. Expectation decisions carry `expected` and `expected_role`
when available. Reports record `values_excluded`; with omission enabled, value
fields are absent. Missing objects/fields, JSON `null`, empty strings, `false`,
and zero remain distinguishable. Containers may include complete objects/arrays;
ordinary equal leaves still require `--matching` to appear.

Policy `sensitive` selectors continue to redact values and identity/provenance
conservatively for their object type, dependent types, and types connected through
known policy reference rules. Expected values and derivation values are redacted
too. The omission flag does not hide names, IDs, labels or declared evidence;
put no secrets in labels or evidence metadata. `--matching`
adds ordinary equal leaves; policy decisions remain visible without it.

Exit `1` is an expected result when drift is found. If using a shell with
`set -e` or a CI runner, handle exit `1` separately from exit `2` when collecting
reports.

## Input format and scope

Inputs use the format produced by the sibling
[`configexporter`](../configexporter/README.md) module:

```text
snapshot-root/
  manifest.json
  SOURCE/
    id-61.json
  ROLE/
    id-726f6c652d31.json
```

Every object file uses the exporter envelope containing `self.type`, `self.id`, and
an object-valued `object` payload. The quick-start files illustrate this envelope.
The canonical
filename is `TYPE/id-<lowercase hexadecimal UTF-8 ID>.json`: ID `a` becomes
`SOURCE/id-61.json`. If the payload includes `id`, it must equal `self.id`.
Arbitrary JSON payload files or filenames are not accepted.

Each input independently chooses a **reading mode**:

| Reading mode | Manifest | Behavior |
| --- | --- | --- |
| `snapshot` | Required | Reads manifest-listed artifacts and validates checksums, counts, scope, paths, and envelopes |
| `tree` | Optional | Inventories current object files, including edits, additions, and deletions; manifest checksums/counts are historical provenance |

Comparison mode (`--mode drift` or `deployment`) and reading mode
(`--left-mode snapshot`, for example) are separate choices.

The exporter generates `manifest.json`; it records schema version, tenant/job,
completion time, selected types, counts, artifact hashes, and publication changes.
A supplied manifest must remain structurally valid, including in tree mode. For a
manifest-free tree, declare its available types with `--<role>-types SOURCE,ROLE`,
even when a declared type currently contains zero objects. Do not combine that
flag with a manifest. Comma-separated lists must contain exact type names without
extra spaces.

Without `--scope`, the primary inputs must declare the same type set. Use
`--scope SOURCE,ROLE` to compare a subset that is declared by every supplied input,
including a deployment baseline. An unavailable type is not treated as an empty
inventory.

To compare just one object, combine one explicit type with `--name`:

```sh
./bin/isc-configdiffer --mode drift \
  --left ../configexporter/exports/papajohns-sb --left-mode snapshot --left-env dev \
  --right ../configexporter/exports/papajohns --right-mode snapshot --right-env prod \
  --scope SOURCE --name "Active Directory" \
  --policy policy.json
```

Use your own export paths and policy file. `--name` is an exact, case-sensitive
logical key, normally the object's `name` under ISC profiles. It is not a physical
ID, substring, or pattern. For custom/composite identities, supply the logical key
shown in the report; explicit pairings use the source/left key. Omit `--name` to
compare every object in the selected types.

The filter requires exactly one type in `--scope` and works in drift and deployment
modes. An object present on only one side still produces a difference. If no
object matches in any supplied input, the result is incomplete with exit `2`.
Deployment still requires an operation for the selected key. Other operations
remain context for reference checks but are not validated by this filtered run.

All inputs are still read, validated, and matched, so other objects can resolve
references; invalid inputs or ambiguous identities can still prevent completion.
Required policy rules remain enforced. JSON reports record `object_key`, include
it in the request digest, and count objects skipped by the filter separately from
the full type inventory. A filtered result makes no claim about other objects.


Roots can be separate directories or root symlinks. Nested links and special
files are rejected. Keep editable trees unchanged while they are being read.
**Keep policies, operations, mappings, and unrelated JSON outside configuration
roots**: tree mode expects configuration `.json` artifacts there, apart from its
manifest and excluded version-control administration directories.

## Which files do I need?

For ordinary environment-aware drift, your **policy file is the only file you
write for configuration**. The exports are generated input data. The deployment
and Hub files are needed only for their corresponding workflows.

| Workflow | Files you author | Inputs generated elsewhere | Options |
| --- | --- | --- | --- |
| Compare environments with your own approved mappings | One root policy containing `equivalences`, optionally split into fragments (start from `examples/policy-drift.json` or `examples/policy-includes.json`) | Left and right configuration exports | `--mode drift --policy FILE` plus input paths/environments |
| Compare literally after standard metadata handling | A minimal policy (such as `examples/policy.json`) | Left and right exports | `--mode drift --policy FILE` |
| Validate a projected deployment draft | One expectation policy (start from `examples/policy-deployment.json`) and one operation list (adapt `examples/operations.json`) | Source export, actual draft, optional target baseline | `--mode deployment --policy FILE --operations FILE` |
| Use mappings already saved from Configuration Hub | One comparison policy without overlapping value rules | Exports and optional Hub mapping array/bundle | Add `--hub PATH`; tenant context is required for a bare array |
| Promote saved Hub mappings into deployment expectations | Expectation policy, operation list, and an attribution/evidence JSON file | Source/draft/baseline and Hub mappings | Add `--hub-deployment-authority` and `--hub-deployment-evidence` |

File roles are different even though all use JSON:

- **Policy:** instructions for interpreting and comparing objects. Mappings go in
  `equivalences`; required values go in `expectations`; other arrays control
  matching, references, collections, exclusions, and report redaction. You can
  combine nonconflicting categories in one policy.
- **Operations:** deployment-only list of intended objects and `create`, `update`,
  or `delete` actions. It contains no environment value mappings. See the
  [operations walkthrough](#deployment-operations-file) for what each action checks.
- **Hub mappings:** an optional vendor-format mapping export. It is an alternative
  source of mapping rules to authoring all equivalences yourself; do not duplicate
  the same active fields in both sources.
- **`manifest.json`:** exporter-generated inventory and integrity/provenance data
  inside a configuration export. It contains no comparison policy.
- **Mapping `provenance.json`:** exporter-generated context/checksum metadata next
  to `mappings.json`, used only when importing that mapping bundle.
- **Deployment evidence:** your declared attribution for promoting Hub mappings;
  it contains no mapping values and is not required for ordinary drift.

The CLI reads **one** policy path supplied with `--policy`. It does not discover,
merge, or load the other example files automatically. `policy.json` is just a
filename; it has no special behavior. Store your own policies outside export
roots. `configdiffer` also does not use the exporter's authentication/settings
files (`config.json` or `exporter.json`).

Continue with the mapping walkthrough for routine drift comparisons. For draft
validation, use the deployment section and its operation-list instructions.

## Configure environment-specific mappings

**For a normal comparison between environments, put your environment mappings in
one policy JSON file and pass it with `--policy`.** Start with
[`examples/policy-drift.json`](examples/policy-drift.json).
[`examples/policy.json`](examples/policy.json) is a smaller example with **no
environment mappings**; it only selects built-in matching and metadata handling.

A policy is the tool's comparison instructions. Environment mappings are one
part of a policy, called `equivalences`. Other optional parts control object
matching, array ordering, exclusions, and required values. There is no separate
mandatory mapping file for ordinary drift comparison.

### 1. Identify the object, field, and environment values

For example, suppose the two exports contain a source named `Invented Directory`:

| Item | Development export | Production export |
| --- | --- | --- |
| `object.name` | `Invented Directory` | `Invented Directory` |
| `object.connectorAttributes.host` | `directory-dev.example.invalid` | `directory-prod.example.invalid` |
| `object.connectorAttributes.timeout` | `60` | `60` |

The objects match by their common name. You want to allow the two hostnames while
still reporting an unexpected difference in `timeout` or another ordinary field.

### 2. Save a complete policy file

Copy `examples/policy-drift.json` to a location outside your export directories,
for example `/policies/dev-to-prod.json`. Its complete contents are:

```json
{
  "version": 2,
  "id": "invented-drift",
  "profile": "isc-portable-v2",
  "equivalences": [
    {
      "id": "directory-host",
      "type": "SOURCE",
      "path": "/connectorAttributes/host",
      "left_env": "dev",
      "right_env": "prod",
      "pairs": [
        {
          "left": "directory-dev.example.invalid",
          "right": "directory-prod.example.invalid"
        }
      ],
      "evidence": {
        "kind": "invented-policy",
        "record_id": "environment-hosts"
      },
      "key": "Invented Directory"
    }
  ]
}
```

Edit these fields for your own exports:

| Field | What you put there |
| --- | --- |
| Root `id` | Your name for this policy; it is recorded in reports |
| `profile` | `isc-portable-v2` supplies ISC object matching and excludes physical IDs and specified generated metadata |
| Rule `id` | Your unique name for this one mapping rule |
| `type` | The exported object's `self.type`, such as `SOURCE` |
| `key` | Its exact logical name, here `object.name`; omitting `key` applies the rule to all objects of that type |
| `path` | The field inside the payload: `object.connectorAttributes.host` becomes `/connectorAttributes/host` |
| `left_env` / `right_env` | The environment labels in which this approved value pair applies |
| `pairs[].left` / `pairs[].right` | The exact approved development and production values, preserving their JSON types |
| `evidence` | Optional attribution stored in reports; it does not require another file for drift |

`key` identifies the object; `path` identifies the field inside it. Do not include
`/object` at the beginning of a policy path. Matching is case-sensitive. If your
objects have different names, define an explicit `pairings` rule as described in
[matching and references](#matching-references-arrays-and-exclusions); a field
mapping does not establish object correspondence.

### 3. Assign environments to the two inputs and run

Using the executable built earlier, compare your actual export roots:

```sh
./bin/isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --scope SOURCE \
  --policy /policies/dev-to-prod.json \
  --format json
```

`--left` and `--right` locate the exported data. `--left-env dev` assigns the label
`dev` to the left input, which matches the rule's `left_env`; `--right-env prod`
connects the right input to `right_env`. **The labels in a rule do not assign
environments to inputs.** Directory names, manifest tenant names, and display
`--*-label` options do not assign them either.

Choose your own labels, such as `sandbox` and `production`, and use those exact
strings in both the policy and the input environment options. Alternatively, add
this root property to the policy:

```json
{"bindings":{"left":"dev","right":"prod"}}
```

This is a property to merge into the complete policy, not another configuration
file. With those bindings, you can omit `--left-env` and `--right-env` from the
command. Conflicting CLI and policy environment assignments are rejected. In the
Go library, the equivalent input setting is `snapshot.Options.Environment`; the
same policy file is used.

For edited or manifest-free input directories, choose `tree` instead of
`snapshot`; a manifest-free tree also needs `--left-types SOURCE` or
`--right-types SOURCE`. The [quick start](#quick-start) creates runnable sample
data so you can try the workflow without tenant exports.

### 4. Read the result and extend the policy

For the mapped object and field:

| Compared values | Drift result |
| --- | --- |
| Approved development hostname on the left and production hostname on the right | `equivalent` |
| Approved development hostname on the left, an unexpected hostname on the right | `different` |
| Approved hostname pair, but different timeouts | Hostnames are equivalent; timeout is different |

The engine records the equivalence and leaves the export files unchanged. These
`pairs` are directional: reversing the environments requires a reversed pair or
an explicitly symmetric `groups` rule. To allow another field, add another
`equivalences` rule with its own ID, object key, path, and values. To allow several
pairs at the same object/path, add entries to that rule's `pairs` array rather
than overlapping rules.

Equivalence rules account for approved differences. Ordinary equal values still
compare equal even if they are not a listed pair. **If each environment must have
a particular value—even when both sides contain the same wrong value—use an
`expectations` rule instead.** For example, this alternative rule requires the
correct hostname on each side:

```json
{
  "id": "directory-host-required",
  "type": "SOURCE",
  "key": "Invented Directory",
  "path": "/connectorAttributes/host",
  "modes": ["drift"],
  "operation": "literal",
  "values": {
    "dev": {"state":"present","value":"directory-dev.example.invalid"},
    "prod": {"state":"present","value":"directory-prod.example.invalid"}
  },
  "authority": "environment-configuration-policy",
  "evidence": {"kind":"approved-policy","record_id":"directory-hosts"}
}
```

Put this object in the policy's `expectations` array **instead of** the hostname
`equivalences` rule. Both cannot own the same active field. The
[configuration reference](#configuration-reference) below explains the other rule
categories; you can leave them out until needed.

### Forest names that differ between environments

The shipped Active Directory rules identify forests by `forestName` within
`/connectorAttributes/forestSettings`. Names must be unique nonblank strings
within each source; array order is ignored. Reports show each input's Forest
name beside its original pointer.

Your policy supplies environment differences, rather than repeating those
structural rules. Use `collection_mappings` to extend `isc.SOURCE.ad-forests`
for one source, then a separate `equivalences` rule to accept the name difference:

```json
{
  "version": 2,
  "id": "directory-forest-drift",
  "profile": "isc-portable-v2",
  "collection_mappings": [
    {
      "id": "directory-forests",
      "key_mappings": [
        {
          "path": "/forestName",
          "groups": [
            {
              "key": "corporate-forest",
              "values": {
                "dev": "dev.example.invalid",
                "prod": "prod.example.invalid"
              }
            }
          ]
        }
      ],
      "evidence": {
        "kind": "invented-policy",
        "record_id": "environment-forests"
      },
      "collection": "isc.SOURCE.ad-forests",
      "key": "Invented Directory"
    }
  ],
  "equivalences": [
    {
      "id": "directory-forest-names",
      "type": "SOURCE",
      "key": "Invented Directory",
      "path": "/connectorAttributes/forestSettings/*/forestName",
      "left_env": "dev",
      "right_env": "prod",
      "groups": [
        {
          "dev": "dev.example.invalid",
          "prod": "prod.example.invalid"
        }
      ],
      "evidence": {
        "kind": "invented-policy",
        "record_id": "environment-forests"
      }
    }
  ]
}
```

Replace `Invented Directory` and the environment values for your implementation.
Run with `--left-env dev --right-env prod` or equivalent policy bindings.
`corporate-forest` is a policy-defined logical identity used for alignment, not a
value written to either export. Add another group for another corresponding
forest. Exact, case-sensitive matching applies; there is no inferred case folding,
substitution, or mapping chain. Unmapped names retain exact matching.

The extension inherits the shipped collection's keys, connector scope, and report
labels. Its `key` restricts the mapping to your source's logical name. Another AD
source uses the shipped structure without inheriting these environment mappings.
Unknown, invalid, overlapping, or disabled extension targets fail compilation.
Missing/unlisted input environments and duplicate keys produce conflicts.

Only sources identified as Active Directory activate the AD rules. Renaming a
source does not change its connector; update the extension's exact object key if
you rename it. See [shipped rules and implementation policy](#shipped-rules-and-implementation-policy)
for how connectors are identified and how to override defaults.

Mapped names remain original values in the report. Alignment does not itself
approve different values or verify a deployment. Other forest settings and
membership changes remain comparable. Domain settings use their own mappings;
there is no implicit propagation to references elsewhere.

### Domain settings between environments

The shipped AD domain rule `isc.SOURCE.ad-domains` identifies each entry in
`/connectorAttributes/domainSettings` by `(forestName, domainDN)` within its
source. Both fields must be nonblank strings; duplicate tuples are conflicts.
Each component can repeat independently when the complete tuple is unique.
Domain entries are unordered, and reports display Forest name and Domain DN.
These are differ requirements, not claims about ISC enforcing array uniqueness.

Put this extension in your policy's `collection_mappings` array:

```json
{
  "id": "directory-domains",
  "key_mappings": [
    {
      "path": "/forestName",
      "groups": [
        {
          "key": "corporate-forest",
          "values": {
            "dev": "dev.example.invalid",
            "prod": "prod.example.invalid"
          }
        }
      ]
    },
    {
      "path": "/domainDN",
      "groups": [
        {
          "key": "corporate-domain",
          "values": {
            "dev": "DC=dev,DC=example,DC=invalid",
            "prod": "DC=prod,DC=example,DC=invalid"
          }
        }
      ]
    }
  ],
  "evidence": {
    "kind": "invented-policy",
    "record_id": "directory-environment-domains"
  },
  "collection": "isc.SOURCE.ad-domains",
  "key": "Invented Directory"
}
```

The extension adds only your source-specific environment values. It inherits the
two-part key and display labels from the AD pack. Add drift `equivalences` rules
for the aligned values at:

| Path | Approved environment values |
| --- | --- |
| `/connectorAttributes/domainSettings/*/forestName` | `dev.example.invalid` ↔ `prod.example.invalid` |
| `/connectorAttributes/domainSettings/*/domainDN` | `DC=dev,DC=example,DC=invalid` ↔ `DC=prod,DC=example,DC=invalid` |

[`examples/policy-drift.json`](examples/policy-drift.json) contains both mapping
extensions and their equivalences for `Invented Directory`. Keep these values
consistent with the forest extension; definitions are not shared automatically
between arrays. DN canonicalization, suffix replacement, and domain-to-forest
referential integrity validation are not performed.

Other domain settings remain comparable. The `servers` array remains ordered
because its order controls fallback priority. To accept different complete server
lists, author a drift equivalence at
`/connectorAttributes/domainSettings/*/servers` with the complete ordered arrays
as the environment values. No additional collection definition is needed.

Collection extensions align entries in deployment too, but destination values
still need independent expectations. `copy_source` checks the original source
name/DN; use destination literals or `preserve_target` with a baseline when needed.

### Different service accounts for multiple domains

Use an equivalence's `item` selector to restrict approval to one logical domain.
Keep the field path `/connectorAttributes/domainSettings/*/user`; the engine
aligns domains by their complete mapped forest/domain tuple before evaluating
this rule. No array index is used in the policy.

The [example drift policy](examples/policy-drift.json) includes two domains in
the same forest, with these independent approvals:

| Logical domain | Development account | Production account |
| --- | --- | --- |
| `corporate-domain` | `svc-corporate@dev.example.invalid` | `svc-corporate@prod.example.invalid` |
| `regional-domain` | `svc-regional@regional.dev.example.invalid` | `svc-regional@regional.prod.example.invalid` |

Here is the corporate-domain rule from its `equivalences` array:

```json
{
  "id": "directory-corporate-domain-user",
  "type": "SOURCE",
  "key": "Invented Directory",
  "path": "/connectorAttributes/domainSettings/*/user",
  "item": {
    "collection": "isc.SOURCE.ad-domains",
    "keys": {
      "/forestName": {
        "mapped": "corporate-forest"
      },
      "/domainDN": {
        "mapped": "corporate-domain"
      }
    }
  },
  "left_env": "dev",
  "right_env": "prod",
  "groups": [
    {
      "dev": "svc-corporate@dev.example.invalid",
      "prod": "svc-corporate@prod.example.invalid"
    }
  ],
  "evidence": {
    "kind": "invented-policy",
    "record_id": "corporate-domain-service-account"
  }
}
```

The second rule uses `"mapped":"regional-domain"` for `/domainDN` and its own
account values. Both use `corporate-forest` for `/forestName`. Those names refer
to the logical group `key` values declared in `collection_mappings`, not exported
forest names, domain DNs, or report hashes. All key components are required.
The shipped AD domain definition supplies the two key paths and report labels.

The same account pair approved for one domain is not approved for another domain.
Reordering the arrays does not change the result, and reports retain the original
indices, domain context, and selected logical identity. For an unmapped key
component, use `{"value":"the exact raw key"}` instead of `{"mapped":"group"}`;
provide exactly one form per component. Raw and mapped identities are distinct.

An unscoped wildcard equivalence approves its listed pairs across all aligned
domains. To switch that field to domain-specific approvals, remove the unscoped
rule and author one scoped rule per intended domain. Overlapping scoped/broad
rules or duplicate item scopes fail compilation; there is no first-match order.
Unexpected account differences and domain additions/removals remain findings;
duplicate domain keys remain conflicts. Equivalence does not assert that an
account or domain must exist. Set `required:true` if an unused approval must make
the comparison incomplete.

`item` and `items` are supported on drift equivalences. It does not introduce
per-item deployment expectations or change deployment authority requirements.
The same selector can scope other drift fields, including complete server arrays,
or target another keyed collection; it contains no AD-specific engine logic.

### Map a provisioning field within a specific policy

Provisioning policies and their fields are nested keyed arrays. Use `items` when
an approval must match both ancestors. For example, this rule from the
[drift example](examples/policy-drift.json) approves only the exported CREATE
policy's `userPrincipalName` template:

```json
{
  "id": "directory-create-user-principal-name",
  "type": "SOURCE",
  "key": "Invented Directory",
  "path": "/provisioningPolicies/*/fields/*/transform/attributes/value",
  "items": [
    {
      "collection": "isc.SOURCE.provisioning-policies",
      "keys": {
        "/usageType": {
          "value": "CREATE"
        }
      }
    },
    {
      "collection": "isc.SOURCE.provisioning-fields",
      "keys": {
        "/name": {
          "value": "userPrincipalName"
        }
      }
    }
  ],
  "left_env": "dev",
  "right_env": "prod",
  "groups": [
    {
      "dev": "$uid@dev.example.invalid",
      "prod": "$uid@prod.example.invalid"
    }
  ],
  "evidence": {
    "kind": "invented-policy",
    "record_id": "create-upn-template"
  }
}
```

All entries in `items` must match: `/usageType` must be `CREATE` **and** the
field's `/name` must be `userPrincipalName`. Both arrays can move independently;
the field path uses wildcards, and the report retains each input's original
indices. The same template pair on another field or in another policy remains
a difference unless independently approved.

These are raw keys (`value`), because the policy usage and field names are the
same between environments. Use `mapped` for an environment-mapped key component
when needed. Every referenced collection still requires its complete key tuple.
The existing single `item` form remains supported; use either `item` or nonempty
`items`, not both. Conditions may be authored in either order; the compiler
orders them from outer to inner and rejects duplicate collection conditions.

The differ compares the template strings literally. It does not evaluate `$uid`
or execute transforms, and this rule does not approve the transform's `type`,
other attributes, or the field's flags. The same mechanism can combine schema
and attribute identities or other nested collection keys without source-specific
logic in the engine. It remains a drift equivalence, not deployment authority.

## Organize policies across files

Use one root policy to select the profile and shared settings, and include any
number of **rule-only fragments**. This lets you maintain source mappings by
connector and keep transform mappings in another file. Continue passing only the
root file to `--policy`; run the command from any directory.

For a complete example, see [policy-includes.json](examples/policy-includes.json)
and its [rule fragments](examples/policy-rules). The existing
[policy-drift.json](examples/policy-drift.json) remains a single-file example.
The modular example adds invented Entra client-ID and static-transform domain
mappings to the directory examples; adapt their object names, paths, and values
to your actual exports.

A policy directory can look like this:

```text
policies/
  policy.json
  sources/
    shared.json
    active-directory.json
    entra.json
  transforms.json
```

Root `policies/policy.json`:

```json
{
  "version": 2,
  "id": "environment-drift",
  "profile": "isc-portable-v2",
  "bindings": {"left": "dev", "right": "prod"},
  "includes": [
    "sources/shared.json",
    "sources/active-directory.json",
    "sources/entra.json",
    "transforms.json"
  ]
}
```

A fragment, such as `sources/active-directory.json`, uses exactly the same rule
syntax as a single-file policy, without policy headers:

```json
{
  "equivalences": [
    {
      "id": "ad-iqservice-host",
      "type": "SOURCE",
      "key": "Active Directory",
      "path": "/connectorAttributes/iqServiceHost",
      "left_env": "dev",
      "right_env": "prod",
      "groups": [{"dev": "iq-dev.example.invalid", "prod": "iq-prod.example.invalid"}]
    }
  ]
}
```

Each listed file must exist; an empty fragment can contain `{}`. File and folder
names organize your policy but do not select objects or connectors. A rule's
`type`, `variant`, `key`, `path`, and item selectors still control its scope.
Use object keys or connector variants to keep connector-specific rules isolated.

| Where | Allowed content |
| --- | --- |
| Root only | `version`, `id`, `profile`, `bindings`, `null_handling`, policy-level `modes`, `includes` |
| Root or fragments | `identities`, `pairings`, `ignores`, `expectations`, `equivalences`, `references`, `collections`, `collection_mappings`, `sensitive`, `disabled_rules`, `evidence` |

Rule-level `modes` remain allowed inside fragment rules. Root-level `evidence`
and fragment-level `evidence` are arrays of attribution records; individual rule
`evidence` remains a single record. File attribution does not supply rule
evidence or deployment authority.

Includes resolve relative to the **root policy directory**, never the command's
working directory. A symlink to the root policy is supported: the loader resolves
its target first, then uses the target policy's directory and basename for
includes and provenance. Include paths are normalized before opening them
(`./rules.json` and `folder/../rules.json` both identify `rules.json`). They must
stay inside the root directory, including through symlinks. Absolute paths, parent-directory escapes, duplicate files (including
aliases), directories, unknown properties, and duplicate JSON properties are
rejected. Files must contain strict JSON, so comments are not supported.
Fragments cannot include other fragments. There are no wildcard includes or
automatically loaded files; list each file explicitly.

Rules are combined before compilation. Rule IDs must be unique across all files
and categories; disabled rule IDs cannot be repeated. Listing a file last gives
it no special precedence. Existing specificity, conflict checks, `replaces`, and
`disabled_rules` work across the assembled policy. Keep using the shipped common,
SOURCE, and connector defaults; fragments organize your implementation rules
and overrides, without duplicating those defaults.

Run the included modular example:

```sh
./bin/isc-configdiffer \
  --left /data/dev --left-mode snapshot \
  --right /data/prod --right-mode snapshot \
  --policy examples/policy-includes.json
```

The bindings assign `dev` and `prod`, and the default report is
`reports/invented-drift-modular.html`. Only the root `id` determines the default
report name. HTML and Markdown list policy files and show file origin for applied
rules and collection mapping extensions. JSON retains each file's relative name,
SHA-256 digest, role, rule IDs, and disabled IDs in `evidence` records with
`kind: "policy-file"`. File evidence is visible metadata, even with
`--exclude-values`; keep secrets out of filenames and evidence.

Validation errors name the responsible rule and file; cross-file conflicts name
the involved files. Library callers can use `errors.As` with `*v2.RuleError` to
read the rule IDs and `errors.Is` to inspect underlying filesystem errors.
Invalid rule IDs and attribution records with an empty `kind` are rejected during
loading; call `Compile` or `Compare` to validate the full assembled policy.

Policy and request digests include this provenance. Splitting a policy, editing
file whitespace, or changing the manifest order changes those digests, even when
the comparison decisions stay the same. Moving an unchanged policy directory
preserves its relative file provenance.

For local policy bundles in this repository, use `policy.json` and
`policy-rules/`; both are ignored by Git, as is `reports/`. Versioned invented
examples live separately under `examples/`.

## Deployment validation

```sh
./bin/isc-configdiffer --mode deployment \
  --source /data/dev --source-mode snapshot --source-env dev \
  --actual /data/draft --actual-mode tree --actual-types SOURCE --actual-env prod \
  --baseline /data/prod --baseline-mode snapshot --baseline-env prod \
  --scope SOURCE \
  --operations examples/operations.json \
  --policy examples/policy-deployment.json \
  --format json
```

The source is the starting configuration; actual is the projected deployment
configuration being checked; baseline is the destination's existing configuration.
The draft must be readable in exporter envelope format. Native proprietary draft
formats are not currently supported.

The example policy covers `Invented Directory`'s name, hostname, timeout, and
target-preserved token endpoint. Adapt it and the operations file to your objects.
Every relevant deployment value needs an authoritative expectation or an explicit
exclusion. Uncovered values remain `unverified`, including values equal to the
source; an otherwise identical draft does not automatically pass.

A baseline is optional for independently specified or source-derived values.
`preserve_target`, baseline-derived references, and delete validation need it.
Without a baseline, create/update content can be checked but destination existence
is not established. The actual draft is never locally substituted. Full deployment
impact analysis and authorization gates are outside this tool's scope.

### Deployment operations file

**`operations.json` declares which configuration objects belong to the deployment
you are validating, and whether each is intended to be created, updated, or
deleted.** It is required with `--mode deployment`. For normal drift comparison,
leave it out; drift mode does not accept `--operations`.

The files answer different questions:

| Setting/file | Question it answers |
| --- | --- |
| `--scope SOURCE,ROLE` | Which object types are available for this comparison? |
| `--operations operations.json` | Which individual objects are in this deployment, and what action is intended for each? |
| `--policy policy.json` | What values and states should those objects have? |
| `--baseline /data/prod` | What objects and values existed in the destination before the deployment? |

The operation list supplies your declared intent. The tool validates the supplied
source, projected draft, and baseline against that intent; it does not generate
a deployment plan, apply changes, or establish an external approval.

#### A worked update example

Suppose you are deploying the source `Invented Directory` from development to
production. Save this as your operations file, or use the included
[`examples/operations.json`](examples/operations.json):

```json
[
  {"type":"SOURCE","key":"Invented Directory","action":"update"}
]
```

This entry selects the **whole source object**, not one changed field. It means:

1. Match the source and actual draft objects identified as `Invented Directory`.
2. When a baseline is supplied, check that the corresponding destination object
   already exists; otherwise an `update` is inconsistent with that baseline.
3. Check the draft object's content using the expectation policy. For example,
   the policy may require the production hostname, copy the source timeout, and
   preserve a target-specific value from the baseline.

The operation entry contains no hostname, replacement value, or JSON field path.
Those belong in the policy. Even a change to just one field uses one `update`
entry for the object; the rest of its relevant content still needs expectations
or explicit exclusions. Listing an object does not automatically verify its
values or authorize every difference inside it.

Use the operation file together with the expectation policy:

```sh
./bin/isc-configdiffer --mode deployment \
  --source /data/dev --source-mode snapshot --source-env dev \
  --actual /data/draft --actual-mode tree --actual-types SOURCE --actual-env prod \
  --baseline /data/prod --baseline-mode snapshot --baseline-env prod \
  --scope SOURCE \
  --operations examples/operations.json \
  --policy examples/policy-deployment.json \
  --format json
```

`--actual` is the projected configuration being checked; `--baseline` is the
independently captured destination export. For this example the baseline must
contain `Invented Directory`, and the actual input must contain its projected
updated object. The included policy covers the fields described in the
[deployment validation example](#deployment-validation); adapt it to your data.

#### Fields and actions

The file is a plain JSON array. Each entry has exactly these required fields:

| Field | Meaning |
| --- | --- |
| `type` | Exact exported object type, such as `SOURCE` or `ROLE` |
| `key` | Logical object identity, usually its exact `object.name`; use the source key for an explicitly renamed pairing |
| `action` | One of `create`, `update`, or `delete` |

Use the logical key produced by the identity strategy. For standard named ISC
objects this is `object.name`, rather than `self.id`. A filename, wildcard, or
JSON Pointer does not select an object here.
Composite string identities use a JSON tuple encoded as a string, for example
`"[\"request\",\"en_US\",\"EMAIL\"]"` for a notification template. Singleton
identities use `"singleton"`. For a baseline-only deletion, use the baseline
object's logical key. Matching still uses the policy's identity and pairing rules.

| Action | Source and actual inputs | Destination baseline check |
| --- | --- | --- |
| `create` | Both must contain the corresponding object for content validation | If supplied, it must not contain that object; an existing target is a violation |
| `update` | Both must contain the corresponding object for content validation | If supplied, it must contain that object; an absent target is a violation |
| `delete` | The actual draft must omit the object; the source object may be present or absent | Required to verify deletion; it must contain the object |

A baseline is optional for independently established create/update content
expectations. Without it, the tool cannot establish whether the object already
exists at the destination. `preserve_target` and baseline-derived reference rules
still need baseline evidence even when the action is `create` or `update`.

A `delete` validates projected absence. It does not issue a delete request or
attest that a proprietary draft encodes a remote deletion instruction. If a
`reference_target` expectation resolves to an object marked for deletion, it
reports a violation rather than verifying the old reference against the baseline.

For illustration, a deployment containing several actions could use:

```json
[
  {"type":"SOURCE","key":"Invented Directory","action":"update"},
  {"type":"ROLE","key":"New Finance Access","action":"create"},
  {"type":"ROLE","key":"Retired Finance Access","action":"delete"}
]
```

For that list, declare `SOURCE` and `ROLE` in the input scopes and comparison
scope. The actual input would contain the updated source and new role and omit
the retired role. The baseline would contain the source and retired role and omit
the new role. Author policy expectations for the new role too; the supplied
source-only example policy is not enough to validate this illustrative deployment.

#### What happens when objects are missing or unlisted?

The explicit list lets you validate a partial draft without treating every object
omitted from that draft as a deletion:

| Situation | Result |
| --- | --- |
| Source/baseline object is not listed and is absent from the draft | Outside the operation comparison; no deletion is inferred |
| Draft contains an object not listed in operations | `unverified` because it is outside the declared deployment intent |
| Listed `create`/`update` has a matched group but lacks a source or actual object | `violation` for the missing object |
| Listed object cannot be identified in any supplied input | `unverified`; the requested action cannot be validated |
| Listed `delete` object remains in the draft | `violation` |
| Listed `delete` has no corresponding baseline object | `unverified` |
| Listed object matches, but its values lack authoritative expectations | Its uncovered values are `unverified` |

Unlisted source/baseline objects can still provide matching and reference context;
their content is not automatically subjected to deployment expectations. Every
object in the supplied draft must be listed, including an unchanged object if it
is present there. Provide the projected objects intended for this validation
rather than an unrelated full destination export as the actual input.

The list must be nonempty, each `(type, key)` pair must occur once, and each type
must be available in the declared comparison scope. You cannot list the same
object once as `delete` and again as `create`. File order does not specify an
execution sequence. `operations.json` is an arbitrary filename: pass its path
explicitly, keep it outside export roots, and maintain it alongside the policy
for the deployment being reviewed.

## Configuration Hub mapping files and evidence

The optional adapter accepts either a bare JSON array or a separate exporter
bundle:

```text
mapping-bundle/
  mappings.json
  provenance.json
```

A bare `mappings.json` contains saved Hub records such as:

```json
[
  {
    "objectMappingId": "invented-host-map",
    "objectType": "SOURCE",
    "jsonPath": "$.connectorAttributes.host",
    "sourceValue": "directory-dev.example.invalid",
    "targetValue": "directory-prod.example.invalid",
    "enabled": true
  }
]
```

Optional `created` and `modified` fields retain mapping timestamps. Unlike policy
pointers, Hub `jsonPath` uses payload-relative JSONPath. Supported syntax includes
`$`, `.member`, quoted bracket members, `[0]`, and `[*]`; recursive descent,
filters, slices, and scripts are unsupported. Values remain strings. Unsupported
active semantics fail import; disabled mappings stay inert evidence.

The exporter creates the bundle's `provenance.json`, recording schema, retrieval
ID/time, source organization, target tenant, API origin/version/endpoint, record
count, SHA-256 of `mappings.json`, and optional export-job correlation. Keep the
original files together; their context and checksum are validated on import.
The sidecar is distinct from a configuration snapshot manifest. See the
[exporter mapping guide](../configexporter/docs/MAPPINGS.md) for retrieval.

To import a bundle, use a portable policy without a competing hostname rule:

```sh
./bin/isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --policy /policies/portable-policy.json \
  --hub /data/mapping-bundle --format json
```

For a bare array, additionally provide `--hub-source-tenant` and
`--hub-target-tenant`. They must agree with the manifest tenants; labels alone do
not establish tenant identity. A manifest-free tree cannot supply this evidence.
The `default` source group requires `--hub-default-source-tenant REAL-SOURCE`.
Mappings remain directional, and importing one does not establish object pairing.

Deployment promotion additionally requires `--hub-deployment-authority` and
`--hub-deployment-evidence`. The latter names a JSON file such as:

```json
{
  "kind": "approved-policy",
  "record_id": "release-1",
  "attributes": {"review": "configuration-change-review"}
}
```

`kind` is required; `record_id`, `digest`, and string-valued `attributes` are
optional. Use meaningful references for your own process. Promotion creates
source-guarded expectations. Multi-case groups require authored deployment
expectations instead of automatic promotion. Do not duplicate an imported rule's
field with another active value rule. No parity with undocumented Hub global
substitution rules is assumed.

## CLI options

For the flag listing, run `./bin/isc-configdiffer --mode drift --help`.
The current CLI returns `2` after displaying help. All filenames and directories
are resolved relative to the current working directory unless absolute.

| Option | Default | Meaning |
| --- | --- | --- |
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

## Configuration reference

This section describes the policy syntax and rule categories used by the drift
and deployment workflows above. The file-selection overview explains which files
each workflow requires; all user-authored paths are passed explicitly. Policy
filenames are arbitrary, and `--policy` selects one file.

### Policy syntax and profiles

Policy files are strict JSON: no comments, trailing commas, duplicate keys,
unknown properties, or case-insensitive property aliases. Policy/rule IDs contain
1–200 ASCII letters/digits or `_ . : -`, starting with a letter/digit. Rule IDs
must be unique across all categories. Values retain JSON types; strings are not
converted to numbers or booleans. ISC profiles treat a null-valued payload
property and an absent property as equivalent by default. Reports preserve the
original null/absent states and identify `null_equals_absent` normalization.
Empty strings, `false`, zero, empty objects/arrays, null array members, identity
components, and envelope metadata remain distinct. This setting does not remove
array members or supply deployment authority.

Set `"null_handling": "strict"` in your policy to compare null and absent payload
properties strictly. A neutral policy without an ISC profile defaults to strict;
it can opt in with `"null_handling": "null_equals_absent"`. Whole-value expectations,
source guards, and multiset object members use the same property semantics.
Original exports and captured report values are never rewritten.

A minimal comparison policy, suitable as `portable-policy.json`, is:

```json
{
  "version": 2,
  "id": "portable-comparison",
  "profile": "isc-portable-v2"
}
```

With this policy, ordinary fields use the declared profile's exclusions and
null/absent-property equivalence. It does not add environment-specific hostname equivalence.

| Root property | Meaning |
| --- | --- |
| `version` | Required `2` for the comparison policy schema |
| `id` | Required stable policy identifier |
| `profile` | Optional preset selecting shipped common, SOURCE, and connector rule packs |
| `disabled_rules` | Optional list of shipped rule IDs to disable explicitly |
| `collection_mappings` | Optional source-scoped extensions supplying environment mappings for existing keyed collections |
| `null_handling` | Optional `strict` or `null_equals_absent`; defaults to the selected profile, otherwise `strict`. Applies to payload properties only |
| `modes` | Optional allowed comparison modes, e.g. `["drift"]` |
| `bindings` | Optional role-to-environment object, e.g. `{"left":"dev","right":"prod"}`; deployment uses `source`, `actual`, `baseline` |
| Rule arrays | The categories described below; omitted arrays are empty |
| `evidence` | Optional array of attribution records |

Built-in ISC profiles:

| Profile | Drift behavior |
| --- | --- |
| `isc-identity-v2` | Supplies logical identities, SOURCE schema/attribute alignment, connector-scoped AD alignment, Entra inventory exclusion, and provisioning-policy alignment/exclusions; other content and envelope metadata remain comparable |
| `isc-envelope-v2` | Also excludes envelope JWS header/signature fields; used when `--policy` is omitted |
| `isc-portable-v2` | Also excludes envelope `/self/id`, payload `/id`, `/created`, `/modified`, `/owner/id`, SOURCE `/schemas/*/id`, `/schemas/*/created`, `/schemas/*/modified`, `/connectorAttributes/cloudCacheUpdate`, `/connectorAttributes/since`, and lifecycle-state `/identityCount` |

All three default to `null_equals_absent` for payload properties and include
deployment structural expectations for envelope version/type.
The portable profile excludes those three generated metadata fields on every
SOURCE schema array item in both drift and deployment modes, including with keyed
schema matching. For example, `/schemas/2/modified` is excluded. All ISC profiles
also exclude SOURCE attribute schema reference IDs at
`/schemas/*/attributes/*/schema/id`; reference names and types remain comparable.
Other nested IDs, owners, arbitrary runtime tokens, and other array order remain comparable.
Choose the portable profile explicitly when differences in generated IDs and
these metadata fields should be excluded. A supplied policy without a profile
must define its own identity strategies; no ISC profile is added implicitly.

Objects normally match by exact, case-sensitive name within their type, regardless
of physical ID. Notification templates use key/locale/medium; lifecycle states
include their parent identity profile; attribute-sync configurations use their
parent source. `ACCESS_REQUEST_CONFIG` and `AUTH_ORG` are singletons. Ambiguous
identities do not pass. See the [identity contract](docs/contracts.md#scope-and-identities)
for the named-type list.

### Shipped rules and implementation policy

The first three rule layers ship with the module; your policy supplies the fourth:

| Layer | Contents | Where it lives |
| --- | --- | --- |
| ISC common defaults | Object identities, null handling, envelope and selected metadata rules | `internal/iscprofile/packs/common.json`, `envelope.json`, `portable.json` |
| SOURCE defaults | Schema/attribute and provisioning keys, reference-ID exclusions | `source.json`, `source-portable.json` in the same directory |
| ROLE defaults | Access profile, entitlement, and entitlement-source reference IDs | `role.json` in the same directory |
| ACCESS_PROFILE defaults | Entitlement and source reference IDs | `access-profile.json` in the same directory |
| WORKFLOW defaults | Creator and modifier reference IDs | `workflow.json` in the same directory |
| Connector defaults | AD forest/domain structure; Entra subscribed-SKU inventory exclusion | `active-directory.json`, `microsoft-entra.json` in the same directory |
| Implementation policy | Environment mappings/equivalences, expectations, deliberate exceptions | Your `--policy` JSON file |

The [JSON rule packs](internal/iscprofile/packs) are embedded in the executable.
Users normally leave them unchanged. Maintainers update these files and build a
new executable to change defaults; adding connector identity patterns does not
require adding comparison logic. See the [rule-pack format](docs/rule-packs.md).
The core engine supplies vendor-neutral comparison mechanisms and does not know
what SOURCE, Active Directory, or Entra mean.

The ISC adapter identifies connectors from exact exported metadata declared in
the packs: AD's connector class, connector type, or `connectorName`; Entra's
`connectorName` of `Microsoft Entra`. Source display names and the presence of
arrays do not identify a connector. Unknown connectors receive common/SOURCE
rules and ordinary comparison, without guessed connector rules. Contradictory
recognized identifiers fail input adaptation; different classified connectors
across matched inputs produce a conflict. Generic selectors can use `variant`
(`active-directory` or `microsoft-entra` here) to restrict a rule to a connector.

Use `collection_mappings` to attach mappings to a collection without copying its
structure. Use `collections` only to define additional array semantics or to make
an explicit full replacement. A replacement retains the original selector,
including its `variant`, and declares `replaces` with the shipped rule ID.
For example, a deliberate AD forest replacement must retain
`"variant":"active-directory"`.

To turn off a shipped normalization or matching rule entirely, explicitly list
its ID in root `disabled_rules`, for example:

```json
{
  "version": 2,
  "id": "compare-reference-ids",
  "profile": "isc-portable-v2",
  "disabled_rules": ["isc.SOURCE.before-provisioning-rule-id"]
}
```

That policy compares the reference ID normally. Unknown/duplicate disabled IDs,
replacing a disabled rule, or extending a disabled collection are errors. These
are explicit choices, not file-order precedence. Reports retain shipped-pack
versions/digests, rule evidence, extension provenance, replacements, and disabled
IDs so a result can be traced to its effective policy.

### Microsoft Entra subscribed SKUs

Microsoft Entra SOURCE configurations exclude the entire
`/connectorAttributes/subscribedSkus` subtree, including nested service plans,
in both drift and deployment modes under all built-in ISC profiles. The shipped
connector rule is `isc.SOURCE.entra-subscribed-skus`. It applies by recognized
Entra connector metadata, independently of the source's display name. Other
connectors and adjacent Entra settings remain comparable.

The exclusion replaces the SKU/service-plan collection defaults. To compare this
inventory explicitly, add `"isc.SOURCE.entra-subscribed-skus"` to `disabled_rules`;
arrays then follow normal comparison unless you define your own collection rules.
The former `isc.SOURCE.entra-skus` and `isc.SOURCE.entra-service-plans` collection
IDs are no longer shipped, so remove references to them from implementation
extensions or replacements. No local mapping is needed for the default exclusion.

### SOURCE cache-update metadata

The portable profile excludes `/connectorAttributes/cloudCacheUpdate` across
all SOURCE connector types in drift and deployment. It is treated as operational
cache-update metadata, rather than an environment-specific configuration value.
The identity and envelope profiles continue comparing it normally.

This rule is defined in `internal/iscprofile/packs/source-portable.json` with ID
`isc.SOURCE.cloud-cache-update`. To restore ordinary comparison, add that ID to
your policy's `disabled_rules` array. Other connector attributes remain comparable.

### SOURCE health-check timing metadata

The portable profile excludes `/connectorAttributes/since` across SOURCE
connector types in drift and deployment, treating it as operational health/status
timing metadata. Identity and envelope profiles continue comparing it normally.
The rule does not exclude health/status values, aggregation cursors, a top-level
`/since`, or other similarly named fields.

The declaration is in `internal/iscprofile/packs/source-portable.json`, with rule
ID `isc.SOURCE.health-since`. Add that ID to `disabled_rules` to restore ordinary
comparison and deployment coverage requirements. This comparison choice does not
assume that the nested timestamp is updated on every health check.

### Owner references across configuration types

The common portable pack excludes `/owner/id` for every configuration type,
including custom types with declared identities. Owner `name`, `type`, and other
fields remain comparable. Identity/envelope profiles still compare owner IDs.
The rule expands as `isc.TYPE.owner-id`, for example `isc.ROLE.owner-id` or
`isc.SOURCE.owner-id`. Add a concrete rule ID to `disabled_rules` to restore ID
comparison for that type. This is a common portable rule, not an AD-specific or
implementation-specific mapping.

### ROLE access profile and entitlement references

All ISC profiles exclude these ROLE reference IDs in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/accessProfiles/*/id` | `isc.ROLE.access-profile-id` |
| `/entitlements/*/id` | `isc.ROLE.entitlement-id` |
| `/entitlements/*/sourceId` | `isc.ROLE.entitlement-source-id` |

The wildcards apply to every array member, including `/accessProfiles/0/id`,
`/entitlements/0/id`, and `/entitlements/0/sourceId`. Names, types, entitlement
`sourceName`, and other fields remain comparable. Missing members and array
ordering still produce findings. Other nested IDs and ROLE `/source/id` retain
normal comparison; these rules do not apply to other configuration types.

These declarations live in `internal/iscprofile/packs/role.json`. Add an
individual rule ID to `disabled_rules` to restore that ID's comparison and
deployment coverage requirements. No implementation-policy mappings are needed.

### ACCESS_PROFILE entitlement and source references

All ISC profiles exclude these ACCESS_PROFILE reference IDs in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/entitlements/*/id` | `isc.ACCESS_PROFILE.entitlement-id` |
| `/source/id` | `isc.ACCESS_PROFILE.source-id` |

The entitlement wildcard applies to every array member, for example
`/entitlements/0/id` and `/entitlements/2/id`. Entitlement and source names, types,
and other fields remain comparable. Missing entitlements and source references
still produce findings; entitlement array ordering is unchanged. Other nested IDs
and identical paths on other configuration types retain normal comparison.

These declarations live in `internal/iscprofile/packs/access-profile.json`.
No local policy mapping is needed. Add either rule ID to `disabled_rules` to
restore its ID comparison and deployment coverage requirements.

### WORKFLOW creator and modifier references

All ISC profiles exclude these WORKFLOW reference IDs in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/creator/id` | `isc.WORKFLOW.creator-id` |
| `/modifiedBy/id` | `isc.WORKFLOW.modified-by-id` |

The exported field is `creator`, not `create`. These rules are declared in
`internal/iscprofile/packs/workflow.json`. Reference names, types, and other fields
remain comparable, as do identical paths on other configuration types. List either
rule ID in `disabled_rules` to restore its ID comparison and deployment coverage
requirements. No implementation-policy mapping is needed.

### SOURCE correlation references

All ISC profiles exclude these exact SOURCE paths in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/correlationConfigDetails/id` | `isc.SOURCE.correlation-config-details-id` |
| `/managerCorrelationRule/id` | `isc.SOURCE.manager-correlation-rule-id` |

These declarations live in `internal/iscprofile/packs/source.json` and apply to
all SOURCE connector types. Names, types, correlation details, and other fields
remain comparable. The same paths on other configuration types are unchanged.
Use `disabled_rules` to restore comparison for an individual exclusion.

### SOURCE before-provisioning rule references

All ISC profiles exclude `/beforeProvisioningRule/id` in drift and deployment
using `isc.SOURCE.before-provisioning-rule-id`. This reference ID is specific to
the tenant. The rule reference's `name`, `type`, and other fields remain
comparable; changing or removing a name or type still produces a finding.
Deployment still requires independent expectations for the comparable fields.
The exclusion applies only to SOURCE objects and this exact ID path.

### SOURCE schemas and attributes

All ISC profiles automatically match `/schemas` by `name` within each source and
`/schemas/*/attributes` by `name` within each schema. Names must be present,
nonblank strings and unique within their containing array. Duplicates or invalid
names produce conflicts and exit `2`. Attribute names can repeat in different
schemas; schema names can repeat in different sources.

`nativeObjectType` is compared as configuration data, not used as a matching key.
Repeated native object types are allowed. A change to the native object type of a
matched schema remains a reported difference.

Findings show each input's schema name, native object type when present, and
attribute name beside the original JSON Pointer. For example,
`/schemas/0/attributes/42/schema/id` is accompanied by its containing schema and
attribute labels; reordered inputs retain their own indices. Missing counterparts
are not given another item's labels. HTML, Markdown, JSON, omission, and sensitive
redaction use the same context behavior as provisioning findings.

The built-in rules are `isc.SOURCE.schemas` and `isc.SOURCE.schema-attributes`.
Redundant custom rules at these paths can be removed to use the built-ins.
Intentional structural changes must declare `replaces` with the corresponding ID. Replacements define their own
`item_label` and `display_fields`; include these to retain readable context.
Schema `id`, `created`, and `modified` exclusions remain specific to the portable
profile. All ISC profiles exclude `/schemas/*/attributes/*/schema/id` using
`isc.SOURCE.schema-attribute-reference-id`, so tenant-specific reference IDs do
not cause differences. Reference `name` and `type`, other attribute properties,
and schema/attribute additions or removals remain comparable.
Deployment alignment still requires independent expectations for member values;
use wildcard item selectors for assertions beneath these keyed arrays.

### SOURCE provisioning policies

All ISC profiles automatically align `/provisioningPolicies` by `usageType`.
For `CREATE_MACHINE_ACCOUNT`, the key is the pair `(usageType, subtypeId)`, following
SailPoint's subtype-specific policy model. Each policy's `fields` array aligns by
`name`. Policy/field array order does not produce drift.

Usage types, required subtype IDs, and field names must be nonblank strings.
Duplicate ordinary usage types, duplicate machine usage/subtype pairs, duplicate
field names, and missing/invalid key components produce conflicts and exit `2`.
Different subtype IDs remain different identities; cross-environment subtype-ID
mapping is not inferred from policy names.

All ISC profiles exclude `id`, `created`, and `modified` directly on each
provisioning policy and field:

| Item | Excluded paths |
| --- | --- |
| Policy | `/provisioningPolicies/*/id`, `/provisioningPolicies/*/created`, `/provisioningPolicies/*/modified` |
| Field | `/provisioningPolicies/*/fields/*/id`, `/provisioningPolicies/*/fields/*/created`, `/provisioningPolicies/*/fields/*/modified` |

Other properties, additions/removals, and nested IDs remain comparable. Transform
objects and arrays retain their current comparison semantics; transform array
ordering is unchanged. Deployment still requires independent expectations for
non-excluded member values.

Policy and field labels are declared in the SOURCE profile's collection rules;
the core differ does not recognize provisioning-specific paths or names.

The built-in rules are `isc.SOURCE.provisioning-policies`,
`isc.SOURCE.provisioning-fields`, `isc.SOURCE.provisioning-policy-id`,
`isc.SOURCE.provisioning-policy-created`, `isc.SOURCE.provisioning-policy-modified`,
and the corresponding `isc.SOURCE.provisioning-field-*` exclusions. A custom
collection at either built-in path must explicitly use `replaces` with its rule ID.

See [the provisioning policy API](https://developer.sailpoint.com/docs/api/create-provisioning-policy-v-2/)
and [subtype policy documentation](https://developer.sailpoint.com/docs/api/create-source-subtype-provisioning-policy-v-2/).

### Selectors, rule options, and precedence

Value rules select a `type`, a JSON Pointer `path`, and optionally one exact object
logical `key`, plus optional adapter-classified `variant`. Policy paths are **payload-relative**:

| Selector | Meaning |
| --- | --- |
| `"path":"/connectorAttributes/host"` | Payload field `object.connectorAttributes.host` |
| `"path":"/schemas/*/name"` | Each schema item's name |
| `"path":"/a~1b/~0value"` | Member `a/b`, then member `~value` |
| `"path":""` | Entire selected section |
| `"space":"metadata","path":"/self/id"` | Envelope metadata rather than payload content |

`space` defaults to `content`. `*` matches exactly one map member or array item;
recursive wildcards, filters, regexes, and literal `*` member selectors are not
supported. Identity and collection-key pointers must be concrete. Under keyed
arrays, use wildcard item selectors; ordered arrays also allow concrete indices.

Every rule includes `id`. Optional common fields are:

| Field | Meaning |
| --- | --- |
| `modes` | Restrict application to `drift` or `deployment`; identity/pairing rules are unconditional, while equivalence/reference rules are drift-only |
| `required` | If true, an unused rule makes the comparison incomplete |
| `replaces` | Explicitly replace a profile rule of the same category and ownership |
| `evidence` | Attribution object with required `kind`, optional `digest`, `record_id`, and string-valued `attributes` |

File order never determines precedence. Conflicting active rules, including an
ignore that could hide an expectation, are rejected. To override a profile rule,
name its ID with `replaces`; adding another overlapping rule is not an override.
Use `disabled_rules` for an explicit opt-out, or `collection_mappings` to add
implementation mappings without changing structure. Profile rule IDs are documented in the [policy contract](docs/policy.md#isc-profiles).

### Equivalence: approved drift differences

Use the complete [environment mapping walkthrough](#configure-environment-specific-mappings)
and [`examples/policy-drift.json`](examples/policy-drift.json). Each equivalence
rule connects an object/field selector, input environment labels, and approved
whole-value pairs. Optional `groups` can replace `pairs` for explicitly symmetric
relationships, such as `[{"dev":"dev-host","prod":"prod-host"}]`.
Do not combine both forms. Equivalence rules run only in drift mode.

### Expectations: required states and values

An expectation asserts what a field must be, even when both compared values are
incorrectly equal. A deployment policy fragment is:

```json
{
  "expectations": [
    {
      "id": "production-host",
      "type": "SOURCE",
      "path": "/connectorAttributes/host",
      "modes": ["deployment"],
      "operation": "literal",
      "values": {
        "prod": {"state": "present", "value": "directory-prod.example.invalid"}
      },
      "authority": "release-policy",
      "evidence": {"kind": "approved-policy", "record_id": "release-1"}
    }
  ]
}
```

| `operation` | Expected result |
| --- | --- |
| `literal` | Use exactly one of `expected` (one tagged state) or `values` (environment-to-tagged-state object) |
| `absent` | Require a missing field; no expected value is supplied |
| `copy_source` | Preserve the source field/state |
| `preserve_target` | Preserve the corresponding baseline field/state |
| `reference_target` | Resolve the source reference to a corresponding baseline object's ID; requires `target_type` |

Tagged states are `{"state":"absent"}` and
`{"state":"present","value":...}`. A required present `null` is
`{"state":"present","value":null}`. For derivation operations, omitted `from`
uses the aligned field; `"from":"/otherField"` selects a concrete source/baseline
path in the same section; explicit `"from":""` selects its root.

An expectation needs nonempty `authority` and `evidence.kind` to verify a result.
These record your declared authority and attribution; the tool does not validate
an external approval system. Unavailable authority, environment values, baselines,
or references result in `unverified`. Only `literal`/`absent` assertions run in
drift; enable derivation operations for deployment. For example, a drift assertion
can specify both `dev` and `prod` states in `values` and verify each side separately.

### Matching, references, arrays, and exclusions

The other policy arrays use the following syntax:

| Array | Rule fields and behavior |
| --- | --- |
| `identities` | `type`, `strategy` (`fields`, `singleton`, `dependent`); `paths` for fields; `parent_type`/`parent_path` for dependencies; optional `component_type` (`scalar` or `string`) |
| `pairings` | `type`, `left_id`, `right_id`, optional `baseline_id`; explicitly relate physical objects; deployment interprets left/right as source/actual |
| `references` | Selector plus `target_type`, optional one-to-one `external` ID map; compare references through object correspondence in drift |
| `collection_mappings` | `collection` rule ID, optional object `key`, and `key_mappings`; inherits the collection definition |
| `collections` | Selector plus `mode` (`ordered`, `keyed`, `multiset`); `keys`, `component_type`, `key_cases`, and `key_mappings` for keyed arrays; optional `item_label` and `display_fields` for report context |
| `ignores` | Selector; explicitly exclude that subtree from comparison |
| `sensitive` | Selector; conservatively redact values, object identity and physical provenance in reports |

For example, this fragment explicitly replaces the built-in schema rule and adds
a source-specific runtime-field exclusion. The attribute rule remains active:

```json
{
  "collections": [
    {
      "id": "schemas-by-name",
      "replaces": "isc.SOURCE.schemas",
      "type": "SOURCE",
      "path": "/schemas",
      "mode": "keyed",
      "keys": ["/name"],
      "component_type": "string",
      "item_label": "Schema",
      "display_fields": [
        {"label": "Name", "path": "/name"},
        {"label": "Native object type", "path": "/nativeObjectType"}
      ]
    }
  ],
  "ignores": [
    {"id":"last-run","type":"SOURCE","path":"/connectorAttributes/lastRun","modes":["drift"]}
  ]
}
```

Maps are unordered. Arrays default to ordered comparison. Keyed arrays need
unique, present, nonnull scalar keys; `keys` pointers are relative to each item.
Multisets ignore order while retaining duplicate counts. Nested keyed arrays are
supported. Do not sort workflow choices or transform inputs without an explicit
semantic reason. Declared ordering normalization alone does not verify deployment
member values.

Keyed collections can select a complete alternative key tuple with `key_cases`.
Each case has a concrete item-relative `when_path`, a nonnull scalar `when_value`,
and a nonempty `keys` array. All cases in one rule must use the same discriminator
path and have distinct typed values; this avoids first-match precedence. An item
that matches no case uses the rule's default `keys`. Key pointers are concrete and
item-relative. `component_type` defaults to `scalar`; `string` requires every key
component to be a nonblank string, preserving the original spelling.

Keyed collections can also declare environment-specific `key_mappings` for one
or more key components. Each mapping specifies a concrete item-relative key
`path` and `groups` of logical `key` plus environment-indexed `values`. It changes
correspondence only; see the [forest mapping example](#forest-names-that-differ-between-environments)
for a complete policy and the [policy contract](docs/policy.md#collection-key-mappings)
for validation and fallback behavior.

Collection rules may also declare `item_label` and `display_fields` for readable
report context. Each display field has a nonblank, unique `label` and a concrete
item-relative `path`. Only present, nonblank scalar values are captured; absent,
null, blank, or structured values are skipped. These annotations do not change
matching or validation. Treat the selected fields as visible identity metadata,
including when payload values are excluded. Sensitive redaction still applies.

This policy shows the built-in provisioning key definition as an explicit
replacement; the built-in field alignment remains active:

```json
{
  "version": 2,
  "id": "custom-provisioning-alignment",
  "profile": "isc-portable-v2",
  "collections": [
    {
      "id": "provisioning-by-usage-and-subtype",
      "replaces": "isc.SOURCE.provisioning-policies",
      "type": "SOURCE",
      "path": "/provisioningPolicies",
      "mode": "keyed",
      "component_type": "string",
      "item_label": "Provisioning policy",
      "display_fields": [
        {"label": "Name", "path": "/name"},
        {"label": "Usage type", "path": "/usageType"},
        {"label": "Subtype ID", "path": "/subtypeId"}
      ],
      "keys": ["/usageType"],
      "key_cases": [
        {
          "when_path": "/usageType",
          "when_value": "CREATE_MACHINE_ACCOUNT",
          "keys": ["/usageType", "/subtypeId"]
        }
      ]
    }
  ]
}
```

ISC profile identity components are nonempty strings. Unknown object types need
an identity rule. Overriding a built-in identity requires an explicit replacement,
such as `"replaces":"isc.identity.SOURCE"`. Pairing renamed objects does not hide
the name difference. Reference IDs are resolved only at declared paths; IDs are
not stripped globally.

## Use as a Go library

The root package is named `differ`. This function compares two validated snapshots
using a policy file; its environment labels match the included drift example:

```go
package example

import (
    differ "github.com/elianderson/iscutilities/configdiffer"
    "github.com/elianderson/iscutilities/configdiffer/v2"
    "github.com/elianderson/iscutilities/configexporter/snapshot"
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
`differ.Compile` or `Compare` to expand built-in ISC profiles. The root package also exposes `DecodePolicy` and aliases for the current `Policy`
and `Report` types. Use `LoadPolicy(path)` to read a root policy and its fragments.
`DecodePolicy(bytes)` and `v2.Decode(bytes)` decode a self-contained policy without
filesystem access and reject `includes`. File composition belongs to the adapter;
the neutral compiler receives one assembled policy.

For a separate consuming Go module, replacement directives in a dependency's
`go.mod` are not inherited. Until published versions are used, add replacements
for **both** modules in your application's `go.mod`, using your local paths:

```gomod
go 1.27.2

require (
    github.com/elianderson/iscutilities/configdiffer v0.0.0
    github.com/elianderson/iscutilities/configexporter v0.0.0
)

replace github.com/elianderson/iscutilities/configdiffer => /path/to/iscutilities/configdiffer
replace github.com/elianderson/iscutilities/configexporter => /path/to/iscutilities/configexporter
```

These entries go below your application's own `module` declaration. Run
`go mod tidy` after adding imports.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Build cannot find `configexporter` | Check the sibling directory and `go.mod` replacement; external consumers need their own replacements |
| Selected scopes differ | Supply matching input type declarations or an explicit shared `--scope` |
| Invalid artifact or unexpected JSON | Check envelope format, hexadecimal filename, and whether policies/mappings were placed inside an input tree |
| Checksum/count failure in snapshot mode | Use an intact exported generation; choose tree mode explicitly for intended edits |
| Identity unavailable or ambiguous | Check exact names, parent objects, duplicates, and explicit strategies for unknown types |
| Overlapping rule/profile error | Use `collection_mappings` for environment mappings; remove competing ownership or use an explicit same-category replacement for structural changes |
| Connector rule does not apply | Check the exported connector metadata against the shipped pack identifiers; source display name and array shape are not classifiers |
| Unknown collection mapping target | Reference the effective collection ID; a disabled or replaced target cannot be extended |
| Unused required selector | Check type, object key, path, available collection items, and rule mode |
| Deployment remains unverified | Inspect uncovered fields, authority/evidence, environment values, baseline availability, and reference correspondence |
| Hub context mismatch | Check source/target direction, resolved environments, manifest tenants, and explicit `default` binding |

## Development and further documentation

From this module, run:

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Run the same checks from the sibling exporter when changing shared input
contracts. Automated fixtures use invented data and temporary directories; they
do not require credentials or tenant exports. Format changed Go files with
`gofmt` and check whitespace with `git diff --check`.

- [Policy and report contract](docs/policy.md): exact fields, semantics, and adapter boundaries.
- [Export input contract](docs/contracts.md): snapshot format and validated readers.
- [Exporter mapping retrieval](../configexporter/docs/MAPPINGS.md): optional retrieval and bundle provenance.
- [Implementation record](docs/implementation.md) and [correctness review](docs/review.md): acceptance evidence and compatibility notes.
