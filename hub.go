package differ

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/easqr/iscconfigexporter/snapshot"
	"github.com/elianderson/iscutilities/configdiffer/v2"
)

type HubOptions struct {
	SourceTenant, TargetTenant        string
	LeftEnvironment, RightEnvironment string
	// DefaultSourceTenant explicitly binds the remote "default" group.
	DefaultSourceTenant string
	// DeploymentAuthority/Evidence explicitly promote mappings to expectations.
	DeploymentAuthority string
	DeploymentEvidence  *v2.Evidence
}
type HubResult struct {
	Equivalences []v2.Equivalence
	Expectations []v2.Expectation
	Evidence     []v2.Evidence
	Diagnostics  []v2.Diagnostic
}
type mappingSidecar struct {
	Schema       int    `json:"schema"`
	RetrievalID  string `json:"retrieval_id"`
	SourceOrg    string `json:"source_org"`
	TargetTenant string `json:"target_tenant"`
	APIOrigin    string `json:"api_origin"`
	APIVersion   string `json:"api_version"`
	Endpoint     string `json:"endpoint"`
	Retrieved    string `json:"retrieved"`
	ExportJobID  string `json:"export_job_id,omitempty"`
	Count        int    `json:"count"`
	SHA256       string `json:"sha256"`
}

