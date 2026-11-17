package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mywant "mywant/engine/core"
)

// The thing store parses its file once and answers from that until the file
// changes — by its own writes, or by anyone else's — and what it hands out is
// a copy, so a caller editing it does not edit the store.
func TestThingStoreKeepsWhatItRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thing.yaml")
	store := &ThingStore{path: path}
	if _, err := store.Add("station", "中野"); err != nil {
		t.Fatal(err)
	}
	got := store.Entries()
	if len(got) != 1 || got[0].Value != "中野" {
		t.Fatalf("entries = %v", got)
	}
	got[0].Value = "edited"
	if again := store.Entries(); again[0].Value != "中野" {
		t.Errorf("a caller's edit reached the store: %v", again)
	}

	// Someone else rewrites the file: read again.
	other := &ThingStore{path: path}
	if _, err := other.Add("station", "荻窪"); err != nil {
		t.Fatal(err)
	}
	if n := len(store.Entries()); n != 2 {
		t.Errorf("after another writer, %d entries, want 2", n)
	}

	// The file goes away: nothing, not what was kept.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if n := len(store.Entries()); n != 0 {
		t.Errorf("after the file is gone, %d entries", n)
	}
}

// Same for the notice log: counts follow the file, and marking read through
// one store is seen by a reader that kept an older parse.
func TestNotificationStoreKeepsWhatItRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	a := &NotificationStore{path: path}
	b := &NotificationStore{path: path}
	alert := NotificationEntry{Message: "done", Kind: "alert", TargetType: "want", TargetID: "w1"}
	if err := a.Record(alert); err != nil {
		t.Fatal(err)
	}
	if got := b.UnreadWantCounts()["w1"]; got != 1 {
		t.Fatalf("unread = %d, want 1", got)
	}
	alert.ID = "second"
	if err := a.Record(alert); err != nil {
		t.Fatal(err)
	}
	if got := b.UnreadWantCounts()["w1"]; got != 2 {
		t.Errorf("after another write, unread = %d, want 2", got)
	}
	if _, err := a.MarkWantRead("w1"); err != nil {
		t.Fatal(err)
	}
	if got := b.UnreadWantCounts()["w1"]; got != 0 {
		t.Errorf("after marking read, unread = %d", got)
	}
	all := a.All(0)
	all[0].Message = "edited"
	if a.All(0)[0].Message != "done" {
		t.Error("a caller's edit reached the log")
	}
}

// An alert's large answer is kept by its id only: the log is written whole for
// every notice. A small one stays.
func TestNoticeLargeAnswerKeptByIDOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	n := &NotificationStore{path: path}
	big := strings.Repeat("x", noticeResultMax+1)
	if err := n.Record(NotificationEntry{Message: "big", Kind: "alert", TargetType: "want", TargetID: "w1",
		Output: &mywant.ResultHistoryEntry{ID: "out-big", Result: big, Fields: map[string]any{"first": "22:00"}}}); err != nil {
		t.Fatal(err)
	}
	if err := n.Record(NotificationEntry{Message: "small", Kind: "alert", TargetType: "want", TargetID: "w1",
		Output: &mywant.ResultHistoryEntry{ID: "out-small", Result: "song"}}); err != nil {
		t.Fatal(err)
	}
	fresh := &NotificationStore{path: path}
	got := fresh.All(0)
	if len(got) != 2 {
		t.Fatalf("entries: %d", len(got))
	}
	small, large := got[0], got[1]
	if large.Output == nil || large.Output.ID != "out-big" || large.Output.Result != nil || large.Output.Fields["first"] != "22:00" {
		t.Fatalf("large answer: %+v", large.Output)
	}
	if small.Output == nil || small.Output.Result != "song" {
		t.Fatalf("small answer: %+v", small.Output)
	}
	if ids := fresh.UnreadWantOutputs()["w1"]; len(ids) != 2 {
		t.Fatalf("unread outputs: %v", ids)
	}
}
