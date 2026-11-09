# Validate production drift against a known Git snapshot

This guide compares the current configuration of a production SailPoint Identity Security Cloud (ISC) tenant with an approved configexporter snapshot stored in Git. It is intended for readers who have not used either tool before.

- **configexporter** retrieves production configuration and saves a local snapshot.
- **configdiffer** compares the approved snapshot with that new export, using a policy that defines how to match objects and interpret differences.

The result is a local report. Neither tool applies configuration changes to production.

## 1. Prepare the tools and production access

Use these latest published releases, verified on November 9, 2026:

| Tool | Release | Installation and build instructions |
| --- | --- | --- |
| configexporter | [v0.8.0](https://github.com/easqr/iscconfigexporter/releases/tag/v0.8.0) | [configexporter README](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/README.md#build-and-run) |
| configdiffer | [v0.6.0](https://github.com/easqr/iscconfigdiffer/releases/tag/v0.6.0) | [configdiffer README](https://github.com/easqr/iscconfigdiffer/blob/v0.6.0/README.md#build-and-install) |

Follow the respective README to install or build each tool. The examples below assume their executables are available on your shell's `PATH` as `isc-exporter` and `isc-configdiffer`. If you build from source, you can instead use the absolute path to each executable in every command. Both releases require Go 1.27.3 or newer to build. This guide assumes the workflow is available on macOS, Linux, and Windows. Use a private local filesystem for the working folders. The macOS/Linux examples use Bash or Zsh; the Windows examples use PowerShell 7 or newer, which provides the UTF-8 file-writing options shown below. On Windows, the executable filenames are `isc-exporter.exe` and `isc-configdiffer.exe`; PowerShell can invoke them without the `.exe` suffix when they are on `PATH`. To invoke a quoted absolute executable path in PowerShell, prefix it with `&`.

For the **production PAT and connection setup**, follow the [configexporter README](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/README.md#build-and-run) and its linked [SDK connection instructions](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/docs/CONFIGURATION.md#native-sdk-connection-and-authentication). Use credentials authorized to export production configuration; the documented configuration permission is `sp:config:read`. Confirm any required tenant role with your administrator.

**Apart from arranging the PAT and its access, no manual actions are required inside the ISC tenant.** You do not need to create a Configuration Hub backup, configure mappings in the tenant, import a snapshot, or deploy anything. The exporter starts and polls an ISC export job through the API automatically. Configdiffer runs entirely locally and needs no ISC credentials.

Use private local storage: exports and reports can contain sensitive configuration values and secrets.

## 2. Choose the approved Git snapshot and working folders

Check out the repository at the approved commit or tag using your normal Git workflow. The examples use `~/isc-drift` on macOS/Linux and `$HOME\isc-drift` on Windows (for example, `C:\Users\Alex\isc-drift`). Both have this layout:

```text
~/isc-drift/
  approved-config/                  # Your Git repository checkout
    snapshots/
      example-tenant/               # Known configexporter snapshot
        manifest.json
        SOURCE/
          id-<encoded-object-id>.json
        ROLE/
          id-<encoded-object-id>.json
    policies/
      production-drift.json         # Comparison policy you create below
  runs/
    <run-id>/
      exporter.json                # Export settings
      config.json                  # Production credentials, if using a file
      exports/
        <production-tenant>/       # Current production snapshot link
      reports/
```

The `example-tenant` folder is an example snapshot name. Use the actual path in your repository. The selected root must contain `manifest.json` and the original object files. Do not select its `SOURCE` subfolder or the repository root unless that root itself is the snapshot.

Confirm what the snapshot represents. A previous approved production export supports a comparison with production over time. If `example-tenant` was exported from a sandbox, this is a comparison of production against that approved sandbox configuration; intentional environment differences need explicit policy rules. The folder name alone does not establish that the snapshot is approved or assign its environment.

Choose the commands for your operating system and keep using that shell in one terminal session. Run the steps in order; variables defined here are reused below. Replace `your-production-tenant` with the actual production tenant identity; it must match the export and the API connection configured in step 4.

**macOS / Linux**

```sh
umask 077
WORK="$HOME/isc-drift"
REPO="$WORK/approved-config"
KNOWN="$REPO/snapshots/example-tenant"
PROD_TENANT="your-production-tenant"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN="$WORK/runs/$RUN_ID"
POLICY="$REPO/policies/production-drift.json"

mkdir -p "$RUN/reports" "$REPO/policies"
git -C "$REPO" rev-parse HEAD > "$RUN/approved-commit.txt"
git -C "$REPO" status --short
cat "$KNOWN/manifest.json"
```

**Windows (PowerShell 7+)**

```powershell
$PSNativeCommandUseErrorActionPreference = $false
$WORK = Join-Path $HOME 'isc-drift'
$REPO = Join-Path $WORK 'approved-config'
$KNOWN = Join-Path $REPO 'snapshots\example-tenant'
$PROD_TENANT = 'your-production-tenant'
$RUN_ID = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$RUN = Join-Path $WORK "runs\$RUN_ID"
$POLICY = Join-Path $REPO 'policies\production-drift.json'

New-Item -ItemType Directory -Force -Path (Join-Path $RUN 'reports'), (Join-Path $REPO 'policies') | Out-Null
$Account = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
icacls $RUN /inheritance:r /grant:r "${Account}:(OI)(CI)F"
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict access to the run folder.' }
git -C $REPO rev-parse HEAD | Set-Content -LiteralPath (Join-Path $RUN 'approved-commit.txt') -Encoding utf8NoBOM
git -C $REPO status --short
Get-Content -LiteralPath (Join-Path $KNOWN 'manifest.json')
```

The Windows setup restricts access to the run folder to the current account using `icacls`; the macOS/Linux setup uses `umask 077` for newly created files and directories. Keep the repository checkout in an appropriately protected location as well. PowerShell examples use `utf8NoBOM` to write JSON without a byte-order mark.

Check that the snapshot has no uncommitted changes. If it does, resolve them before calling it the approved Git baseline. Record the commit above so a reviewer can identify the exact reference used.

In the manifest, inspect `tenant`, `completed`, `types`, and `counts`. This guide uses `SOURCE` and `ROLE` as an example scope. Both must appear in the manifest's `types` array. A declared type with a count of zero is valid; a type missing from `types` is unavailable, not an empty collection.

Preserve snapshot bytes and filenames. JSON formatting or line-ending changes can invalidate manifest checksums. On Windows, ensure Git preserves the committed snapshot bytes rather than converting JSON line endings during checkout; use your repository’s `.gitattributes` rules or an appropriate Git checkout configuration. If the repository stores a snapshot as a symbolic link, its target generation must also be present in the checkout; a link alone is insufficient. On Windows, Git must materialize it as a working link rather than a text file containing the target path. A repository containing a regular snapshot directory avoids that checkout dependency.

## 3. Create the drift comparison policy

There are three distinct configuration files in this workflow:

| File | Read by | Purpose |
| --- | --- | --- |
| `exporter.json` | configexporter | Select the production tenant, output folder, and exported types |
| `config.json`, or SDK environment variables | configexporter | Authenticate to the production API |
| `production-drift.json` | configdiffer | Define comparison rules; contains no API credentials |

### Start with a small policy

Create `production-drift.json` outside the snapshot directories:

**macOS / Linux**

```sh
cat > "$POLICY" <<'JSON'
{
  "version": 2,
  "id": "production-drift",
  "profile": "isc-portable-v2",
  "modes": ["drift"]
}
JSON
```

**Windows (PowerShell 7+)**

```powershell
@'
{
  "version": 2,
  "id": "production-drift",
  "profile": "isc-portable-v2",
  "modes": ["drift"]
}
'@ | Set-Content -LiteralPath $POLICY -Encoding utf8NoBOM
```

If the repository already contains an approved policy, review and use it instead of overwriting it.

`isc-portable-v2` is a useful starting point for configuration drift, especially when the known snapshot comes from another tenant. It matches objects using logical identities, normally exact, case-sensitive names, and suppresses selected generated IDs and metadata such as creation/modification timestamps. It also includes ISC type and connector rules for references and array alignment. You do not need to copy the built-in rules into your policy.

**Review these defaults before accepting a clean result.** The profile also excludes some operational fields, reference IDs, and connector inventory values. It treats absent and null payload properties as equivalent. Consequently, a clean result means no unexpected differences under this policy and scope; it does not mean the exports are byte-for-byte identical.

For a previous snapshot of the same production tenant, decide whether physical IDs or metadata must also be monitored. `isc-envelope-v2` retains more of those fields than the portable profile. Add `"null_handling": "strict"` if null-versus-absent changes must be reported. All ISC profiles still include some type/connector normalization; select explicit `disabled_rules` for exclusions you need to reverse. Review the [profile reference](https://github.com/easqr/iscconfigdiffer/blob/v0.6.0/docs/configuration.md#policy-syntax-and-profiles) and [shipped rules](https://github.com/easqr/iscconfigdiffer/blob/v0.6.0/docs/rule-packs.md) before making these changes.

### Add only reviewed, specific exceptions

For the first run, retain the minimal policy and inspect the findings. Do not copy the full example mapping policy unchanged: its object names and values are invented.

If the snapshot is from the same production tenant, you normally do not need environment mappings. If it is from a sandbox, prefer **exact value equivalences** over ignoring entire fields. For example, the following complete replacement policy approves one hostname pair for one source, while leaving other values comparable:

```json
{
  "version": 2,
  "id": "production-drift",
  "profile": "isc-portable-v2",
  "modes": ["drift"],
  "equivalences": [
    {
      "id": "approved-directory-host",
      "type": "SOURCE",
      "key": "Corporate Directory",
      "path": "/connectorAttributes/host",
      "left_env": "snapshot",
      "right_env": "prod",
      "pairs": [
        {
          "left": "directory-sb.example.invalid",
          "right": "directory-prod.example.invalid"
        }
      ],
      "evidence": {
        "kind": "approved-change",
        "record_id": "CHANGE-1234"
      }
    }
  ]
}
```

Use this only after replacing the source name, field path, values, and approval reference with reviewed values from your exports. Policy paths are relative to the object's payload: use `/connectorAttributes/host`, not `/object/connectorAttributes/host`. The `snapshot` and `prod` labels must match the command in step 6; these labels do not connect to a tenant.

This rule accepts the stated pair. A different production hostname remains drift. It does not require the field to exist or enforce a production value independently; use an `expectations` rule when you need that additional assertion. A mapping also does not match differently named objects. Resolve such findings using the documented identity/pairing rules rather than broadly excluding them. See the [mapping walkthrough](https://github.com/easqr/iscconfigdiffer/blob/v0.6.0/docs/environment-mappings.md).

Maintain policies as follows:

- Version and review the policy alongside the baseline. Keep credentials, new production exports, and reports outside the repository unless your approved storage process explicitly includes them.
- Keep all relevant objects in scope initially. An `object_filters` include list can hide new, unexpected production objects. Use filters only for an intentionally limited check and document the coverage.
- Add an `ignores` rule only when every value at that narrowly scoped field is deliberately irrelevant. Do not ignore a whole source or connector-attributes object to clear a report.
- Keep one root policy for a small comparison. As rules grow, use explicit `includes` for rule-only fragments grouped by source or connector. Fragments stay beneath the root policy directory, use unique rule IDs, and contain no root headers or nested includes. The CLI receives only the root policy path. See [policy file organization](https://github.com/easqr/iscconfigdiffer/blob/v0.6.0/docs/policy-files.md).
- Use strict JSON: no comments or trailing commas. No Configuration Hub mapping export or deployment operations file is required for this workflow.

## 4. Configure the production export

Create the exporter settings. This command inserts the actual production tenant identity from step 2:

**macOS / Linux**

```sh
cat > "$RUN/exporter.json" <<JSON
{
  "tenant": "$PROD_TENANT",
  "output": "exports",
  "include": ["SOURCE", "ROLE"],
  "backup": {
    "enabled": false
  }
}
JSON
```

**Windows (PowerShell 7+)**

```powershell
@{
    tenant = $PROD_TENANT
    output = 'exports'
    include = @('SOURCE', 'ROLE')
    backup = @{ enabled = $false }
} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $RUN 'exporter.json') -Encoding utf8NoBOM
```

Match `include` to the types you intend to compare. Change the `--scope` value in step 6 to the same selection. For broader checks, add every required type that the known snapshot declares and production supports. `SOURCE,ROLE` covers only sources and roles; it is not a complete tenant drift assessment.

Using the PAT/SDK setup referenced in step 1, place the production connection file named exactly `config.json` in `$RUN`, or supply the documented `SAIL_*` environment variables. If using the file:

**macOS / Linux**

```sh
chmod 600 "$RUN/config.json"
```

**Windows (PowerShell 7+)**

```powershell
$ConfigFile = Join-Path $RUN 'config.json'
$Account = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
icacls $ConfigFile /inheritance:r /grant:r "${Account}:F"
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict access to the credentials file.' }
```

Confirm the connection URL and `$PROD_TENANT` refer to the intended production tenant. If `SAIL_BASE_URL` is set, the exporter uses the environment configuration as a whole instead of the file. Otherwise, it checks for `config.json` beside the executable before checking the current working directory. Avoid a stale credentials file beside the executable taking precedence over your run's file. See [connection lookup](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/docs/CONFIGURATION.md#native-sdk-connection-and-authentication).

## 5. Export current production configuration

Run from the new run directory so the relative `exports` output stays outside the Git baseline:

**macOS / Linux**

```sh
cd "$RUN"
isc-exporter -version > "$RUN/exporter-version.txt"
isc-exporter -config "$RUN/exporter.json" -validate
```

**Windows (PowerShell 7+)**

```powershell
Set-Location -LiteralPath $RUN
isc-exporter -version | Set-Content -LiteralPath (Join-Path $RUN 'exporter-version.txt') -Encoding utf8NoBOM
isc-exporter -config (Join-Path $RUN 'exporter.json') -validate
if ($LASTEXITCODE -ne 0) { throw 'Correct the exporter settings before continuing.' }
```

Expect `configuration valid`. This validates local settings only; it does not test credentials, production access, or whether production supports the selected types.

Run the actual export:

**macOS / Linux**

```sh
isc-exporter -config "$RUN/exporter.json"
EXPORT_EXIT=$?
printf 'Exporter exit code: %s\n' "$EXPORT_EXIT"
```

**Windows (PowerShell 7+)**

```powershell
isc-exporter -config (Join-Path $RUN 'exporter.json')
$EXPORT_EXIT = $LASTEXITCODE
Write-Host "Exporter exit code: $EXPORT_EXIT"
if ($EXPORT_EXIT -ne 0) { throw 'Export failed. Correct the problem before comparing.' }
```

**Continue only after the export succeeds with exit code `0`.** If it fails, correct the reported problem and rerun it; do not compare against an old or incomplete export.

After success, locate and inspect its manifest:

**macOS / Linux**

```sh
PROD="$RUN/exports/$PROD_TENANT"
test -f "$PROD/manifest.json"
cat "$PROD/manifest.json"
```

**Windows (PowerShell 7+)**

```powershell
$PROD = Join-Path $RUN "exports\$PROD_TENANT"
$ManifestFile = Join-Path $PROD 'manifest.json'
if (-not (Test-Path -LiteralPath $ManifestFile -PathType Leaf)) {
    throw 'The production snapshot manifest is missing.'
}
Get-Content -LiteralPath $ManifestFile
```

Verify the tenant identity, completion time, and declared types. `$PROD` is normally a symbolic link to the completed snapshot generation; configdiffer accepts this root link. Do not edit the exported files or run another export into this run directory during comparison. A new run directory for each check keeps each production capture separate.

## 6. Compare the Git snapshot with production

Run this command in the same terminal session:

**macOS / Linux**

```sh
isc-configdiffer --mode drift \
  --left "$KNOWN" --left-mode snapshot --left-env snapshot \
  --right "$PROD" --right-mode snapshot --right-env prod \
  --scope SOURCE,ROLE \
  --policy "$POLICY" \
  --format html \
  --output-dir "$RUN/reports" \
  --output-file production-drift.html
DIFF_EXIT=$?
printf 'Comparison exit code: %s\n' "$DIFF_EXIT"
```

**Windows (PowerShell 7+)**

```powershell
$DiffArgs = @(
    '--mode', 'drift',
    '--left', $KNOWN, '--left-mode', 'snapshot', '--left-env', 'snapshot',
    '--right', $PROD, '--right-mode', 'snapshot', '--right-env', 'prod',
    '--scope', 'SOURCE,ROLE',
    '--policy', $POLICY,
    '--format', 'html',
    '--output-dir', (Join-Path $RUN 'reports'),
    '--output-file', 'production-drift.html'
)
isc-configdiffer @DiffArgs
$DIFF_EXIT = $LASTEXITCODE
Write-Host "Comparison exit code: $DIFF_EXIT"
if ($DIFF_EXIT -notin @(0, 1)) { throw 'The comparison did not complete successfully.' }
```

| Option | Meaning |
| --- | --- |
| `--mode drift` | Compare two captured configurations |
| `--left` | The approved snapshot from Git; the reference side |
| `--right` | The newly captured production snapshot |
| `--left-mode snapshot` and `--right-mode snapshot` | Validate original snapshot files against their manifests, including checksums |
| `--left-env snapshot` and `--right-env prod` | Assign labels used by any environment-specific policy rules |
| `--scope SOURCE,ROLE` | Compare all objects of these types unless the policy filters them |
| `--policy` | Load your explicitly selected comparison policy |
| `--output-dir` and `--output-file` | Save the report at a predictable location |

Type names are case-sensitive; do not add spaces in comma-separated lists. Both manifests must declare every scoped type. If you omit `--scope`, both snapshots must declare the same complete type set.

Use `snapshot` reading mode for this workflow. Do not switch to `tree` merely to bypass a checksum error: investigate why the checked-in baseline or production capture changed. Tree mode is intended for deliberately edited or manifest-free inputs, as described in the [input guide](https://github.com/easqr/iscconfigdiffer/blob/v0.6.0/docs/inputs.md).

The macOS/Linux examples assume an interactive session without automatic exit-on-error enabled. In a script, explicitly handle comparison exit `1` as a completed run with findings; do not let it be mistaken for a failed execution. In PowerShell, capture native executable exit codes from `$LASTEXITCODE`, not `$?`. The Windows comparison example accepts both `0` and `1` as completed runs. If your PowerShell profile enables `$PSNativeCommandUseErrorActionPreference`, set it to `$false` for this session so a drift exit code of `1` can be handled explicitly.

## 7. Read the report and determine the outcome

Open the report in a browser using your file manager: `$RUN/reports/production-drift.html` on macOS/Linux or `$RUN\reports\production-drift.html` on Windows. In PowerShell, `Invoke-Item -LiteralPath (Join-Path $RUN 'reports\production-drift.html')` opens it with the associated application. The HTML is self-contained and needs no network access. Reports are written to files, not printed to the terminal.

| Comparison exit code | Meaning | Action |
| --- | --- | --- |
| `0` | Complete comparison; no unexpected differences or violations | Confirm the selected inputs, policy, and coverage before accepting the result |
| `1` | Complete comparison with differences or violations | Review and classify each finding |
| `2` | Invalid settings/inputs, incomplete comparison, or report-writing failure | Correct the error and rerun; this is not a clean drift result |

An exit `2` may still leave a diagnostic report. Read it together with the terminal error; do not approve an incomplete run.

Review in this order:

1. Confirm the input paths, tenant provenance, capture time, policy, and scope. Check that production was exported after the changes you wanted to assess.
2. Review excluded configurations and applied normalization/equivalence rules. These define what a clean result actually covers.
3. Review object-presence findings. An object only on the production/right side is additional relative to the known snapshot; an object only on the snapshot/left side is missing from production relative to that reference.
4. Inspect changed values in matched objects. Objects normally match by exact names, so renames can appear as unmatched or presence findings unless explicitly paired.
5. Investigate unexpected findings through your normal change process. For intentional differences, approve a narrow policy exception or update the baseline through review, then rerun. Do not automatically replace the baseline with the new production export.

Configdiffer reports drift; it does not repair it. No in-tenant action is required merely to complete or review the comparison. Any later remediation is a separate approved change.

On Windows, edit the corresponding values in `$DiffArgs` and run `isc-configdiffer @DiffArgs` again. For a Markdown report, rerun the comparison command with `--format md` and `--output-file production-drift.md`. For structured evidence, use `--format json` and `--output-file production-drift.json`. Existing files with the same report name are replaced.

Reports include configuration values by default. Add `--exclude-values` when appropriate for sharing, but still review the report: object names, paths, and evidence metadata can remain visible.

## 8. Retain the evidence and repeat consistently

Retain the approved Git commit, the exact policy revision, tool versions, production snapshot, report, and exit code in your approved private evidence location. If the policy was modified after checkout, retain its exact files or commit the reviewed changes so the policy used is reproducible. For configdiffer installed with Go, display embedded build/module information with `go version -m "$(command -v isc-configdiffer)"` on macOS/Linux or `go version -m (Get-Command isc-configdiffer -CommandType Application).Source` in PowerShell. For a source build, also record its release tag and commit.

For the next check, select the intended approved baseline again and create a new run directory. Export production afresh and rerun the same reviewed comparison scope and policy. Do not reuse an earlier production capture as evidence of the tenant's current state.

### Common problems

| Problem | What to check |
| --- | --- |
| Export authentication or permission error | Production PAT, API URL, connection lookup precedence, and documented access requirements |
| Export reports a tenant mismatch | `exporter.json` tenant and SDK connection must identify the same tenant |
| Missing manifest or broken snapshot link | Point to the actual snapshot root and ensure its linked generation exists in the Git checkout |
| Snapshot checksum failure | Restore approved, unmodified snapshot bytes; check Git line-ending or formatting transformations |
| Requested type unavailable | Inspect both manifest `types` arrays; export the required supported types or explicitly limit the comparison and document that limitation |
| Unexpected name/matching findings | Check exact names, duplicates, and documented pairing/identity rules |
| Policy compilation failure | Check strict JSON, unique rule IDs, exact selectors, include paths, and overlaps with shipped rules |
| No visible report | Read terminal errors; invalid requests may fail before a report exists; verify the output directory is writable |

For further diagnostics, see [exporter operations](https://github.com/easqr/iscconfigexporter/blob/v0.8.0/docs/OPERATIONS.md) and [configdiffer troubleshooting](https://github.com/easqr/iscconfigdiffer/blob/v0.6.0/docs/troubleshooting.md).
