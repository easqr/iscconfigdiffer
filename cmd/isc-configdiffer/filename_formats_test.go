package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	exporter "github.com/easqr/iscconfigexporter"
	"github.com/easqr/iscconfigexporter/snapshot"
)

func formatTree(t *testing.T, opts snapshot.FilenameOptions, manifest bool) string {
	t.Helper()
	raw := json.RawMessage(`{"tenant":"example","objects":[{"version":1,"self":{"type":"ROLE","id":"a:b","name":"Review/Role"},"object":{"id":"a:b","name":"Review/Role"}}]}`)
	s, err := exporter.BuildSnapshotWithFilenameOptions("example", exporter.Download{JobID: "job", Types: []string{"ROLE"}, Raw: raw}, time.Now(), opts)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Mkdir(filepath.Join(root, "ROLE"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Artifacts {
		if err = os.WriteFile(filepath.Join(root, a.Path), a.Data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if manifest {
		b, _ := json.Marshal(s.Manifest)
		if err = os.WriteFile(filepath.Join(root, "manifest.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCLINewExporterFormats(t *testing.T) {
	hex := formatTree(t, snapshot.FilenameOptions{Mode: snapshot.HexFilename}, true)
	for _, mode := range []snapshot.FilenameMode{snapshot.IDFilename, snapshot.NameFilename} {
		for _, appendID := range []bool{false, true} {
			opts := snapshot.FilenameOptions{Mode: mode, Escape: true, AppendID: appendID}
			for _, manifest := range []bool{false, true} {
				right := formatTree(t, opts, manifest)
				readMode := "snapshot"
				if !manifest {
					readMode = "tree"
				}
				args := []string{"--left", hex, "--left-mode", "snapshot", "--right", right, "--right-mode", readMode, "--scope", "ROLE", "--format", "json"}
				if !manifest {
					args = append(args, "--right-types", "ROLE", "--right-filename-mode", string(mode), "--right-filename-escape")
					if appendID {
						args = append(args, "--right-filename-append-id")
					}
				}
				var out, errOut bytes.Buffer
				if code := runWithReports(t, args, &out, &errOut); code != 0 {
					t.Fatal(mode, appendID, manifest, code, errOut.String())
				}
			}
		}
	}
}

func TestCLIFilenameOptionsRejectConflicts(t *testing.T) {
	left := formatTree(t, snapshot.FilenameOptions{Mode: snapshot.HexFilename}, true)
	right := formatTree(t, snapshot.FilenameOptions{Mode: snapshot.NameFilename, Escape: true}, true)
	for _, extra := range [][]string{{"--right-filename-mode", "id"}, {"--right-filename-append-id"}, {"--right-filename-mode", "unknown"}, {"--baseline-filename-mode", "name"}} {
		args := append([]string{"--left", left, "--left-mode", "snapshot", "--right", right, "--right-mode", "snapshot", "--format", "json"}, extra...)
		var out, errOut bytes.Buffer
		if code := runWithReports(t, args, &out, &errOut); code != 2 {
			t.Fatal(extra, code, errOut.String())
		}
	}
}
