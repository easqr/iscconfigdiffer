# Authoring shipped rule packs

Contributor reference for changing the defaults embedded in the executable.
For selecting profiles and writing custom policies, see [shipped rules](../rule-packs.md).

Each file has this shape (the full runnable definitions are in [`internal/iscprofile/packs`](../../internal/iscprofile/packs)):

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
engine work. User exceptions belong in custom comparison policies.

## Checks

Run the [contributor checks](../DEVELOPMENT.md#local-checks) after changing packs.
