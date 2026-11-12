# Runtime dependencies

Configdiffer pins `github.com/easqr/iscconfigexporter` **v0.10.0**, licensed under
Apache-2.0, for snapshot and mapping validation. Its snapshot package uses
`golang.org/x/text` **v0.41.0**, licensed under BSD-3-Clause, for Unicode normalization
and case folding in filename collision checks. The full Go Authors notice is in
[licenses/x-text-LICENSE.txt](licenses/x-text-LICENSE.txt).

The CLI does not import the exporter SDK integration or contact ISC. Other SDK
modules may appear in the module graph without being linked into configdiffer.
Use `go list -deps` for the actual runtime dependency graph. Builds require
Go 1.27.4 or newer, matching the exporter dependency's patched minimum toolchain.
