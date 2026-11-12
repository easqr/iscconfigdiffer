package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestBuildIdentity(t *testing.T) {
	for _, tc := range []struct{ name, version, release, want string }{
		{"source", "(devel)", "", "development"},
		{"module", "v0.8.0", "", "v0.8.0"},
		{"release", "(devel)", "v0.8.0", "v0.8.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bi := &debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: tc.version}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "invented-commit"}, {Key: "vcs.modified", Value: "true"}}}
			got := read(bi, tc.release)
			if got.Version != tc.want || got.Commit != "invented-commit" || got.Modified != "true" {
				t.Fatal(got)
			}
		})
	}
	for _, bi := range []*debug.BuildInfo{nil, {Main: debug.Module{Path: modulePath}, Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "invalid"}}}} {
		got := read(bi, "")
		if got.Version != "development" || got.Commit != "unknown" || got.Modified != "unknown" || got.GoVersion == "" || got.GOOS == "" || got.GOARCH == "" {
			t.Fatal(got)
		}
	}
	host := read(&debug.BuildInfo{Main: debug.Module{Path: "example.com/host"}, Deps: []*debug.Module{{Path: modulePath, Version: "v0.8.0"}}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "host-commit"}}}, "host-release")
	if host.Version != "v0.8.0" || host.Commit != "unknown" {
		t.Fatal(host)
	}
}
