package types

import (
	"fmt"
	. "mywant/engine/src"
	"mywant/engine/src/chain"
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

func NewEvidenceWant(metadata Metadata, spec WantSpec) *EvidenceWant {
	evidence := &EvidenceWant{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		EvidenceType: "document",
	}

	if et, ok := spec.Params["evidence_type"]; ok {
		if ets, ok := et.(string); ok {
			evidence.EvidenceType = ets
		}
	}

	if aid, ok := spec.Params["approval_id"]; ok {
		if aids, ok := aid.(string); ok {
			evidence.ApprovalID = aids
		}
	}

	return evidence
}

func (e *EvidenceWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1, // Must have output connection to coordinator before executing
		MaxInputs:       0,
		MaxOutputs:      -1, // Unlimited outputs - broadcasts to all connected coordinators
		WantType:        "evidence",
		Description:     "Evidence provider for approval processes",
	}
}

func (e *EvidenceWant) InitializePaths(inCount, outCount int) {
	e.paths.In = make([]PathInfo, inCount)
	e.paths.Out = make([]PathInfo, outCount)
}

func (e *EvidenceWant) GetType() string {
	return "evidence"
}

func (e *EvidenceWant) GetWant() *Want {
	return &e.Want
}

func (e *EvidenceWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Check if already provided evidence
	provided, _ := e.State["evidence_provided"].(bool)

	if len(outputs) == 0 {
		return true
	}

	if provided {
		return true
	}

	// Mark as provided in state
	e.State["evidence_provided"] = true

	// Create evidence data
	evidenceData := &ApprovalData{
		ApprovalID:  e.ApprovalID,
		Evidence:    fmt.Sprintf("Evidence of type '%s' for approval %s", e.EvidenceType, e.ApprovalID),
		Description: "Supporting evidence for approval process",
		Timestamp:   time.Now(),
	}

	// Store state
	e.StoreState("evidence_type", e.EvidenceType)
	e.StoreState("approval_id", e.ApprovalID)
	e.StoreState("evidence_provided_at", evidenceData.Timestamp.Format(time.RFC3339))
	e.StoreState("total_processed", 1)

	InfoLog("[EVIDENCE] ✅ Evidence %s provided for approval %s to %d coordinator(s)\n", e.EvidenceType, e.ApprovalID, len(outputs))

	// Broadcast evidence to all output channels using SendPacketMulti
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

func NewDescriptionWant(metadata Metadata, spec WantSpec) *DescriptionWant {
	description := &DescriptionWant{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		DescriptionFormat: "Request for approval: %s",
	}

	if df, ok := spec.Params["description_format"]; ok {
		if dfs, ok := df.(string); ok {
			description.DescriptionFormat = dfs
		}
	}

	if aid, ok := spec.Params["approval_id"]; ok {
		if aids, ok := aid.(string); ok {
			description.ApprovalID = aids
		}
	}

	return description
}

func (d *DescriptionWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1, // Must have output connection to coordinator before executing
		MaxInputs:       0,
		MaxOutputs:      -1, // Unlimited outputs - broadcasts to all connected coordinators
		WantType:        "description",
		Description:     "Description provider for approval processes",
	}
}

func (d *DescriptionWant) InitializePaths(inCount, outCount int) {
	d.paths.In = make([]PathInfo, inCount)
	d.paths.Out = make([]PathInfo, outCount)
}

func (d *DescriptionWant) GetType() string {
	return "description"
}

func (d *DescriptionWant) GetWant() *Want {
	return &d.Want
}

func (d *DescriptionWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Check if already provided description
	provided, _ := d.State["description_provided"].(bool)

	if len(outputs) == 0 {
		return true
	}

	if provided {
		return true
	}

	// Mark as provided in state
	d.State["description_provided"] = true

	// Create description data
	description := fmt.Sprintf(d.DescriptionFormat, d.ApprovalID)
	descriptionData := &ApprovalData{
		ApprovalID:  d.ApprovalID,
		Evidence:    nil,
		Description: description,
		Timestamp:   time.Now(),
	}

	// Store state
	d.StoreState("description_format", d.DescriptionFormat)
	d.StoreState("approval_id", d.ApprovalID)
	d.StoreState("description", description)
	d.StoreState("description_provided_at", descriptionData.Timestamp.Format(time.RFC3339))
	d.StoreState("total_processed", 1)

	InfoLog("[DESCRIPTION] 📝 Description provided: %s to %d coordinator(s)\n", description, len(outputs))

	// Broadcast description to all output channels using SendPacketMulti
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

func NewLevel1CoordinatorWant(metadata Metadata, spec WantSpec) *Level1CoordinatorWant {
	coordinator := &Level1CoordinatorWant{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		CoordinatorType: "level1",
	}

	if aid, ok := spec.Params["approval_id"]; ok {
		if aids, ok := aid.(string); ok {
			coordinator.ApprovalID = aids
		}
	}

	if ct, ok := spec.Params["coordinator_type"]; ok {
		if cts, ok := ct.(string); ok {
			coordinator.CoordinatorType = cts
		}
	}

	return coordinator
}

func (l *Level1CoordinatorWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  2, // evidence and description
		RequiredOutputs: 0, // coordinator only processes, doesn't output
		MaxInputs:       2,
		MaxOutputs:      1, // Can output approval result
		WantType:        "level1_coordinator",
		Description:     "Level 1 approval coordinator",
	}
}

