# Deployment validation

CLI examples assume you have [built the tool](../README.md#build-and-install)
and are running commands from the configdiffer repository root.

Use deployment mode to validate an exporter-format projected draft against
explicit expectations. Prepare the source export, actual draft, optional target
baseline, an expectation policy, and an operation list. This checks the proposed
configuration; it does not execute or authorize a deployment.

The command below uses the repository's invented examples. Adapt the policy and
operation list before using your own configurations.

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
formats are not supported.

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

## Deployment operations file

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

### A worked update example

Suppose you are deploying the source `Invented Directory` from development to
production. Save this as your operations file, or use the included
[`examples/operations.json`](../examples/operations.json):

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

### Fields and actions

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

### What happens when objects are missing or unlisted?

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
