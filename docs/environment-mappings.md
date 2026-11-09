# Configure environment-specific mappings

CLI examples assume you have [built the tool](../README.md#build-and-install)
and are running commands from the configdiffer repository root.

**For a normal comparison between environments, put your environment mappings in
one policy JSON file and pass it with `--policy`.** Start with
[`examples/policy-drift.json`](../examples/policy-drift.json).
[`examples/policy.json`](../examples/policy.json) is a smaller example with **no
environment mappings**; it only selects built-in matching and metadata handling.

A policy is the tool's comparison instructions. Environment mappings are one
part of a policy, called `equivalences`. Other optional parts control object
matching, array ordering, exclusions, and required values. There is no separate
mandatory mapping file for ordinary drift comparison.

## 1. Identify the object, field, and environment values

For example, suppose the two exports contain a source named `Invented Directory`:

| Item | Development export | Production export |
| --- | --- | --- |
| `object.name` | `Invented Directory` | `Invented Directory` |
| `object.connectorAttributes.host` | `directory-dev.example.invalid` | `directory-prod.example.invalid` |
| `object.connectorAttributes.timeout` | `60` | `60` |

The objects match by their common name. You want to allow the two hostnames while
still reporting an unexpected difference in `timeout` or another ordinary field.

## 2. Save a complete policy file

Save the following complete policy outside your export directories, for example
as `/policies/dev-to-prod.json`. It allows one hostname pair for one source.
The repository's `examples/policy-drift.json` contains this rule plus additional
mapping examples; use it when you want to explore those cases.

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
[matching and references](configuration.md#matching-references-arrays-and-exclusions); a field
mapping does not establish object correspondence.

## 3. Assign environments to the two inputs and run

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
`--right-types SOURCE`. The [quick start](quick-start.md#quick-start-with-sample-data) creates runnable sample
data so you can try the workflow without tenant exports.

## 4. Read the result and extend the policy

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
[configuration reference](configuration.md#configuration-reference) explains the other rule
categories; you can leave them out until needed.

## Forest names that differ between environments

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

Replace `Invented Directory` and the example values with those from your exports.
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
you rename it. See [shipped rules and custom policies](configuration.md#shipped-rules-and-custom-policies)
for how connectors are identified and how to override defaults.

Mapped names remain original values in the report. Alignment does not itself
approve different values or verify a deployment. Other forest settings and
membership changes remain comparable. Domain settings use their own mappings;
there is no implicit propagation to references elsewhere.

## Domain settings between environments

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

[`examples/policy-drift.json`](../examples/policy-drift.json) contains both mapping
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

## Different service accounts for multiple domains

Use an equivalence's `item` selector to restrict approval to one logical domain.
Keep the field path `/connectorAttributes/domainSettings/*/user`; the engine
aligns domains by their complete mapped forest/domain tuple before evaluating
this rule. No array index is used in the policy.

The [example drift policy](../examples/policy-drift.json) includes two domains in
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
or target another keyed collection.

## Map a provisioning field within a specific policy

Provisioning policies and their fields are nested keyed arrays. Use `items` when
an approval must match both ancestors. For example, this rule from the
[drift example](../examples/policy-drift.json) approves only the exported CREATE
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
other attributes, or the field's flags. The same selectors can combine schema
and attribute identities or other nested collection keys. This approves only
drift differences; deployment requires independent expectations.
