package types

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Where the on-device agent is looked for, and — the part that matters on a
// server — what happens when there is none. A machine with no fmtool must say
// so rather than fail the request, because that answer is what sends the
// question to Claude instead (see claudeCodeRequester's `fm` case).
func TestFMToolPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if _, ok := fmToolPath(); ok {
			t.Fatal("found an on-device model somewhere that has none")
		}
		return
	}

	t.Run("named binary is used", func(t *testing.T) {
		dir := t.TempDir()
		binary := filepath.Join(dir, "fmtool")
		if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("MYWANT_FM_BIN", binary)
		got, ok := fmToolPath()
		if !ok || got != binary {
			t.Fatalf("path = %q, %v; want %q, true", got, ok, binary)
		}
	})

	t.Run("a named binary that is not there is no model at all", func(t *testing.T) {
		// Deliberately NOT falling through to PATH: naming one and silently
		// using another is how a machine ends up answering from somewhere the
		// operator did not choose.
		t.Setenv("MYWANT_FM_BIN", filepath.Join(t.TempDir(), "missing"))
		if got, ok := fmToolPath(); ok {
			t.Fatalf("path = %q, true; want no model", got)
		}
	})
}
