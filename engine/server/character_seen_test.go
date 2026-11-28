package server

import (
	"testing"
	"time"

	ws "github.com/onelittlenightmusic/want-spec"
)

func TestSeenStopsAndCells(t *testing.T) {
	ip := func(v int) *int { return &v }
	at := "2026-11-28T10:00:00+09:00"
	events := []ws.StoryEvent{
		{At: at, Kind: ws.StoryKindCanvasMove, To: &ws.StoryPlace{Kind: "cell", X: ip(0), Y: ip(0)}},
		{At: at, Kind: ws.StoryKindWantArrive, From: &ws.StoryPlace{Kind: "want", X: ip(0), Y: ip(0)}, To: &ws.StoryPlace{Kind: "want", X: ip(10), Y: ip(0)}},
		{At: at, Kind: "memory.keep", To: &ws.StoryPlace{Kind: "want", X: ip(50), Y: ip(50)}},
		{At: at, Kind: ws.StoryKindCanvasMove, To: &ws.StoryPlace{Kind: "cell", X: ip(200), Y: ip(0)}},
	}
	stops := seenStops(events)
	if len(stops) != 3 {
		t.Fatalf("stops = %v, want 3 (repeats merged, memory.keep not a stop)", stops)
	}
	cells := seenCells(stops, 2)
	for _, c := range [][2]int{{0, 0}, {5, 2}, {12, 0}, {200, 0}} {
		if _, ok := cells[c]; !ok {
			t.Errorf("cell %v not seen", c)
		}
	}
	// A jump (10 → 200) is not walked; 50,50 was only heard from.
	for _, c := range [][2]int{{100, 0}, {50, 50}, {5, 3}} {
		if _, ok := cells[c]; ok {
			t.Errorf("cell %v seen", c)
		}
	}
	_ = time.Now
}
