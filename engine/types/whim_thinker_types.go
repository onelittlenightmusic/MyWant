package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[WhimThinkerWant, WhimThinkerLocals]("whim_thinker")
}

// WhimThinkerLocals holds type-specific local state for WhimThinkerWant
type WhimThinkerLocals struct{}

// WhimThinkerWant is a persistent interactive thinker child of a whim want.
// It enables the user to refine their "want" memo interactively via AI conversation,
// and to dispatch sibling wants based on AI recommendations.
// Unlike Draft, it never achieves — it persists and accumulates conversation history.
type WhimThinkerWant struct {
	Want
}

func (w *WhimThinkerWant) GetLocals() *WhimThinkerLocals {
	return CheckLocalsInitialized[WhimThinkerLocals](&w.Want)
}

func (w *WhimThinkerWant) Initialize() {
	w.StoreLog("[WHIM-THINKER] Initializing whim thinker: %s\n", w.Metadata.Name)
}

// Progress is a no-op — all interaction happens via API handlers
func (w *WhimThinkerWant) Progress() {}

// IsAchieved always returns false — the thinker persists indefinitely
func (w *WhimThinkerWant) IsAchieved() bool {
	return false
}

func (w *WhimThinkerWant) CalculateAchievingPercentage() int {
	return 0
}
