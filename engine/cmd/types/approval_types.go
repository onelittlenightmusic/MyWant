package types

import (
	"fmt"
	. "mywant/engine/src"
	"time"
)

// ApprovalData represents shared evidence and description data
type ApprovalData struct {
	ApprovalID  string
	Evidence    interface{}
	Description string
	Timestamp   time.Time
}

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
	return &EvidenceWant{*NewWantWithLocals(
		metadata,
		spec,
		&EvidenceWantLocals{},
		"evidence",
	)}
}

// IsAchieved checks if evidence has been provided
func (e *EvidenceWant) IsAchieved() bool {
	provided, _ := e.GetStateBool("evidence_provided", false)
	return provided
}

func (e *EvidenceWant) Progress() {
	locals, ok := e.Locals.(*EvidenceWantLocals)
	if !ok {
		e.StoreLog("ERROR: Failed to access EvidenceWantLocals from Want.Locals")
		return
	}

	provided, _ := e.GetStateBool("evidence_provided", false)

	if provided {
		return
	}
	e.StoreState("evidence_provided", true)
	evidenceData := &ApprovalData{
		ApprovalID:  locals.ApprovalID,
		Evidence:    fmt.Sprintf("Evidence of type '%s' for approval %s", locals.EvidenceType, locals.ApprovalID),
		Description: "Supporting evidence for approval process",
		Timestamp:   time.Now(),
	}
	e.StoreStateMulti(map[string]interface{}{
		"evidence_type":        locals.EvidenceType,
		"approval_id":          locals.ApprovalID,
		"evidence_provided_at": evidenceData.Timestamp.Format(time.RFC3339),
		"total_processed":      1,
		"achieving_percentage": 100,
	})

	outCount := e.GetOutCount()
	e.StoreLog(fmt.Sprintf("Evidence %s provided for approval %s to %d coordinator(s)", locals.EvidenceType, locals.ApprovalID, outCount))

	// Broadcast evidence to all output channels using Provide
	e.Provide(evidenceData)
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
	return &DescriptionWant{*NewWantWithLocals(
		metadata,
		spec,
		&DescriptionWantLocals{},
		"description",
	)}
}

// IsAchieved checks if description has been provided
func (d *DescriptionWant) IsAchieved() bool {
	provided, _ := d.GetStateBool("description_provided", false)
	return provided
}

func (d *DescriptionWant) Progress() {
	locals, ok := d.Locals.(*DescriptionWantLocals)
	if !ok {
		d.StoreLog("ERROR: Failed to access DescriptionWantLocals from Want.Locals")
		return
	}

	provided, _ := d.GetStateBool("description_provided", false)

	if provided {
		return
	}
	d.StoreState("description_provided", true)
	description := fmt.Sprintf(locals.DescriptionFormat, locals.ApprovalID)
	descriptionData := &ApprovalData{
		ApprovalID:  locals.ApprovalID,
		Evidence:    nil,
		Description: description,
		Timestamp:   time.Now(),
	}
	d.StoreStateMulti(map[string]interface{}{
		"description_format":      locals.DescriptionFormat,
		"approval_id":             locals.ApprovalID,
		"description":             description,
		"description_provided_at": descriptionData.Timestamp.Format(time.RFC3339),
		"total_processed":         1,
		"achieving_percentage":    100,
	})

	outCount := d.GetOutCount()
	d.StoreLog(fmt.Sprintf("Description provided: %s to %d coordinator(s)", description, outCount))

	// Broadcast description to all output channels using Provide
	d.Provide(descriptionData)
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
	builder.RegisterWantType("evidence", NewEvidenceWant)
	builder.RegisterWantType("description", NewDescriptionWant)
	// Coordinator type - handles all coordinator variations (approval, travel, buffet) Configuration is determined by type name and params (coordinator_type, coordinator_level, is_buffet, required_inputs)
	builder.RegisterWantType("coordinator", NewCoordinatorWant)
}