package types

import (
	"fmt"
	. "mywant/engine/src"
	"time"
)

// DataHandler defines the interface for processing received coordinator data
type DataHandler interface {
	// ProcessData handles incoming data from a channel
	ProcessData(want *CoordinatorWant, data interface{}) bool
	// GetStateUpdates returns state updates to apply after data collection
	GetStateUpdates(want *CoordinatorWant) map[string]interface{}
	// GetCompletionKey returns the key used to check if processing is complete
	GetCompletionKey() string
}

// CompletionChecker defines the interface for determining when coordinator work is complete
type CompletionChecker interface {
	// IsComplete checks if all required data has been collected
	IsComplete(want *CoordinatorWant, requiredInputs int) bool
	// OnCompletion handles final processing when completion is detected
	OnCompletion(want *CoordinatorWant)
}

// CoordinatorWant is a generic coordinator that collects data from multiple input channels
// and processes it according to customizable handlers
type CoordinatorWant struct {
	Want
	RequiredInputCount int
	DataHandler        DataHandler
	CompletionChecker  CompletionChecker
	CoordinatorType    string
	paths              Paths
}

// NewCoordinatorWant creates a new generic coordinator want
// It automatically determines the required inputs and handlers based on the want type
func NewCoordinatorWant(
	metadata Metadata,
	spec WantSpec,
) interface{} {
	coordinator := &CoordinatorWant{
		Want: Want{},
	}

	// Initialize base Want fields
	coordinator.Init(metadata, spec)

	// Determine coordinator configuration based on want type
	coordinatorType := metadata.Type
	requiredInputs, dataHandler, completionChecker := getCoordinatorConfig(coordinatorType, &coordinator.Want)

	coordinator.RequiredInputCount = requiredInputs
	coordinator.DataHandler = dataHandler
	coordinator.CompletionChecker = completionChecker
	coordinator.CoordinatorType = coordinatorType

	// Set fields for base Want methods
	coordinator.WantType = coordinatorType
	coordinator.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  -1,  // Unified: accept any number of inputs
		RequiredOutputs: 0,
		MaxInputs:       -1,  // No maximum
		MaxOutputs:      0,
		WantType:        coordinatorType,
		Description:     fmt.Sprintf("Generic coordinator want (%s)", coordinatorType),
	}

	return coordinator
}

// getCoordinatorConfig returns the configuration for a coordinator based on its type and parameters
// It determines the data handler and completion checker based on:
// 1. The want type from metadata
// 2. Coordinator-specific parameters (coordinator_level, coordinator_type, is_buffet, required_inputs)
// This enables full customization of coordinator behavior through parameters
func getCoordinatorConfig(coordinatorType string, want *Want) (int, DataHandler, CompletionChecker) {
	// Get parameters from want specs
	requiredInputsParam := want.GetIntParam("required_inputs", -1)
	coordinatorLevel := want.GetIntParam("coordinator_level", -1)
	isBuffetParam := want.GetBoolParam("is_buffet", false)
	coordinatorTypeParam := want.GetStringParam("coordinator_type", "")

	// Determine handler based on coordinator type and parameters
	// Priority: explicit params > type-specific defaults > generic fallback

	// Determine approval level from coordinator type name (backward compat) or parameter
	approvalLevel := coordinatorLevel
	if approvalLevel <= 0 && (coordinatorType == "level2_coordinator") {
		approvalLevel = 2
	}

	// Check if this is an approval coordinator
	if coordinatorTypeParam == "approval" || approvalLevel > 0 || coordinatorType == "level1_coordinator" || coordinatorType == "level2_coordinator" {
		if approvalLevel <= 0 {
			approvalLevel = 1
		}
		return 2,
			&ApprovalDataHandler{Level: approvalLevel},
			&ApprovalCompletionChecker{Level: approvalLevel}
	}

	// Check if this is a travel/buffet coordinator
	requiredInputs := requiredInputsParam
	if requiredInputs <= 0 {
		// Handle legacy coordinator type names
		switch coordinatorType {
		case "travel coordinator":
			requiredInputs = 3
		case "buffet coordinator":
			requiredInputs = 1
		default:
			// Default based on is_buffet parameter
			if isBuffetParam {
				requiredInputs = 1
			} else {
				requiredInputs = 3
			}
		}
	}
	return requiredInputs,
		&TravelDataHandler{IsBuffet: isBuffetParam || coordinatorType == "buffet coordinator"},
		&TravelCompletionChecker{IsBuffet: isBuffetParam || coordinatorType == "buffet coordinator"}
}

// GetWant returns the base Want struct
func (c *CoordinatorWant) GetWant() *Want {
	return &c.Want
}

