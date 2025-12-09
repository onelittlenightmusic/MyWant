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

func NewEvidenceWant(metadata Metadata, spec WantSpec) interface{} {
	want := NewWant(
		metadata,
		spec,
		func() WantLocals { return &EvidenceWantLocals{} },
		ConnectivityMetadata{
			RequiredInputs:  0,
			RequiredOutputs: 1,
			MaxInputs:       0,
			MaxOutputs:      -1,
			WantType:        "evidence",
			Description:     "Evidence provider for approval processes",
		},
		"evidence",
	).(*Want)

	locals := want.Locals.(*EvidenceWantLocals)
	locals.EvidenceType = want.GetStringParam("evidence_type", "document")
	locals.ApprovalID = want.GetStringParam("approval_id", "")

	return &EvidenceWant{Want: *want}
}

func (e *EvidenceWant) Exec() bool {
	locals, ok := e.Locals.(*EvidenceWantLocals)
	if !ok {
		e.StoreLog("ERROR: Failed to access EvidenceWantLocals from Want.Locals")
		return true
	}

	provided, _ := e.GetStateBool("evidence_provided", false)

	if provided {
		return true
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

	e.StoreLog(fmt.Sprintf("Evidence %s provided for approval %s to %d coordinator(s)", locals.EvidenceType, locals.ApprovalID, e.GetOutCount()))

	// Broadcast evidence to all output channels using SendPacketMulti
	e.SendPacketMulti(evidenceData)
	return true
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

func NewDescriptionWant(metadata Metadata, spec WantSpec) interface{} {
	want := NewWant(
		metadata,
		spec,
		func() WantLocals { return &DescriptionWantLocals{} },
		ConnectivityMetadata{
			RequiredInputs:  0,
			RequiredOutputs: 1,
			MaxInputs:       0,
			MaxOutputs:      -1,
			WantType:        "description",
			Description:     "Description provider for approval processes",
		},
		"description",
	).(*Want)

	locals := want.Locals.(*DescriptionWantLocals)
	locals.DescriptionFormat = want.GetStringParam("description_format", "Request for approval: %s")
	locals.ApprovalID = want.GetStringParam("approval_id", "")

	return &DescriptionWant{Want: *want}
}

func (d *DescriptionWant) Exec() bool {
	locals, ok := d.Locals.(*DescriptionWantLocals)
	if !ok {
		d.StoreLog("ERROR: Failed to access DescriptionWantLocals from Want.Locals")
		return true
	}

	provided, _ := d.GetStateBool("description_provided", false)

	if provided {
		return true
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

	d.StoreLog(fmt.Sprintf("Description provided: %s to %d coordinator(s)", description, d.GetOutCount()))

	// Broadcast description to all output channels using SendPacketMulti
	d.SendPacketMulti(descriptionData)
	return true
}

// CalculateAchievingPercentage calculates the progress toward completion for DescriptionWant Returns 100 if description has been provided, 0 otherwise
func (d *DescriptionWant) CalculateAchievingPercentage() int {
	provided, _ := d.GetStateBool("description_provided", false)
	if provided {
		return 100
	}
	return 0
}

// Level1CoordinatorWant handles Level 1 approval coordination Note: This type is kept for backward compatibility but NewCoordinatorWant is preferred
// Deprecated: Use NewCoordinatorWant with coordinator_level=1 parameter instead
type Level1CoordinatorWant struct {
	Want
	ApprovalID      string
	CoordinatorType string
}

// Deprecated: Use NewCoordinatorWant with coordinator_level=1 parameter instead
func NewLevel1CoordinatorWant(metadata Metadata, spec WantSpec) interface{} {
	coordinator := &Level1CoordinatorWant{
		Want:            Want{},
		CoordinatorType: "level1",
	}
	coordinator.Init(metadata, spec)

	coordinator.ApprovalID = coordinator.GetStringParam("approval_id", "")

	coordinator.CoordinatorType = coordinator.GetStringParam("coordinator_type", "level1")
	coordinator.WantType = "level1_coordinator"
	coordinator.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  2,
		RequiredOutputs: 0,
		MaxInputs:       2,
		MaxOutputs:      1,
		WantType:        "level1_coordinator",
		Description:     "Level 1 approval coordinator",
	}

	return coordinator
}

func (l *Level1CoordinatorWant) Exec() bool {
	processed, _ := l.GetStateBool("approval_processed", false)

	if processed {
		return true
	}

	inCount := l.GetInCount()

	// If no channels are connected, mark as completed
	if inCount == 0 {
		return true
	}

	if inCount < 2 {
		return false // Wait for evidence and description
	}

	// Collect evidence and description
	evidenceReceived := false
	descriptionReceived := false
	var evidenceTimestamp time.Time
	var descriptionTimestamp time.Time

	for i := 0; i < l.GetInCount(); i++ {
		in, inChannelAvailable := l.GetInputChannel(i)
		if !inChannelAvailable {
			continue
		}
		select {
		case data := <-in:
			if approvalData, ok := data.(*ApprovalData); ok {
				if approvalData.Evidence != nil {
					evidenceReceived = true
					evidenceTimestamp = approvalData.Timestamp
					l.StoreStateMulti(map[string]interface{}{
						"evidence_received":    true,
						"evidence_type":        approvalData.Evidence,
						"evidence_provided":    true,
						"evidence_provided_at": approvalData.Timestamp.Format(time.RFC3339),
					})
				}
				if approvalData.Description != "" {
					descriptionReceived = true
					descriptionTimestamp = approvalData.Timestamp
					l.StoreStateMulti(map[string]interface{}{
						"description_received":    true,
						"description_text":        approvalData.Description,
						"description_provided":    true,
						"description_provided_at": approvalData.Timestamp.Format(time.RFC3339),
					})
				}
			}
		default:
			// No more data
		}
	}
	if evidenceReceived && descriptionReceived {
		l.StoreState("approval_processed", true)

		// Simulate Level 1 approval decision
		result := &ApprovalResult{
			ApprovalID:   l.ApprovalID,
			Level:        1,
			Status:       "approved",
			ApprovalTime: time.Now(),
			ApproverID:   "level1-manager",
			Comments:     "Level 1 approval granted",
		}
		stateUpdates := map[string]interface{}{
			"approval_status":             result.Status,
			"approval_level":              result.Level,
			"approver_id":                 result.ApproverID,
			"approval_time":               result.ApprovalTime.Format(time.RFC3339),
			"comments":                    result.Comments,
			"total_processed":             1,
			"evidence_provider_complete":  true,
			"description_provider_complete": true,
		}
		if !evidenceTimestamp.IsZero() {
			stateUpdates["evidence_received_at"] = evidenceTimestamp.Format(time.RFC3339)
		}
		if !descriptionTimestamp.IsZero() {
			stateUpdates["description_received_at"] = descriptionTimestamp.Format(time.RFC3339)
		}
		l.StoreStateMulti(stateUpdates)

		l.StoreLog(fmt.Sprintf("Approval %s: %s by %s at %s",
			result.ApprovalID, result.Status, result.ApproverID,
			result.ApprovalTime.Format("15:04:05")))

		return true
	}

	return false // Continue waiting for inputs
}

// Level1CoordinatorWant and Level2CoordinatorWant now use the generic CoordinatorWant with ApprovalDataHandler and ApprovalCompletionChecker The type field in metadata determines the configuration automatically

// RegisterApprovalWantTypes registers all approval-related want types
func RegisterApprovalWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("evidence", NewEvidenceWant)
	builder.RegisterWantType("description", NewDescriptionWant)
	// Coordinator type - handles all coordinator variations (approval, travel, buffet) Configuration is determined by type name and params (coordinator_type, coordinator_level, is_buffet, required_inputs)
	builder.RegisterWantType("coordinator", NewCoordinatorWant)
}