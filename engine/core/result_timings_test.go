package mywant

import (
	"sync"
	"testing"
	"time"
)

// A player whose every song is kept, told of at the timings its type names:
// "track" every answer, "album" when the album changes — and by the standard
// function (Timing) from its Progress, "liked" when it says so.
func timingsTestWant(labels map[string]string) (*Want, func(track, album string, said ...string) []string) {
	want := NewWantWithLocals(Metadata{Name: "player", Labels: labels}, WantSpec{FinalResultField: "track"}, nil, "base")
	want.WantTypeDefinition = &WantTypeDefinition{
		State: []StateDef{{Name: "track"}, {Name: "album", Persistent: true}},
		Timings: []TimingDef{
			{Name: "track", Description: "曲が変わるたび", OnChange: "track", Notify: true},
			{Name: "album", Description: "アルバムが変わったとき", OnChange: "album", Murmur: true},
			{Name: "liked", Description: "お気に入りの曲"},
		},
	}
	var mu sync.Mutex
	var told [][]string
	OnWantOutput = func(w *Want, out ResultHistoryEntry) {
		mu.Lock()
		told = append(told, out.Timings)
		mu.Unlock()
	}
	play := func(track, album string, said ...string) []string {
		mu.Lock()
		told = nil
		mu.Unlock()
		want.BeginProgressCycle()
		want.storeState("track", track)
		want.storeState("album", album)
		for _, s := range said {
			want.Timing(s)
		}
		want.EndProgressCycle()
		waitFor(func() bool { mu.Lock(); defer mu.Unlock(); return len(told) > 0 })
		mu.Lock()
		defer mu.Unlock()
		if len(told) == 0 {
			return nil
		}
		return told[0]
	}
	return want, play
}

func waitFor(ok func() bool) {
	for i := 0; i < 50 && !ok(); i++ {
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAnswersComeAtTheirTypesTimings(t *testing.T) {
	defer func() { OnWantOutput = nil }()
	_, play := timingsTestWant(nil)
	if got := join(play("a1", "A")); got != "track album" {
		t.Errorf("first song: %q", got)
	}
	if got := join(play("a2", "A")); got != "track" {
		t.Errorf("same album: %q", got)
	}
	if got := join(play("b1", "B", "liked")); got != "track album liked" {
		t.Errorf("new album, said liked: %q", got)
	}
	// Said with no new answer: told of with the answer it has.
	if got := join(play("b1", "B", "liked")); got != "liked" {
		t.Errorf("said again on the same song: %q", got)
	}
}

func TestAWantChoosesWhichTimingsItMurmursAndNotifiesAt(t *testing.T) {
	defer func() { OnWantOutput = nil }()
	def, _ := timingsTestWant(nil)
	tell := def.TellingOf(ResultHistoryEntry{Timings: []string{"track"}})
	if tell.Murmur || !tell.Notify || tell.NotifyChosen {
		t.Errorf("defaults, track: %+v (murmurs at album, notifies at track)", tell)
	}
	if tell := def.TellingOf(ResultHistoryEntry{Timings: []string{"track", "album"}}); !tell.Murmur {
		t.Errorf("defaults, album: %+v", tell)
	}
	chosen, _ := timingsTestWant(map[string]string{MurmurLabel: "liked", NotifyLabel: "none"})
	if tell := chosen.TellingOf(ResultHistoryEntry{Timings: []string{"track", "album"}}); tell.Murmur || tell.Notify || !tell.NotifyChosen {
		t.Errorf("chosen liked/none, album: %+v", tell)
	}
	if tell := chosen.TellingOf(ResultHistoryEntry{Timings: []string{"liked"}}); !tell.Murmur || tell.Notify {
		t.Errorf("chosen liked/none, liked: %+v", tell)
	}
	plain := NewWantWithLocals(Metadata{Name: "plain"}, WantSpec{}, nil, "base")
	if tell := plain.TellingOf(ResultHistoryEntry{}); !tell.Murmur || !tell.Notify {
		t.Errorf("a type with no timings tells of every answer: %+v", tell)
	}
}