// ReadHub reads an array file or a pinned exporter bundle directory offline.
func ReadHub(path string, options HubOptions) (HubResult, error) {
	st, e := os.Stat(path)
	if e != nil {
		return HubResult{}, errors.New("cannot inspect mapping input")
	}
	if !st.IsDir() {
		if !st.Mode().IsRegular() {
			return HubResult{}, errors.New("mapping input must be a regular file or bundle directory")
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return HubResult{}, errors.New("cannot read mapping array")
		}
		return ImportHub(b, nil, options)
	}
	r, e := os.OpenRoot(path)
	if e != nil {
		return HubResult{}, errors.New("cannot open mapping bundle")
	}
	defer r.Close()
	data := map[string][]byte{}
	for _, name := range []string{"mappings.json", "provenance.json"} {
		st, e := r.Lstat(name)
		if e != nil || !st.Mode().IsRegular() {
			return HubResult{}, errors.New("unsafe or missing mapping bundle file")
		}
		b, e := r.ReadFile(name)
		if e != nil {
			return HubResult{}, errors.New("cannot read mapping bundle")
		}
		data[name] = b
	}
	return ImportHub(data["mappings.json"], data["provenance.json"], options)
}
func ImportHub(data, sidecar []byte, o HubOptions) (HubResult, error) {
	out := HubResult{}
	raw, e := snapshot.DecodeJSON(data)
	if e != nil {
		return out, errors.New("invalid mapping JSON")
	}
	records, ok := raw.([]any)
	if !ok {
		return out, errors.New("mapping array required")
	}
	digest := snapshot.Checksum(data)
	if len(sidecar) > 0 {
		v, e := snapshot.DecodeJSON(sidecar)
		if e != nil {
			return out, errors.New("invalid mapping provenance")
		}
		fields, ok := v.(map[string]any)
		if !ok {
			return out, errors.New("mapping provenance object required")
		}
		allowed := map[string]bool{"schema": true, "retrieval_id": true, "source_org": true, "target_tenant": true, "api_origin": true, "api_version": true, "endpoint": true, "retrieved": true, "count": true, "sha256": true, "export_job_id": true}
		for key, value := range fields {
			if !allowed[key] {
				return out, errors.New("unknown mapping provenance property")
			}
			switch key {
			case "schema", "count":
				if _, ok := value.(json.Number); !ok {
					return out, errors.New("mapping provenance integer required")
				}
			default:
				if _, ok := value.(string); !ok {
					return out, errors.New("mapping provenance string required")
				}
			}
		}
		for key := range allowed {
			if key == "export_job_id" {
				continue
			}
			if _, ok := fields[key]; !ok {
				return out, errors.New("missing mapping provenance property")
			}
		}
		var p mappingSidecar
		if e = json.Unmarshal(sidecar, &p); e != nil {
			return out, errors.New("invalid mapping provenance")
		}
		u, e := url.Parse(p.APIOrigin)
		retrieved, te := time.Parse(time.RFC3339Nano, p.Retrieved)
		if p.Schema != 1 || p.RetrievalID == "" || p.SourceOrg == "" || p.TargetTenant == "" || p.Count != len(records) || p.SHA256 != digest || p.APIVersion != "v2026" || p.Endpoint != "/v2026/configuration-hub/object-mappings/"+url.PathEscape(p.SourceOrg) || te != nil || retrieved.IsZero() || e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return out, errors.New("inconsistent mapping bundle provenance")
		}
		if o.SourceTenant != "" && o.SourceTenant != p.SourceOrg {
			return out, errors.New("mapping source context conflicts with sidecar")
		}
		if o.TargetTenant != "" && o.TargetTenant != p.TargetTenant {
			return out, errors.New("mapping target context conflicts with sidecar")
		}
		o.SourceTenant = p.SourceOrg
		o.TargetTenant = p.TargetTenant
		out.Evidence = append(out.Evidence, v2.Evidence{Kind: "hub-retrieval", Digest: snapshot.Checksum(sidecar), RecordID: p.RetrievalID, Attributes: map[string]string{"source_org": p.SourceOrg, "target_tenant": p.TargetTenant, "api_origin": p.APIOrigin, "api_version": p.APIVersion, "endpoint": p.Endpoint, "retrieved": p.Retrieved, "export_job_id": p.ExportJobID}})
	}
	if o.SourceTenant == "default" {
		if o.DefaultSourceTenant == "" || o.DefaultSourceTenant == "default" {
			return out, errors.New("default mapping group requires explicit source tenant binding")
		}
		o.SourceTenant = o.DefaultSourceTenant
	} else if o.DefaultSourceTenant != "" {
		return out, errors.New("default binding supplied for a tenant-specific mapping group")
	}
	if o.SourceTenant == "" || o.TargetTenant == "" || o.LeftEnvironment == "" || o.RightEnvironment == "" {
		return out, errors.New("mapping tenant context and environment bindings required")
	}
	if (o.DeploymentAuthority == "") != (o.DeploymentEvidence == nil) {
		return out, errors.New("deployment promotion requires authority and evidence together")
	}
	if o.DeploymentEvidence != nil && o.DeploymentEvidence.Kind == "" {
		return out, errors.New("deployment evidence kind required")
	}
	seen := map[string]bool{}
	rules := []hubRule{}
	for _, record := range records {
		m, ok := record.(map[string]any)
		if !ok {
			return out, errors.New("invalid mapping record")
		}
		for _, key := range []string{"objectMappingId", "objectType", "jsonPath", "sourceValue", "targetValue"} {
			s, ok := m[key].(string)
			if !ok || (s == "" && key != "sourceValue" && key != "targetValue") {
				return out, errors.New("mapping string field required")
			}
		}
		id := m["objectMappingId"].(string)
		enabled, ok := m["enabled"].(bool)
		if !ok || seen[id] {
			return out, errors.New("mapping enabled status or unique ID required")
		}
		seen[id] = true
		for _, key := range []string{"created", "modified"} {
			if v, ok := m[key]; ok {
				if _, ok := v.(string); !ok {
					return out, errors.New("mapping timestamp must be string")
				}
			}
		}
		b, _ := json.Marshal(m)
		ev := v2.Evidence{Kind: "hub-mapping", Digest: hashMappingRecord(b), RecordID: id, Attributes: map[string]string{"json_path": m["jsonPath"].(string), "enabled": "false", "source_tenant": o.SourceTenant, "target_tenant": o.TargetTenant, "array_digest": digest}}
		for _, key := range []string{"created", "modified"} {
			if v, ok := m[key].(string); ok {
				ev.Attributes[key] = v
			}
		}
		if enabled {
			ev.Attributes["enabled"] = "true"
		}
		out.Evidence = append(out.Evidence, ev)
		if !enabled {
			out.Diagnostics = append(out.Diagnostics, v2.Diagnostic{Code: "disabled Hub mapping retained as evidence", RuleID: id})
			continue
		}
		allowed := map[string]bool{"objectMappingId": true, "objectType": true, "jsonPath": true, "sourceValue": true, "targetValue": true, "enabled": true, "created": true, "modified": true}
		for key := range m {
			if !allowed[key] {
				return out, errors.New("unsupported active mapping property")
			}
		}
		path, e := HubPointer(m["jsonPath"].(string))
		if e != nil {
			return out, e
		}
		if err := snapshot.ValidateTypes([]string{m["objectType"].(string)}); err != nil {
			return out, errors.New("unsupported mapping object type")
		}
		rules = append(rules, hubRule{ID: "hub." + snapshot.Checksum([]byte(id))[:24], Type: m["objectType"].(string), Path: path, Source: m["sourceValue"].(string), Target: m["targetValue"].(string), SourceTenant: o.SourceTenant, TargetTenant: o.TargetTenant, Provenance: digest})
	}
	p := v2.Policy{Version: 2, ID: "hub-import"}
	appendHubRules(&p, rules, o.LeftEnvironment, o.RightEnvironment)
	if _, err := v2.Compile(p); err != nil {
		return out, err
	}
	out.Equivalences = p.Equivalences
	if o.DeploymentAuthority != "" {
		for _, r := range out.Equivalences {
			// Several source cases need conditional expectation semantics, which the
			// approved literal operation does not express. Do not guess a target.
			if len(r.Pairs) != 1 {
				return out, errors.New("multiple mapping cases require explicit deployment expectations")
			}
			b, _ := json.Marshal(r.Pairs[0].Right)
			rr := r.Rule
			rr.Modes = []string{"deployment"}
			rr.Evidence = o.DeploymentEvidence
			source, _ := json.Marshal(r.Pairs[0].Left)
			out.Expectations = append(out.Expectations, v2.Expectation{Rule: rr, Selector: r.Selector, Operation: "literal", Values: map[string]v2.State{o.RightEnvironment: {State: "present", Value: b}}, Authority: o.DeploymentAuthority, SourceEquals: &v2.State{State: "present", Value: source}, SourceEnvironment: o.LeftEnvironment, SourceTenant: o.SourceTenant, TargetTenant: o.TargetTenant})
		}
		out.Equivalences = nil
	}
	return out, nil
}
func hashMappingRecord(b []byte) string { return snapshot.Checksum(b) }
func appendHubRules(p *v2.Policy, rules []hubRule, leftEnv, rightEnv string) {
	groups := map[string][]hubRule{}
	for _, r := range rules {
		groups[r.Type+"\x00"+r.Path] = append(groups[r.Type+"\x00"+r.Path], r)
	}
	for _, k := range sortedKeys(groups) {
		g := groups[k]
		sort.Slice(g, func(i, j int) bool { return g[i].ID < g[j].ID })
		first := g[0]
		ids := []string{}
		pairs := []v2.ValuePair{}
		for _, r := range g {
			ids = append(ids, r.ID)
			pairs = append(pairs, v2.ValuePair{Left: r.Source, Right: r.Target})
		}
		p.Equivalences = append(p.Equivalences, v2.Equivalence{Rule: v2.Rule{ID: first.ID, Modes: []string{"drift"}, Evidence: &v2.Evidence{Kind: "hub-mapping-group", Digest: first.Provenance, RecordID: strings.Join(ids, ",")}}, Selector: v2.Selector{Type: first.Type, Path: first.Path}, LeftEnv: leftEnv, RightEnv: rightEnv, LeftTenant: first.SourceTenant, RightTenant: first.TargetTenant, Pairs: pairs})
	}
}

// hubRule is an adapter-local record, never part of the neutral policy contract.
type hubRule struct {
	ID, Type, Path, Source, Target, SourceTenant, TargetTenant, Provenance string
}
