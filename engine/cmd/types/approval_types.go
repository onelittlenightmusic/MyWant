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

// EvidenceWant provides evidence data for approval processes
type EvidenceWant struct {
	Want
	EvidenceType string
	ApprovalID   string
	paths        Paths
}

func NewEvidenceWant(metadata Metadata, spec WantSpec) interface{} {
	evidence := &EvidenceWant{
		Want:         Want{},
		EvidenceType: "document",
	}

	// Initialize base Want fields
	evidence.Init(metadata, spec)

	evidence.EvidenceType = evidence.GetStringParam("evidence_type", "document")

	evidence.ApprovalID = evidence.GetStringParam("approval_id", "")

	// Set fields for base Want methods
	evidence.WantType = "evidence"
	evidence.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       0,
		MaxOutputs:      -1,
		WantType:        "evidence",
		Description:     "Evidence provider for approval processes",
	}

	return evidence
}

func (e *EvidenceWant) GetWant() *Want {
	return &e.Want
}

func (e *EvidenceWant) Exec() bool {
	// Check if already provided evidence
	provided, _ := e.GetStateBool("evidence_provided", false)

	if provided {
		return true
	}

	// Mark as provided in state
	e.StoreState("evidence_provided", true)

	// Create evidence data
	evidenceData := &ApprovalData{
		ApprovalID:  e.ApprovalID,
		Evidence:    fmt.Sprintf("Evidence of type '%s' for approval %s", e.EvidenceType, e.ApprovalID),
		Description: "Supporting evidence for approval process",
		Timestamp:   time.Now(),
	}

	// Store state
	e.StoreStateMulti(map[string]interface{}{
		"evidence_type":        e.EvidenceType,
		"approval_id":          e.ApprovalID,
		"evidence_provided_at": evidenceData.Timestamp.Format(time.RFC3339),
		"total_processed":      1,
	})

	e.StoreLog(fmt.Sprintf("Evidence %s provided for approval %s to %d coordinator(s)", e.EvidenceType, e.ApprovalID, e.paths.GetOutCount()))

	// Broadcast evidence to all output channels using SendPacketMulti
	outputs := make([]Chan, e.paths.GetOutCount())
	for i := 0; i < e.paths.GetOutCount(); i++ {
		outputs[i] = e.paths.Out[i].Channel
	}
	e.SendPacketMulti(evidenceData, outputs)
	return true
}

// DescriptionWant provides description data for approval processes
type DescriptionWant struct {
	Want
	DescriptionFormat string
	ApprovalID        string
	paths             Paths
}

func NewDescriptionWant(metadata Metadata, spec WantSpec) interface{} {
	description := &DescriptionWant{
		Want:              Want{},
		DescriptionFormat: "Request for approval: %s",
	}

	// Initialize base Want fields
	description.Init(metadata, spec)

	description.DescriptionFormat = description.GetStringParam("description_format", "Request for approval: %s")

	description.ApprovalID = description.GetStringParam("approval_id", "")

	// Set fields for base Want methods
	description.WantType = "description"
	description.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       0,
		MaxOutputs:      -1,
		WantType:        "description",
		Description:     "Description provider for approval processes",
	}

	return description
}

func (d *DescriptionWant) GetWant() *Want {
	return &d.Want
}

func (d *DescriptionWant) Exec() bool {
	// Check if already provided description
	provided, _ := d.GetStateBool("description_provided", false)

	if provided {
		return true
	}

	// Mark as provided in state
	d.StoreState("description_provided", true)

	// Create description data
	description := fmt.Sprintf(d.DescriptionFormat, d.ApprovalID)
	descriptionData := &ApprovalData{
		ApprovalID:  d.ApprovalID,
		Evidence:    nil,
		Description: description,
		Timestamp:   time.Now(),
	}

	// Store state
	d.StoreStateMulti(map[string]interface{}{
		"description_format":      d.DescriptionFormat,
		"approval_id":             d.ApprovalID,
		"description":             description,
		"description_provided_at": descriptionData.Timestamp.Format(time.RFC3339),
		"total_processed":         1,
	})

	d.StoreLog(fmt.Sprintf("Description provided: %s to %d coordinator(s)", description, d.paths.GetOutCount()))

	// Broadcast description to all output channels using SendPacketMulti
	outputs := make([]Chan, d.paths.GetOutCount())
	for i := 0; i < d.paths.GetOutCount(); i++ {
		outputs[i] = d.paths.Out[i].Channel
	}
	d.SendPacketMulti(descriptionData, outputs)
	return true
}

// Level1CoordinatorWant handles Level 1 approval coordination
type Level1CoordinatorWant struct {
	Want
	ApprovalID      string
	CoordinatorType string
	paths           Paths
}

func NewLevel1CoordinatorWant(metadata Metadata, spec WantSpec) interface{} {
	coordinator := &Level1CoordinatorWant{
		Want:            Want{},
		CoordinatorType: "level1",
	}

	// Initialize base Want fields
	coordinator.Init(metadata, spec)

	coordinator.ApprovalID = coordinator.GetStringParam("approval_id", "")

	coordinator.CoordinatorType = coordinator.GetStringParam("coordinator_type", "level1")

	// Set fields for base Want methods
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

func (l *Level1CoordinatorWant) GetWant() interface{} {
	return &l.Want
}

func (l *Level1CoordinatorWant) Exec() bool {
	// Check if approval already processed
	processed, _ := l.GetStateBool("approval_processed", false)

	if processed {
		return true
	}

	inCount := l.paths.GetInCount()

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

	// Process approval when both evidence and description are received
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

		// Store final state including evidence and description completion info
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

// Level1CoordinatorWant and Level2CoordinatorWant now use the generic CoordinatorWant
// with ApprovalDataHandler and ApprovalCompletionChecker
// The type field in metadata determines the configuration automatically

// RegisterApprovalWantTypes registers all approval-related want types
func RegisterApprovalWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("evidence", NewEvidenceWant)
	builder.RegisterWantType("description", NewDescriptionWant)
	// Coordinator type - handles all coordinator variations (approval, travel, buffet)
	// Configuration is determined by type name and params (coordinator_type, coordinator_level, is_buffet, required_inputs)
	builder.RegisterWantType("coordinator", NewCoordinatorWant)
}