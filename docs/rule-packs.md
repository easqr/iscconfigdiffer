# Shipped rules and custom policies

ISC profiles supply default identities, normalization, exclusions, and array
matching rules. Your custom policy adds environment-specific decisions and
exceptions.

| Layer | Packs | Responsibility |
| --- | --- | --- |
| Common ISC defaults | `common`, `envelope`, `portable` | Identities, null handling, envelope expectations, selected metadata exclusions |
| SOURCE defaults | `source`, `source-portable` | Schema/provisioning structure and specific generated/reference ID exclusions |
| IDENTITY_PROFILE defaults | `identity-profile` | Relative priority equivalence in drift |
| IDENTITY_OBJECT_CONFIG defaults | `identity-object-config` | Attribute definitions matched by name; contextual attribute-source precedence and reference-ID exclusions |
| Connector defaults | `active-directory`, `microsoft-entra` | Connector-specific array keys, conditional applicability, display labels |
| Custom policy | The user's policy | Environment mapping extensions, approved differences, authoritative expectations, explicit overrides |

Profile names select the pack presets. `isc-identity-v2` includes common,
SOURCE, and connector semantics; `isc-envelope-v2` also includes envelope
normalization; `isc-portable-v2` additionally includes portable metadata
normalization. Connector rules apply only to matching classified documents.
A neutral policy with no ISC profile still uses its own authored rules.

The rule packs are included in the executable. Select them through `profile`;
external pack files cannot be loaded at runtime. Add environment mappings,
expectations, and exceptions through your `--policy` file.

Reports record each selected pack's ID, version, layer, and SHA-256 digest.
Applied rules and their pack origins are included in the effective policy digest.

## Connector classification

Connector classification uses recognized exported metadata. Any recognized
identifier can establish the variant, but recognized identifiers must agree.
Unknown values do not claim a variant; they remain ordinary comparable configuration data. An unknown identifier
alongside a recognized identifier does not contradict that recognized identity.

The AD pack recognizes its exported connector class, direct connector type, and
connector name. The Entra pack recognizes `connectorName:"Microsoft Entra"`.
Source display names, tenant-specific connector UUIDs, and array shapes are not
classifiers. Renaming a source cannot activate a different connector pack.
Unclassified sources use common/SOURCE rules and ordinary array comparison.
Conflicting recognized identities fail input validation. Different variants among
corresponding source/actual/baseline documents produce a comparison conflict.

## Custom policy extensions and overrides

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
Custom policy additions are covered by the effective policy digest; extensions,
replacements, disable decisions, and pack origin are visible in reports.
