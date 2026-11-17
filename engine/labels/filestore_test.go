package labels

import (
	"os"
	"path/filepath"
	"testing"
)

// The file is the truth: a hand edit is seen on the next read, though reads
// in between come from what was last read or written.
func TestFileStoreReadsAgainOnlyWhenTheFileChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "labels.yaml")
	s := NewFileStore(path)
	if err := s.Set("a", "k", "v"); err != nil {
		t.Fatal(err)
	}
	if got := s.Get("a")["k"]; got != "v" {
		t.Fatalf("after Set: %q", got)
	}
	// A read hands out a copy: changing it changes nothing kept.
	s.All()["a"]["k"] = "changed"
	if got := s.Get("a")["k"]; got != "v" {
		t.Fatalf("a copy changed the store: %q", got)
	}
	// By hand, another size: read again.
	if err := os.WriteFile(path, []byte("a:\n  k: by-hand\nb:\n  x: \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := s.Get("a")["k"]; got != "by-hand" {
		t.Fatalf("hand edit not seen: %q", got)
	}
	if ids := s.IDsWithLabel("x"); len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("IDsWithLabel after hand edit: %v", ids)
	}
	// Removed: no labels.
	os.Remove(path)
	if got := s.All(); len(got) != 0 {
		t.Fatalf("file gone, labels left: %v", got)
	}
	// Moved to another file: that file's labels.
	other := filepath.Join(t.TempDir(), "other.yaml")
	os.WriteFile(other, []byte("c:\n  k: other\n"), 0o644)
	s.SetPath(other)
	if got := s.Get("c")["k"]; got != "other" {
		t.Fatalf("after SetPath: %q", got)
	}
}
