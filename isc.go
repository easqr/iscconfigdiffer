// Package differ compares validated ISC configuration inputs offline.
package differ

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

// Request keeps validated ISC storage inputs outside the neutral engine.
// Left/Right mean source/actual when Mode is deployment.
type Request struct {
	Mode            string
	Left, Right     snapshot.Input
	Baseline        *snapshot.Input
	Scope           []string
	Operations      []v2.Operation
	IncludeMatching bool
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
		out.Documents = append(out.Documents, v2.Document{Type: o.Type, ID: o.ID, Path: o.Path, Content: o.Envelope["object"], Metadata: metadata})
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
	req := v2.Request{Mode: r.Mode, Left: left, Right: right, Scope: r.Scope, Operations: r.Operations, IncludeMatching: r.IncludeMatching}
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
		return v2.Compile(p)
	}
	profile, e := ISCProfile(p)
	if e != nil {
		return nil, e
	}
	return v2.Compile(p, profile)
}

// ISCProfile expands explicit type identities and normalization rules. The identity
// profile compares all fields; envelope and portable profiles add named exclusions.
func ISCProfile(p v2.Policy) (v2.Profile, error) {
	if p.Profile != "isc-identity-v2" && p.Profile != "isc-envelope-v2" && p.Profile != "isc-portable-v2" {
		return v2.Profile{}, errors.New("unknown ISC v2 profile")
	}
	profile := v2.Profile{ID: p.Profile}
	rule := func(id string) v2.Rule {
		return v2.Rule{ID: id, Evidence: &v2.Evidence{Kind: "isc-profile", RecordID: p.Profile}}
	}
	types := map[string]bool{}
	addIdentity := func(typ, strategy string, paths ...string) {
		types[typ] = true
		componentType := ""
		if len(paths) > 0 {
			componentType = "string"
		}
		profile.Rules.Identities = append(profile.Rules.Identities, v2.Identity{Rule: rule("isc.identity." + typ), Type: typ, Strategy: strategy, Paths: paths, ComponentType: componentType})
	}
	for _, typ := range namedTypes {
		addIdentity(typ, "fields", "/name")
	}
	for _, typ := range []string{"ACCESS_REQUEST_CONFIG", "AUTH_ORG"} {
		addIdentity(typ, "singleton")
	}
	addIdentity("NOTIFICATION_TEMPLATE", "fields", "/key", "/locale", "/medium")
	for _, r := range []v2.Identity{{Rule: rule("isc.identity.LIFECYCLE_STATE"), Type: "LIFECYCLE_STATE", Strategy: "dependent", Paths: []string{"/technicalName"}, ComponentType: "string", ParentType: "IDENTITY_PROFILE", ParentPath: "/identityProfileRef/id"}, {Rule: rule("isc.identity.ATTR_SYNC_SOURCE_CONFIG"), Type: "ATTR_SYNC_SOURCE_CONFIG", Strategy: "dependent", ParentType: "SOURCE", ParentPath: "/source/id"}} {
		types[r.Type] = true
		profile.Rules.Identities = append(profile.Rules.Identities, r)
	}
	for _, r := range p.Identities {
		types[r.Type] = true
	}
	ignore := func(typ, sp, path, label string) {
		profile.Rules.Ignores = append(profile.Rules.Ignores, v2.Ignore{Rule: rule("isc." + typ + "." + label), Selector: v2.Selector{Type: typ, Space: sp, Path: path}})
	}
	for _, typ := range sortedKeys(types) {
		if p.Profile != "isc-identity-v2" {
			ignore(typ, "metadata", "/jwsHeader", "jws-header")
			ignore(typ, "metadata", "/jwsSignature", "jws-signature")
		}
		if p.Profile == "isc-portable-v2" {
			ignore(typ, "metadata", "/self/id", "physical-id")
			for _, field := range []string{"id", "created", "modified"} {
				ignore(typ, "content", "/"+field, field)
			}
			if typ == "LIFECYCLE_STATE" {
				ignore(typ, "content", "/identityCount", "identity-count")
			}
		}
		// Structural envelope expectations are explicit and deployment-only.
		for _, path := range []string{"/version", "/self/type"} {
			r := rule("isc." + typ + ".envelope" + strings.ReplaceAll(path, "/", "."))
			r.Modes = []string{"deployment"}
			profile.Rules.Expectations = append(profile.Rules.Expectations, v2.Expectation{Rule: r, Selector: v2.Selector{Type: typ, Space: "metadata", Path: path}, Operation: "copy_source", Authority: "isc-envelope-structure"})
		}
	}
	return profile, nil
}

// Policy and Report use the current comparison schema.
type Policy = v2.Policy
type Report = v2.Report

func DecodePolicy(data []byte) (Policy, error) { return v2.Decode(data) }

var namedTypes = strings.Fields("ACCESS_PROFILE CAMPAIGN_FILTER CONNECTOR_RULE FORM_DEFINITION GOVERNANCE_GROUP IDENTITY_OBJECT_CONFIG IDENTITY_PROFILE PASSWORD_POLICY PASSWORD_SYNC_GROUP PUBLIC_IDENTITIES_CONFIG ROLE RULE SEGMENT SERVICE_DESK_INTEGRATION SOD_POLICY SOURCE SPS_PARAMETER TAG TRANSFORM TRIGGER_SUBSCRIPTION WORKFLOW")

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
