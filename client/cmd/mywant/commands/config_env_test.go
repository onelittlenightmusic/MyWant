package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEnvKey(t *testing.T) {
	valid := []string{"TICKETMASTER_API_KEY", "A", "_X1", "MYWANT_AUTH_PASSWORD"}
	for _, key := range valid {
		if err := validateEnvKey(key); err != nil {
			t.Errorf("validateEnvKey(%q) = %v, want nil", key, err)
		}
	}

	// Names that survive a YAML write but fail to export are rejected up front.
	invalid := []string{"", "lower_case", "1LEADING_DIGIT", "WITH-DASH", "WITH=EQUALS", "WITH SPACE"}
	for _, key := range invalid {
		if err := validateEnvKey(key); err == nil {
			t.Errorf("validateEnvKey(%q) = nil, want an error", key)
		}
	}
}

// Save merges onto whatever is on disk, so an unrelated key written by the
// server has to survive an environments write, and the block itself has to
// disappear once its last entry is removed.
func TestSaveEnvironmentsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	SetConfigPath(path)
	t.Cleanup(func() { SetConfigPath("") })

	if err := os.WriteFile(path, []byte("system_font_size: large\n"), 0600); err != nil {
		t.Fatalf("seeding config: %v", err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	config.Environments = map[string]string{"TICKETMASTER_API_KEY": "abcd1234"}
	if err := config.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if !strings.Contains(string(written), "TICKETMASTER_API_KEY: abcd1234") {
		t.Errorf("environments entry missing from:\n%s", written)
	}
	if !strings.Contains(string(written), "system_font_size: large") {
		t.Errorf("unknown key written by the server was dropped:\n%s", written)
	}

	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if reloaded.Environments["TICKETMASTER_API_KEY"] != "abcd1234" {
		t.Errorf("environments = %v, want the key to round-trip", reloaded.Environments)
	}

	delete(reloaded.Environments, "TICKETMASTER_API_KEY")
	if err := reloaded.Save(); err != nil {
		t.Fatalf("Save after unset: %v", err)
	}

	written, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if strings.Contains(string(written), "environments") {
		t.Errorf("empty environments block left behind:\n%s", written)
	}
	if !strings.Contains(string(written), "system_font_size: large") {
		t.Errorf("unknown key lost on unset:\n%s", written)
	}
}

func TestMaskSecret(t *testing.T) {
	if got := maskSecret("abcd1234"); got != "abcd****" {
		t.Errorf("maskSecret = %q, want abcd****", got)
	}
	if got := maskSecret("abc"); got != "****" {
		t.Errorf("maskSecret of a short value = %q, want ****", got)
	}
}
