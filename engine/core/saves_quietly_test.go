package mywant

import (
	"testing"
	"time"
)

// Bookkeeping and volatile state do not ask for the memory file to be
// written; content does. The value is kept either way.
func TestBookkeepingStateSavesQuietly(t *testing.T) {
	w := &Want{Metadata: Metadata{ID: "w-quiet", Name: "quiet"}}
	w.WantTypeDefinition = &WantTypeDefinition{State: []StateDef{
		{Name: "progress_ms", Volatile: true, Persistent: true},
		{Name: "last_poll_at", Persistent: false, Label: WantTypeStateLabelCurrent},
		{Name: "first_time", Persistent: true, Label: WantTypeStateLabelCurrent},
		{Name: "target", Persistent: false, Label: WantTypeStateLabelGoal},
	}}

	for _, key := range []string{StateFieldAchievingPercent, StateFieldError, StateFieldActionByAgent, StateFieldAchieved, StateFieldCompleted, "progress_ms", "last_poll_at"} {
		rev, epoch := w.persistRev.Load(), currentStateEpoch()
		w.StoreState(key, "v-"+key)
		if w.persistRev.Load() != rev || currentStateEpoch() != epoch {
			t.Errorf("%s asked for a write", key)
		}
		if v, _ := w.State.Load(key); v != "v-"+key {
			t.Errorf("%s not kept: %v", key, v)
		}
	}
	for _, key := range []string{"agent_result", "desired_dispatch", "first_time", "target", "undeclared"} {
		rev, epoch := w.persistRev.Load(), currentStateEpoch()
		w.StoreState(key, "v-"+key)
		if w.persistRev.Load() == rev || currentStateEpoch() == epoch {
			t.Errorf("%s did not ask for a write", key)
		}
	}
}

// An answer found again only moves its LastSeen, without asking for the
// history to be written again; a new answer does.
func TestAnswerSeenAgainSavesQuietly(t *testing.T) {
	h := NewHistoryManager()
	t0 := time.Now()
	h.AddResultEntry(ResultHistoryEntry{ID: "a", Timestamp: t0, LastSeen: t0, Result: "slots"})
	v := h.ResultHistoryRing.Version()
	h.AddResultEntry(ResultHistoryEntry{ID: "b", Timestamp: t0.Add(time.Minute), Result: "slots"})
	if h.ResultHistoryRing.Version() != v {
		t.Fatal("seen again moved the version")
	}
	if last, _ := h.ResultHistoryRing.PeekLast(); !last.LastSeen.Equal(t0.Add(time.Minute)) {
		t.Fatalf("LastSeen not moved: %v", last.LastSeen)
	}
	h.AddResultEntry(ResultHistoryEntry{ID: "c", Timestamp: t0.Add(2 * time.Minute), Result: "other slots"})
	if h.ResultHistoryRing.Version() == v {
		t.Fatal("a new answer did not move the version")
	}
}

// A want that keeps all of its state over a restart saves what it changes,
// declared persistent or not.
func TestKeptStateSavesItsChanges(t *testing.T) {
	keep := false
	w := &Want{Metadata: Metadata{ID: "w-kept", Name: "kept"}, Spec: WantSpec{ResetOnRestart: &keep}}
	w.WantTypeDefinition = &WantTypeDefinition{State: []StateDef{{Name: "last_poll_at", Label: WantTypeStateLabelCurrent}}}
	rev := w.persistRev.Load()
	w.StoreState("last_poll_at", 1)
	if w.persistRev.Load() == rev {
		t.Fatal("a kept want's change did not ask for a write")
	}
}
