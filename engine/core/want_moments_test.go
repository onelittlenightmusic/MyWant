package mywant

import (
	"sync"
	"testing"
	"time"
)

// An impact written is a moment, told as one (OnWantMoment), and not a change
// of the want a page reads it again for (OnWantChanged).
func TestImpactIsAMomentNotAChange(t *testing.T) {
	var changed []string
	savedChanged := OnWantChanged
	OnWantChanged = func(id string) { changed = append(changed, id) }
	var mu sync.Mutex
	var heard []string
	savedMoment := OnWantMoment
	OnWantMoment = func(_ *Want, key string, _, _ any) { mu.Lock(); heard = append(heard, key); mu.Unlock() }
	defer func() { OnWantChanged, OnWantMoment = savedChanged, savedMoment }()

	w := &Want{Metadata: Metadata{ID: "w1", Name: "w1"}}
	w.StoreState(ImpactField, map[string]any{"kind": "feed", "to": []string{"w2"}, "n": 1})
	w.StoreState("error", "boom")
	w.SetStatus(WantStatusFailed)

	if len(changed) != 2 {
		t.Fatalf("changed told %d times (%v), want 2: the error and the status — not the impact", len(changed), changed)
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(heard) != 3 {
		t.Fatalf("moments heard %v, want impact, error and status", heard)
	}
}
