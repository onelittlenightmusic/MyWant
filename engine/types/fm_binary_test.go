package types

import (
	"os"
	"path/filepath"
	"testing"
)

// The CLI the server runs is the CLI the server IS.
//
// Both halves of the robot read a command list from a mywant binary and then
// run commands through one, and they have to be the same binary: a list read
// from one build and commands run against another is how the labels stop
// meaning anything. Looking on PATH first broke that quietly — the server ran
// v0.7.0 from bin/ and every command it ran went through a v0.6.0 install.
func TestMywantBinaryPathPrefersMYWANTBIN(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mywant")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYWANT_BIN", fake)
	got, err := mywantBinaryPath()
	if err != nil {
		t.Fatalf("mywantBinaryPath() error: %v", err)
	}
	if got != fake {
		t.Errorf("mywantBinaryPath() = %q, want %q", got, fake)
	}
}

// Whatever it finds, it must never be the binary running the tests: exec'ing
// that with "commands --json" runs the test suite again, and reads its output
// as a catalogue.
func TestMywantBinaryPathIsNotTheTestBinary(t *testing.T) {
	t.Setenv("MYWANT_BIN", "")
	self, err := os.Executable()
	if err != nil {
		t.Skip("no executable path on this platform")
	}
	got, err := mywantBinaryPath()
	if err != nil {
		t.Skip("no mywant CLI on this machine to find")
	}
	if got == self {
		t.Errorf("mywantBinaryPath() returned the test binary %q", got)
	}
	if filepath.Base(got) != "mywant" {
		t.Errorf("mywantBinaryPath() = %q, which is not a mywant binary", got)
	}
}
