package types

import (
	. "mywant/engine/src"
	"path/filepath"
)

// DraftWant represents a temporary, persisted want for storing work-in-progress state.
// It is primarily a container for state and doesn't perform active work.
type DraftWant struct {
	Want
}

// NewDraftWant creates a new DraftWant
func NewDraftWant(metadata Metadata, spec WantSpec) Progressable {
	// DraftWant doesn't seem to have a specific Locals struct defined in the file,
	// passing nil for locals as per original implementation which didn't set it.
	return &DraftWant{Want: *NewWantWithLocals(
		metadata,
		spec,
		nil,
		"draft",
	)}
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
	InfoLog("[INFO] Registering draft want type")
	// Register the factory for the "draft" type
	err := builder.RegisterWantTypeFromYAML("draft", NewDraftWant, filepath.Join(WantTypesDir, "system/draft.yaml"))
	if err != nil {
		ErrorLog("[ERROR] Failed to register draft want type: %v", err)
	} else {
		InfoLog("[INFO] Successfully registered draft want type")
	}
}
