package v2

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"sort"
	"strings"
	texttemplate "text/template"
)

type statusCount struct {
	Status string
	Count  int
}
type reportField struct{ Name, Value string }
type humanInput struct {
	Role   string
	Fields []reportField
}
type humanLocation struct {
	CollapseValue                                                   bool
	Role, State, Kind, Pointer, ID, Artifact, Value, ContextSummary string
	Context                                                         []ItemContext
}
type humanDecision struct {
	HasCollapsedValues                                            bool
	Category, StatusTokens                                        string
	WholeSchema                                                   bool
	FindingTitle, ChildrenNotice                                  string
	Children                                                      []humanDecision
	Nested                                                        bool
	DecisionCount                                                 int
	HasContext                                                    bool
	Status, Code, Explanation, Space, RuleID, Authority, Replaces string
	Normalization                                                 string
	ExpectedRole, ExpectedValue                                   string
	ItemKeys                                                      []string
	Locations                                                     []humanLocation
	Evidence                                                      []reportField
	PolicyFiles                                                   []reportField
	ItemScope                                                     []reportField
	Extensions                                                    []reportField
	Derivation                                                    []reportField
}
type humanGroup struct {
	Sections                []humanSection
	DecisionCount           int
	ID, Type, Key, Presence string
	Matches                 int
	Matching                []reportField
	Counts                  []statusCount
	Decisions               []humanDecision
}
type humanTypeGroup struct {
	ID, Type string
	Groups   []humanGroup
}
type humanSection struct {
	ID, Category, Title string
	Decisions           []humanDecision
	Counts              []statusCount
	DecisionCount       int
}
type humanReport struct {
	ValueNotice                                            string
	Title, Outcome, ObjectType, ObjectKey                  string
	Complete, Correct                                      bool
	ExitCode                                               int
	Counts                                                 []statusCount
	FindingCounts                                          []statusCount
	NestedDecisions                                        int
	Provenance, Coverage                                   []reportField
	Inputs                                                 []humanInput
	ExcludedObjects                                        []ExcludedObject
	Groups                                                 []humanGroup
	Types                                                  []humanTypeGroup
	Diagnostics, OptionalDiagnostics                       []Diagnostic
	UnusedOptional                                         int
	MissingObjects, ContainerDifferences, FieldDifferences int
}

