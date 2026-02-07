package types

import (
	"fmt"
	. "mywant/engine/src"
	"time"
)

// ApprovalResult represents the outcome of an approval process
type ApprovalResult struct {
	ApprovalID   string
	Level        int
	Status       string // "pending", "approved", "rejected"
	ApprovalTime time.Time
	ApproverID   string
	Comments     string
}

// EvidenceWantLocals holds type-specific local state for EvidenceWant
type EvidenceWantLocals struct {
	EvidenceType string
	ApprovalID   string
}

// EvidenceWant provides evidence data for approval processes
type EvidenceWant struct {
	Want
}

func NewEvidenceWant(metadata Metadata, spec WantSpec) Progressable {
	// Populate locals from parameters
	var evidenceType, approvalID string
	if spec.Params != nil {
		if v, ok := spec.Params["evidence_type"].(string); ok {
			evidenceType = v
		}
		if v, ok := spec.Params["approval_id"].(string); ok {
			approvalID = v
		}
	}

	locals := &EvidenceWantLocals{
		EvidenceType: evidenceType,
		ApprovalID:   approvalID,
	}

	return &EvidenceWant{*NewWantWithLocals(
		metadata,
		spec,
		locals,
		"evidence",
	)}
}

// Initialize resets state before execution begins
func (e *EvidenceWant) Initialize() {
	// No state reset needed for approval wants
}

// IsAchieved checks if evidence has been provided
func (e *EvidenceWant) IsAchieved() bool {
	provided, _ := e.GetStateBool("evidence_provided", false)
	return provided
}

func (e *EvidenceWant) Progress() {
	outCount := e.GetOutCount()
	paths := e.GetPaths()
	pathsOutLen := 0
	if paths != nil {
		pathsOutLen = len(paths.Out)
	}
	e.StoreLog("[EVIDENCE] Progress() called, OutCount=%d, paths.Out len=%d, Status=%s", outCount, pathsOutLen, e.Status)

	locals, ok := e.Locals.(*EvidenceWantLocals)
	if !ok {
		e.StoreLog("ERROR: Failed to access EvidenceWantLocals from Want.Locals")
		return
	}

	provided, _ := e.GetStateBool("evidence_provided", false)
	e.StoreLog("[EVIDENCE] evidence_provided=%v", provided)

	if provided {
		e.StoreLog("[EVIDENCE] Already provided, returning")
		return
	}

	// NOTE: Framework ensures output connections exist before Progress() is called
	// due to require: "users" in type-evidence.yaml

	e.StoreLog("[EVIDENCE] Setting evidence_provided=true and sending data")
	e.StoreState("evidence_provided", true)

	evidence := fmt.Sprintf("Evidence of type '%s' for approval %s", locals.EvidenceType, locals.ApprovalID)

	evidenceData := &ApprovalData{
		ApprovalID:  locals.ApprovalID,
		Evidence:    evidence,
		Description: "Supporting evidence for approval process",
		Timestamp:   time.Now(),
	}
	e.StoreStateMulti(Dict{
		"evidence_type":        locals.EvidenceType,
		"approval_id":          locals.ApprovalID,
		"evidence_provided_at": evidenceData.Timestamp.Format(time.RFC3339),
		"total_processed":      1,
		"achieving_percentage": 100,
		"final_result":         evidence,
	})

	e.StoreLog("📦 Evidence %s provided for approval %s to %d coordinator(s)", locals.EvidenceType, locals.ApprovalID, outCount)

	// Broadcast evidence to all output channels using Provide
	e.Provide(evidenceData)
	e.ProvideDone()

	// Mark evidence as achieved to complete the want and emit OwnerCompletionEvent if owned by a Target
	e.SetStatus(WantStatusAchieved)
}

// CalculateAchievingPercentage calculates the progress toward completion for EvidenceWant Returns 100 if evidence has been provided, 0 otherwise
func (e *EvidenceWant) CalculateAchievingPercentage() int {
	provided, _ := e.GetStateBool("evidence_provided", false)
	if provided {
		return 100
	}
	return 0
}

// DescriptionWantLocals holds type-specific local state for DescriptionWant
type DescriptionWantLocals struct {
	DescriptionFormat string
	ApprovalID        string
}

