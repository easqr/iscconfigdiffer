# Shipped rule packs and implementation policies

The comparison engine implements generic mechanisms. ISC domain defaults are
embedded JSON files in [`internal/iscprofile/packs`](../internal/iscprofile/packs).
An implementation policy adds environment decisions without copying those files.

| Layer | Packs | Responsibility |
| --- | --- | --- |
| Common ISC defaults | `common`, `envelope`, `portable` | Identities, null handling, envelope expectations, selected metadata exclusions |
| SOURCE defaults | `source`, `source-portable` | Schema/provisioning structure and specific generated/reference ID exclusions |
| Connector defaults | `active-directory`, `microsoft-entra` | Connector-specific array keys, conditional applicability, display labels |
| Implementation | The user's policy | Environment mapping extensions, approved differences, authoritative expectations, explicit overrides |

Profile names select the pack presets. `isc-identity-v2` includes common,
SOURCE, and connector semantics; `isc-envelope-v2` also includes envelope
normalization; `isc-portable-v2` additionally includes portable metadata
normalization. Connector rules apply only to matching classified documents.
A neutral policy with no ISC profile still uses its own authored rules.

## Authoring shipped packs

Each file has this shape (the full runnable definitions are in the pack directory):

```json
{
  "id": "isc.example",
  "version": 1,
  "layer": "connector",
  "profiles": ["isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"],
  "connector": {
    "variant": "example-connector",
    "identifiers": [
      {"path":"/connectorAttributes/connectorName","values":["Example Connector"]}
    ]
  },
  "rules": {
    "version": 2,
    "id": "isc.example",
    "collections": [
      {
        "id": "isc.SOURCE.example-items",
        "type": "SOURCE",
        "variant": "example-connector",
        "path": "/connectorAttributes/items",
        "mode": "keyed",
        "keys": ["/name"],
        "component_type": "string",
        "item_label": "Item",
        "display_fields": [{"label":"Name","path":"/name"}]
      }
    ]
  }
}
```

Pack version `1` describes this wrapper format; `rules.version:2` describes the
independent comparison policy schema. Pack and nested policy IDs must agree.
Layers are `common`, `type`, or `connector`. Packs cannot assign request bindings,
request modes, implementation collection extensions, or disabled rules.

Rules use the same strict policy decoder and compiler as implementation rules.
The wrapper rejects duplicate JSON properties, unknown fields, unsupported pack
versions, invalid connector declarations, and connector rules escaping their
SOURCE/variant scope. Connector identifiers are exact, case-sensitive strings at
concrete payload-relative pointers. Their declaration is allowed only for a
connector pack. No arbitrary executable expressions or remote retrieval exist.

Common selector rules may use `type:"$TYPE"` and `$TYPE` in their rule ID as
loader templates. They expand over the shipped identity inventory plus explicitly
supplied implementation identity types. This retains common exclusions and
structural expectations for custom types. The engine receives only expanded,
validated rules; it knows nothing about this ISC template syntax.

Do not author per-rule evidence in packs. The loader attaches `isc-profile`
evidence with the selected profile, pack ID, pack version, layer, and SHA-256 of
the embedded file. The report also inventories selected packs as `isc-rule-pack`
evidence. Every effective rule is included in profile/policy digest calculation.

Maintainers edit JSON, run the test/build checks, and ship a new executable.
Adding a supported selector, key, exclusion, label, or connector alias ordinarily
requires no Go rule declarations. A new comparison mechanism still requires
engine work. Users do not need to modify the packs. Runtime loading of external
pack files is not implemented; user exceptions belong in the implementation
policy.

## Connector classification

The ISC adapter resolves `Document.variant` from recognized identifiers before
passing documents to the neutral engine. Any recognized identifier can establish
the variant, but recognized identifiers must agree. Unknown values do not claim a
variant; they remain ordinary comparable configuration data. An unknown identifier
alongside a recognized identifier does not contradict that recognized identity.

The AD pack recognizes its exported connector class, direct connector type, and
connector name. The Entra pack recognizes `connectorName:"Microsoft Entra"`.
Source display names, tenant-specific connector UUIDs, and array shapes are not
classifiers. Renaming a source cannot activate a different connector pack.
Unclassified sources use common/SOURCE rules and ordinary array comparison.
Conflicting recognized identities fail adaptation. Different variants among
corresponding source/actual/baseline documents produce a comparison conflict.

## Implementation extensions and overrides

Use `collection_mappings` to reference an effective keyed rule by ID and attach
only environment mappings. An optional `key` restricts the extension to a source's
logical name. It inherits structural keys, selector variant, and display labels.
See the [drift example](../examples/policy-drift.json).

Use a same-category, same-selector `replaces` rule for an intentional structural
change. Preserve `variant` when replacing connector rules. Use `disabled_rules`
to deliberately omit a shipped rule. Unknown/duplicate disable IDs, competing
replacements/extensions, widening replacement ownership, and extending disabled
or replaced IDs fail compilation. There is no implicit last-writer precedence.

Collection alignment does not accept different values or authorize a deployment.
Drift equivalences and deployment expectations remain separately authored.
Implementation additions are covered by the effective policy digest; extensions,
replacements, disable decisions, and pack origin are visible in reports.
