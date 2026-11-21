package mywant

import (
	"path/filepath"
	"testing"
)

func TestDeviceNoticesOff(t *testing.T) {
	m := NewDeviceStoreAt(filepath.Join(t.TempDir(), "devices.yaml"))
	if !m.ShowsNotices("a") {
		t.Fatal("a device shows notices unless turned off")
	}
	m.SetNoticesOff([]string{"a", "a", ""})
	if m.ShowsNotices("a") || !m.ShowsNotices("b") || len(m.Settings().NoticesOff) != 1 {
		t.Fatalf("off: %v", m.Settings().NoticesOff)
	}
	again := NewDeviceStoreAt(m.path)
	if again.ShowsNotices("a") {
		t.Fatal("not kept in the file")
	}
	m.SetNoticesOff(nil)
	if !m.ShowsNotices("a") {
		t.Fatal("turned back on")
	}
}
