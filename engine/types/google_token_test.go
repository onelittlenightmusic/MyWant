package types

import (
	"testing"

	. "mywant/engine/core"
)

func TestGoogleRefreshTokenMigratesFromBackup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveSecretField("backup_google", "want-1", "refresh_token", "old-tok"); err != nil {
		t.Fatal(err)
	}
	if got := loadGoogleRefreshToken("want-1"); got != "old-tok" {
		t.Fatalf("first load = %q", got)
	}
	// Moved, not copied: the per-want entry is gone and the shared one answers
	// for any want, including one that never had a token of its own.
	if left := LoadSecretField("backup_google", "want-1", "refresh_token"); left != "" {
		t.Fatalf("per-want token left behind: %q", left)
	}
	if got := loadGoogleRefreshToken("want-2"); got != "old-tok" {
		t.Fatalf("other want sees %q", got)
	}

	clearGoogleRefreshToken()
	if got := loadGoogleRefreshToken("want-2"); got != "" {
		t.Fatalf("after clear = %q", got)
	}
}
