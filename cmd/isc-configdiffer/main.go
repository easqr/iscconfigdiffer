package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	differ "github.com/elianderson/iscutilities/configdiffer"
	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

type inputFlags struct{ dir, mode, label, env, types *string }

func addInput(f *flag.FlagSet, role string) inputFlags {
	return inputFlags{f.String(role, "", role+" input directory"), f.String(role+"-mode", "", "snapshot or tree (required)"), f.String(role+"-label", role, "display label"), f.String(role+"-env", "", "environment"), f.String(role+"-types", "", "manifest-free tree types, comma separated")}
}
func readInput(f inputFlags) (snapshot.Input, error) {
	return snapshot.Read(*f.dir, snapshot.Options{Mode: snapshot.Mode(*f.mode), Label: *f.label, Environment: *f.env, Types: list(*f.types)})
}
func run(args []string, errOut io.Writer) int {
	f := flag.NewFlagSet("isc-configdiffer", flag.ContinueOnError)
	f.SetOutput(errOut)
	mode := f.String("mode", "drift", "drift or deployment")
	left, right := addInput(f, "left"), addInput(f, "right")
	source, actual, baseline := addInput(f, "source"), addInput(f, "actual"), addInput(f, "baseline")
	policyPath := f.String("policy", "", "comparison policy JSON file")

	format := f.String("format", "text", "text or json")
	outputDir := f.String("output-dir", "reports", "report output directory (relative to working directory)")
	outputFile := f.String("output-file", "", "report filename override (default: policy ID plus .txt or .json)")
	scope := f.String("scope", "", "explicit common type scope")
	name := f.String("name", "", "exact logical object name/key within one explicit scope type")
	matching := f.Bool("matching", false, "include ordinary equal leaves")
	operations := f.String("operations", "", "explicit deployment operations JSON file")
	hub := f.String("hub", "", "offline Hub array or bundle directory")
	hs := f.String("hub-source-tenant", "", "source tenant/group")
	ht := f.String("hub-target-tenant", "", "target tenant")
	hd := f.String("hub-default-source-tenant", "", "bind default mapping group to this source")
	ha := f.String("hub-deployment-authority", "", "explicit authority for deployment expectations")
	he := f.String("hub-deployment-evidence", "", "JSON evidence file for explicit deployment promotion")
	if e := f.Parse(args); e != nil {
		return 2
	}
	fail := func(e error) int { fmt.Fprintln(errOut, e); return 2 }
	bad := func(s string) int { return fail(fmt.Errorf("%s", s)) }
	emptyName := false
	emptyOutput := false
	f.Visit(func(fl *flag.Flag) {
		if (fl.Name == "output-dir" && *outputDir == "") || (fl.Name == "output-file" && *outputFile == "") {
			emptyOutput = true
		}
		if fl.Name == "name" && *name == "" {
			emptyName = true
		}
	})
	if emptyOutput {
		return bad("output directory and filename must not be empty")
	}
	if *outputFile != "" && (*outputFile == "." || *outputFile == ".." || filepath.Base(*outputFile) != *outputFile || strings.ContainsAny(*outputFile, "/\\")) {
		return bad("output-file must be a filename without directory components")
	}
	if emptyName {
		return bad("name must not be empty")
	}
	if f.NArg() != 0 {
		return bad("unexpected arguments")
	}
	if (*mode != "drift" && *mode != "deployment") || (*format != "json" && *format != "text") {
		return bad("valid drift/deployment mode and text/json format required")
	}
	if *mode == "deployment" {
		if *left.dir != "" || *right.dir != "" {
			return bad("deployment uses source/actual roles")
		}
		left, right = source, actual
		if *operations == "" {
			return bad("deployment requires explicit operations")
		}
	} else if *source.dir != "" || *actual.dir != "" || *baseline.dir != "" || *operations != "" || *ha != "" || *he != "" {
		return bad("deployment-only options in drift request")
	}
	// Reject orphaned flags rather than silently ignoring their meaning.
	orphan := false
	f.Visit(func(fl *flag.Flag) {
		for _, name := range []string{"left", "right", "source", "actual", "baseline"} {
			if fl.Name == name || strings.HasPrefix(fl.Name, name+"-") {
				if *mode == "drift" && (name == "source" || name == "actual" || name == "baseline") {
					orphan = true
				}
				if *mode == "deployment" && (name == "left" || name == "right") {
					orphan = true
				}
				if name == "baseline" && *baseline.dir == "" {
					orphan = true
				}
			}
		}
	})
	if orphan {
		return bad("input options conflict with comparison roles")
	}
	if *left.dir == "" || *right.dir == "" {
		return bad("both comparison input directories required")
	}
	l, e := readInput(left)
	if e != nil {
		return fail(e)
	}
	r, e := readInput(right)
	if e != nil {
		return fail(e)
	}
	p := differ.DefaultPolicy()
	if *policyPath != "" {
		b, e := os.ReadFile(*policyPath)
		if e != nil {
			return bad("cannot read policy")
		}
		p, e = differ.DecodePolicy(b)
		if e != nil {
			return fail(e)
		}
	}

	req := differ.Request{Mode: *mode, Left: l, Right: r, Scope: list(*scope), ObjectKey: *name, IncludeMatching: *matching}
	if *baseline.dir != "" {
		b, e := readInput(baseline)
		if e != nil {
			return fail(e)
		}
		req.Baseline = &b
	}
	if *operations != "" {
		b, e := os.ReadFile(*operations)
		if e != nil {
			return bad("cannot read operations")
		}
		req.Operations, e = v2.DecodeOperations(b)
		if e != nil {
			return fail(e)
		}
	}
	adapterDiagnostics := []v2.Diagnostic{}
	if *hub != "" {
		leftRole, rightRole := "left", "right"
		if *mode == "deployment" {
			leftRole, rightRole = "source", "actual"
		}
		leftEnv, e := effectiveEnvironment(l.Environment, p.Bindings[leftRole])
		if e != nil {
			return fail(e)
		}
		rightEnv, e := effectiveEnvironment(r.Environment, p.Bindings[rightRole])
		if e != nil {
			return fail(e)
		}
		opts := differ.HubOptions{SourceTenant: *hs, TargetTenant: *ht, LeftEnvironment: leftEnv, RightEnvironment: rightEnv, DefaultSourceTenant: *hd, DeploymentAuthority: *ha}
		if *he != "" {
			b, e := os.ReadFile(*he)
			if e != nil {
				return bad("cannot read deployment evidence")
			}
			ev, e := v2.DecodeEvidence(b)
			if e != nil {
				return fail(e)
			}
			opts.DeploymentEvidence = &ev
		}
		if *mode == "deployment" && (*ha == "" || *he == "") {
			return bad("deployment Hub import requires explicit authority and evidence")
		}
		imported, e := differ.ReadHub(*hub, opts)
		if e != nil {
			return fail(e)
		}
		p.Equivalences = append(p.Equivalences, imported.Equivalences...)
		p.Expectations = append(p.Expectations, imported.Expectations...)
		p.Evidence = append(p.Evidence, imported.Evidence...)
		adapterDiagnostics = imported.Diagnostics
	} else if *hs != "" || *ht != "" || *hd != "" || *ha != "" || *he != "" {
		return bad("Hub options require a mapping input")
	}
	report, e := differ.Compare(req, p)
	if e != nil {
		return fail(e)
	}
	report.Diagnostics = append(report.Diagnostics, adapterDiagnostics...)
	var rendered bytes.Buffer
	if *format == "json" {
		enc := json.NewEncoder(&rendered)
		enc.SetIndent("", "  ")
		e = enc.Encode(report)
	} else {
		e = report.Text(&rendered)
	}
	if e != nil {
		return bad("cannot write report")
	}
	filename := *outputFile
	if filename == "" {
		ext := ".txt"
		if *format == "json" {
			ext = ".json"
		}
		filename = p.ID + ext
	}
	if e = saveReport(*outputDir, filename, rendered.Bytes()); e != nil {
		return fail(fmt.Errorf("cannot save report: %w", e))
	}
	return report.ExitCode()
}

// saveReport replaces the destination only after the complete report is written.
func saveReport(dir, filename string, data []byte) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".isc-configdiffer-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, filename))
}

func effectiveEnvironment(input, bound string) (string, error) {
	if bound != "" {
		if input != "" && input != bound {
			return "", fmt.Errorf("conflicting environment binding")
		}
		return bound, nil
	}
	return input, nil
}

func list(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
func main() { os.Exit(run(os.Args[1:], os.Stderr)) }
