package buildinfo

import "runtime/debug"

// version is optionally set by ldflags for CI/GoReleaser builds:
//
//	go build -ldflags "-X github.com/alphaleonis/cctote/internal/buildinfo.version=v1.0.0" .
var version string

// Version returns the build version string. Resolution order:
//  1. ldflags override (for GoReleaser/CI)
//  2. Module version from go install ...@tag (e.g. "v1.2.3")
//  3. VCS revision + dirty flag (e.g. "815b174a-dirty")
//  4. Fallback "dev"
func Version() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return vcsVersion(info)
}

func vcsVersion(info *debug.BuildInfo) string {
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	if len(rev) > 8 {
		rev = rev[:8]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
