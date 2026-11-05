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

- [Configure environment-specific mappings](#configure-environment-specific-mappings)
- [Which files do I need?](#which-files-do-i-need)
- [Build and install](#build-and-install)
- [Quick start](#quick-start)
- [Input format and scope](#input-format-and-scope)
- [Compare exports and validate drafts](#compare-exports-and-validate-drafts)
- [CLI options](#cli-options)
- [Configuration files](#configuration-files)
- [Use as a Go library](#use-as-a-go-library)
- [Reports and exit codes](#reports-and-exit-codes)
- [Troubleshooting](#troubleshooting)
- [Development and further documentation](#development-and-further-documentation)

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

After [building the CLI](#build-and-install), compare your actual export roots:

```sh
./bin/isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --scope SOURCE \
  --policy /policies/dev-to-prod.json \
  --format json > drift-report.json
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
[configuration reference](#configuration-files) below explains the other rule
categories; you can leave them out until needed.

## Which files do I need?

For ordinary environment-aware drift, your **policy file is the only file you
write for configuration**. The exports are generated input data. The deployment
and Hub files below are needed only for those workflows.

| Workflow | Files you author | Inputs generated elsewhere | Options |
| --- | --- | --- | --- |
| Compare environments with your own approved mappings | One policy containing `equivalences` (start from `examples/policy-drift.json`) | Left and right configuration exports | `--mode drift --policy FILE` plus input paths/environments |
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
  or `delete` actions. It contains no environment value mappings.
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
an object-valued `object` payload, as shown in the quick start. The canonical
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

Roots can be separate directories or root symlinks. Nested links and special
files are rejected. Keep editable trees unchanged while they are being read.
**Keep policies, operations, mappings, and unrelated JSON outside configuration
roots**: tree mode expects configuration `.json` artifacts there, apart from its
manifest and excluded version-control administration directories.

## Compare exports and validate drafts

### Drift between snapshots

```sh
./bin/isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --scope SOURCE \
  --policy examples/policy-drift.json \
  --format json > drift-report.json
```

Use `tree` for an edited working copy and `snapshot` for an immutable export; the
two sides can use different reading modes. Environment labels such as `dev` and
`prod` are policy lookup keys. They are distinct from the tenant identity recorded
in a manifest and must match the labels used in your policy.

### Deployment validation

```sh
./bin/isc-configdiffer --mode deployment \
  --source /data/dev --source-mode snapshot --source-env dev \
  --actual /data/draft --actual-mode tree --actual-types SOURCE --actual-env prod \
  --baseline /data/prod --baseline-mode snapshot --baseline-env prod \
  --scope SOURCE \
  --operations examples/operations.json \
  --policy examples/policy-deployment.json \
  --format json > deployment-report.json
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

## CLI options

For the flag listing, run `./bin/isc-configdiffer --mode drift --help`.
The current CLI returns `2` after displaying help. All filenames and directories
are resolved relative to the current working directory unless absolute.

| Option | Default | Meaning |
| --- | --- | --- |
| `--mode` | `drift` | `drift` or `deployment` |
| `--policy FILE` | Built-in `default` / `isc-envelope-v2` | Read a comparison policy JSON file using schema `2` |
| `--scope TYPES` | Full declared type set; primary sets must match | Explicit comma-separated type subset available in all inputs |
| `--format` | `text` | `text` or `json` |
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

## Configuration files

There is no automatically discovered CLI configuration file. Pass every policy,
operation list, mapping input, or evidence file explicitly. Their filenames are
arbitrary; their JSON schema determines their purpose.

| File | Who supplies it | Use |
| --- | --- | --- |
| `manifest.json` and object envelopes | Exporter, or a correctly structured edited tree | Comparison input data and provenance |
| Policy, e.g. `portable-policy.json` | You | Matching, normalization, equivalence, and expectations; pass with `--policy` |
| `examples/policy-drift.json` | Included example | Approved hostname equivalence for `dev` to `prod` |
| `examples/policy-deployment.json` | Included example | Field-by-field deployment expectations |
| `examples/operations.json` | Included example to adapt | Intended objects/actions for deployment; pass with `--operations` |
| `mappings.json` | Saved Hub response or exporter mapping retrieval | Optional vendor mapping input; pass the file or its bundle with `--hub` |
| Mapping `provenance.json` | Exporter mapping retrieval | Validates a bundle's integrity and records retrieval context |
| Deployment evidence JSON | You | Authority attribution for Hub deployment promotion |
| `examples/policy.json` | Included minimal example | Matching and standard metadata exclusions only; **no environment mappings** |

Exporter authentication files such as `config.json` and `exporter.json` belong to
`configexporter`; `configdiffer` does not read them.

### Policy syntax and profiles

Policy files are strict JSON: no comments, trailing commas, duplicate keys,
unknown properties, or case-insensitive property aliases. Policy/rule IDs contain
1–200 ASCII letters/digits or `_ . : -`, starting with a letter/digit. Rule IDs
must be unique across all categories. Values retain JSON types; strings are not
converted to numbers or booleans, and missing values differ from `null`.

A minimal comparison policy, suitable as `portable-policy.json`, is:

```json
{
  "version": 2,
  "id": "portable-comparison",
  "profile": "isc-portable-v2"
}
```

With this policy, ordinary fields are compared literally after the declared
profile's exclusions. It does not add environment-specific hostname equivalence.

| Root property | Meaning |
| --- | --- |
| `version` | Required `2` for the comparison policy schema |
| `id` | Required stable policy identifier |
| `profile` | Optional named ISC identity/metadata profile |
| `modes` | Optional allowed comparison modes, e.g. `["drift"]` |
| `bindings` | Optional role-to-environment object, e.g. `{"left":"dev","right":"prod"}`; deployment uses `source`, `actual`, `baseline` |
| Rule arrays | The categories described below; omitted arrays are empty |
| `evidence` | Optional array of attribution records |

Built-in ISC profiles:

| Profile | Drift behavior |
| --- | --- |
| `isc-identity-v2` | Supplies logical identities; compares all content and envelope metadata |
| `isc-envelope-v2` | Also excludes envelope JWS header/signature fields; used when `--policy` is omitted |
| `isc-portable-v2` | Also excludes envelope `/self/id`, payload `/id`, `/created`, `/modified`, and lifecycle-state `/identityCount` |

All three include deployment structural expectations for envelope version/type.
They do not ignore nested IDs, owners, arbitrary runtime tokens, or array order.
Choose the portable profile explicitly when differences in generated IDs and
these metadata fields should be excluded. A supplied policy without a profile
must define its own identity strategies; no ISC profile is added implicitly.

Objects normally match by exact, case-sensitive name within their type, regardless
of physical ID. Notification templates use key/locale/medium; lifecycle states
include their parent identity profile; attribute-sync configurations use their
parent source. `ACCESS_REQUEST_CONFIG` and `AUTH_ORG` are singletons. Ambiguous
identities do not pass. See the [identity contract](docs/contracts.md#scope-and-identities)
for the named-type list.

### Selectors, rule options, and precedence

Value rules select a `type`, a JSON Pointer `path`, and optionally one exact object
logical `key`. Policy paths are **payload-relative**:

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
Profile rule IDs are documented in the [policy contract](docs/policy.md#isc-profiles).

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
| `collections` | Selector plus `mode` (`ordered`, `keyed`, `multiset`); `keys` for keyed arrays only |
| `ignores` | Selector; explicitly exclude that subtree from comparison |
| `sensitive` | Selector; conservatively redact object identity and physical provenance in reports |

For example, this fragment defines a keyed schema array and a source-specific
runtime-field exclusion:

```json
{
  "collections": [
    {"id":"schemas-by-name","type":"SOURCE","path":"/schemas","mode":"keyed","keys":["/name"]}
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

ISC profile identity components are nonempty strings. Unknown object types need
an identity rule. Overriding a built-in identity requires an explicit replacement,
such as `"replaces":"isc.identity.SOURCE"`. Pairing renamed objects does not hide
the name difference. Reference IDs are resolved only at declared paths; IDs are
not stripped globally.

### Deployment operations file

Pass a JSON array with `--operations`:

```json
[
  {"type":"SOURCE","key":"Invented Directory","action":"update"}
]
```

`type` is an exact object type; `key` is its logical key, usually its name;
`action` is `create`, `update`, or `delete`. Use the source key for a renamed
pairing. Composite string keys are JSON tuple strings, for example
`"[\"request\",\"en_US\",\"EMAIL\"]"` for a notification template; singleton keys
are `"singleton"`.

Operations must be unique and within declared type scope. Unexpected draft
objects are unverified. Unselected source/baseline objects are outside operation
scope. Missing draft objects never imply deletion; an explicit delete requires
baseline evidence and checks projected absence. References to explicitly deleted
targets are violations.

### Configuration Hub mapping files and evidence

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

## Use as a Go library

The root package is named `differ`. This function compares two validated snapshots
using a policy file; its environment labels match the included drift example:

```go
package example

import (
    "os"

    differ "github.com/elianderson/iscutilities/configdiffer"
    "github.com/elianderson/iscutilities/configdiffer/v2"
    "github.com/elianderson/iscutilities/configexporter/snapshot"
)

func CompareSnapshots(leftDir, rightDir, policyPath string) (v2.Report, error) {
    var empty v2.Report
    data, err := os.ReadFile(policyPath)
    if err != nil {
        return empty, err
    }
    policy, err := v2.Decode(data)
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
`report.ExitCode()`, `report.Text(writer)`, or `json.NewEncoder(writer).Encode(report)`.
For deployment, `Request.Left` is the source, `Right` is the actual draft,
`Baseline` is an optional `*snapshot.Input`, and `Operations` is a
`[]v2.Operation`.

`snapshot.FromFiles` accepts exporter-format bytes in memory and applies the same
validation as the disk reader. Construct validated inputs with `Read` or
`FromFiles`; the ISC adapter rejects arbitrary or mutated `snapshot.Input`
structures. Comparison operates on detached representations. For neutral document
formats, use `v2.Compile` and `v2.Run` with your own profiles and adapters; use
`differ.Compile` or `Compare` to expand built-in ISC profiles. The root package also exposes `DecodePolicy` and aliases for the current `Policy`
and `Report` types.

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

## Reports and exit codes

Text and JSON reports go to standard output; input/policy/request errors go to
standard error. Use `--format json > report.json` for automation. A comparison can
produce an incomplete diagnostic report on stdout and still exit `2`.

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

Actual and expected payload values are omitted in both output formats. Names,
IDs, labels, and declared evidence may be visible as provenance. `sensitive`
selectors conservatively hide identity/provenance for the selected object type
and dependent types; put no secrets in labels or evidence metadata. `--matching`
adds ordinary equal leaves; policy decisions remain visible without it.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Build cannot find `configexporter` | Check the sibling directory and `go.mod` replacement; external consumers need their own replacements |
| Selected scopes differ | Supply matching input type declarations or an explicit shared `--scope` |
| Invalid artifact or unexpected JSON | Check envelope format, hexadecimal filename, and whether policies/mappings were placed inside an input tree |
| Checksum/count failure in snapshot mode | Use an intact exported generation; choose tree mode explicitly for intended edits |
| Identity unavailable or ambiguous | Check exact names, parent objects, duplicates, and explicit strategies for unknown types |
| Overlapping rule/profile error | Remove competing ownership or use an explicit same-category profile replacement |
| Unused required selector | Check type, object key, path, available collection items, and rule mode |
| Deployment remains unverified | Inspect uncovered fields, authority/evidence, environment values, baseline availability, and reference correspondence |
| Hub context mismatch | Check source/target direction, resolved environments, manifest tenants, and explicit `default` binding |

Exit `1` is an expected result when drift is found. If using a shell with
`set -e` or a CI runner, handle exit `1` separately from exit `2` when collecting
reports.

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
