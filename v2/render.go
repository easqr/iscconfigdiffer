package v2

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"strings"
)

type statusCount struct {
	Status string
	Count  int
}
type humanReport struct {
	Title                 string
	Complete, Correct     bool
	ExitCode              int
	ObjectType, ObjectKey string
	Counts                []statusCount
	Decisions             []Decision
	Diagnostics           []Diagnostic
	UnusedOptional        int
}

// humanView preserves the content of the human report without changing decisions.
func (r Report) humanView() humanReport {
	v := humanReport{Title: fmt.Sprintf("Comparison v%d (%s)", r.Version, r.Mode), Complete: r.Complete, Correct: r.Correct, ExitCode: r.ExitCode(), Decisions: r.Decisions}
	if r.ObjectKey != "" && len(r.Scope) == 1 {
		v.ObjectType = r.Scope[0]
		v.ObjectKey = r.ObjectKey
	}
	for _, status := range keys(r.Counts) {
		v.Counts = append(v.Counts, statusCount{status, r.Counts[status]})
	}
	for _, d := range r.Diagnostics {
		if d.Code == "unused rule" && !d.Required {
			v.UnusedOptional++
			continue
		}
		v.Diagnostics = append(v.Diagnostics, d)
	}
	return v
}

var reportHTML = template.Must(template.New("report").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root { color-scheme: light; font-family: system-ui, sans-serif; color: #172b3a; background: #f4f6f8; }
body { margin: 0; padding: 2rem; }
main { max-width: 90rem; margin: auto; background: white; padding: 2rem; border-radius: .5rem; }
h1 { margin-top: 0; } h2 { margin-top: 2rem; }
table { width: 100%; border-collapse: collapse; margin: 1rem 0; }
th, td { padding: .65rem; text-align: left; vertical-align: top; border: 1px solid #cbd5df; overflow-wrap: anywhere; white-space: pre-wrap; }
th { background: #edf2f6; } caption { text-align: left; font-weight: 600; padding-bottom: .5rem; }
.summary { max-width: 32rem; } .table-wrap { overflow-x: auto; }
@media print { body, main { padding: 0; background: white; } th { background: white; } }
</style>
</head>
<body><main>
<h1>{{.Title}}</h1>
<table class="summary"><caption>Result</caption><tbody>
<tr><th scope="row">Complete</th><td>{{.Complete}}</td></tr>
<tr><th scope="row">Correct</th><td>{{.Correct}}</td></tr>
<tr><th scope="row">Exit code</th><td>{{.ExitCode}}</td></tr>
{{if .ObjectKey}}<tr><th scope="row">Object filter</th><td>{{.ObjectType}} &quot;{{.ObjectKey}}&quot;</td></tr>{{end}}
</tbody></table>
<h2>Decision counts</h2>
{{if .Counts}}<table class="summary"><thead><tr><th scope="col">Status</th><th scope="col">Count</th></tr></thead><tbody>
{{range .Counts}}<tr><td>{{.Status}}</td><td>{{.Count}}</td></tr>
{{end}}</tbody></table>{{else}}<p>No decisions counted.</p>{{end}}
<h2>Decisions</h2>
{{if .Decisions}}<div class="table-wrap"><table><thead><tr><th scope="col">Status</th><th scope="col">Type</th><th scope="col">Object</th><th scope="col">Rule</th><th scope="col">Reason</th></tr></thead><tbody>
{{range .Decisions}}<tr><td>{{.Status}}</td><td>{{.Type}}</td><td>{{.Key}}</td><td>{{.RuleID}}</td><td>{{.Code}}</td></tr>
{{end}}</tbody></table></div>{{else}}<p>No decisions to display.</p>{{end}}
<h2>Diagnostics</h2>
{{if .Diagnostics}}<div class="table-wrap"><table><thead><tr><th scope="col">Diagnostic</th><th scope="col">Rule</th><th scope="col">Type</th></tr></thead><tbody>
{{range .Diagnostics}}<tr><td>{{.Code}}</td><td>{{.RuleID}}</td><td>{{.Type}}</td></tr>
{{end}}</tbody></table></div>{{else}}<p>No diagnostics to display.</p>{{end}}
{{if .UnusedOptional}}<p>Unused optional rules: {{.UnusedOptional}} (details in JSON rule_usage).</p>{{end}}
</main></body>
</html>
`))

// HTML writes a self-contained HTML document with no external assets or scripts.
func (r Report) HTML(w io.Writer) error {
	return reportHTML.Execute(w, r.humanView())
}

// Markdown writes a Markdown document with result, count, decision and diagnostic tables.
func (r Report) Markdown(w io.Writer) error {
	v := r.humanView()
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n| Result | Value |\n| --- | --- |\n| Complete | %t |\n| Correct | %t |\n| Exit code | %d |\n", markdownCell(v.Title), v.Complete, v.Correct, v.ExitCode)
	if v.ObjectKey != "" {
		fmt.Fprintf(&b, "| Object filter | %s &quot;%s&quot; |\n", markdownCell(v.ObjectType), markdownCell(v.ObjectKey))
	}
	b.WriteString("\n## Decision counts\n\n")
	if len(v.Counts) == 0 {
		b.WriteString("No decisions counted.\n")
	} else {
		b.WriteString("| Status | Count |\n| --- | ---: |\n")
		for _, c := range v.Counts {
			fmt.Fprintf(&b, "| %s | %d |\n", markdownCell(c.Status), c.Count)
		}
	}
	b.WriteString("\n## Decisions\n\n")
	if len(v.Decisions) == 0 {
		b.WriteString("No decisions to display.\n")
	} else {
		b.WriteString("| Status | Type | Object | Rule | Reason |\n| --- | --- | --- | --- | --- |\n")
		for _, d := range v.Decisions {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", markdownCell(d.Status), markdownCell(d.Type), markdownCell(d.Key), markdownCell(d.RuleID), markdownCell(d.Code))
		}
	}
	b.WriteString("\n## Diagnostics\n\n")
	if len(v.Diagnostics) == 0 {
		b.WriteString("No diagnostics to display.\n")
	} else {
		b.WriteString("| Diagnostic | Rule | Type |\n| --- | --- | --- |\n")
		for _, d := range v.Diagnostics {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", markdownCell(d.Code), markdownCell(d.RuleID), markdownCell(d.Type))
		}
	}
	if v.UnusedOptional > 0 {
		fmt.Fprintf(&b, "\nUnused optional rules: %d (details in JSON rule_usage).\n", v.UnusedOptional)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// Escape untrusted text so it stays inside a Markdown table cell as literal text.
func markdownCell(s string) string {
	return strings.NewReplacer("\\", "\\\\", "|", "&#124;", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "\r\n", "<br>", "\r", "<br>", "\n", "<br>").Replace(html.EscapeString(s))
}
