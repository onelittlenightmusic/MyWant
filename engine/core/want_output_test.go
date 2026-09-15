package mywant

import (
	"sync"
	"testing"
	"time"
)

// A want's output is announced once per new answer: a changed result is news,
// the same result seen again is not, and neither is the same result arriving
// with its supporting fields moved.
func TestOnWantOutputFiresOnlyForNewResults(t *testing.T) {
	var mu sync.Mutex
	var heard []ResultHistoryEntry
	prevHook := OnWantOutput
	OnWantOutput = func(_ *Want, out ResultHistoryEntry) {
		mu.Lock()
		heard = append(heard, out)
		mu.Unlock()
	}
	defer func() { OnWantOutput = prevHook }()

	want := NewWantWithLocals(
		Metadata{Name: "player"},
		WantSpec{FinalResultField: "track_name"},
		nil,
		"base",
	)
	want.WantTypeDefinition = &WantTypeDefinition{
		State: []StateDef{
			{Name: "track_name", SubType: "song", Exposable: true},
			{Name: "album_name", SubType: "album", Exposable: true},
		},
	}

	cycle := func(updates map[string]any) {
		want.BeginProgressCycle()
		for k, v := range updates {
			want.storeState(k, v)
		}
		want.EndProgressCycle()
	}

	cycle(map[string]any{"track_name": "Mordecai", "album_name": "A"})
	cycle(map[string]any{"track_name": "Mordecai", "album_name": "A"}) // seen again
	cycle(map[string]any{"track_name": "Mordecai", "album_name": "B"}) // same result, fields moved
	cycle(map[string]any{"track_name": "Jellybelly", "album_name": "B"})

	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		n := len(heard)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // let any stray announcement arrive

	mu.Lock()
	defer mu.Unlock()
	if len(heard) != 2 {
		t.Fatalf("announcements = %d, want 2 (%+v)", len(heard), heard)
	}
	got := map[any]ResultHistoryEntry{}
	for _, e := range heard {
		got[e.Result] = e
	}
	for _, track := range []string{"Mordecai", "Jellybelly"} {
		e, ok := got[track]
		if !ok {
			t.Fatalf("no announcement for %q", track)
		}
		if e.Type != "song" {
			t.Errorf("%q: type = %q, want song", track, e.Type)
		}
		if e.ID == "" {
			t.Errorf("%q: output has no id", track)
		}
	}
}
