package server

import (
	"runtime/debug"
	"sync"
)

// injectedVersion is the build version reported by /health. This package is a
// library, so it can never know its own version: the value has to come from
// the binary that embeds it, which is why SetVersion exists. The release build
// stamps the git tag into the CLI's main.version (-X, see .goreleaser.yaml and
// the Makefile's build-cli target) and main hands it over from there.
var injectedVersion = "dev"

// SetVersion records the version of the binary embedding this package. Call it
// before Start; an empty value is ignored so callers can pass their (possibly
// unstamped) main.version unconditionally.
func SetVersion(v string) {
	if v != "" {
		injectedVersion = v
	}
}

// BuildInfo reports the running server's version and source commit. It is
// resolved once, lazily, so SetVersion still lands if it runs after init.
var BuildInfo = sync.OnceValues(buildInfo)

func buildInfo() (version string, commit string) {
	version = injectedVersion

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version, ""
	}

	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
			if len(commit) > 7 {
				commit = commit[:7]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}

	// Nothing was stamped in — the binary was built straight from source (make
	// build-cli without ldflags, `go run`, a test). Fall back to whatever the
	// toolchain recorded so /health still says something true. `go install
	// module@v0.3.16` puts the tag in Main.Version; a plain `go build` puts the
	// placeholder "(devel)" there, which tells us nothing.
	if version == "dev" {
		switch {
		case bi.Main.Version != "" && bi.Main.Version != "(devel)":
			version = bi.Main.Version
		case commit != "":
			version = "dev+" + commit
			if dirty {
				version += "-dirty"
			}
		}
	}

	return version, commit
}
