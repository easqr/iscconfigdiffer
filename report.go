package differ

import (
	"fmt"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
	"io"
	"time"
)

type Diagnostic struct {
	Code     string `json:"code"`
	Side     string `json:"side,omitempty"`
	RuleID   string `json:"rule_id,omitempty"`
	Type     string `json:"type,omitempty"`
	Path     string `json:"path,omitempty"`
	Required bool   `json:"required,omitempty"`
}
type Difference struct {
	Path        string   `json:"path"`
	Kind        string   `json:"kind"`
	RuleID      string   `json:"rule_id,omitempty"`
	FailedSides []string `json:"failed_sides,omitempty"`
}
type Provenance struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}
type Entry struct {
	Type        string       `json:"type"`
	Key         string       `json:"key"`
	Status      string       `json:"status"`
	PairingRule string       `json:"pairing_rule,omitempty"`
	Left        *Provenance  `json:"left,omitempty"`
	Right       *Provenance  `json:"right,omitempty"`
	Differences []Difference `json:"differences,omitempty"`
}
type Counts struct {
	MissingLeft  int `json:"missing_left"`
	MissingRight int `json:"missing_right"`
	Different    int `json:"different"`
	Matching     int `json:"matching"`
}
type InputSummary struct {
	Completed    string        `json:"completed,omitempty"`
	Mode         snapshot.Mode `json:"mode"`
	Label        string        `json:"label"`
	Environment  string        `json:"environment,omitempty"`
	Tenant       string        `json:"tenant,omitempty"`
	JobID        string        `json:"job_id,omitempty"`
	ManifestHash string        `json:"manifest_hash,omitempty"`
	ContentHash  string        `json:"content_hash"`
}
type Report struct {
	Version       int          `json:"version"`
	Complete      bool         `json:"complete"`
	Left          InputSummary `json:"left"`
	Right         InputSummary `json:"right"`
	Scope         []string     `json:"scope"`
	ExcludedLeft  []string     `json:"excluded_left"`
	ExcludedRight []string     `json:"excluded_right"`
	PolicyID      string       `json:"policy_id"`
	PolicyDigest  string       `json:"policy_digest"`
	Profile       string       `json:"profile"`
	Counts        Counts       `json:"counts"`
	Entries       []Entry      `json:"entries"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
}

func summary(i snapshot.Input) InputSummary {
	completed := ""
	if !i.Manifest.Completed.IsZero() {
		completed = i.Manifest.Completed.Format(time.RFC3339Nano)
	}
	return InputSummary{Completed: completed, Mode: i.Mode, Label: i.Label, Environment: i.Environment, Tenant: i.Manifest.Tenant, JobID: i.Manifest.JobID, ManifestHash: i.ManifestHash, ContentHash: i.ContentHash}
}
func (r Report) ExitCode() int {
	if !r.Complete {
		return 2
	}
	if r.Counts.MissingLeft+r.Counts.MissingRight+r.Counts.Different > 0 {
		return 1
	}
	return 0
}
func (r Report) Text(w io.Writer) error {
	if _, e := fmt.Fprintf(w, "ISC comparison v%d: %q / %q\nComplete: %t; missing left: %d; missing right: %d; different: %d; matching: %d\n", r.Version, r.Left.Label, r.Right.Label, r.Complete, r.Counts.MissingLeft, r.Counts.MissingRight, r.Counts.Different, r.Counts.Matching); e != nil {
		return e
	}
	for _, e := range r.Entries {
		if _, err := fmt.Fprintf(w, "%s %s %q\n", e.Status, e.Type, e.Key); err != nil {
			return err
		}
		for _, d := range e.Differences {
			if _, err := fmt.Fprintf(w, "  %s %q rule=%q failed=%v\n", d.Kind, d.Path, d.RuleID, d.FailedSides); err != nil {
				return err
			}
		}
	}
	for _, d := range r.Diagnostics {
		if _, e := fmt.Fprintf(w, "diagnostic: %s side=%s type=%s rule=%s path=%s\n", d.Code, d.Side, d.Type, d.RuleID, d.Path); e != nil {
			return e
		}
	}
	return nil
}
