package types

import (
	"fmt"
	. "mywant/engine/src"
	"time"
)

// DataHandler defines the interface for processing received coordinator data
type DataHandler interface {
	// ProcessData handles incoming data from a specific input channel
	// channelIndex indicates which input channel the data came from
	ProcessData(want *CoordinatorWant, channelIndex int, data interface{}) bool
	// GetStateUpdates returns state updates to apply after data collection
	GetStateUpdates(want *CoordinatorWant) map[string]interface{}
	// GetCompletionKey returns the key used to check if processing is complete
	GetCompletionKey() string
	// GetCompletionTimeout returns the duration to wait after last packet before completing
	GetCompletionTimeout() time.Duration
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
	channelsHeard 		map[int]bool
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
	coordinator.channelsHeard = make(map[int]bool)

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

	// Get completion timeout parameter (default: 0 seconds = immediate completion)
	// Set to non-zero (e.g., 60) to wait for delayed packets like Flight rebooking
	completionTimeoutSeconds := want.GetIntParam("completion_timeout", 0)
	completionTimeout := time.Duration(completionTimeoutSeconds) * time.Second

	return requiredInputs,
		&TravelDataHandler{
			IsBuffet:          isBuffetParam || coordinatorType == "buffet coordinator",
			CompletionTimeout: completionTimeout,
		},
		&TravelCompletionChecker{IsBuffet: isBuffetParam || coordinatorType == "buffet coordinator"}
}

// GetWant returns the base Want struct
func (c *CoordinatorWant) GetWant() *Want {
	return &c.Want
}

// Exec executes the coordinator logic using unified completion strategy
// Strategy: Each input channel must send at least one value. When all connected channels
// have sent at least one value, the coordinator completes.
// When a new channel is added, the coordinator automatically re-executes with the new channel.
// Completion is determined by tracking which channels have sent data in the current execution cycle.
// This simple approach automatically handles topology changes without needing cache resets.
func (c *CoordinatorWant) Exec() bool {
	inCount := c.GetInCount()
	// outCount := c.GetOutCount()
	// paths := c.GetPaths()
	// c.StoreLog(fmt.Sprintf("[COORDINATOR-EXEC] GetInCount()=%d, GetOutCount()=%d, paths.In length=%d, paths.Out length=%d\n", inCount, outCount, len(paths.In), len(paths.Out)))

	// Track which channels we've received data from in this execution cycle
	// This is a local map - NOT persisted to state, only used for completion detection

	// loop while receiving data packets
	for {
		// Try to receive one data packet from any input channel
		channelIndex, data, received := c.ReceiveFromAnyInputChannel()
		if received {
			// Data received: mark channel as heard and process it
			c.channelsHeard[channelIndex] = true
			c.DataHandler.ProcessData(c, channelIndex, data)
		} else {
			// No data available on any channel: exit loop
			break
		}
	}

	// Check completion: have we heard from all required input channels?
	// This approach is simpler than len(data_by_channel) and automatically
	// handles topology changes without needing cache resets
	return c.tryCompletion(inCount, c.channelsHeard)
}

