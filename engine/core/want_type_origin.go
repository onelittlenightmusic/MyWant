package mywant

// Where a want type came from, and which build of it is loaded.
//
// metadata.version in the YAML is what the author typed by hand, so it goes
// stale the moment they forget to bump it. The origin below is derived from
// the artifact itself on every load, so it cannot: a custom package is
// identified by the git tag of its checkout (the same `git describe` the CLI's
// own version comes from), a bundled type by the server build that ships it,
// and anything with no repo behind it by the digest of its YAML.

import (
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"strings"
)

// Want type origin kinds.
const (
	WantTypeOriginBundled = "bundled" // shipped inside the server binary
	WantTypeOriginCustom  = "custom"  // a package directory under ~/.mywant/custom-types
	WantTypeOriginUser    = "user"    // loose YAML dropped straight into ~/.mywant/custom-types
	WantTypeOriginAPI     = "api"     // registered through POST /api/v1/want-types
)

// WantTypeOrigin is the provenance of one loaded want type definition.
type WantTypeOrigin struct {
	Kind string `json:"kind"`
	// Custom is the package the type belongs to, i.e. the directory name under
	// ~/.mywant/custom-types. Empty for every other kind.
	Custom string `json:"custom,omitempty"`
	// Version is the resolved identifier: a git tag ("v1.2.0", or "v1.2.0-3-gab12cd4"
	// when the checkout has moved past one), the server build for bundled types,
	// or "sha256:…" when there is nothing better.
	Version string `json:"version"`
	// Tag is set only when the checkout sits exactly on a tag, which is what an
	// installed release looks like.
	Tag    string `json:"tag,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Dirty means the checkout has local modifications, so Tag no longer
	// describes what is actually loaded.
	Dirty  bool   `json:"dirty,omitempty"`
	Digest string `json:"digest,omitempty"`
	Path   string `json:"path,omitempty"`
}

// contentDigest identifies a definition by its bytes. Short enough to read in a
// UI, long enough not to collide in a list of a few hundred want types.
func contentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}

// gitVersion resolves a checkout's version exactly the way the CLI resolves its
// own: from the tag. A repo with no tags yet falls back to the short commit, so
// adopting tags upstream is what turns these into real version numbers.
func gitVersion(dir string) (version, tag, commit string, dirty bool) {
	if !isGitRepo(dir) {
		return "", "", "", false
	}
	commit = gitCommit(dir)
	tag = gitOutput(dir, "describe", "--tags", "--exact-match")
	version = gitOutput(dir, "describe", "--tags", "--always", "--dirty")
	if strings.HasSuffix(version, "-dirty") {
		dirty = true
	}
	if version == "" {
		version = commit
	}
	return version, tag, commit, dirty
}

func gitOutput(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "" // not on a tag, no tags at all, or no git — all "unknown" here
	}
	return strings.TrimSpace(string(out))
}

// customRoot maps a file under ~/.mywant/custom-types to the package it belongs
// to: the entry directly beneath that directory. Loose YAML sitting in the
// directory itself belongs to no package and returns "".
func customRoot(baseDir, path string) (root, name string) {
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return "", ""
	}
	first, rest, found := strings.Cut(rel, string(filepath.Separator))
	if !found || rest == "" || first == "." || first == ".." {
		return "", ""
	}
	return filepath.Join(baseDir, first), first
}

// originResolver resolves origins for one load pass. Running git costs a fork
// per package, so the answer is cached per package rather than per want type —
// a package with a dozen types still pays for one `git describe`.
type originResolver struct {
	cache map[string]WantTypeOrigin
}

func newOriginResolver() *originResolver {
	return &originResolver{cache: make(map[string]WantTypeOrigin)}
}

// forCustomFile resolves the origin of a want type loaded from baseDir.
func (r *originResolver) forCustomFile(baseDir, path string, data []byte) WantTypeOrigin {
	digest := contentDigest(data)

	root, name := customRoot(baseDir, path)
	if root == "" {
		return WantTypeOrigin{Kind: WantTypeOriginUser, Version: digest, Digest: digest, Path: path}
	}

	pkg, cached := r.cache[root]
	if !cached {
		pkg = WantTypeOrigin{Kind: WantTypeOriginCustom, Custom: name}
		pkg.Version, pkg.Tag, pkg.Commit, pkg.Dirty = gitVersion(root)
		r.cache[root] = pkg
	}

	origin := pkg
	origin.Path = path
	origin.Digest = digest
	if origin.Version == "" {
		// The package is not a git checkout, so it has no tag to speak for it
		// and the file's own content is the only honest identifier left.
		origin.Version = digest
	}
	return origin
}

// bundledOrigin describes a want type that ships inside the binary: its version
// is the binary's version, because that is what it was released with.
func bundledOrigin(path string) WantTypeOrigin {
	version, commit := BuildInfo()
	return WantTypeOrigin{Kind: WantTypeOriginBundled, Version: version, Commit: commit, Path: path}
}
