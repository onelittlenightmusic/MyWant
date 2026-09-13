package server

import (
	"path/filepath"
	"testing"

	mywant "mywant/engine/core"
)

// A world must not be able to carry device settings, in either direction.
//
// This is the whole of the bug that moved these out of gui_state: a world's GUI
// state is written when the world is left and replayed over the live state when
// it is entered, so a pinned browser kept reverting to whichever one had been
// pinned the last time that board was left. Being in this set is what stops
// both the export and the replay.
func TestDeviceKeysAreNotWorldState(t *testing.T) {
	for _, k := range mywant.DeviceGUIStateKeys {
		if !guiStateVolatileKeys[k] {
			t.Errorf("device key %q is missing from guiStateVolatileKeys: a world switch would replay it", k)
		}
	}
	// The roster is in that list for a different reason than the settings —
	// presence, not configuration — so guard that it is actually named.
	if !guiStateVolatileKeys["devices"] || !guiStateVolatileKeys["homeBrowserDevice"] {
		t.Error("expected both the roster and the pinned browser to be excluded")
	}
}

// An empty value has to be storable: "" is how the GUI unpins a browser, and a
// store that treated it as "no opinion" would make unpinning impossible.
func TestApplyGUIUpdatesKeepsEmpty(t *testing.T) {
	// Its own file: the store persists by design, so reaching for the process
	// singleton would mean a test rewriting the machine's real settings.
	store := mywant.NewDeviceStoreAt(filepath.Join(t.TempDir(), "devices.yaml"))

	store.ApplyGUIUpdates(map[string]any{mywant.DeviceKeyHome: "dev-1"})
	if got := store.Home(); got != "dev-1" {
		t.Fatalf("home = %q, want dev-1", got)
	}
	// A key that is absent is left alone; only what was sent changes.
	store.ApplyGUIUpdates(map[string]any{mywant.DeviceKeyActiveLocation: "dev-2"})
	if got := store.Home(); got != "dev-1" {
		t.Errorf("home = %q after an unrelated update, want dev-1", got)
	}
	store.ApplyGUIUpdates(map[string]any{mywant.DeviceKeyHome: ""})
	if got := store.Home(); got != "" {
		t.Errorf("home = %q after unpinning, want empty", got)
	}
}