// tryCompletion checks if all required data has been received and handles completion
// This now uses channelsHeard (local tracking) instead of len(data_by_channel)
// which eliminates the need for cache resets on topology changes
func (c *CoordinatorWant) tryCompletion(inCount int, channelsHeard map[int]bool) bool {
	// Simple completion check: have we heard from all connected channels?
	if len(channelsHeard) != inCount {
		return false // Still waiting for data from some channels
	}

	// All channels have sent at least one value: mark completion
	completionKey := c.DataHandler.GetCompletionKey()

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

// checkAllChannelsRepresentedInCache verifies the data handler's cache has
// packets from all connected channels using common logic
func (c *CoordinatorWant) checkAllChannelsRepresentedInCache(inCount int) bool {
	// Common logic for all handlers: check data_by_channel
	dataByChannelVal, _ := c.GetState("data_by_channel")
	if dataByChannelVal == nil {
		return false
	}

	// Handle both map[int]interface{} (direct) and map[string]interface{} (after JSON serialization)
	var dataCount int
	switch v := dataByChannelVal.(type) {
	case map[int]interface{}:
		dataCount = len(v)
	case map[string]interface{}:
		dataCount = len(v)
	default:
		c.StoreLog(fmt.Sprintf("[ERROR] CoordinatorWant.checkAllChannelsRepresentedInCache: type assertion failed for data_by_channel. Expected map[int]interface{} or map[string]interface{}, got %T", dataByChannelVal))
		return false
	}

	// Check that we have received packets from ALL connected channels
	if dataCount != inCount {
		return false
	}

	// All channels have sent at least one packet
	// Now check if enough time has passed since the last packet (allows for delayed packets like Rebook)
	completionTimeout := c.DataHandler.GetCompletionTimeout()
	if completionTimeout > 0 {
		lastPacketTimeVal, _ := c.GetState("last_packet_time")
		if lastPacketTimeVal != nil {
			if lastPacketTime, ok := lastPacketTimeVal.(time.Time); ok {
				timeSinceLastPacket := time.Since(lastPacketTime)
				if timeSinceLastPacket < completionTimeout {
					// Waiting for completion timeout - removed verbose debug log
					return false
				}
				// Coordinator completing - removed verbose debug log
			}
		}
	}

	return true
}

// ============================================================================
// Approval-Specific Handlers
// ============================================================================

// ApprovalDataHandler handles approval-specific data processing
type ApprovalDataHandler struct {
	Level int // 1 or 2 for Level1 or Level2 approval
}

func (h *ApprovalDataHandler) ProcessData(want *CoordinatorWant, channelIndex int, data interface{}) bool {
	if approvalData, ok := data.(*ApprovalData); ok {
		// Get existing data map (keyed by channel index) - generic state key
		dataByChannelVal, _ := want.GetState("data_by_channel")
		dataByChannel, ok := dataByChannelVal.(map[int]interface{})
		if !ok {
			if dataByChannelVal == nil {
				dataByChannel = make(map[int]interface{})
			}
		}

		// Store approval data for this channel
		dataByChannel[channelIndex] = approvalData

		// Prepare state updates (includes legacy keys for backward compatibility)
		stateUpdates := make(map[string]interface{})
		stateUpdates["data_by_channel"] = dataByChannel

		// Track total packets received
		totalPacketsVal, _ := want.GetStateInt("total_packets_received", 0)
		stateUpdates["total_packets_received"] = totalPacketsVal + 1
		stateUpdates["last_packet_time"] = time.Now()

		// Legacy state keys (for backward compatibility)
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

		want.StoreStateMulti(stateUpdates)
		return true
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

func (h *ApprovalDataHandler) GetCompletionTimeout() time.Duration {
	// Approval coordinators complete immediately (no timeout needed)
	return 0
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
	IsBuffet          bool          // If true, expect TravelSchedule from buffet
	CompletionTimeout time.Duration // Time to wait after last packet before completing (allows for Rebook packets)
}

func (h *TravelDataHandler) ProcessData(want *CoordinatorWant, channelIndex int, data interface{}) bool {
	schedule, ok := data.(*TravelSchedule)
	if !ok {
		want.StoreLog(fmt.Sprintf("[ERROR] TravelDataHandler.ProcessData: type assertion failed for data. Expected *TravelSchedule, got %T", data))
	}
	// Log packet reception with content details
	eventDetails := ""
	if len(schedule.Events) > 0 {
		eventDetails = fmt.Sprintf(" [event0: %s, %s-%s]",
			schedule.Events[0].Name,
			schedule.Events[0].Start.Format("15:04:05"),
			schedule.Events[0].End.Format("15:04:05"))
	}
	want.StoreLog(fmt.Sprintf("[PACKET-RECV] Coordinator received TravelSchedule on channel %d: Date=%s, Events=%d%s",
		channelIndex,
		schedule.Date.Format("2006-01-02"),
		len(schedule.Events),
		eventDetails))

	// Get existing data map (keyed by channel index) - generic state key
	dataByChannelVal, _ := want.GetState("data_by_channel")
	dataByChannel, ok := dataByChannelVal.(map[int]interface{})
	if !ok {
		if dataByChannelVal == nil {
			dataByChannel = make(map[int]interface{})
		}
	}

	// Store schedule data for this channel
	dataByChannel[channelIndex] = schedule

	// Track total packets received
	totalPacketsVal, _ := want.GetStateInt("total_packets_received", 0)
	totalPackets := totalPacketsVal + 1

	// Update persistent state with generic key
	want.StoreStateMulti(map[string]interface{}{
		"data_by_channel":         dataByChannel,
		"total_packets_received":  totalPackets,
		"last_packet_time":        time.Now(),
	})

	return true
}

func (h *TravelDataHandler) GetStateUpdates(want *CoordinatorWant) map[string]interface{} {
	// For travel coordinator, generate final itinerary from all channels
	dataByChannelVal, _ := want.GetState("data_by_channel")

	stateUpdates := make(map[string]interface{})

	// Handle both map[int]interface{} (direct) and map[string]interface{} (after JSON serialization)
	allEvents := make([]TimeSlot, 0)
	switch v := dataByChannelVal.(type) {
	case map[int]interface{}:
		for _, data := range v {
			if schedule, ok := data.(*TravelSchedule); ok {
				allEvents = append(allEvents, schedule.Events...)
			} else {
				want.StoreLog(fmt.Sprintf("[ERROR] TravelDataHandler.GetStateUpdates: type assertion failed for channel data. Expected *TravelSchedule, got %T", data))
			}
		}
	case map[string]interface{}:
		for _, data := range v {
			if schedule, ok := data.(*TravelSchedule); ok {
				allEvents = append(allEvents, schedule.Events...)
			} else {
				want.StoreLog(fmt.Sprintf("[ERROR] TravelDataHandler.GetStateUpdates: type assertion failed for channel data. Expected *TravelSchedule, got %T", data))
			}
		}
	default:
		if dataByChannelVal != nil {
			want.StoreLog(fmt.Sprintf("[ERROR] TravelDataHandler.GetStateUpdates: type assertion failed for data_by_channel. Expected map[int]interface{} or map[string]interface{}, got %T", dataByChannelVal))
		}
		return stateUpdates
	}

	if len(allEvents) > 0 {
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

func (h *TravelDataHandler) GetCompletionTimeout() time.Duration {
	return h.CompletionTimeout
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
	schedulesByChannelVal, _ := want.GetState("schedules_by_channel")
	schedulesByChannel, _ := schedulesByChannelVal.(map[int][]*TravelSchedule)

	if schedulesByChannel == nil {
		return false
	}

	// Check that each connected channel has at least one schedule
	totalSchedules := 0
	for i := 0; i < requiredInputs; i++ {
		if schedules, exists := schedulesByChannel[i]; !exists || len(schedules) == 0 {
			return false
		}
		totalSchedules += len(schedulesByChannel[i])
	}

	return totalSchedules >= requiredInputs
}

func (c *TravelCompletionChecker) OnCompletion(want *CoordinatorWant) {
	// Completion logging disabled to reduce log noise
}
