package server

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain keeps the server's translator off the person's own: its store in a
// directory of the run's, and nobody to translate for — a test that wants
// translating gives its translator its own languages and model.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mywant-server-test-")
	if err != nil {
		panic(err)
	}
	translationsPath = func() string { return filepath.Join(dir, "translations.json") }
	translationReaders = func() []string { return nil }
	// The stories (story.go) and their ledger: the tests' own, not the
	// person's ~/.mywant/stories, whose statistics a test's turns would join.
	story.dir = filepath.Join(dir, "stories")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
