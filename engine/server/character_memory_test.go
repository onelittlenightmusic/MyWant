package server

import (
	"testing"
	"time"
)

func newTestMemories(t *testing.T, now *time.Time) *characterMemories {
	t.Helper()
	return &characterMemories{dir: t.TempDir(), now: func() time.Time { return *now }, held: map[string]*CharacterMemory{}}
}

func heardAt(now time.Time, want, say, mood string, impact float64) hearing {
	return hearing{character: "robot", mem: Memory{WantID: want, Say: say, Mood: mood, Impact: impact, At: now.Format(time.RFC3339)}}
}

func TestImpactOf(t *testing.T) {
	// A delay on one's own line moves more than on another's, and more near.
	own := impactOf("worried", 1, 0)
	other := impactOf("worried", 0.35, 0)
	far := impactOf("worried", 1, 2)
	if !(own > far && far > other) {
		t.Fatalf("own %v, far %v, other %v", own, far, other)
	}
	if impactOf("calm", 0.35, 2) >= memoryMoved {
		t.Fatalf("a calm word from a want not one's own, from afar, should not move anyone")
	}
}

func TestHearKeepsOnlyWhatMoves(t *testing.T) {
	now := time.Date(2026, 11, 26, 7, 0, 0, 0, time.Local)
	m := newTestMemories(t, &now)

	if kept, _ := m.hear(heardAt(now, "w-quiet", "いつもどおり", "calm", 0.05)); kept {
		t.Fatal("kept what did not move them")
	}
	if kept, _ := m.hear(heardAt(now, "w-rain", "雨になりました", "worried", 0.8)); !kept {
		t.Fatal("did not keep what moved them")
	}
	got := m.get("robot")
	if got.Mood != "worried" || len(got.Memories) != 1 {
		t.Fatalf("mood %q, memories %d", got.Mood, len(got.Memories))
	}
	// The same words again are not news.
	if kept, _ := m.hear(heardAt(now, "w-rain", "雨になりました", "worried", 0.8)); kept {
		t.Fatal("kept the same words twice")
	}
	// New words from the same want replace the old.
	kept, forgot := m.hear(heardAt(now, "w-rain", "晴れてきました", "joy", 0.5))
	if !kept || forgot == nil || forgot.Say != "雨になりました" || len(m.get("robot").Memories) != 1 {
		t.Fatalf("kept %v, forgot %+v", kept, forgot)
	}
}

func TestHearForgetsTheLeastMoving(t *testing.T) {
	now := time.Date(2026, 11, 26, 7, 0, 0, 0, time.Local)
	m := newTestMemories(t, &now)
	for i, imp := range []float64{0.5, 0.3, 0.6, 0.4, 0.7} {
		m.hear(heardAt(now, "w"+string(rune('a'+i)), "news", "worried", imp))
	}
	// Weaker than all five: not kept.
	if kept, _ := m.hear(heardAt(now, "w-weak", "news", "calm", 0.2)); kept {
		t.Fatal("kept something weaker than all it remembers")
	}
	// Stronger than the weakest (wb, 0.3): that one goes.
	kept, forgot := m.hear(heardAt(now, "w-strong", "news", "worried", 0.9))
	if !kept || forgot == nil || forgot.WantID != "wb" {
		t.Fatalf("kept %v, forgot %+v", kept, forgot)
	}
	if n := len(m.get("robot").Memories); n != memorySlots {
		t.Fatalf("remembers %d, not %d", n, memorySlots)
	}
}

func TestMoodFadesAndOldMemoriesMakeRoom(t *testing.T) {
	now := time.Date(2026, 11, 26, 7, 0, 0, 0, time.Local)
	m := newTestMemories(t, &now)
	m.hear(heardAt(now, "w-old", "old news", "worried", 0.9))
	for i, imp := range []float64{0.5, 0.5, 0.5, 0.5} {
		m.hear(heardAt(now.Add(30*time.Hour), "w"+string(rune('a'+i)), "news", "eager", imp))
	}
	now = now.Add(30 * time.Hour)
	// 0.9 thirty hours ago is less than 0.5 now: it is the one to go.
	_, forgot := m.hear(heardAt(now, "w-new", "news", "eager", 0.45))
	if forgot == nil || forgot.WantID != "w-old" {
		t.Fatalf("forgot %+v", forgot)
	}
	// And the mood went with time: hours later it is calm again.
	now = now.Add(12 * time.Hour)
	if got := m.get("robot"); got.Mood != "calm" {
		t.Fatalf("mood %q after twelve hours", got.Mood)
	}
	// Kept on disk: read again by a fresh store.
	again := &characterMemories{dir: m.dir, now: m.now, held: map[string]*CharacterMemory{}}
	if n := len(again.get("robot").Memories); n != memorySlots {
		t.Fatalf("read back %d memories", n)
	}
}