func (l *Level1CoordinatorWant) InitializePaths(inCount, outCount int) {
	l.paths.In = make([]PathInfo, inCount)
	l.paths.Out = make([]PathInfo, outCount)
}

func (l *Level1CoordinatorWant) GetType() string {
	return "level1_coordinator"
}

func (l *Level1CoordinatorWant) GetWant() *Want {
	return &l.Want
}

func (l *Level1CoordinatorWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Check if approval already processed
	processed, _ := l.State["approval_processed"].(bool)

	if processed {
		return true
	}

	if len(using) < 2 {
		return false // Wait for evidence and description
	}

	// Collect evidence and description
	evidenceReceived := false
	descriptionReceived := false
	var evidenceTimestamp time.Time
	var descriptionTimestamp time.Time

	for _, input := range using {
		select {
		case data := <-input:
			if approvalData, ok := data.(*ApprovalData); ok {
				if approvalData.Evidence != nil {
					evidenceReceived = true
					evidenceTimestamp = approvalData.Timestamp
					l.StoreState("evidence_received", true)
					l.StoreState("evidence_type", approvalData.Evidence)
					l.StoreState("evidence_provided", true)
					l.StoreState("evidence_provided_at", approvalData.Timestamp.Format(time.RFC3339))
				}
				if approvalData.Description != "" {
					descriptionReceived = true
					descriptionTimestamp = approvalData.Timestamp
					l.StoreState("description_received", true)
					l.StoreState("description_text", approvalData.Description)
					l.StoreState("description_provided", true)
					l.StoreState("description_provided_at", approvalData.Timestamp.Format(time.RFC3339))
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
		l.StoreState("approval_status", result.Status)
		l.StoreState("approval_level", result.Level)
		l.StoreState("approver_id", result.ApproverID)
		l.StoreState("approval_time", result.ApprovalTime.Format(time.RFC3339))
		l.StoreState("comments", result.Comments)
		l.StoreState("total_processed", 1)

		// Store evidence and description provider completion info for memory dump
		l.StoreState("evidence_provider_complete", true)
		l.StoreState("description_provider_complete", true)
		if !evidenceTimestamp.IsZero() {
			l.StoreState("evidence_received_at", evidenceTimestamp.Format(time.RFC3339))
		}
		if !descriptionTimestamp.IsZero() {
			l.StoreState("description_received_at", descriptionTimestamp.Format(time.RFC3339))
		}

		InfoLog("[LEVEL1] ✅ Approval %s: %s by %s at %s\n",
			result.ApprovalID, result.Status, result.ApproverID,
			result.ApprovalTime.Format("15:04:05"))

		return true
	}

	return false // Continue waiting for inputs
}

// Level2CoordinatorWant handles Level 2 approval coordination
type Level2CoordinatorWant struct {
	Want
	ApprovalID      string
	CoordinatorType string
	Level2Authority string
	paths           Paths
}

func NewLevel2CoordinatorWant(metadata Metadata, spec WantSpec) *Level2CoordinatorWant {
	coordinator := &Level2CoordinatorWant{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		CoordinatorType: "level2",
		Level2Authority: "senior_manager",
	}

	if aid, ok := spec.Params["approval_id"]; ok {
		if aids, ok := aid.(string); ok {
			coordinator.ApprovalID = aids
		}
	}

	if ct, ok := spec.Params["coordinator_type"]; ok {
		if cts, ok := ct.(string); ok {
			coordinator.CoordinatorType = cts
		}
	}

	if l2a, ok := spec.Params["level2_authority"]; ok {
		if l2as, ok := l2a.(string); ok {
			coordinator.Level2Authority = l2as
		}
	}

	return coordinator
}

func (l *Level2CoordinatorWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  2, // evidence and description
		RequiredOutputs: 0, // coordinator doesn't output - it only processes inputs
		MaxInputs:       2,
		MaxOutputs:      1, // Can output approval result but not required
		WantType:        "level2_coordinator",
		Description:     "Level 2 approval coordinator",
	}
}

func (l *Level2CoordinatorWant) InitializePaths(inCount, outCount int) {
	l.paths.In = make([]PathInfo, inCount)
	l.paths.Out = make([]PathInfo, outCount)
}

func (l *Level2CoordinatorWant) GetType() string {
	return "level2_coordinator"
}

func (l *Level2CoordinatorWant) GetWant() *Want {
	return &l.Want
}

func (l *Level2CoordinatorWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Check if final approval already processed
	processed, _ := l.State["final_approval_processed"].(bool)

	if processed {
		return true
	}

	if len(using) < 2 {
		return false // Wait for evidence and description
	}

	// Collect evidence and description
	evidenceReceived := false
	descriptionReceived := false
	var evidenceTimestamp time.Time
	var descriptionTimestamp time.Time

	for _, input := range using {
		select {
		case data := <-input:
			if approvalData, ok := data.(*ApprovalData); ok {
				if approvalData.Evidence != nil {
					evidenceReceived = true
					evidenceTimestamp = approvalData.Timestamp
					l.StoreState("evidence_received", true)
					l.StoreState("evidence_type", approvalData.Evidence)
					l.StoreState("evidence_provided", true)
					l.StoreState("evidence_provided_at", approvalData.Timestamp.Format(time.RFC3339))
				}
				if approvalData.Description != "" {
					descriptionReceived = true
					descriptionTimestamp = approvalData.Timestamp
					l.StoreState("description_received", true)
					l.StoreState("description_text", approvalData.Description)
					l.StoreState("description_provided", true)
					l.StoreState("description_provided_at", approvalData.Timestamp.Format(time.RFC3339))
				}
			}
		default:
			// No more data
		}
	}

	// Process final approval when both evidence and description are received
	if evidenceReceived && descriptionReceived {
		l.StoreState("final_approval_processed", true)

		// Simulate Level 2 final approval decision
		result := &ApprovalResult{
			ApprovalID:   l.ApprovalID,
			Level:        2,
			Status:       "approved",
			ApprovalTime: time.Now(),
			ApproverID:   l.Level2Authority,
			Comments:     "Level 2 final approval granted",
		}

		// Store final state including evidence and description completion info
		l.StoreState("final_approval_status", result.Status)
		l.StoreState("approval_level", result.Level)
		l.StoreState("approver_id", result.ApproverID)
		l.StoreState("approval_time", result.ApprovalTime.Format(time.RFC3339))
		l.StoreState("level2_authority", l.Level2Authority)
		l.StoreState("comments", result.Comments)
		l.StoreState("total_processed", 1)

		// Store evidence and description provider completion info for memory dump
		l.StoreState("evidence_provider_complete", true)
		l.StoreState("description_provider_complete", true)
		if !evidenceTimestamp.IsZero() {
			l.StoreState("evidence_received_at", evidenceTimestamp.Format(time.RFC3339))
		}
		if !descriptionTimestamp.IsZero() {
			l.StoreState("description_received_at", descriptionTimestamp.Format(time.RFC3339))
		}

		InfoLog("[LEVEL2] ✅ Final approval %s: %s by %s at %s\n",
			result.ApprovalID, result.Status, result.ApproverID,
			result.ApprovalTime.Format("15:04:05"))

		return true
	}

	return false // Continue waiting for inputs
}

// RegisterApprovalWantTypes registers all approval-related want types
func RegisterApprovalWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("evidence", func(metadata Metadata, spec WantSpec) interface{} {
		return NewEvidenceWant(metadata, spec)
	})

	builder.RegisterWantType("description", func(metadata Metadata, spec WantSpec) interface{} {
		return NewDescriptionWant(metadata, spec)
	})

	builder.RegisterWantType("level1_coordinator", func(metadata Metadata, spec WantSpec) interface{} {
		return NewLevel1CoordinatorWant(metadata, spec)
	})

	builder.RegisterWantType("level2_coordinator", func(metadata Metadata, spec WantSpec) interface{} {
		return NewLevel2CoordinatorWant(metadata, spec)
	})
}
