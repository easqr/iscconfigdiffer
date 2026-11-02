package main

import (
	"encoding/json"
	"flag"
	"fmt"
	differ "github.com/elianderson/iscutilities/configdiffer"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"io"
	"os"
	"strings"
)

func list(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
func run(args []string, out, errOut io.Writer) int {
	f := flag.NewFlagSet("isc-configdiffer", flag.ContinueOnError)
	f.SetOutput(errOut)
	left := f.String("left", "", "left input directory")
	right := f.String("right", "", "right input directory")
	lm := f.String("left-mode", "", "snapshot or tree (required)")
	rm := f.String("right-mode", "", "snapshot or tree (required)")
	ll := f.String("left-label", "left", "left display label")
	rl := f.String("right-label", "right", "right display label")
	le := f.String("left-env", "", "left environment")
	re := f.String("right-env", "", "right environment")
	lt := f.String("left-types", "", "tree scope without manifest (comma separated)")
	rt := f.String("right-types", "", "tree scope without manifest (comma separated)")
	scope := f.String("scope", "", "explicit common comparison types")
	policy := f.String("policy", "", "version 1 JSON policy")
	format := f.String("format", "text", "text or json")
	matching := f.Bool("matching", false, "include matching entries")
	hub := f.String("hub", "", "saved Object Mapping JSON array")
	hs := f.String("hub-source-tenant", "", "Hub source tenant")
	ht := f.String("hub-target-tenant", "", "Hub target tenant")
	if e := f.Parse(args); e != nil {
		return 2
	}
	fail := func(msg string) int { fmt.Fprintln(errOut, msg); return 2 }
	if f.NArg() != 0 || *left == "" || *right == "" || (*format != "text" && *format != "json") {
		return fail("left/right directories, explicit modes and a valid format are required")
	}
	p := differ.DefaultPolicy()
	if *policy != "" {
		b, e := os.ReadFile(*policy)
		if e != nil {
			return fail("cannot read policy")
		}
		p, e = differ.DecodePolicy(b)
		if e != nil {
			return fail(e.Error())
		}
	}
	if *hub != "" {
		b, e := os.ReadFile(*hub)
		if e != nil {
			return fail("cannot read Hub mappings")
		}
		rules, e := differ.ImportHub(b, differ.HubContext{SourceTenant: *hs, TargetTenant: *ht, Provenance: "sha256:" + snapshot.Checksum(b)})
		if e != nil {
			return fail(e.Error())
		}
		p.Hub = append(p.Hub, rules...)
	}
	l, e := snapshot.Read(*left, snapshot.Options{Mode: snapshot.Mode(*lm), Label: *ll, Environment: *le, Types: list(*lt)})
	if e != nil {
		return fail("invalid left input: " + e.Error())
	}
	r, e := snapshot.Read(*right, snapshot.Options{Mode: snapshot.Mode(*rm), Label: *rl, Environment: *re, Types: list(*rt)})
	if e != nil {
		return fail("invalid right input: " + e.Error())
	}
	report, e := differ.Compare(l, r, p, differ.Options{Scope: list(*scope), IncludeMatching: *matching})
	if e != nil {
		return fail(e.Error())
	}
	if *format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		e = enc.Encode(report)
	} else {
		e = report.Text(out)
	}
	if e != nil {
		return fail("cannot write report")
	}
	return report.ExitCode()
}
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
