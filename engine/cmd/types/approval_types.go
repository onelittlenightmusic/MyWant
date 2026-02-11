package types

import (
	"fmt"
	. "mywant/engine/core"
	"time"
)

func init() {
	RegisterWantImplementation[EvidenceWant, EvidenceWantLocals]("evidence")
	RegisterWantImplementation[DescriptionWant, DescriptionWantLocals]("description")
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

func (e *EvidenceWant) GetLocals() *EvidenceWantLocals {
	return GetLocals[EvidenceWantLocals](&e.Want)
}

// Initialize resets state before execution begins
func (e *EvidenceWant) Initialize() {
	// Get or initialize locals
	locals := e.GetLocals()
	if locals == nil {
		locals = &EvidenceWantLocals{}
		e.Locals = locals
	}

	// Populate locals from parameters
	locals.EvidenceType = e.GetStringParam("evidence_type", "")
	locals.ApprovalID = e.GetStringParam("approval_id", "")
}

// IsAchieved checks if evidence has been provided
func (e *EvidenceWant) IsAchieved() bool {
	provided, _ := e.GetStateBool("evidence_provided", false)
	return provided
}

func (e *EvidenceWant) Progress() {
	locals := CheckLocalsInitialized[EvidenceWantLocals](&e.Want)

	provided, _ := e.GetStateBool("evidence_provided", false)
	if provided {
		return
	}

	// NOTE: Framework ensures output connections exist before Progress() is called
	// due to require: "users" in type-evidence.yaml

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

	e.StoreLog("📦 Evidence %s provided for approval %s to %d coordinator(s)", locals.EvidenceType, locals.ApprovalID, e.GetOutCount())

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

func (d *DescriptionWant) GetLocals() *DescriptionWantLocals {
	return GetLocals[DescriptionWantLocals](&d.Want)
}

// Initialize resets state before execution begins
func (d *DescriptionWant) Initialize() {
	// Get or initialize locals
	locals := d.GetLocals()
	if locals == nil {
		locals = &DescriptionWantLocals{}
		d.Locals = locals
	}

	// Populate locals from parameters
	locals.DescriptionFormat = d.GetStringParam("description_format", "")
	locals.ApprovalID = d.GetStringParam("approval_id", "")
}

// IsAchieved checks if description has been provided
func (d *DescriptionWant) IsAchieved() bool {
	provided, _ := d.GetStateBool("description_provided", false)
	return provided
}

func (d *DescriptionWant) Progress() {
	locals := CheckLocalsInitialized[DescriptionWantLocals](&d.Want)

	provided, _ := d.GetStateBool("description_provided", false)
	if provided {
		return
	}

	// NOTE: Framework ensures output connections exist before Progress() is called
	// due to require: "users" in type-description.yaml

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

	d.StoreLog("📦 Description provided: %s to %d coordinator(s)", description, d.GetOutCount())

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