func contextSummary(contexts []ItemContext) string {
	parts := make([]string, 0, len(contexts))
	for _, c := range contexts {
		fields := make([]string, 0, len(c.Fields))
		for _, f := range c.Fields {
			fields = append(fields, f.Label+": "+f.Value)
		}
		text := c.Label
		if len(fields) > 0 {
			text += " — " + strings.Join(fields, "; ")
		} else {
			text += " at " + c.Pointer
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " → ")
}
func valueView(v *State) string {
	if v == nil {
		return "omitted"
	}
	if v.State != "present" {
		return v.State
	}
	parsed, err := JSON(v.Value)
	if err != nil {
		return "unavailable"
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(parsed); err != nil {
		return "unavailable"
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func countView(counts map[string]int) []statusCount {
	var out []statusCount
	for _, s := range keys(counts) {
		out = append(out, statusCount{s, counts[s]})
	}
	return out
}
func locationView(role string, loc Location) humanLocation {
	state := "absent"
	if loc.Present {
		state = "present"
	}
	path := loc.Pointer
	if path == "" {
		path = "(root)"
	}
	value := valueView(loc.Value)
	collapse := loc.Value != nil && loc.Value.State == "present" && (loc.ValueKind == "object" || loc.ValueKind == "array" || len(value) > 240)
	return humanLocation{Role: role, State: state, Kind: loc.ValueKind, Pointer: path, ID: loc.ID, Artifact: loc.Artifact, Value: value, CollapseValue: collapse, Context: loc.Context, ContextSummary: contextSummary(loc.Context)}
}
func decisionView(d Decision, roles []string, excludeValues bool) humanDecision {
	out := humanDecision{Normalization: d.Normalization, Status: d.Status, Code: d.Code, Space: d.Space, RuleID: d.RuleID, Authority: d.Authority, Replaces: d.Replaces, ItemKeys: d.ItemKeys}
	out.Category = decisionCategory(d)
	scopes := d.Items
	if d.Item != nil {
		scopes = []CollectionItemSelector{*d.Item}
	}
	for _, scope := range scopes {
		out.ItemScope = append(out.ItemScope, reportField{"Collection", scope.Collection})
		for _, path := range keys(scope.Keys) {
			key := scope.Keys[path]
			if key.Mapped != "" {
				out.ItemScope = append(out.ItemScope, reportField{"Mapped " + path, key.Mapped})
			} else {
				out.ItemScope = append(out.ItemScope, reportField{"Raw " + path, string(key.Value)})
			}
		}
	}
	if d.ExpectedRole != "" {
		out.ExpectedRole = d.ExpectedRole
		out.ExpectedValue = valueView(d.Expected)
		if d.Expected == nil && !excludeValues {
			out.ExpectedValue = "unavailable"
		}
	}
	switch d.Status {
	case "different":
		out.Explanation = "Unexpected drift under the selected policy."
	case "violation":
		out.Explanation = "A declared deployment operation or expectation is not satisfied."
	case "equivalent":
		out.Explanation = "The policy approves this difference or reference relationship."
	case "excluded":
		out.Explanation = "Explicitly excluded from comparison by a normalization rule."
	case "verified":
		out.Explanation = "The declared expectation or operation is satisfied."
	case "equal":
		out.Explanation = "The compared values are equal."
	case "unverified":
		out.Explanation = "Available inputs or policy authority cannot verify this result."
	case "conflict":
		out.Explanation = "A conflict prevents a conclusive result."
	}
	switch d.Code {
	case "relative priority order differs":
		out.Explanation = "Relative precedence changed. Lower numeric priorities take precedence; the Priority order context lists entries from highest to lowest precedence."
	case "relative priority order unchanged":
		out.Explanation = "Relative precedence is unchanged. Numeric priority spacing and exported array positions do not determine equivalence."
	case "object missing":
		out.Explanation = "The object is absent from one of the primary inputs."
	case "container presence differs":
		out.Explanation = "An object or array exists on only one side. Repetitive descendant decisions are grouped below when their relationship is known."
	case "type mismatch":
		out.Explanation = "The JSON value types differ, including possible null versus non-null values."
	case "missing left":
		out.Explanation = "The field or collection item is absent from left."
	case "missing right":
		out.Explanation = "The field or collection item is absent from right."
	}
	for _, role := range roles {
		loc, ok := d.Locations[role]
		if ok {
			out.HasContext = out.HasContext || len(loc.Context) > 0
			out.Locations = append(out.Locations, locationView(role, loc))
		} else {
			out.Locations = append(out.Locations, humanLocation{Role: role, State: "object unavailable", Pointer: "not available", Value: "not available"})
		}
	}
	if d.Evidence != nil {
		out.Evidence = []reportField{{"Kind", d.Evidence.Kind}, {"Record", d.Evidence.RecordID}, {"Digest", d.Evidence.Digest}}
		for _, k := range keys(d.Evidence.Attributes) {
			out.Evidence = append(out.Evidence, reportField{k, d.Evidence.Attributes[k]})
		}
	}
	for _, extension := range d.Extensions {
		out.Extensions = append(out.Extensions, reportField{"Extension rule", extension.RuleID}, reportField{"Collection rule", extension.Collection})
		if extension.Evidence != nil {
			out.Extensions = append(out.Extensions, reportField{"Evidence kind", extension.Evidence.Kind}, reportField{"Evidence record", extension.Evidence.RecordID}, reportField{"Evidence digest", extension.Evidence.Digest})
			for _, key := range keys(extension.Evidence.Attributes) {
				out.Extensions = append(out.Extensions, reportField{key, extension.Evidence.Attributes[key]})
			}
		}
	}
	if d.Derivation != nil {
		loc := locationView(d.Derivation.Role, d.Derivation.Location)
		out.Derivation = []reportField{{"Context", loc.ContextSummary}, {"Role", loc.Role}, {"Type", d.Derivation.Type}, {"Path", loc.Pointer}, {"Artifact", loc.Artifact}, {"ID", loc.ID}, {"Presence", loc.State}, {"Value", loc.Value}}
	}
	return out
}

// humanView derives presentation only from the already-redacted structured report.
// It never reopens artifacts or reads values outside the structured report.
func (r Report) humanView() humanReport {
	v := humanReport{Title: fmt.Sprintf("Comparison v%d (%s)", r.Version, r.Mode), Complete: r.Complete, Correct: r.Correct, ExitCode: r.ExitCode(), Counts: countView(r.Counts)}
	if r.ValuesExcluded {
		v.ValueNotice = "Configuration values are omitted (--exclude-values)."
	} else {
		v.ValueNotice = "Observed and expected values are shown when available. Values covered by sensitive rules are redacted."
	}
	switch v.ExitCode {
	case 0:
		v.Outcome = "Complete: no unexpected drift or violations."
	case 1:
		v.Outcome = "Complete: unexpected drift or expectation violations found."
	default:
		v.Outcome = "Incomplete: review diagnostics and unverified or conflicting decisions."
	}
	if r.ObjectKey != "" && len(r.Scope) == 1 {
		v.ObjectType = r.Scope[0]
		v.ObjectKey = r.ObjectKey
	}
	v.Provenance = []reportField{{"Policy", r.PolicyID}, {"Profile", r.Profile}, {"Null handling", r.NullHandling}, {"Scope", strings.Join(r.Scope, ", ")}, {"Policy digest", r.PolicyDigest}, {"Profile digest", r.ProfileDigest}, {"Request digest", r.RequestDigest}}
	v.ExcludedObjects = r.ExcludedObjects
	for _, typ := range keys(r.ObjectFilters) {
		filter := r.ObjectFilters[typ]
		if len(filter.Include) > 0 {
			v.Provenance = append(v.Provenance, reportField{typ + " included names", strings.Join(filter.Include, "; ")})
		}
		if len(filter.Exclude) > 0 {
			v.Provenance = append(v.Provenance, reportField{typ + " excluded names", strings.Join(filter.Exclude, "; ")})
		}
	}
	fileOrigins := map[string]Evidence{}
	if len(r.DisabledRules) > 0 {
		v.Provenance = append(v.Provenance, reportField{"Disabled shipped rules", strings.Join(r.DisabledRules, ", ")})
	}
	for _, e := range r.Evidence {
		if e.Kind == "policy-file" {
			v.Provenance = append(v.Provenance, reportField{"Policy file", e.RecordID + "; role: " + e.Attributes["role"] + "; digest: " + e.Digest})
			for _, id := range strings.Fields(e.Attributes["rule_ids"]) {
				fileOrigins[id] = e
			}
		}
		if e.Kind == "isc-rule-pack" {
			v.Provenance = append(v.Provenance, reportField{"Rule pack", e.RecordID + "; version: " + e.Attributes["version"] + "; layer: " + e.Attributes["layer"] + "; digest: " + e.Digest})
		}
	}
	for _, k := range keys(r.ObjectCoverage) {
		v.Coverage = append(v.Coverage, reportField{k, fmt.Sprint(r.ObjectCoverage[k])})
	}
	roles := []string{"left", "right"}
	if r.Mode == "deployment" {
		roles = []string{"source", "actual"}
		if _, ok := r.Inputs["baseline"]; ok {
			roles = append(roles, "baseline")
		}
	}
	for _, role := range roles {
		if in, ok := r.Inputs[role]; ok {
			v.Inputs = append(v.Inputs, humanInput{role, []reportField{{"Label", in.Label}, {"Environment", in.Environment}, {"Tenant", in.Tenant}, {"Reading mode", in.ReadMode}, {"Export completed", in.Completed}, {"Objects in type scope", fmt.Sprint(r.ObjectCoverage[role+"_in_scope"])}, {"Outside object filter", fmt.Sprint(r.ObjectCoverage[role+"_outside_object_filter"])}, {"Excluded types", strings.Join(r.Excluded[role], ", ")}, {"Content hash", in.ContentHash}, {"Document hash", in.DocumentHash}, {"Manifest hash", in.ManifestHash}}})
		}
	}
	groups := map[string]*humanGroup{}
	decisions := map[string][]Decision{}
	findingCounts := map[string]int{}
	presence := map[string]map[string]bool{}
	getGroup := func(typ, key string) *humanGroup {
		token := physical(typ, key)
		if g := groups[token]; g != nil {
			return g
		}
		g := &humanGroup{Type: typ, Key: key}
		groups[token] = g
		decisions[token] = []Decision{}
		presence[token] = map[string]bool{}
		return g
	}
	for _, m := range r.Matches {
		g := getGroup(m.Type, m.Key)
		g.Matches++
		g.Matching = append(g.Matching, reportField{"Matching rule", m.RuleID})
		for _, role := range roles {
			if loc, ok := m.Locations[role]; ok {
				presence[physical(m.Type, m.Key)][role] = true
				g.Matching = append(g.Matching, reportField{role, fmt.Sprintf("ID: %s; artifact: %s", loc.ID, loc.Artifact)})
			}
		}
	}
	for _, d := range r.Decisions {
		g := getGroup(d.Type, d.Key)
		view := decisionView(d, roles, r.ValuesExcluded)
		if origin, ok := fileOrigins[d.RuleID]; ok {
			view.PolicyFiles = append(view.PolicyFiles, reportField{"File", origin.RecordID}, reportField{"Digest", origin.Digest})
		}
		for _, extension := range d.Extensions {
			if origin, ok := fileOrigins[extension.RuleID]; ok {
				view.PolicyFiles = append(view.PolicyFiles, reportField{"Extension " + extension.RuleID, origin.RecordID}, reportField{"Digest", origin.Digest})
			}
		}
		g.Decisions = append(g.Decisions, view)
		token := physical(d.Type, d.Key)
		decisions[token] = append(decisions[token], d)
		for role := range d.Locations {
			presence[token][role] = true
		}
		if d.Status == "different" || d.Status == "violation" {
			switch d.Code {
			case "object missing":
				v.MissingObjects++
			case "container presence differs":
				v.ContainerDifferences++
			default:
				v.FieldDifferences++
			}
		}
	}
	for _, g := range groups {
		token := physical(g.Type, g.Key)
		g.DecisionCount = len(g.Decisions)
		remaining, views := groupMissingSchemas(decisions[token], g.Decisions, roles, r.Inputs)
		remaining, views = groupTransformImplementations(remaining, views, roles)
		g.Decisions = groupMissingSubtrees(remaining, views, roles)
		counts := map[string]int{}
		for i := range g.Decisions {
			d := &g.Decisions[i]
			d.StatusTokens = decisionStatuses(*d)
			markCollapsedValues(d)
			counts[d.Status]++
			findingCounts[d.Status]++
		}
		g.Counts = countView(counts)
		v.NestedDecisions += g.DecisionCount - len(g.Decisions)
		var parts []string
		for _, role := range roles {
			state := "absent"
			if presence[token][role] {
				state = "present"
			}
			parts = append(parts, role+": "+state)
		}
		g.Presence = strings.Join(parts, "; ")
		if g.Key == "[redacted]" {
			g.Presence = "Redacted identities are combined; per-object presence cannot be inferred from this group."
		}
		v.Groups = append(v.Groups, *g)
	}
	v.FindingCounts = countView(findingCounts)
	sort.Slice(v.Groups, func(i, j int) bool {
		a, b := v.Groups[i], v.Groups[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Key < b.Key
	})
	for i := range v.Groups {
		v.Groups[i].ID = fmt.Sprintf("object-%d", i+1)
		v.Groups[i].Sections = decisionSections(v.Groups[i])
		g := v.Groups[i]
		if len(v.Types) == 0 || v.Types[len(v.Types)-1].Type != g.Type {
			v.Types = append(v.Types, humanTypeGroup{ID: fmt.Sprintf("type-%d", len(v.Types)+1), Type: g.Type})
		}
		last := &v.Types[len(v.Types)-1]
		last.Groups = append(last.Groups, g)
	}
	for _, d := range r.Diagnostics {
		if d.Code == "unused rule" && !d.Required {
			v.UnusedOptional++
			v.OptionalDiagnostics = append(v.OptionalDiagnostics, d)
		} else {
			v.Diagnostics = append(v.Diagnostics, d)
		}
	}
	return v
}

//go:embed report.html.tmpl
var htmlReportTemplate string

//go:embed report.md.tmpl
var markdownReportTemplate string
var reportHTML = template.Must(template.New("report").Parse(htmlReportTemplate))
var reportMarkdown = texttemplate.Must(texttemplate.New("report").Funcs(texttemplate.FuncMap{"cell": markdownCell}).Parse(markdownReportTemplate))

// HTML writes an offline HTML report with expandable object groups and local filters.
func (r Report) HTML(w io.Writer) error { return reportHTML.Execute(w, r.humanView()) }

// Markdown writes a grouped Markdown report using the captured report values.
func (r Report) Markdown(w io.Writer) error { return reportMarkdown.Execute(w, r.humanView()) }
func markdownCell(s string) string {
	return strings.NewReplacer("\\", "\\\\", "|", "&#124;", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "\r\n", "<br>", "\r", "<br>", "\n", "<br>").Replace(html.EscapeString(s))
}
