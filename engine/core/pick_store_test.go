package mywant

import (
	"testing"
	"time"
)

func TestPickStoreKeepsNewestFirstAndTrims(t *testing.T) {
	s := &pickStore{dir: t.TempDir()}
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	for i := 0; i < pickKeep*2+5; i++ {
		p := Pick{Typed: "nakano kion", Chose: PickChoice{Kind: "want", Name: "nakano-weather", Field: "weather_text", Text: "快晴"}, Rank: i}
		if err := s.add("chr-1", []Pick{p}, now); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.list("chr-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Rank != pickKeep*2+4 || got[0].At != "2026-10-12T09:00:00Z" {
		t.Fatalf("newest first, stamped: %+v", got)
	}
	all, _ := s.list("chr-1", 0)
	if len(all) > pickKeep*2 {
		t.Fatalf("trimmed to at most %d, got %d", pickKeep*2, len(all))
	}
	if err := s.clear("chr-1"); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.list("chr-1", 0); len(all) != 0 {
		t.Fatalf("cleared, got %d", len(all))
	}
}

func TestPickStoreRefusesPathsAsIDs(t *testing.T) {
	s := &pickStore{dir: t.TempDir()}
	if err := s.add("../x", []Pick{{Typed: "a"}}, time.Now()); err == nil {
		t.Fatal("a path is not a character id")
	}
}
