// Package differ compares validated ISC configuration inputs offline.
package differ

import (
	"sort"
	"time"

	"github.com/elianderson/iscutilities/configdiffer/internal/iscprofile"
	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

// Request keeps validated ISC storage inputs outside the neutral engine.
// Left/Right mean source/actual when Mode is deployment.
type Request struct {
	Mode        string
	Left, Right snapshot.Input
	Baseline    *snapshot.Input
	Scope       []string
	// ObjectKey selects one logical object key within a single explicit scope.
	ObjectKey       string
	Operations      []v2.Operation
	IncludeMatching bool
	// ExcludeValues omits observed and expected values from report decisions.
	ExcludeValues bool
}

func DefaultPolicy() v2.Policy {
	return v2.Policy{Version: 2, ID: "default", Profile: "isc-envelope-v2"}
}
func AdaptSnapshot(in snapshot.Input) (v2.Input, error) {
	in, e := snapshot.Clone(in)
	if e != nil {
		return v2.Input{}, e
	}
	completed := ""
	if !in.Manifest.Completed.IsZero() {
		completed = in.Manifest.Completed.Format(time.RFC3339Nano)
	}
	out := v2.Input{Label: in.Label, Environment: in.Environment, Tenant: in.Manifest.Tenant, ContentHash: in.ContentHash, ManifestHash: in.ManifestHash, JobID: in.Manifest.JobID, Completed: completed, ReadMode: string(in.Mode), Types: append([]string{}, in.Manifest.Types...), Documents: []v2.Document{}}
	for _, o := range in.Objects {
		metadata := map[string]any{}
		for k, v := range o.Envelope {
			if k != "object" {
				metadata[k] = v
			}
		}
		variant := ""
		if o.Type == "SOURCE" {
			variant, e = iscprofile.ConnectorVariant(o.Envelope["object"])
			if e != nil {
				return v2.Input{}, e
			}
		}
		out.Documents = append(out.Documents, v2.Document{Type: o.Type, Variant: variant, ID: o.ID, Path: o.Path, Content: o.Envelope["object"], Metadata: metadata})
	}
	return out, nil
}
func Compare(r Request, p v2.Policy) (v2.Report, error) {
	var report v2.Report
	left, e := AdaptSnapshot(r.Left)
	if e != nil {
		return report, e
	}
	right, e := AdaptSnapshot(r.Right)
	if e != nil {
		return report, e
	}
	req := v2.Request{Mode: r.Mode, Left: left, Right: right, Scope: r.Scope, ObjectKey: r.ObjectKey, Operations: r.Operations, IncludeMatching: r.IncludeMatching, ExcludeValues: r.ExcludeValues}
	if r.Baseline != nil {
		b, e := AdaptSnapshot(*r.Baseline)
		if e != nil {
			return report, e
		}
		req.Baseline = &b
	}
	c, e := Compile(p)
	if e != nil {
		return report, e
	}
	return v2.Run(req, c)
}
func Compile(p v2.Policy) (*v2.Compiled, error) {
	if p.Profile == "" {
		compiled, err := v2.Compile(p)
		return compiled, policyFileError(p, err)
	}
	profile, e := ISCProfile(p)
	if e != nil {
		return nil, policyFileError(p, e)
	}
	compiled, err := v2.Compile(p, profile)
	return compiled, policyFileError(p, err)
}

// ISCProfile builds ISC domain rules for the vendor-neutral comparison engine.
func ISCProfile(p v2.Policy) (v2.Profile, error) {
	return iscprofile.Build(p)
}

// Policy and Report use the current comparison schema.
type Policy = v2.Policy
type Report = v2.Report

func DecodePolicy(data []byte) (Policy, error) { return v2.Decode(data) }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
