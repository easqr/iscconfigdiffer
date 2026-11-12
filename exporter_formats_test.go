package differ

import (
	"encoding/json"
	"testing"
	"time"

	exporter "github.com/easqr/iscconfigexporter"
	"github.com/easqr/iscconfigexporter/snapshot"
)

func formatFixture(t *testing.T, opts snapshot.FilenameOptions) snapshot.Snapshot {
	t.Helper()
	raw := json.RawMessage(`{"tenant":"example","objects":[
	{"version":1,"self":{"type":"ROLE","id":"role%id","name":"Review% Role"},"object":{"id":"role%id","name":"Review% Role"}},
	{"version":1,"self":{"type":"IDENTITY_PROFILE","id":"profile","name":"Employees%"},"object":{"id":"profile","name":"Employees%"}},
	{"version":1,"self":{"type":"LIFECYCLE_STATE","id":"state","name":"Active"},"object":{"id":"state","name":"Active","technicalName":"active","identityProfileRef":{"type":"IDENTITY_PROFILE","id":"profile","name":"Employees%"}}}
	]}`)
	s, err := exporter.BuildSnapshotWithFilenameOptions("example", exporter.Download{JobID: "fixture", Types: []string{"ROLE", "IDENTITY_PROFILE", "LIFECYCLE_STATE"}, Raw: raw}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), opts)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func formatFiles(t *testing.T, s snapshot.Snapshot) map[string][]byte {
	t.Helper()
	b, err := json.Marshal(s.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"manifest.json": b}
	for _, a := range s.Artifacts {
		files[a.Path] = a.Data
	}
	return files
}

func TestExporterFilenameFormatComparisons(t *testing.T) {
	options := []snapshot.FilenameOptions{{Mode: snapshot.HexFilename}, {Mode: snapshot.IDFilename}, {Mode: snapshot.IDFilename, Escape: true}, {Mode: snapshot.NameFilename}, {Mode: snapshot.NameFilename, Escape: true}, {Mode: snapshot.NameFilename, Escape: true, AppendID: true}}
	inputs := make([]snapshot.Input, len(options))
	for i, opts := range options {
		s := formatFixture(t, opts)
		in, err := snapshot.FromFiles(formatFiles(t, s), snapshot.Options{Mode: snapshot.SnapshotMode})
		if err != nil {
			t.Fatal(opts, err)
		}
		inputs[i] = in
	}
	for i, left := range inputs {
		for j, right := range inputs {
			r, err := Compare(Request{Mode: "drift", Left: left, Right: right}, DefaultPolicy())
			if err != nil || r.ExitCode() != 0 || len(r.Matches) != 3 {
				t.Fatalf("format %d vs %d: exit=%d error=%v", i, j, r.ExitCode(), err)
			}
		}
	}
}

func TestLegacyExporterNamingSchemas(t *testing.T) {
	// Preserve older manifest interpretations, including lifecycle files without
	// profile prefixes and files that always include an ID suffix.
	for _, schema := range []int{2, 3, 4} {
		s := formatFixture(t, snapshot.FilenameOptions{Mode: snapshot.NameFilename, AppendID: true})
		files := map[string][]byte{}
		s.Manifest.Schema = schema
		s.Manifest.FilenameAppendID = false
		for i := range s.Artifacts {
			a := &s.Artifacts[i]
			v, err := snapshot.DecodeJSON(a.Data)
			if err != nil {
				t.Fatal(err)
			}
			env := v.(map[string]any)
			self := env["self"].(map[string]any)
			typ, id, name := self["type"].(string), self["id"].(string), self["name"].(string)
			stem := name
			if schema >= 4 && typ == "LIFECYCLE_STATE" {
				stem = snapshot.IdentityProfileName(env) + "--" + name
			}
			if schema >= 3 {
				stem += "--" + id
			}
			a.Path = typ + "/" + stem + ".json"
			files[a.Path] = a.Data
		}
		s.Manifest.Artifacts = s.Artifacts
		files["manifest.json"], _ = json.Marshal(s.Manifest)
		in, err := snapshot.FromFiles(files, snapshot.Options{Mode: snapshot.SnapshotMode})
		if err != nil {
			t.Fatal(schema, err)
		}
		r, err := Compare(Request{Mode: "drift", Left: in, Right: in}, DefaultPolicy())
		if err != nil || r.ExitCode() != 0 {
			t.Fatal(schema, err)
		}
	}
}
