# Quick start with sample data

CLI examples assume you have [built the tool](../README.md#build-and-install)
and are running commands from the configdiffer repository root.

This example creates two invented, manifest-free configuration trees in a
temporary directory. Run these commands from the module directory in the same
shell after building the CLI:

```sh
demo_dir=$(mktemp -d)
mkdir -p "$demo_dir/dev/SOURCE" "$demo_dir/prod/SOURCE"

cat > "$demo_dir/dev/SOURCE/id-61.json" <<'JSON'
{
  "version": 1,
  "self": {"type": "SOURCE", "id": "a"},
  "object": {
    "id": "a",
    "name": "Invented Directory",
    "connectorAttributes": {
      "host": "directory-dev.example.invalid",
      "timeout": 60
    }
  }
}
JSON

cat > "$demo_dir/prod/SOURCE/id-62.json" <<'JSON'
{
  "version": 1,
  "self": {"type": "SOURCE", "id": "b"},
  "object": {
    "id": "b",
    "name": "Invented Directory",
    "connectorAttributes": {
      "host": "directory-prod.example.invalid",
      "timeout": 60
    }
  }
}
JSON

./bin/isc-configdiffer --mode drift \
  --left "$demo_dir/dev" --left-mode tree --left-types SOURCE --left-env dev \
  --right "$demo_dir/prod" --right-mode tree --right-types SOURCE --right-env prod \
  --policy examples/policy-drift.json
```

The report is saved as `reports/invented-drift.html`. Open it in a browser.
The comparison completes successfully with exit code `0`. Objects match by their
name, the policy permits the two hostnames, and the portable profile excludes
physical IDs. Change the production `timeout` to `90` and rerun the command to
see an unexpected difference and exit code `1`. Add `--format json` to obtain a
machine-readable report.
