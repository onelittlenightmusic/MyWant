package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[DraftWant, DraftLocals]("draft")
}

// DraftLocals holds type-specific local state for DraftWant
type DraftLocals struct{}

// DraftWant represents a temporary, persisted want for storing work-in-progress state.
// It is primarily a container for state and doesn't perform active work.
type DraftWant struct {
	Want
}

func (d *DraftWant) GetLocals() *DraftLocals {
	return CheckLocalsInitialized[DraftLocals](&d.Want)
}

// Initialize prepares the draft want
func (d *DraftWant) Initialize() {
	// Draft is a passive container, initialization just ensures state is consistent
	d.StoreLog("[DRAFT] Initializing draft container: %s\n", d.Metadata.Name)
}

// Progress implements Progressable for DraftWant
func (d *DraftWant) Progress() {
	// Draft is passive, it doesn't "do" anything in the reconcile loop
}

// IsAchieved returns false as drafts are meant to be deleted rather than completed
func (d *DraftWant) IsAchieved() bool {
	return false
}

// CalculateAchievingPercentage returns a static value for drafts
func (d *DraftWant) CalculateAchievingPercentage() int {
	return 0
}