// Exec executes the coordinator logic using unified completion strategy
// Strategy: Each input channel must send at least one value. When all connected channels
// have sent at least one value, the coordinator completes.
// When a new channel is added, the coordinator resets and waits again.
// Completion is determined by checking the data handler's State cache (e.g., "schedules")
// to verify it has packets from all connected channels.
func (c *CoordinatorWant) Exec() bool {
	inCount := c.GetInCount()

	// If no channels are connected, mark as completed
	if inCount == 0 {
		return true
	}

	// Collect data from all available input channels using the want-level function
	// ReceiveFromAnyInputChannel watches all channels asynchronously and returns
	// the first available data without manual iteration
	for {
		_, data, ok := c.ReceiveFromAnyInputChannel()
		if !ok {
			// No more data available on any channel
			break
		}
		// Let the data handler process the data and store in State cache
		c.DataHandler.ProcessData(c, data)
	}

	// Check completion condition: verify cache in state has all packets from connected channels
	// The cache is maintained by the data handler (e.g., TravelDataHandler stores "schedules")
	// We need at least one packet from each of the currently connected channels
	completionKey := c.DataHandler.GetCompletionKey()

	// Check if data handler's cache has packets from all connected channels
	allChannelsReceived := c.checkAllChannelsRepresentedInCache(inCount)

	// If all channels have sent at least one value, mark completion
	if allChannelsReceived {
		// Let the completion checker perform final processing
		c.CompletionChecker.OnCompletion(c)

		// Apply any state updates from data handler
		stateUpdates := c.DataHandler.GetStateUpdates(c)
		if len(stateUpdates) > 0 {
			c.StoreStateMulti(stateUpdates)
		}

		// Mark as completed
		c.StoreState(completionKey, true)
		return true
	}

	return false // Continue waiting for more data
}

// checkAllChannelsRepresentedInCache verifies the data handler's cache has
// packets from all connected channels by checking the cache size
func (c *CoordinatorWant) checkAllChannelsRepresentedInCache(inCount int) bool {
	// Get the data handler's cache from state
	// For TravelDataHandler, this is "schedules"
	// For ApprovalDataHandler, check if evidence_received AND description_received
	switch c.DataHandler.(type) {
	case *TravelDataHandler:
		schedulesVal, _ := c.GetState("schedules")
		schedules, _ := schedulesVal.([]*TravelSchedule)
		if schedules == nil {
			return false
		}
		return len(schedules) >= inCount
	case *ApprovalDataHandler:
		// For approval, we need both evidence and description
		evidenceVal, _ := c.GetStateBool("evidence_received", false)
		descriptionVal, _ := c.GetStateBool("description_received", false)
		return evidenceVal && descriptionVal
	default:
		// Generic fallback: if we have received data, assume completion
		// This allows for custom DataHandler implementations
		totalProcessed, _ := c.GetStateInt("total_processed", 0)
		return totalProcessed >= inCount
	}
}

// ============================================================================
// Approval-Specific Handlers
// ============================================================================

// ApprovalDataHandler handles approval-specific data processing
type ApprovalDataHandler struct {
	Level int // 1 or 2 for Level1 or Level2 approval
}

func (h *ApprovalDataHandler) ProcessData(want *CoordinatorWant, data interface{}) bool {
	if approvalData, ok := data.(*ApprovalData); ok {
		stateUpdates := make(map[string]interface{})

		if approvalData.Evidence != nil {
			stateUpdates["evidence_received"] = true
			stateUpdates["evidence_type"] = approvalData.Evidence
			stateUpdates["evidence_provided"] = true
			stateUpdates["evidence_provided_at"] = approvalData.Timestamp.Format(time.RFC3339)
			if !approvalData.Timestamp.IsZero() {
				stateUpdates["evidence_received_at"] = approvalData.Timestamp.Format(time.RFC3339)
			}
		}

		if approvalData.Description != "" {
			stateUpdates["description_received"] = true
			stateUpdates["description_text"] = approvalData.Description
			stateUpdates["description_provided"] = true
			stateUpdates["description_provided_at"] = approvalData.Timestamp.Format(time.RFC3339)
			if !approvalData.Timestamp.IsZero() {
				stateUpdates["description_received_at"] = approvalData.Timestamp.Format(time.RFC3339)
			}
		}

		if len(stateUpdates) > 0 {
			want.StoreStateMulti(stateUpdates)
			return true
		}
	}
	return false
}

func (h *ApprovalDataHandler) GetStateUpdates(want *CoordinatorWant) map[string]interface{} {
	// Get level2 authority from want if available
	level2Authority := want.GetStringParam("level2_authority", "senior_manager")

	stateUpdates := map[string]interface{}{
		"total_processed":             1,
		"evidence_provider_complete":  true,
		"description_provider_complete": true,
	}

	// Check which level this is
	levelKey := "approval_level"
	approverIDKey := "approver_id"
	commentsKey := "comments"
	statusKey := "approval_status"
	timeKey := "approval_time"

	if h.Level == 2 {
		levelKey = "approval_level"
		statusKey = "final_approval_status"
		approverIDKey = "approver_id"
		commentsKey = "comments"
		timeKey = "approval_time"

		stateUpdates["level2_authority"] = level2Authority
		stateUpdates[approverIDKey] = level2Authority
		stateUpdates[commentsKey] = "Level 2 final approval granted"
	} else {
		stateUpdates[approverIDKey] = "level1-manager"
		stateUpdates[commentsKey] = "Level 1 approval granted"
	}

	stateUpdates[levelKey] = h.Level
	stateUpdates[statusKey] = "approved"
	stateUpdates[timeKey] = time.Now().Format(time.RFC3339)

	return stateUpdates
}

