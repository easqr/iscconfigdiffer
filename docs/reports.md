# Reports and exit codes

CLI examples assume you have [built the tool](../README.md#build-and-install)
and are running commands from the configdiffer repository root.

## Report files

Reports are saved in `./reports`, relative to the directory where you run the
command. The directory is created automatically. The default filename is the
policy's `id` plus `.html` (the default), `.md` for Markdown, or `.json` for JSON.
For example, a policy with
`"id": "source-drift"` produces `reports/source-drift.json` with `--format json`.
Without `--policy`, the built-in policy uses `default.html`, `default.md`, or
`default.json`.

HTML reports are self-contained documents that open locally in a browser without
network access. Use `--format md` for a Markdown document, or `--format json` for
structured processing. Choosing a format does not change comparison results.

Exceptions identify accepted known differences separately from environment
`equivalent` decisions. Each applied exception includes its rule ID and required
`rationale` in JSON, HTML, and Markdown. Exceptions alone allow exit `0`; other
unexpected differences still return `1`. Invalid exception definitions, including
missing, blank, or non-string rationales, return exit `2` before comparison.
Rationales are policy metadata and remain visible with `--exclude-values`.

## Read findings and counts

Start with the result and input summary: it identifies the compared environments,
export dates, policy, scope, and object inventory. Decision counts describe
comparison decisions, not independent problems. The breakdown separates missing
objects, container-presence differences, and other field or operation differences;
a missing container can also contribute descendant decisions.

The object index groups results by type and logical name. Its status badges count
**grouped findings**; a separate column shows the underlying displayed decisions.
A missing subtree appears once with its per-input path, presence, and value. Open
**Nested decisions** to inspect its repetitive container and leaf decisions, each
with its original path and provenance. For example, one missing branch and its
seven descendants count as one finding representing eight decisions. Other fields
and explicit policy checks remain separate findings.

When a nested transform's `type` changes, **Transform implementation differs**
shows the old and new types at their original `.../transform/type` paths. Its
related attribute differences appear under **Nested decisions**, so a replacement
of one transform implementation appears as one finding. Each provisioning policy
and field keeps its own finding and identifying context. Explicit checks and
changes to other transform properties remain separate.

This grouping recognizes the JSON convention `transform/type` with related
`transform/attributes` paths, independently of the object or connector type.
An unchanged or explicitly approved type comparison keeps attribute differences
in their usual findings; grouping does not declare the implementations equivalent.

The summary distinguishes **Displayed findings** from **Decision counts**. The
latter retain every underlying comparison decision, including ordinary equal
leaves that may be omitted from the details. Grouping changes presentation only:
JSON decisions, comparison totals, rule usage, digests, and exit codes are
unchanged. Redacted identities are combined explicitly; their findings remain
ungrouped because per-object subtree relationships cannot be established. Matched
objects with no displayed findings still appear in the index.

## Navigate the HTML report

HTML is a single self-contained file with embedded styles and JavaScript; it needs
no internet connection, server, hosted libraries, or companion pages. Its layout
provides:

- An **Overview** with counts, expandable input/policy information, and diagnostics.
- An **Objects** index with collapsed type headers. Expand a type to see its
  object table, then select an object to view its findings.
- A persistent **Browse and filter** navigator on wide screens. On narrow screens,
  use **Browse** in the top navigation to reach the expandable navigator.
- A focused object view with sticky **Back to objects**, **Previous**, and **Next**
  controls. Previous/Next follow the filtered object list across types. Returning
  to the index expands the type containing the last selected object.
- **All object details** for reviewing several objects in one continuous view.
  Browser Back/Forward and links to individual objects or report sections work
  within the same file.

Search by object/path/rule, section filters, and status filters update both the
navigator and object tables. **Clear filters** restores the full list. Counts in
the overview retain the comparison totals; object badges retain their full
displayed-finding counts. The navigator's result counter reflects the current
filters. Printing includes the overview and all matching objects with details
expanded, rather than only the focused object.

With JavaScript disabled, all report content remains available through native
expandable sections, including the type tables and object details. Interactive
filtering and the focused object view require JavaScript. Markdown provides
the same information in sections and tables, with nested decisions inside
`<details>` blocks for viewers that support expansion. All child content remains
in the Markdown file. Searching for a nested path in HTML opens its child details;
status filters and the visible-result counter operate on grouped findings.

## Inspect an individual decision

Each decision shows its reason, content/metadata section, per-input JSON Pointer,
presence and JSON type. Artifact paths and IDs identify the source files. Keyed
array item identities explain alignment when input indices differ. Applied rules,
authority, evidence and expectation derivations explain policy decisions. Pointer
`(root)` means the entire content payload or metadata envelope; pointers are not
prefixed with `/object`. Observed values are shown for each input, and expectation
checks also show the independently computed expected value and target role.

Provisioning findings include containing-item context for each input: policy name,
usage type, subtype ID when present, and field name. For example, a long transform
path can be accompanied by `Provisioning policy — Name: Enable Account Policy;
Usage type: ENABLE → Field — Name: AC_NewParent`. The original full JSON Pointer
is retained. Each side uses its own names and indices, including when policies or
fields move or names differ. Missing items are not labelled using the other side's
item. HTML search also finds these names. Markdown and JSON retain the same context.

These names/types are report identity metadata and remain visible with
`--exclude-values`, like object names and IDs. Explicit `sensitive` rules remove
context along with other identity/provenance for affected types. Schema and
attribute findings use the same per-input context model. Renderers use
captured context and do not reopen the configuration files.

Diagnostics distinguish required or blocking findings from unused optional rules.
The latter are listed separately and do not themselves make a comparison fail.
Policy and input hashes are available as provenance for identifying a particular
run; the renderer does not reopen input files.

## Choose output filenames

Use `--output-dir /data/comparison-reports` to choose another directory and
`--output-file ad-only.json` to override the exact filename:

```sh
./bin/isc-configdiffer --mode drift \
  --left /data/dev --left-mode snapshot --left-env dev \
  --right /data/prod --right-mode snapshot --right-env prod \
  --scope SOURCE --name "Active Directory" \
  --policy examples/policy-drift.json --format json \
  --output-dir ./reports --output-file ad-only.json
```

`--output-file` accepts a filename, including its desired extension, without
directory components. It does not select the output format; `--format` does.
Existing reports with the same filename are replaced only after the new report
has been completely written. Filtered and full comparisons use the same default
policy filename, so give them different overrides if you want to retain both.
The default `reports/` directory is excluded from Git tracking.

Reports are written only to the output directory, not to standard output.
Read the saved file to inspect or process a report. Input/policy/request and
output errors go to standard error. A completed
comparison saves its diagnostic report even when it is incomplete and exits `2`.
Requests rejected before comparison do not create a new report. An output failure
returns `2`; an older report may remain in place, so always check the exit code.

## Exit codes and completeness

| Exit code | Meaning |
| --- | --- |
| `0` | Complete and correct; no unexpected drift or violations |
| `1` | Complete with known differences or expectation violations |
| `2` | Invalid input/policy/request, incomplete or unverified comparison, or output failure |

A report's `complete` flag and correctness are separate. `correct` is false
whenever the result is incomplete. Reports include effective scope, input hashes,
policy/profile/request digests, matching decisions, concrete per-input locations,
applied rule IDs, evidence, coverage, and diagnostics. Statuses include `equal`,
`equivalent`, `exception`, `excluded`, `verified`, `different`, `violation`, `unverified`, and
`conflict`. Decision counts are not object counts or percentage leaf coverage.

## Values and privacy

Observed and expected configuration values are included by default in HTML,
Markdown, and JSON. Use `--exclude-values` to omit them from all three formats;
this does not change comparison decisions, counts, exit codes, or request digests.
The library exposes the same option as `Request.ExcludeValues`.

JSON locations carry an optional tagged `value`: `{"state":"present","value":...}`
(including explicit `null`), `{"state":"absent"}`, `{"state":"unavailable"}`, or
`{"state":"redacted"}`. Expectation decisions carry `expected` and `expected_role`
when available. Reports record `values_excluded`; with omission enabled, value
fields are absent. Missing objects/fields, JSON `null`, empty strings, `false`,
and zero remain distinguishable. Containers may include complete objects/arrays;
ordinary equal leaves still require `--matching` to appear.

Policy `sensitive` selectors continue to redact values and identity/provenance
conservatively for their object type, dependent types, and types connected through
known policy reference rules. Expected values and derivation values are redacted
too. The omission flag does not hide names, IDs, labels or declared evidence;
put no secrets in labels or evidence metadata. `--matching`
adds ordinary equal leaves; policy decisions remain visible without it.

Exit `1` is an expected result when drift is found. If using a shell with
`set -e` or a CI runner, handle exit `1` separately from exit `2` when collecting
reports.

## Reading source report sections

Source findings are separated into **Connection settings**, **Schemas**,
**Provisioning policies**, **Other settings**, and **Export metadata**. Connection
settings come first, so paths such as `/connectorAttributes/clientID` and
`/connectorAttributes/domainName` are not buried among schema details. HTML reports
provide section links and a Section filter that combines with status and search.
Markdown reports use the same section order and headings.

An entirely missing `/schemas/<index>` item appears as one named finding, such as
`Schema "group" missing from left (dev)`. Expand **Schema details** for its
attributes, field comparisons, and informational normalization results. Full schema
values are collapsed separately. A missing attribute inside a shared schema is
still an attribute finding; it is not labeled as an entire missing schema.
Explicit assertions, violations, conflicts, and unverified rule checks remain
independent so grouping cannot hide a policy problem. Ordinary missing-subtree
summaries also remain available for changes within shared schemas.

Grouped finding counts count the summaries. Underlying decision counts retain
all original comparisons, including nested details. Status and search filters can
match nested details and automatically open the relevant details; they do not
change totals or the comparison outcome. Container values and long text values are expandable, while short connection values remain visible.
`--exclude-values` continues to omit values completely. Schema names and paths come
from captured report context; renderers do not reread exports. JSON output, policy
behavior, and exit codes are unchanged by this presentation.
