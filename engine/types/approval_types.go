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
	return CheckLocalsInitialized[EvidenceWantLocals](&e.Want)
}

// Initialize resets state before execution begins
func (e *EvidenceWant) Initialize() {
	// Get locals (guaranteed to be initialized by framework)
	locals := e.GetLocals()

	// Populate locals from parameters
	locals.EvidenceType = e.GetStringParam("evidence_type", "")
	locals.ApprovalID = e.GetStringParam("approval_id", "")
}

// IsAchieved checks if evidence has been provided
func (e *EvidenceWant) IsAchieved() bool {
	return GetCurrent(e, "evidence_provided", false)
}

func (e *EvidenceWant) Progress() {
	locals := e.GetLocals()

	if GetCurrent(e, "evidence_provided", false) {
		return
	}

	// NOTE: Framework ensures output connections exist before Progress() is called
	// due to require: "users" in type-evidence.yaml

	e.SetCurrent("evidence_provided", true)

	evidence := fmt.Sprintf("Evidence of type '%s' for approval %s", locals.EvidenceType, locals.ApprovalID)

	evidenceData := &ApprovalData{
		ApprovalID:  locals.ApprovalID,
		Evidence:    evidence,
		Description: "Supporting evidence for approval process",
		Timestamp:   time.Now(),
	}
	
	e.SetCurrent("evidence", evidence)
	e.SetCurrent("evidence_type", locals.EvidenceType)
	e.SetGoal("approval_id", locals.ApprovalID)
	e.SetCurrent("evidence_provided_at", evidenceData.Timestamp.Format(time.RFC3339))
	e.SetCurrent("total_processed", 1)
	e.SetCurrent("achieving_percentage", 100)

	e.StoreLog("📦 Evidence %s provided for approval %s to %d coordinator(s)", locals.EvidenceType, locals.ApprovalID, e.GetOutCount())

	// Broadcast evidence to all output channels using Provide
	e.Provide(evidenceData)
	e.ProvideDone()

	// Mark evidence as achieved to complete the want and emit OwnerCompletionEvent if owned by a Target
	e.SetStatus(WantStatusAchieved)
}

// CalculateAchievingPercentage calculates the progress toward completion for EvidenceWant Returns 100 if evidence has been provided, 0 otherwise
func (e *EvidenceWant) CalculateAchievingPercentage() int {
	if GetCurrent(e, "evidence_provided", false) {
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
	return CheckLocalsInitialized[DescriptionWantLocals](&d.Want)
}

// Initialize resets state before execution begins
func (d *DescriptionWant) Initialize() {
	// Get locals (guaranteed to be initialized by framework)
	locals := d.GetLocals()

	// Populate locals from parameters
	locals.DescriptionFormat = d.GetStringParam("description_format", "")
	locals.ApprovalID = d.GetStringParam("approval_id", "")
}

// IsAchieved checks if description has been provided
func (d *DescriptionWant) IsAchieved() bool {
	return GetCurrent(d, "description_provided", false)
}

func (d *DescriptionWant) Progress() {
	locals := d.GetLocals()

	if GetCurrent(d, "description_provided", false) {
		return
	}

	// NOTE: Framework ensures output connections exist before Progress() is called
	// due to require: "users" in type-description.yaml

	d.SetCurrent("description_provided", true)

	description := fmt.Sprintf(locals.DescriptionFormat, locals.ApprovalID)

	descriptionData := &ApprovalData{
		ApprovalID:  locals.ApprovalID,
		Evidence:    nil,
		Description: description,
		Timestamp:   time.Now(),
	}
	
	d.SetCurrent("description_format", locals.DescriptionFormat)
	d.SetGoal("approval_id", locals.ApprovalID)
	d.SetCurrent("description", description)
	d.SetCurrent("description_provided_at", descriptionData.Timestamp.Format(time.RFC3339))
	d.SetCurrent("total_processed", 1)
	d.SetCurrent("achieving_percentage", 100)

	d.StoreLog("📦 Description provided: %s to %d coordinator(s)", description, d.GetOutCount())

	// Broadcast description to all output channels using Provide
	d.Provide(descriptionData)
	d.ProvideDone()

	// Mark description as achieved to complete the want and emit OwnerCompletionEvent if owned by a Target
	d.SetStatus(WantStatusAchieved)
}

// CalculateAchievingPercentage calculates the progress toward completion for DescriptionWant Returns 100 if description has been provided, 0 otherwise
func (d *DescriptionWant) CalculateAchievingPercentage() int {
	if GetCurrent(d, "description_provided", false) {
		return 100
	}
	return 0
}