func (h *ApprovalDataHandler) GetCompletionKey() string {
	if h.Level == 2 {
		return "final_approval_processed"
	}
	return "approval_processed"
}

// ApprovalCompletionChecker checks if approval data is complete
// In unified coordinator: completion is handled by checking if all connected channels
// have sent at least one value. This checker is now optional but kept for backward compatibility.
type ApprovalCompletionChecker struct {
	Level int // 1 or 2
}

func (c *ApprovalCompletionChecker) IsComplete(want *CoordinatorWant, requiredInputs int) bool {
	// In unified coordinator, completion is determined by whether all channels
	// have sent at least one value (handled in Exec). This is kept for compatibility.
	evidenceVal, _ := want.GetStateBool("evidence_received", false)
	descriptionVal, _ := want.GetStateBool("description_received", false)
	return evidenceVal && descriptionVal
}

func (c *ApprovalCompletionChecker) OnCompletion(want *CoordinatorWant) {
	approvalID := want.GetStringParam("approval_id", "")
	statusKey := "approval_status"
	if c.Level == 2 {
		statusKey = "final_approval_status"
	}

	statusVal, _ := want.GetState(statusKey)
	status := "approved"
	if statusVal != nil {
		status = fmt.Sprintf("%v", statusVal)
	}

	approverID := want.GetStringParam("level2_authority", "level1-manager")
	if c.Level == 1 {
		approverID = "level1-manager"
	}

	approverVal, _ := want.GetState("approver_id")
	if approverVal != nil {
		approverID = fmt.Sprintf("%v", approverVal)
	}

	want.StoreLog(fmt.Sprintf("Approval %s: %s by %s at %s",
		approvalID, status, approverID, time.Now().Format("15:04:05")))
}

// ============================================================================
// Travel-Specific Handlers
// ============================================================================

// TravelDataHandler handles travel-specific data processing
type TravelDataHandler struct {
	IsBuffet bool // If true, expect TravelSchedule from buffet
}

func (h *TravelDataHandler) ProcessData(want *CoordinatorWant, data interface{}) bool {
	if schedule, ok := data.(*TravelSchedule); ok {
		// Get existing schedules from state
		schedulesVal, _ := want.GetState("schedules")
		schedules, _ := schedulesVal.([]*TravelSchedule)
		if schedules == nil {
			schedules = make([]*TravelSchedule, 0)
		}

		// Append new schedule
		schedules = append(schedules, schedule)

		// Update persistent state
		want.StoreStateMulti(map[string]interface{}{
			"schedules":      schedules,
			"total_processed": len(schedules),
		})

		return true
	}
	return false
}

func (h *TravelDataHandler) GetStateUpdates(want *CoordinatorWant) map[string]interface{} {
	// For travel coordinator, generate final itinerary
	schedulesVal, _ := want.GetState("schedules")
	schedules, _ := schedulesVal.([]*TravelSchedule)

	stateUpdates := make(map[string]interface{})

	if schedules != nil && len(schedules) > 0 {
		// Combine and sort all events
		allEvents := make([]TimeSlot, 0)
		for _, schedule := range schedules {
			allEvents = append(allEvents, schedule.Events...)
		}

		// Sort events by start time
		for i := 0; i < len(allEvents)-1; i++ {
			for j := i + 1; j < len(allEvents); j++ {
				if allEvents[i].Start.After(allEvents[j].Start) {
					allEvents[i], allEvents[j] = allEvents[j], allEvents[i]
				}
			}
		}

		// Generate readable timeline format
		timeline := generateTravelTimeline(allEvents)

		stateUpdates["final_itinerary"] = allEvents
		stateUpdates["finalResult"] = timeline
	}

	return stateUpdates
}

func (h *TravelDataHandler) GetCompletionKey() string {
	if h.IsBuffet {
		return "buffet_schedule_received"
	}
	return "travel_itinerary_complete"
}

// TravelCompletionChecker checks if all travel schedules have been collected
// In unified coordinator: completion is handled by checking if all connected channels
// have sent at least one value (handled in Exec). This checker is now optional but kept for backward compatibility.
type TravelCompletionChecker struct {
	IsBuffet bool // If true, expect only 1 schedule
}

func (c *TravelCompletionChecker) IsComplete(want *CoordinatorWant, requiredInputs int) bool {
	// In unified coordinator, completion is determined by whether all channels
	// have sent at least one value (handled in Exec). This is kept for compatibility.
	schedulesVal, _ := want.GetState("schedules")
	schedules, _ := schedulesVal.([]*TravelSchedule)

	if schedules == nil {
		return false
	}

	return len(schedules) >= requiredInputs
}

func (c *TravelCompletionChecker) OnCompletion(want *CoordinatorWant) {
	schedulesVal, _ := want.GetState("schedules")
	schedules, _ := schedulesVal.([]*TravelSchedule)

	count := 0
	if schedules != nil {
		count = len(schedules)
	}

	want.StoreLog(fmt.Sprintf("Travel coordinator completed: collected %d schedules", count))
}
