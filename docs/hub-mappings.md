# Configuration Hub mapping files and evidence

CLI examples assume you have [built the tool](../README.md#build-and-install)
and are running commands from the configdiffer repository root.

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
`$`, `.member`, quoted bracket members, `[0]`, and `[*]`. Values remain strings. Unsupported
active semantics fail import; disabled mappings stay inert evidence.

The exporter creates the bundle's `provenance.json`, recording schema, retrieval
ID/time, source organization, target tenant, API origin/version/endpoint, record
count, SHA-256 of `mappings.json`, and optional export-job correlation. Keep the
original files together; their context and checksum are validated on import.
The sidecar is distinct from a configuration snapshot manifest. See the
[exporter mapping guide](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/docs/MAPPINGS.md) for retrieval.

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
source-guarded expectations. Do not duplicate an imported rule's
field with another active value rule. No parity with undocumented Hub global
substitution rules is assumed.
