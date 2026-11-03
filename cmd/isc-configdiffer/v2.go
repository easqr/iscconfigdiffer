package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	differ "github.com/elianderson/iscutilities/configdiffer"
	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func run(args []string, out, errOut io.Writer) int {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=")
		if name == "mode" || name == "translate-policy" {
			return runV2(args, out, errOut)
		}
		if !hasValue && name != "matching" && name != "h" && name != "help" {
			i++
		}
	}
	return runLegacy(args, out, errOut)
}

type inputFlags struct{ dir, mode, label, env, types *string }

func addInput(f *flag.FlagSet, role string) inputFlags {
	return inputFlags{f.String(role, "", role+" input directory"), f.String(role+"-mode", "", "snapshot or tree (required)"), f.String(role+"-label", role, "display label"), f.String(role+"-env", "", "environment"), f.String(role+"-types", "", "manifest-free tree types, comma separated")}
}
func readInput(f inputFlags) (snapshot.Input, error) {
	return snapshot.Read(*f.dir, snapshot.Options{Mode: snapshot.Mode(*f.mode), Label: *f.label, Environment: *f.env, Types: list(*f.types)})
}
func runV2(args []string, out, errOut io.Writer) int {
	f := flag.NewFlagSet("isc-configdiffer v2", flag.ContinueOnError)
	f.SetOutput(errOut)
	mode := f.String("mode", "", "drift or deployment")
	left, right := addInput(f, "left"), addInput(f, "right")
	source, actual, baseline := addInput(f, "source"), addInput(f, "actual"), addInput(f, "baseline")
	policyPath := f.String("policy", "", "v2 or translatable v1 JSON policy")
	translation := f.String("translate-policy", "", "write migrated v1 policy to stdout without comparison")
	format := f.String("format", "text", "text or json")
	scope := f.String("scope", "", "explicit common type scope")
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
	if f.NArg() != 0 {
		return bad("unexpected arguments")
	}
	if *translation != "" {
		other := false
		f.Visit(func(fl *flag.Flag) {
			if fl.Name != "translate-policy" {
				other = true
			}
		})
		if other {
			return bad("translation accepts only --translate-policy")
		}
		if *mode != "" || *policyPath != "" || *left.dir != "" || *right.dir != "" || *source.dir != "" || *actual.dir != "" || *baseline.dir != "" || *operations != "" || *hub != "" {
			return bad("translation cannot be combined with execution")
		}
		b, e := os.ReadFile(*translation)
		if e != nil {
			return bad("cannot read legacy policy")
		}
		p, e := differ.DecodePolicy(b)
		if e != nil {
			return fail(e)
		}
		next, e := differ.TranslateV1(p)
		if e != nil {
			return fail(e)
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if e = enc.Encode(next); e != nil {
			return bad("cannot write migrated policy")
		}
		return 0
	}
	if (*mode != "drift" && *mode != "deployment") || (*format != "json" && *format != "text") {
		return bad("explicit drift/deployment mode and valid format required")
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
	p := differ.DefaultV2Policy()
	if *policyPath != "" {
		b, e := os.ReadFile(*policyPath)
		if e != nil {
			return bad("cannot read policy")
		}
		raw, e := snapshot.DecodeJSON(b)
		if e != nil {
			return fail(e)
		}
		m, ok := raw.(map[string]any)
		if !ok {
			return bad("policy object required")
		}
		if n, ok := m["version"].(json.Number); ok && n.String() == "1" {
			if *mode != "drift" {
				return bad("legacy policies are drift-only; write an explicit v2 deployment policy")
			}
			legacy, e := differ.DecodePolicy(b)
			if e != nil {
				return fail(e)
			}
			if len(legacy.Hub) > 0 {
				if legacy.Bindings == nil {
					legacy.Bindings = map[string]string{}
				}
				if legacy.Bindings["left"] == "" && l.Environment != "" {
					legacy.Bindings["left"] = l.Environment
				}
				if legacy.Bindings["right"] == "" && r.Environment != "" {
					legacy.Bindings["right"] = r.Environment
				}
			}
			p, e = differ.TranslateV1(legacy)
			if e != nil {
				return fail(e)
			}
		} else {
			p, e = v2.Decode(b)
			if e != nil {
				return fail(e)
			}
		}
	}
	req := differ.V2Request{Mode: *mode, Left: l, Right: r, Scope: list(*scope), IncludeMatching: *matching}
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
		opts := differ.HubV2Options{SourceTenant: *hs, TargetTenant: *ht, LeftEnvironment: leftEnv, RightEnvironment: rightEnv, DefaultSourceTenant: *hd, DeploymentAuthority: *ha}
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
		imported, e := differ.ReadHubV2(*hub, opts)
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
	report, e := differ.CompareV2(req, p)
	if e != nil {
		return fail(e)
	}
	report.Diagnostics = append(report.Diagnostics, adapterDiagnostics...)
	if *format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		e = enc.Encode(report)
	} else {
		e = report.Text(out)
	}
	if e != nil {
		return bad("cannot write report")
	}
	return report.ExitCode()
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
