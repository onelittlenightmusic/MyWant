package mywant

import (
	"os"
	"path/filepath"
	"testing"
)

// resetEnvState makes each test start from a process that has applied nothing.
func resetEnvState(t *testing.T, path string) {
	t.Helper()
	envState.mu.Lock()
	envState.path = path
	envState.applied = map[string]bool{}
	envState.mu.Unlock()
	t.Cleanup(func() {
		envState.mu.Lock()
		envState.path = ""
		envState.applied = map[string]bool{}
		envState.mu.Unlock()
	})
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
}

func TestApplyConfigEnvironments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	resetEnvState(t, path)

	writeConfig(t, path, "environments:\n  TEST_TM_KEY: first\n")
	names, err := ApplyConfigEnvironments()
	if err != nil {
		t.Fatalf("ApplyConfigEnvironments: %v", err)
	}
	if len(names) != 1 || names[0] != "TEST_TM_KEY" {
		t.Errorf("names = %v, want [TEST_TM_KEY]", names)
	}
	if got := os.Getenv("TEST_TM_KEY"); got != "first" {
		t.Errorf("TEST_TM_KEY = %q, want first", got)
	}
	t.Cleanup(func() { os.Unsetenv("TEST_TM_KEY") })

	// `config env set` on a running server: the new value must win, even though
	// the variable is already set — this process is the one that set it.
	writeConfig(t, path, "environments:\n  TEST_TM_KEY: second\n")
	if _, err := ApplyConfigEnvironments(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := os.Getenv("TEST_TM_KEY"); got != "second" {
		t.Errorf("after reload TEST_TM_KEY = %q, want second", got)
	}

	// `config env unset`: a key that left the file leaves the environment too.
	writeConfig(t, path, "environments: {}\n")
	if _, err := ApplyConfigEnvironments(); err != nil {
		t.Fatalf("reload after unset: %v", err)
	}
	if _, ok := os.LookupEnv("TEST_TM_KEY"); ok {
		t.Errorf("TEST_TM_KEY survived being removed from the config")
	}
}

// A variable exported in the shell that started the server outranks the file,
// and no reload may overwrite or remove it.
func TestApplyConfigEnvironmentsKeepsShellValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	resetEnvState(t, path)
	t.Setenv("TEST_TM_SHELL", "from-shell")

	writeConfig(t, path, "environments:\n  TEST_TM_SHELL: from-file\n")
	if _, err := ApplyConfigEnvironments(); err != nil {
		t.Fatalf("ApplyConfigEnvironments: %v", err)
	}
	if got := os.Getenv("TEST_TM_SHELL"); got != "from-shell" {
		t.Errorf("TEST_TM_SHELL = %q, want the shell value to win", got)
	}

	writeConfig(t, path, "environments: {}\n")
	if _, err := ApplyConfigEnvironments(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := os.Getenv("TEST_TM_SHELL"); got != "from-shell" {
		t.Errorf("reload removed a shell-owned variable (now %q)", got)
	}
}

func TestApplyConfigEnvironmentsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")
	resetEnvState(t, path)

	names, err := ApplyConfigEnvironments()
	if err != nil {
		t.Fatalf("a missing config is not an error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("names = %v, want none", names)
	}
}
