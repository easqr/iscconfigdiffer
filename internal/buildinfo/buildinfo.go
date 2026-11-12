// Package buildinfo identifies the differ without reading Git or configuration at runtime.
package buildinfo

import (
	"runtime"
	"runtime/debug"
)

const modulePath = "github.com/easqr/iscconfigdiffer"

// releaseVersion is set by release builds using the Go linker's -X flag.
var releaseVersion string

// Info describes the differ module, rather than an importing application's revision.
type Info struct {
	Version, Commit, Modified, GoVersion, GOOS, GOARCH string
}

// Read returns embedded metadata; unavailable fields are reported as unknown.
func Read() Info {
	bi, _ := debug.ReadBuildInfo()
	return read(bi, releaseVersion)
}

func read(bi *debug.BuildInfo, release string) Info {
	out := Info{Version: "development", Commit: "unknown", Modified: "unknown", GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if bi == nil {
		return out
	}
	if bi.GoVersion != "" {
		out.GoVersion = bi.GoVersion
	}
	if bi.Main.Path != modulePath {
		for _, dep := range bi.Deps {
			if dep.Path == modulePath && dep.Replace == nil && dep.Version != "" && dep.Version != "(devel)" {
				out.Version = dep.Version
				break
			}
		}
		// VCS settings and linker overrides belong to the host application.
		return out
	}
	if release != "" {
		out.Version = release
	} else if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		out.Version = bi.Main.Version
	}
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			if setting.Value != "" {
				out.Commit = setting.Value
			}
		case "vcs.modified":
			if setting.Value == "true" || setting.Value == "false" {
				out.Modified = setting.Value
			}
		}
	}
	return out
}