// DescriptionWant provides description data for approval processes
type DescriptionWant struct {
	Want
}

func NewDescriptionWant(metadata Metadata, spec WantSpec) Progressable {
	// Populate locals from parameters
	var descriptionFormat, approvalID string
	if spec.Params != nil {
		if v, ok := spec.Params["description_format"].(string); ok {
			descriptionFormat = v
		}
		if v, ok := spec.Params["approval_id"].(string); ok {
			approvalID = v
		}
	}

	locals := &DescriptionWantLocals{
		DescriptionFormat: descriptionFormat,
		ApprovalID:        approvalID,
	}

	return &DescriptionWant{*NewWantWithLocals(
		metadata,
		spec,
		locals,
		"description",
	)}
}

// Initialize resets state before execution begins
func (d *DescriptionWant) Initialize() {
	// No state reset needed for approval wants
}

// IsAchieved checks if description has been provided
func (d *DescriptionWant) IsAchieved() bool {
	provided, _ := d.GetStateBool("description_provided", false)
	return provided
}

func (d *DescriptionWant) Progress() {
	outCount := d.GetOutCount()
	paths := d.GetPaths()
	pathsOutLen := 0
	if paths != nil {
		pathsOutLen = len(paths.Out)
	}
	d.StoreLog("[DESCRIPTION] Progress() called, OutCount=%d, paths.Out len=%d, Status=%s", outCount, pathsOutLen, d.Status)

	locals, ok := d.Locals.(*DescriptionWantLocals)
	if !ok {
		d.StoreLog("ERROR: Failed to access DescriptionWantLocals from Want.Locals")
		return
	}

	provided, _ := d.GetStateBool("description_provided", false)
	d.StoreLog("[DESCRIPTION] description_provided=%v", provided)

	if provided {
		d.StoreLog("[DESCRIPTION] Already provided, returning")
		return
	}

	// NOTE: Framework ensures output connections exist before Progress() is called
	// due to require: "users" in type-description.yaml

	d.StoreLog("[DESCRIPTION] Setting description_provided=true and sending data")
	d.StoreState("description_provided", true)

	description := fmt.Sprintf(locals.DescriptionFormat, locals.ApprovalID)

	descriptionData := &ApprovalData{
		ApprovalID:  locals.ApprovalID,
		Evidence:    nil,
		Description: description,
		Timestamp:   time.Now(),
	}
	d.StoreStateMulti(Dict{
		"description_format":      locals.DescriptionFormat,
		"approval_id":             locals.ApprovalID,
		"description":             description,
		"description_provided_at": descriptionData.Timestamp.Format(time.RFC3339),
		"total_processed":         1,
		"achieving_percentage":    100,
		"final_result":            description,
	})

	d.StoreLog("📦 Description provided: %s to %d coordinator(s)", description, outCount)

	// Broadcast description to all output channels using Provide
	d.Provide(descriptionData)
	d.ProvideDone()

	// Mark description as achieved to complete the want and emit OwnerCompletionEvent if owned by a Target
	d.SetStatus(WantStatusAchieved)
}

// CalculateAchievingPercentage calculates the progress toward completion for DescriptionWant Returns 100 if description has been provided, 0 otherwise
func (d *DescriptionWant) CalculateAchievingPercentage() int {
	provided, _ := d.GetStateBool("description_provided", false)
	if provided {
		return 100
	}
	return 0
}

// RegisterApprovalWantTypes registers all approval-related want types
func RegisterApprovalWantTypes(builder *ChainBuilder) {
	// Register evidence type with YAML definition that includes require: "users"
	// This ensures evidence wants wait for output connections before providing data
	if err := builder.RegisterWantTypeFromYAML("evidence", NewEvidenceWant, "yaml/agents/type-evidence.yaml"); err != nil {
		// Fallback to basic registration if YAML fails
		builder.RegisterWantType("evidence", NewEvidenceWant)
	}

	// Register description type with YAML definition that includes require: "users"
	// This ensures description wants wait for output connections before providing data
	if err := builder.RegisterWantTypeFromYAML("description", NewDescriptionWant, "yaml/agents/type-description.yaml"); err != nil {
		// Fallback to basic registration if YAML fails
		builder.RegisterWantType("description", NewDescriptionWant)
	}

}
