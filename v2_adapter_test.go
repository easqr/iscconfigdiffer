package differ

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elianderson/iscutilities/configdiffer/v2"
	exporter "github.com/elianderson/iscutilities/configexporter"
	"github.com/elianderson/iscutilities/configexporter/snapshot"
)

func ruleV2(id string) v2.Rule {
	return v2.Rule{ID: id, Evidence: &v2.Evidence{Kind: "invented-approved-policy"}}
}
func portableV2() v2.Policy { p := DefaultV2Policy(); p.Profile = "isc-portable-v2"; return p }
func snapshotV2(t *testing.T, tenant, env string, types []string, objects ...map[string]any) snapshot.Input {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"tenant": tenant, "objects": objects})
	snap, e := exporter.Organize(tenant, exporter.Download{JobID: "invented-job", Types: types, Raw: raw}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	files := map[string][]byte{}
	for _, a := range snap.Artifacts {
		files[a.Path] = a.Data
	}
	files["manifest.json"], _ = json.Marshal(snap.Manifest)
	in, e := snapshot.FromFiles(files, snapshot.Options{Mode: snapshot.SnapshotMode, Environment: env})
	if e != nil {
		t.Fatal(e)
	}
	return in
}
func compareV2Test(t *testing.T, r V2Request, p v2.Policy) v2.Report {
	t.Helper()
	report, e := CompareV2(r, p)
	if e != nil {
		t.Fatal(e)
	}
	return report
}
func TestV2SnapshotAndLegacyCompatibility(t *testing.T) {
	types := []string{"ROLE"}
	left := input(t, "dev", types, obj("ROLE", "source-id", "Invented", map[string]any{"host": "d", "timeout": 60}))
	right := input(t, "prod", types, obj("ROLE", "actual-id", "Invented", map[string]any{"host": "p", "timeout": 60}))
	p := policy()
	p.Expectations = []Expectation{{ID: "host", Selector: Selector{Type: "ROLE", Path: "/object/host"}, Values: map[string]any{"dev": "d", "prod": "p"}}}
	p2, e := TranslateV1(p)
	if e != nil {
		t.Fatal(e)
	}
	for _, timeout := range []int{60, 90} {
		right = input(t, "prod", types, obj("ROLE", "actual-id", "Invented", map[string]any{"host": "p", "timeout": timeout}))
		legacy := runCompare(t, left, right, p)
		next := compareV2Test(t, V2Request{Mode: "drift", Left: left, Right: right}, p2)
		if legacy.ExitCode() != next.ExitCode() {
			t.Fatalf("legacy %d v2 %d", legacy.ExitCode(), next.ExitCode())
		}
	}
	left.Objects[0].Envelope["object"].(map[string]any)["name"] = "mutated"
	if _, e := AdaptSnapshot(left); e == nil {
		t.Fatal("adapter bypassed input validation")
	}
}
func TestV2ISCDependenciesAndProfiles(t *testing.T) {
	types := []string{"IDENTITY_PROFILE", "LIFECYCLE_STATE"}
	makeIn := func(env, prefix string) snapshot.Input {
		return input(t, env, types, obj("IDENTITY_PROFILE", prefix+"p", "People", nil), obj("LIFECYCLE_STATE", prefix+"l", "Active", map[string]any{"technicalName": "active", "identityProfileRef": map[string]any{"id": prefix + "p"}}))
	}
	left, right := makeIn("dev", "l"), makeIn("prod", "r")
	p := portableV2()
	p.References = []v2.Reference{{Rule: ruleV2("parent"), Selector: v2.Selector{Type: "LIFECYCLE_STATE", Path: "/identityProfileRef/id"}, TargetType: "IDENTITY_PROFILE"}}
	r := compareV2Test(t, V2Request{Mode: "drift", Left: left, Right: right}, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 {
		t.Fatal(r)
	}
	p.Pairings = []v2.Pairing{{Rule: ruleV2("rename"), Type: "IDENTITY_PROFILE", LeftID: "lp", RightID: "rp"}}
	right = input(t, "prod", types, obj("IDENTITY_PROFILE", "rp", "Renamed", nil), obj("LIFECYCLE_STATE", "rl", "Active", map[string]any{"technicalName": "active", "identityProfileRef": map[string]any{"id": "rp"}}))
	if r = compareV2Test(t, V2Request{Mode: "drift", Left: left, Right: right}, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
	// Profile replacement must be explicit for known identities.
	p = portableV2()
	p.Identities = []v2.Identity{{Rule: ruleV2("custom"), Type: "ROLE", Strategy: "fields", Paths: []string{"/name"}}}
	if _, e := CompileV2(p); e == nil {
		t.Fatal("implicit identity override")
	}
	p.Identities[0].Replaces = "isc.identity.ROLE"
	if _, e := CompileV2(p); e != nil {
		t.Fatal(e)
	}
}
func TestV2DeploymentReferenceAndPartialScope(t *testing.T) {
	types := []string{"ROLE", "SOURCE"}
	left := input(t, "dev", types, obj("ROLE", "lr", "Role", map[string]any{"reference": "ls"}), obj("SOURCE", "ls", "Directory", nil))
	right := input(t, "prod", types, obj("ROLE", "rr", "Role", map[string]any{"reference": "rs"}), obj("SOURCE", "rs", "Directory", nil))
	p := portableV2()
	for _, typ := range types {
		p.Expectations = append(p.Expectations, v2.Expectation{Rule: ruleV2("name." + typ), Selector: v2.Selector{Type: typ, Path: "/name"}, Operation: "copy_source", Authority: "approved"})
	}
	p.Expectations = append(p.Expectations, v2.Expectation{Rule: ruleV2("reference"), Selector: v2.Selector{Type: "ROLE", Path: "/reference"}, Operation: "reference_target", TargetType: "SOURCE", Authority: "approved"})
	req := V2Request{Mode: "deployment", Left: left, Right: right, Operations: []v2.Operation{{Type: "ROLE", Key: "Role", Action: "update"}, {Type: "SOURCE", Key: "Directory", Action: "update"}}}
	baseline := right
	req.Baseline = &baseline
	r := compareV2Test(t, req, p)
	if r.ExitCode() != 0 {
		t.Fatal(r)
	}
	right = input(t, "prod", types, obj("ROLE", "rr", "Role", map[string]any{"reference": "ls"}), obj("SOURCE", "rs", "Directory", nil))
	req.Right = right
	if r = compareV2Test(t, req, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
	// An unrelated source-only object does not imply a draft deletion.
	req.Left = input(t, "dev", types, obj("ROLE", "lr", "Role", map[string]any{"reference": "ls"}), obj("ROLE", "extra", "Unselected", nil), obj("SOURCE", "ls", "Directory", nil))
	if r = compareV2Test(t, req, p); r.Counts["violation"] != 1 {
		t.Fatal(r)
	}
	req.Right = input(t, "prod", types, obj("ROLE", "rr", "Role", map[string]any{"reference": "rs"}), obj("ROLE", "extra", "UnexpectedDraft", nil), obj("SOURCE", "rs", "Directory", nil))
	if r = compareV2Test(t, req, p); r.ExitCode() != 2 {
		t.Fatal(r)
	}
}
func TestV2HubBundleAndDeploymentPromotion(t *testing.T) {
	raw := hubJSON("$.dn", true)
	api := &retrievedMappingAPI{data: raw}
	saved, e := exporter.RetrieveMappings(context.Background(), api, exporter.MappingRequest{SourceOrg: "invented-prod", TargetTenant: "invented-sb"})
	if e != nil {
		t.Fatal(e)
	}
	directory := filepath.Join(t.TempDir(), "bundle")
	if _, e = (exporter.MappingFileStore{Directory: directory}).Publish(context.Background(), saved); e != nil {
		t.Fatal(e)
	}
	opts := HubV2Options{LeftEnvironment: "production", RightEnvironment: "sandbox"}
	imported, e := ReadHubV2(directory, opts)
	if e != nil {
		t.Fatal(e)
	}
	if len(imported.Equivalences) != 1 || len(imported.Evidence) != 2 {
		t.Fatal(imported)
	}
	left := snapshotV2(t, "invented-prod", "production", []string{"SOURCE"}, obj("SOURCE", "l", "Directory", map[string]any{"dn": "dc=example,dc=com"}))
	right := snapshotV2(t, "invented-sb", "sandbox", []string{"SOURCE"}, obj("SOURCE", "r", "Directory", map[string]any{"dn": "dc=example-dev,dc=com"}))
	p := portableV2()
	p.Equivalences = imported.Equivalences
	p.Evidence = imported.Evidence
	r := compareV2Test(t, V2Request{Mode: "drift", Left: left, Right: right}, p)
	if r.ExitCode() != 0 || r.Counts["equivalent"] != 1 {
		t.Fatal(r)
	}
	opts.DeploymentAuthority = "approved-release"
	opts.DeploymentEvidence = &v2.Evidence{Kind: "invented-release-approval", RecordID: "release-1"}
	promoted, e := ReadHubV2(directory, opts)
	if e != nil {
		t.Fatal(e)
	}
	p = portableV2()
	p.Expectations = promoted.Expectations
	p.Evidence = promoted.Evidence
	p.Expectations = append(p.Expectations, v2.Expectation{Rule: ruleV2("name"), Selector: v2.Selector{Type: "SOURCE", Path: "/name"}, Operation: "copy_source", Authority: "approved"})
	req := V2Request{Mode: "deployment", Left: left, Right: right, Operations: []v2.Operation{{Type: "SOURCE", Key: "Directory", Action: "update"}}}
	if r = compareV2Test(t, req, p); r.ExitCode() != 0 {
		t.Fatal(r)
	}
	req.Right = snapshotV2(t, "invented-sb", "sandbox", []string{"SOURCE"}, obj("SOURCE", "r", "Directory", map[string]any{"dn": "dc=example,dc=com"}))
	if r = compareV2Test(t, req, p); r.ExitCode() != 1 {
		t.Fatal(r)
	}
	// Source guards preserve mapping applicability instead of asserting unrelated values.
	req.Left = snapshotV2(t, "invented-prod", "production", []string{"SOURCE"}, obj("SOURCE", "l", "Directory", map[string]any{"dn": "unrelated"}))
	if r = compareV2Test(t, req, p); r.ExitCode() != 2 || r.Counts["unverified"] == 0 {
		t.Fatal(r)
	}
	if e = os.WriteFile(filepath.Join(directory, "mappings.json"), append(raw, ' '), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = ReadHubV2(directory, opts); e == nil {
		t.Fatal("checksum mismatch accepted")
	}
}
func TestV2HubUnsupportedAndDefault(t *testing.T) {
	opts := HubV2Options{SourceTenant: "invented-prod", TargetTenant: "invented-sb", LeftEnvironment: "production", RightEnvironment: "sandbox"}
	for _, raw := range [][]byte{hubJSON("$..dn", true), []byte(strings.Replace(string(hubJSON("$.dn", true)), `"sourceValue":"dc=example,dc=com"`, `"sourceValue":1`, 1)), []byte(strings.Replace(string(hubJSON("$.dn", true)), `"enabled":true`, `"enabled":true,"futureSemantics":true`, 1))} {
		if _, e := ImportHubV2(raw, nil, opts); e == nil {
			t.Fatal("unsupported mapping accepted")
		}
	}
	raw := []byte(strings.Replace(string(hubJSON("$..dn", false)), `"enabled":false`, `"enabled":false,"futureSemantics":true`, 1))
	out, e := ImportHubV2(raw, nil, opts)
	if e != nil || len(out.Equivalences) != 0 || len(out.Evidence) != 1 || len(out.Diagnostics) != 1 {
		t.Fatal(out, e)
	}
	opts.SourceTenant = "default"
	if _, e = ImportHubV2(hubJSON("$.dn", true), nil, opts); e == nil {
		t.Fatal("default guessed")
	}
	opts.DefaultSourceTenant = "invented-prod"
	if _, e = ImportHubV2(hubJSON("$.dn", true), nil, opts); e != nil {
		t.Fatal(e)
	}
}

func TestV2DocumentedExamples(t *testing.T) {
	load := func(name string) v2.Policy {
		b, e := os.ReadFile(filepath.Join("examples", name))
		if e != nil {
			t.Fatal(e)
		}
		p, e := v2.Decode(b)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = CompileV2(p); e != nil {
			t.Fatal(e)
		}
		return p
	}
	makeSource := func(env, host, endpoint string) snapshot.Input {
		return input(t, env, []string{"SOURCE"}, obj("SOURCE", env+"-id", "Invented Directory", map[string]any{"connectorAttributes": map[string]any{"host": host, "timeout": 60, "tokenEndpoint": endpoint}}))
	}
	left := makeSource("dev", "directory-dev.example.invalid", "dev-endpoint")
	right := makeSource("prod", "directory-prod.example.invalid", "dev-endpoint")
	r := compareV2Test(t, V2Request{Mode: "drift", Left: left, Right: right}, load("policy-v2-drift.json"))
	if r.ExitCode() != 0 {
		t.Fatal(r)
	}
	b := makeSource("prod", "directory-prod.example.invalid", "prod-endpoint")
	right = b
	opsBytes, e := os.ReadFile("examples/operations-v2.json")
	if e != nil {
		t.Fatal(e)
	}
	ops, e := v2.DecodeOperations(opsBytes)
	if e != nil {
		t.Fatal(e)
	}
	r = compareV2Test(t, V2Request{Mode: "deployment", Left: left, Right: right, Baseline: &b, Operations: ops}, load("policy-v2-deployment.json"))
	if r.ExitCode() != 0 {
		t.Fatal(r)
	}
	legacy, e := TranslateV1(policy())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = CompareV2(V2Request{Mode: "deployment", Left: left, Right: right, Operations: ops}, legacy); e == nil {
		t.Fatal("legacy policy silently enabled deployment")
	}
}
