# Organize policies across files

CLI examples assume you have [built the tool](../README.md#build-and-install)
and are running commands from the configdiffer repository root.

Use one root policy to select the profile and shared settings, and include any
number of **rule-only fragments**. This lets you maintain source mappings by
connector and keep transform mappings in another file. Continue passing only the
root file to `--policy`; run the command from any directory.

For a complete example, see [policy-includes.json](../examples/policy-includes.json)
and its [rule fragments](../examples/policy-rules).
[policy-drift.json](../examples/policy-drift.json) shows the same policy syntax in a
single file.
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
it no special precedence. Conflict checks, explicit `replaces`, and
`disabled_rules` work across the assembled policy. A more specific selector does
not override a broader rule. Keep using the shipped common,
SOURCE, and connector defaults; fragments organize your custom rules
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
