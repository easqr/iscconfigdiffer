# Configuration reference

Use this reference when choosing a profile or authoring comparison rules. For a
step-by-step policy example, start with [environment mappings](environment-mappings.md).
For deployment checks, use the [deployment guide](deployment.md).

The [input guide](inputs.md#which-files-do-i-need) explains which files each
workflow requires. Policy filenames are arbitrary; pass your root file explicitly
with `--policy`.

## Policy syntax and profiles

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
identities do not pass. See the [identity contract](contracts.md#scope-and-identities)
for the named-type list.

## Shipped rules and custom policies

ISC profiles supply the defaults below. Your policy adds environment-specific
rules and exceptions:

| Rule group | Default behavior |
| --- | --- |
| ISC common defaults | Object identities, null handling, envelope and selected metadata rules |
| SOURCE defaults | Schema/attribute and provisioning keys, reference-ID exclusions |
| IDENTITY_PROFILE defaults | Relative priority order in drift; attribute transforms matched by identity attribute name and source IDs excluded |
| IDENTITY_OBJECT_CONFIG defaults | Attribute definitions matched by name; attribute-source precedence and reference-ID exclusions |
| ATTR_SYNC_SOURCE_CONFIG defaults | Attribute arrays matched by target |
| ACCESS_REQUEST_CONFIG defaults | Grant-request approval scheme reference IDs |
| ROLE defaults | Access profile, entitlement, and entitlement-source reference IDs |
| ACCESS_PROFILE defaults | Entitlement and source reference IDs |
| WORKFLOW defaults | Creator and modifier reference IDs, and modifier name |
| SPS_PARAMETER defaults | Public/private modification timestamps and modifier subtrees excluded |
| LIFECYCLE_STATE defaults | Account-action source references matched by name; reference IDs and redundant account actions excluded |
| Connector defaults | AD forest/domain structure and recommendation-status exclusion; Entra subscribed-SKU, delete-threshold, and delta-token exclusions |
| Custom policy | Environment mappings/equivalences, expectations, and deliberate exceptions in your `--policy` JSON file |

These defaults are included in the executable and selected through your policy's
`profile`. See [shipped rules and custom policies](rule-packs.md) for
connector classification, mapping extensions, and overrides.

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

## Active Directory recommendation status

Active Directory sources exclude `/connectorAttributes/recommendationStatus`
under all ISC profiles in drift and deployment. The shipped connector rule
`isc.SOURCE.ad-recommendation-status` applies to every recognized Active Directory
source, regardless of its name. It excludes the field even when missing, null,
or differently shaped on the other side. Other connector types and adjacent
attributes remain comparable. This is a shipped comparison choice; no local policy
entry is needed. Add the rule ID to `disabled_rules` to restore comparison and
normal deployment coverage requirements.

## Microsoft Entra connector exclusions

Microsoft Entra SOURCE configurations exclude these connector attributes in both
drift and deployment modes under all built-in ISC profiles:

| Path | Rule ID |
| --- | --- |
| `/connectorAttributes/deleteThresholdPercentage` | `isc.SOURCE.entra-delete-threshold-percentage` |
| `/connectorAttributes/directoryRolesDeltaToken` | `isc.SOURCE.entra-directory-roles-delta-token` |
| `/connectorAttributes/groupsDeltaToken` | `isc.SOURCE.entra-groups-delta-token` |
| `/connectorAttributes/userDeltaToken` | `isc.SOURCE.entra-user-delta-token` |

No local policy entries are needed. To compare one of these attributes, add its
rule ID to the policy's `disabled_rules` array. These exclusions use recognized
Entra connector metadata and apply independently of the source's display name;
other connectors and adjacent settings remain comparable.

### Subscribed SKUs

Microsoft Entra SOURCE configurations exclude the entire
`/connectorAttributes/subscribedSkus` subtree, including nested service plans,
in both drift and deployment modes under all built-in ISC profiles. The shipped
connector rule is `isc.SOURCE.entra-subscribed-skus`. It applies by recognized
Entra connector metadata, independently of the source's display name. Other
connectors and adjacent Entra settings remain comparable.

To compare this inventory explicitly, add `"isc.SOURCE.entra-subscribed-skus"` to `disabled_rules`;
arrays then follow normal comparison unless you define your own collection rules.
No local mapping is needed for the default exclusion.

## SOURCE cache-update metadata

The portable profile excludes `/connectorAttributes/cloudCacheUpdate` across
all SOURCE connector types in drift and deployment. It is treated as operational
cache-update metadata, rather than an environment-specific configuration value.
The identity and envelope profiles continue comparing it normally.

The rule ID is `isc.SOURCE.cloud-cache-update`. To restore ordinary comparison, add that ID to
your policy's `disabled_rules` array. Other connector attributes remain comparable.

## SOURCE health-check timing metadata

The portable profile excludes `/connectorAttributes/since` across SOURCE
connector types in drift and deployment, treating it as operational health/status
timing metadata. Identity and envelope profiles continue comparing it normally.
The rule does not exclude health/status values, aggregation cursors, a top-level
`/since`, or other similarly named fields.

The rule ID is `isc.SOURCE.health-since`. Add that ID to `disabled_rules` to restore ordinary
comparison and deployment coverage requirements. This comparison choice does not
assume that the nested timestamp is updated on every health check.

## Owner references across configuration types

The common portable pack excludes `/owner/id` for every configuration type,
including custom types with declared identities. Owner `name`, `type`, and other
fields remain comparable. Identity/envelope profiles still compare owner IDs.
The rule expands as `isc.TYPE.owner-id`, for example `isc.ROLE.owner-id` or
`isc.SOURCE.owner-id`. Add a concrete rule ID to `disabled_rules` to restore ID
comparison for that type. This is a common portable rule, not an AD-specific or
environment-specific mapping.

## IDENTITY_OBJECT_CONFIG attribute matching

All ISC profiles match `/objectAttributeDetails` entries by their exact,
case-sensitive `/name`, independently of array order. Names must be nonblank
strings and unique within the array; missing, null, invalid, or duplicate names
produce a conflict rather than positional matching. Added, removed, or renamed
attributes remain differences.

The outer collection rule aligns attribute definitions. Nested `attributeSources`
use identity-profile precedence for multi-application entries, as described below.
All ISC profiles also exclude
these nested reference IDs through the same type pack:

| Path | Rule ID |
| --- | --- |
| `/objectAttributeDetails/*/attributeSources/*/ruleRef/id` | `isc.IDENTITY_OBJECT_CONFIG.attribute-source-rule-id` |
| `/objectAttributeDetails/*/attributeSources/*/applicationRef/id` | `isc.IDENTITY_OBJECT_CONFIG.attribute-source-application-id` |

Reference names, types, and other fields remain comparable. These exclusions apply
to every attribute and source entry in drift and deployment. List either rule ID
in `disabled_rules` to restore its ID comparison and deployment coverage
requirements. Matching and exclusions do not approve differing values or supply
deployment expectations. Reports show the identity attribute name alongside the
original input paths.

The shipped rule is `isc.IDENTITY_OBJECT_CONFIG.object-attribute-details`.
No local policy entry is needed. List its ID in `disabled_rules` to restore ordered comparison, or use an
explicit same-selector replacement for a structural exception.

## IDENTITY_PROFILE attribute transforms

All ISC profiles match `/identityAttributeConfig/attributeTransforms` entries by
`identityAttributeName` in drift and deployment. Array order does not affect
equivalence; duplicate or missing names produce conflicts.

Every nested transform setting remains comparable except these two paths:

- `/identityAttributeConfig/attributeTransforms/*/transformDefinition/attributes/sourceId`
- `/identityAttributeConfig/attributeTransforms/*/transformDefinition/attributes/input/attributes/sourceId`

For example, `sourceName`, `attributeName`, transform types, and nested transform
inputs are checked normally. The exclusions do not cover other `sourceId` paths
deeper inside a transform. Matching aligns entries; it does not approve different
transform definitions or supply deployment expectations.

HTML and Markdown reports group findings into sections named for each identity
attribute, with context identifying the attribute and the original JSON paths
from both inputs. Missing context falls back to a general identity-transform
section. JSON reports retain the individual decisions and captured context.

The default rules are:

| Rule ID | Behavior |
| --- | --- |
| `isc.IDENTITY_PROFILE.attribute-transforms` | Match entries by `identityAttributeName` and capture report context |
| `isc.IDENTITY_PROFILE.attribute-transform-source-id` | Exclude the direct transform source ID |
| `isc.IDENTITY_PROFILE.attribute-transform-input-source-id` | Exclude the immediate input transform's source ID |

No local policy entries are needed. Add a rule ID to `disabled_rules` to restore
positional comparison or source-ID comparison, respectively. Other configuration
types retain normal comparison.

## Relative priority and attribute-source precedence

All ISC profiles compare `IDENTITY_PROFILE` `/priority` by relative precedence,
using `isc.IDENTITY_PROFILE.priority-order`. Lower numbers precede higher numbers.
For corresponding profiles, priorities `5, 10, 40` and `10, 20, 30` are equivalent
when they produce the same order. A reversal is reported even if a profile's own
number did not change. Profiles present on only one side remain object-presence
findings; they do not shift the relative order of the shared profiles. An exact
`--name` filter still uses the other matching profiles as priority context.
Original priority values remain in the profile findings unless values are omitted.

For `IDENTITY_OBJECT_CONFIG`, `isc.IDENTITY_OBJECT_CONFIG.attribute-source-priority`
aligns multi-application `attributeSources` entries by `/applicationRef/name` and
compares the application order derived from identity-profile priorities. It resolves
each original `/applicationRef/id` against a profile's `/authoritativeSource/id`
in the same input, then reads `/priority`. Exported array positions and numeric
spacing do not determine equivalence. Other entry fields remain comparable, and
reports retain their original paths. A precedence reversal is one finding with a
named **Priority order** list for each side, rather than mismatched application
names at different array positions.

Both exports must include the relevant `IDENTITY_PROFILE` documents. They can be
used as context with `--scope IDENTITY_OBJECT_CONFIG`; they are not added to the
comparison scope. Context comes only from the supplied exports, without fetching
live tenant data. Missing, null, or nonnumeric required priorities, unresolved
references, duplicate reference matches, and tied priorities prevent a conclusive
result (exit 2). Source IDs remain available for lookup despite their exclusion
from value comparison. Collections with at most one item on every input, or with
no application references at all, retain ordinary positional comparison. Mixed
multi-entry collections with unavailable references remain incomplete.

Priority ordering compares shared entries; added or removed applications still
produce ordinary membership findings. Source names must be unique in each keyed
collection. Pure global-rule arrays retain their existing ordering semantics.

The scalar relative-order equivalence is drift-only. In deployment, priority values
still need explicit expectations; the contextual collection order needs an
authoritative whole-value expectation. Alignment never supplies deployment approval.
Use `disabled_rules` with either shipped rule ID to restore ordinary numeric or
positional comparison.

The neutral policy mechanisms can also be authored explicitly:

```json
{
  "equivalences": [
    {
      "id": "profile-priority-order",
      "type": "IDENTITY_PROFILE",
      "path": "/priority",
      "operation": "relative_order",
      "replaces": "isc.IDENTITY_PROFILE.priority-order"
    }
  ],
  "collections": [
    {
      "id": "attribute-source-precedence",
      "replaces": "isc.IDENTITY_OBJECT_CONFIG.attribute-source-priority",
      "type": "IDENTITY_OBJECT_CONFIG",
      "path": "/objectAttributeDetails/*/attributeSources",
      "mode": "keyed",
      "keys": ["/applicationRef/name"],
      "component_type": "string",
      "priority_order": {
        "type": "IDENTITY_PROFILE",
        "reference_path": "/applicationRef/id",
        "match_path": "/authoritativeSource/id",
        "priority_path": "/priority"
      }
    }
  ]
}
```

This is a rule fragment illustrating explicit replacements; the shipped defaults
need no local policy entries. `relative_order` takes a concrete content path and
uses the type's matched objects; it cannot also declare value pairs, groups,
environment restrictions, or item selectors. `priority_order` requires a keyed
collection with concrete reference, match, and priority paths and no key cases.
The context type need not be selected for comparison. Existing collection key
mappings can still align environment-specific application names; value equivalence
for those names remains a separate policy decision.

## ATTR_SYNC_SOURCE_CONFIG attribute matching

All ISC profiles compare `/attributes` as a keyed collection using each complex
object's `/target` string. Entry order has no meaning for this comparison.
Targets must be nonblank strings and unique within each configuration's array;
missing, null, structured, non-string, blank, or duplicate targets produce a
conflict. The same target can be used by different source configurations.

Matching by target aligns entries; it does not approve differences in their
values. `name`, `displayName`, `enabled`, and other fields remain comparable.
Added or removed targets still produce findings. Reports retain original input
indices and show Target/Name context. Deployment still requires explicit field
expectations. `/source/id` is excluded from value comparison by
`isc.ATTR_SYNC_SOURCE_CONFIG.source-id`; source names, types, and other fields
remain comparable. The original ID is still required to resolve the parent SOURCE
before comparison. Missing, invalid, or unresolved parent references remain
incomplete; changing the logical parent still produces differences. The exclusion
does not substitute name-based lookup.
Disable this rule to restore parent-reference ID comparison and deployment coverage.

The shipped collection is `isc.ATTR_SYNC_SOURCE_CONFIG.attributes`. It applies to all parent source
connectors. Use `disabled_rules` to restore ordinary ordered comparison, or an
explicit same-selector replacement for a structural exception.

Attribute-sync identity depends on SOURCE matching. Include both types with
`--scope SOURCE,ATTR_SYNC_SOURCE_CONFIG`; this also compares SOURCE
objects under that scope. Selecting only ATTR_SYNC_SOURCE_CONFIG still leaves
its parent matching context unavailable and returns an incomplete result.

## ACCESS_REQUEST_CONFIG approval scheme references

All ISC profiles exclude
`/entitlementRequestConfig/grantRequestApprovalSchemes/*/id` for
ACCESS_REQUEST_CONFIG in drift and deployment. The wildcard covers every scheme
array member, including `/entitlementRequestConfig/grantRequestApprovalSchemes/0/id`.
Names, types, and other scheme/configuration fields remain comparable. Missing
schemes and array ordering still produce findings. Other nested IDs, other
approval-scheme paths, and other configuration types retain normal comparison.

The rule ID is `isc.ACCESS_REQUEST_CONFIG.grant-approval-scheme-id`. No local mapping is needed.
List the rule ID in `disabled_rules` to restore ID comparison and deployment
coverage requirements.

## ROLE access profile and entitlement references

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

Add an individual rule ID to `disabled_rules` to restore that ID's comparison and
deployment coverage requirements. No custom-policy mappings are needed.

## ACCESS_PROFILE entitlement and source references

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

No local policy mapping is needed. Add either rule ID to `disabled_rules` to
restore its ID comparison and deployment coverage requirements.

## SPS_PARAMETER modification metadata

All ISC profiles exclude these SPS_PARAMETER fields in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/lastModifiedAt` | `isc.SPS_PARAMETER.last-modified-at` |
| `/lastModifiedBy` | `isc.SPS_PARAMETER.last-modified-by` |
| `/privateFieldsLastModifiedAt` | `isc.SPS_PARAMETER.private-fields-last-modified-at` |
| `/privateFieldsLastModifiedBy` | `isc.SPS_PARAMETER.private-fields-last-modified-by` |

Both modifier exclusions cover the entire subtree, including IDs, names, types,
and any other nested attributes. Parameter settings remain comparable, and these
exclusions do not apply to other configuration types.

No local policy
entries are needed. Add a rule ID to `disabled_rules` to restore comparison and
deployment coverage requirements for that field.

## WORKFLOW creator and modifier references

All ISC profiles exclude these WORKFLOW reference fields in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/creator/id` | `isc.WORKFLOW.creator-id` |
| `/modifiedBy/id` | `isc.WORKFLOW.modified-by-id` |
| `/modifiedBy/name` | `isc.WORKFLOW.modified-by-name` |

The exported field is `creator`, not `create`. The creator name, reference types, and
other fields remain comparable, as do identical paths on other configuration types.
List a rule ID in `disabled_rules` to restore that field's comparison and deployment coverage
requirements. No custom-policy mapping is needed.

## LIFECYCLE_STATE account actions

All ISC profiles match `/accountActionRefs/*/sourceIdsRefs` entries by `/name`,
so source-reference array order does not affect equivalence. Names must be unique
within each source-reference array; missing or duplicate keys are reported as
conflicts. Reference types and other fields remain comparable.

These defaults apply in drift and deployment:

| Rule ID | Behavior |
| --- | --- |
| `isc.LIFECYCLE_STATE.account-action-sources` | Match source references by name |
| `isc.LIFECYCLE_STATE.account-action-source-id` | Exclude `/accountActionRefs/*/sourceIdsRefs/*/id` |
| `isc.LIFECYCLE_STATE.account-actions` | Exclude the redundant `/accountActions` subtree |
| `isc.LIFECYCLE_STATE.identity-profile-id` | Exclude `/identityProfileRef/id`; keep comparing its name and type |

The wildcard covers every account-action entry. The outer `/accountActionRefs`
array retains its normal positional comparison. Other configuration types are
unaffected. No local policy entries are needed; use `disabled_rules` to disable
individual defaults. The identity-profile ID still resolves the parent when
matching lifecycle states; excluding it from comparison does not change that
association.

## SOURCE correlation references

All ISC profiles exclude these exact SOURCE paths in drift and deployment:

| Path | Rule ID |
| --- | --- |
| `/correlationConfigDetails/id` | `isc.SOURCE.correlation-config-details-id` |
| `/managerCorrelationRule/id` | `isc.SOURCE.manager-correlation-rule-id` |

These rules apply to all SOURCE connector types. Names, types, correlation details, and other fields
remain comparable. The same paths on other configuration types are unchanged.
Use `disabled_rules` to restore comparison for an individual exclusion.

## SOURCE before-provisioning rule references

All ISC profiles exclude `/beforeProvisioningRule/id` in drift and deployment
using `isc.SOURCE.before-provisioning-rule-id`. This reference ID is specific to
the tenant. The rule reference's `name`, `type`, and other fields remain
comparable; changing or removing a name or type still produces a finding.
Deployment still requires independent expectations for the comparable fields.
The exclusion applies only to SOURCE objects and this exact ID path.

## SOURCE schemas and attributes

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

## SOURCE provisioning policies

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

## Selectors, rule options, and precedence

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
environment mappings without changing structure. Profile rule IDs are documented in the [policy contract](policy.md#isc-profiles).

## Equivalence: approved drift differences

Use the complete [environment mapping walkthrough](environment-mappings.md#configure-environment-specific-mappings)
and [`examples/policy-drift.json`](../examples/policy-drift.json). Each equivalence
rule connects an object/field selector, input environment labels, and approved
whole-value pairs. Optional `groups` can replace `pairs` for explicitly symmetric
relationships, such as `[{"dev":"dev-host","prod":"prod-host"}]`.
Do not combine both forms. Equivalence rules run only in drift mode.

## Expectations: required states and values

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

## Matching, references, arrays, and exclusions

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
correspondence only; see the [forest mapping example](environment-mappings.md#forest-names-that-differ-between-environments)
for a complete policy and the [policy contract](policy.md#collection-key-mappings)
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
