package mywant

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSplitSourceRefLeavesSSHURLsAlone(t *testing.T) {
	cases := []struct {
		source   string
		wantRest string
		wantRef  string
	}{
		{"owner/repo@v1.2.0", "owner/repo", "v1.2.0"},
		{"transit-plugin@v1.2.0", "transit-plugin", "v1.2.0"},
		{"https://github.com/owner/repo.git@v1.2.0", "https://github.com/owner/repo.git", "v1.2.0"},
		{"owner/repo", "owner/repo", ""},
		// The "@" here belongs to the host, not to a ref.
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git", ""},
		// A trailing "@" names no ref.
		{"owner/repo@", "owner/repo@", ""},
	}
	for _, c := range cases {
		rest, ref := splitSourceRef(c.source)
		if rest != c.wantRest || ref != c.wantRef {
			t.Errorf("splitSourceRef(%q) = (%q, %q), want (%q, %q)", c.source, rest, ref, c.wantRest, c.wantRef)
		}
	}
}

func TestResolveCustomSourceCarriesRef(t *testing.T) {
	resolved, origin, name, ref := ResolveCustomSource("owner/repo@v1.2.0")
	if resolved != "https://github.com/owner/repo.git" || origin != "git" || name != "repo" || ref != "v1.2.0" {
		t.Fatalf("got resolved=%q origin=%q name=%q ref=%q", resolved, origin, name, ref)
	}

	// A pin must not leak into the derived name.
	if _, _, name, _ = ResolveCustomSource("transit-plugin@v1.2.0"); name != "mywant-transit-plugin" {
		t.Fatalf("bare name with ref: got name=%q", name)
	}
}

// TestCheckoutRefPinsTagsButFollowsBranches covers the rule that decides whether
// an installed custom is frozen or keeps moving: a tag or commit is a pin, a
// branch is something to follow.
func TestCheckoutRefPinsTagsButFollowsBranches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	work := filepath.Join(t.TempDir(), "work")
	clone := filepath.Join(t.TempDir(), "clone")

	git := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", remote).CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "clone", remote, work).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v: %s", err, out)
	}
	git(work, "config", "user.email", "test@example.com")
	git(work, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(work, "a.yaml"), []byte("wantType:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(work, "add", ".")
	git(work, "commit", "-m", "initial")
	git(work, "tag", "v1.0.0")
	git(work, "push", "origin", "main", "--tags")
	if out, err := exec.Command("git", "clone", remote, clone).CombinedOutput(); err != nil {
		t.Fatalf("clone2: %v: %s", err, out)
	}

	pinned, err := checkoutRef(clone, "v1.0.0")
	if err != nil || !pinned {
		t.Fatalf("tag: pinned=%v err=%v", pinned, err)
	}
	if _, tag, _, _ := gitVersion(clone); tag != "v1.0.0" {
		t.Fatalf("tag: got %q", tag)
	}

	pinned, err = checkoutRef(clone, "main")
	if err != nil || pinned {
		t.Fatalf("branch: pinned=%v err=%v", pinned, err)
	}
	// Following a branch means the checkout is attached to it, so later updates
	// can fast-forward instead of failing on a detached HEAD.
	if head := gitOutput(clone, "rev-parse", "--abbrev-ref", "HEAD"); head != "main" {
		t.Fatalf("branch: HEAD is %q, want main", head)
	}

	if _, err := checkoutRef(clone, "v9.9.9-nope"); err == nil {
		t.Fatal("expected an error for a ref that does not exist")
	}
}
