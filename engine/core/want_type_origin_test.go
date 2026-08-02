package mywant

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomRootSeparatesPackagesFromLooseFiles(t *testing.T) {
	base := filepath.Join("home", "custom-types")

	root, name := customRoot(base, filepath.Join(base, "mywant-transit-plugin", "transit.yaml"))
	if name != "mywant-transit-plugin" || root != filepath.Join(base, "mywant-transit-plugin") {
		t.Fatalf("package file: got root=%q name=%q", root, name)
	}

	// Nested files still belong to the package directory, not their own folder.
	root, name = customRoot(base, filepath.Join(base, "mywant-skills", "types", "a.yaml"))
	if name != "mywant-skills" || root != filepath.Join(base, "mywant-skills") {
		t.Fatalf("nested file: got root=%q name=%q", root, name)
	}

	// YAML dropped straight into custom-types belongs to no package.
	if root, name = customRoot(base, filepath.Join(base, "note.yaml")); root != "" || name != "" {
		t.Fatalf("loose file: got root=%q name=%q", root, name)
	}
}

func TestOriginFallsBackToDigestWithoutGit(t *testing.T) {
	base := t.TempDir()
	pkg := filepath.Join(base, "mywant-gauge-plugin")
	if err := os.MkdirAll(pkg, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pkg, "gauge.yaml")
	data := []byte("wantType:\n  metadata:\n    name: gauge\n")

	origin := newOriginResolver().forCustomFile(base, path, data)

	if origin.Kind != WantTypeOriginCustom || origin.Custom != "mywant-gauge-plugin" {
		t.Fatalf("got kind=%q custom=%q", origin.Kind, origin.Custom)
	}
	// No repo means no tag to speak for the package, so the content identifies it.
	if !strings.HasPrefix(origin.Version, "sha256:") || origin.Version != origin.Digest {
		t.Fatalf("expected digest version, got %q (digest %q)", origin.Version, origin.Digest)
	}
	if origin.Version == contentDigest([]byte("something else")) {
		t.Fatal("digest does not depend on content")
	}
}

func TestOriginUsesGitTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	base := t.TempDir()
	pkg := filepath.Join(base, "mywant-transit-plugin")
	if err := os.MkdirAll(pkg, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pkg, "transit.yaml")
	if err := os.WriteFile(path, []byte("wantType:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"add", "."},
		{"commit", "-m", "initial"},
		{"tag", "v1.2.0"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", pkg}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	origin := newOriginResolver().forCustomFile(base, path, []byte("wantType:\n"))

	if origin.Version != "v1.2.0" || origin.Tag != "v1.2.0" {
		t.Fatalf("expected the tag to be the version, got version=%q tag=%q", origin.Version, origin.Tag)
	}
	if origin.Commit == "" || origin.Dirty {
		t.Fatalf("got commit=%q dirty=%v", origin.Commit, origin.Dirty)
	}

	// Editing an installed package after the fact must be visible: the tag alone
	// would still claim v1.2.0.
	if err := os.WriteFile(path, []byte("wantType: edited\n"), 0644); err != nil {
		t.Fatal(err)
	}
	edited := newOriginResolver().forCustomFile(base, path, []byte("wantType: edited\n"))
	if !edited.Dirty || !strings.HasPrefix(edited.Version, "v1.2.0") {
		t.Fatalf("expected a dirty v1.2.0, got version=%q dirty=%v", edited.Version, edited.Dirty)
	}
}
