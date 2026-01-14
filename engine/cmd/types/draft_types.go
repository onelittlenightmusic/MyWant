package types

import (
	. "mywant/engine/src"
)

// DraftWant represents a temporary, persisted want for storing work-in-progress state.
// It is primarily a container for state and doesn't perform active work.
type DraftWant struct {
	Want
}

// NewDraftWant creates a new DraftWant
func NewDraftWant(want *Want) *DraftWant {
	return &DraftWant{Want: *want}
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

// RegisterDraftWantType registers the DraftWant type with the ChainBuilder from YAML
func RegisterDraftWantType(builder *ChainBuilder) {
	// Register the factory for the "draft" type
	builder.RegisterWantTypeFromYAML("draft", func(metadata Metadata, spec WantSpec) Progressable {
		want := &Want{
			Metadata: metadata,
			Spec:     spec,
		}
		// Register for event system support if needed in the future
		want.Init()
		return NewDraftWant(want)
	}, "want_types/system/draft.yaml")
}
