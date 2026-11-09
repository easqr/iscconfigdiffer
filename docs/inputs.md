# Input format and scope

CLI examples assume you have [built the tool](../README.md#build-and-install)
and are running commands from the configdiffer repository root.

Inputs use the format produced by the
[`configexporter`](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/README.md) module:

```text
snapshot-root/
  manifest.json
  SOURCE/
    id-61.json
  ROLE/
    id-726f6c652d31.json
```

Every object file uses the exporter envelope containing `self.type`, `self.id`, and
an object-valued `object` payload. The [sample-data walkthrough](quick-start.md) creates files in this format.

Object paths are derived from the export envelope's `self.type` and `self.id`:
`<self.type>/id-<lowercase hexadecimal encoding of the UTF-8 bytes of self.id>.json`.
The ID is read from `self.id`, rather than the nested `object.id` or `self.name`.
Each byte becomes two hexadecimal characters, so a 32-character ASCII ID produces
64 hexadecimal characters in the filename. This is a deterministic, reversible
encoding, not a generated GUID, a hash, or encryption.

For example, an envelope containing:

```json
{
  "version": 1,
  "self": { "type": "ROLE", "id": "abc123", "name": "Example role" },
  "object": {}
}
```

is written to `ROLE/id-616263313233.json`. Hex encoding preserves the exact ID
while avoiding special characters, substitution collisions, and case collisions
on filesystems that treat uppercase and lowercase filenames as equivalent.
The original `self.id` and `self.name` remain readable inside the JSON file.

If the payload includes `id`, it must equal `self.id`. Arbitrary JSON payload
files or filenames are not accepted.

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
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --scope SOURCE --name "Active Directory" \
  --policy policy.json
```

Use your own export paths and policy file. `--name` is an exact, case-sensitive
logical key, normally the object's `name` under ISC profiles. It is not a physical
ID, substring, or pattern. For custom/composite identities, supply the logical key
shown in the report; explicit pairings use the source/left key. Omit `--name` to
compare all objects permitted by the policy within the selected types.

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


## Include or exclude named configurations by type

Keep using `--scope` to choose types. For reusable name lists across those types,
add `object_filters` to the **root policy**:

```json
{
  "version": 2,
  "id": "filtered-drift",
  "profile": "isc-portable-v2",
  "object_filters": {
    "SOURCE": {
      "exclude": ["Dev - Active Directory"]
    },
    "ROLE": {
      "include": ["Employee Access", "Finance Analyst"]
    }
  }
}
```

With `--scope SOURCE,ROLE`, this compares every source except
`Dev - Active Directory`, and only the two listed roles. Other types are selected
only when included in the command's type scope. A type without an `object_filters`
entry keeps its ordinary scope behavior. See the complete
[filtered policy example](../examples/policy-filtered-drift.json).

- `include` is a whitelist: when supplied, only those logical names are validated
  for that type. Omit it to permit all names not excluded.
- `exclude` is a blacklist. It wins when a name occurs in both lists.
- Names are exact and case-sensitive. There are no wildcards, regular expressions,
  substrings, or physical-ID matching. Composite identities use the logical key
  shown in reports; explicit pairings use the left/source key, just like `--name`.
- Omit unused lists instead of writing empty arrays. Blank or duplicate names,
  empty filter entries, and types without an identity strategy fail compilation.
- A listed include name missing from both primary inputs and any baseline makes
  the active comparison incomplete. A name present on one side still produces
  an object-presence finding. An unused exclusion is reported as a diagnostic
  without failing the result. Checks apply only to types in scope and names not
  narrowed away by `--name`.
- Filters that select no configurations in the active scope produce an incomplete
  result (exit `2`). `--name` narrows the policy lists further; it cannot re-enable
  a configuration excluded by policy.

These lists work in drift and deployment. Excluded configurations and operations
are not validated, but remain context for references, dependent identities, and
relative priority comparisons. A referenced target scheduled for deletion is
still treated as such, even when its own validation is excluded. Input integrity,
identity matching, policy compilation, and required-rule checks remain enforced;
a filter is not permission to accept malformed exports or ambiguous identities.
A filtered result makes no claim about the excluded configurations.

Reports list **Configurations excluded by policy**, including each name and reason.
JSON includes `object_filters` and `excluded_objects`; `*_excluded_by_policy` and
`excluded_groups` distinguish policy exclusions from `--name` filtering and value
normalization. Filters contribute to policy/request digests. Sensitive type rules
also redact filter names, skipped-object identities, and named diagnostics.

`object_filters` is a root-policy setting, not a rule array. Keep it out of
rule-only include fragments; the same names apply consistently to both inputs.
Your existing local policy need not change unless you want to enable filtering.

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
  [operations walkthrough](deployment.md#deployment-operations-file) for what each action checks.
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
