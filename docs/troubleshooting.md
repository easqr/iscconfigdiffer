# Troubleshooting

| Symptom | What to check |
| --- | --- |
| Build cannot find `configexporter` | Check access to `github.com/easqr/iscconfigexporter` v0.10.0 and run `go mod download` |
| Selected scopes differ | Supply matching input type declarations or an explicit shared `--scope` |
| Invalid artifact or unexpected JSON | Check envelope format, manifest-declared filename mode, and whether policies/mappings were placed inside an input tree |
| Checksum/count failure in snapshot mode | Use an intact exported generation; choose tree mode explicitly for intended edits |
| Identity unavailable or ambiguous | Check exact names, parent objects, duplicates, and explicit strategies for unknown types |
| Overlapping rule/profile error | Use `collection_mappings` for environment mappings; remove competing ownership or use an explicit same-category replacement for structural changes |
| Connector rule does not apply | Check the exported connector metadata against the shipped pack identifiers; source display name and array shape are not classifiers |
| Unknown collection mapping target | Reference the effective collection ID; a disabled or replaced target cannot be extended |
| Unused required selector | Check type, object key, path, available collection items, and rule mode |
| Deployment remains unverified | Inspect uncovered fields, authority/evidence, environment values, baseline availability, and reference correspondence |
| Hub context mismatch | Check source/target direction, resolved environments, manifest tenants, and explicit `default` binding |
