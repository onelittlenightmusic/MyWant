package types

import (
	"fmt"
	. "mywant/engine/src"
	"time"
)

// DataHandler defines the interface for processing received coordinator data
type DataHandler interface {
	ProcessData(want *CoordinatorWant, channelIndex int, data interface{}) bool
	GetStateUpdates(want *CoordinatorWant) map[string]interface{}
	GetCompletionKey() string
	GetCompletionTimeout() time.Duration
}

// CompletionChecker defines the interface for determining when coordinator work is complete
type CompletionChecker interface {
	// IsComplete checks if all required data has been collected
	IsComplete(want *CoordinatorWant, requiredInputs int) bool
	// OnCompletion handles final processing when completion is detected
	OnCompletion(want *CoordinatorWant)
}

// CoordinatorWant is a generic coordinator that collects data from multiple input channels and processes it according to customizable handlers
type CoordinatorWant struct {
	Want
	RequiredInputCount int
	DataHandler        DataHandler
	CompletionChecker  CompletionChecker
	CoordinatorType    string
	paths              Paths
	channelsHeard 		map[int]bool
}

// NewCoordinatorWant creates a new generic coordinator want It automatically determines the required inputs and handlers based on the want type
func NewCoordinatorWant(
	metadata Metadata,
	spec WantSpec,
) Executable {
	coordinatorType := metadata.Type

	want := NewWantWithLocals(
		metadata,
		spec,
		nil,
		ConnectivityMetadata{
			RequiredInputs:  -1,  // Unified: accept any number of inputs
			RequiredOutputs: 0,
			MaxInputs:       -1,  // No maximum
			MaxOutputs:      0,
			WantType:        coordinatorType,
			Description:     fmt.Sprintf("Generic coordinator want (%s)", coordinatorType),
		},
		coordinatorType,
	)

	// Determine coordinator configuration based on want type
	requiredInputs, dataHandler, completionChecker := getCoordinatorConfig(coordinatorType, want)

	coordinator := &CoordinatorWant{
		Want:                *want,
		RequiredInputCount:  requiredInputs,
		DataHandler:         dataHandler,
		CompletionChecker:   completionChecker,
		CoordinatorType:     coordinatorType,
		channelsHeard:       make(map[int]bool),
	}

	return coordinator
}
// This enables full customization of coordinator behavior through parameters
func getCoordinatorConfig(coordinatorType string, want *Want) (int, DataHandler, CompletionChecker) {
	requiredInputsParam := want.GetIntParam("required_inputs", -1)
	coordinatorLevel := want.GetIntParam("coordinator_level", -1)
	isBuffetParam := want.GetBoolParam("is_buffet", false)
	coordinatorTypeParam := want.GetStringParam("coordinator_type", "")

	// Determine handler based on coordinator type and parameters Priority: explicit params > type-specific defaults > generic fallback

	// Determine approval level from coordinator type name (backward compat) or parameter
	approvalLevel := coordinatorLevel
	if approvalLevel <= 0 && (coordinatorType == "level2_coordinator") {
		approvalLevel = 2
	}
	if coordinatorTypeParam == "approval" || approvalLevel > 0 || coordinatorType == "level1_coordinator" || coordinatorType == "level2_coordinator" {
		if approvalLevel <= 0 {
			approvalLevel = 1
		}
		return 2,
			&ApprovalDataHandler{Level: approvalLevel},
			&ApprovalCompletionChecker{Level: approvalLevel}
	}
	requiredInputs := requiredInputsParam
	if requiredInputs <= 0 {
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
	completionTimeoutSeconds := want.GetIntParam("completion_timeout", 0)
	completionTimeout := time.Duration(completionTimeoutSeconds) * time.Second

	return requiredInputs,
		&TravelDataHandler{
			IsBuffet:          isBuffetParam || coordinatorType == "buffet coordinator",
			CompletionTimeout: completionTimeout,
		},
		&TravelCompletionChecker{IsBuffet: isBuffetParam || coordinatorType == "buffet coordinator"}
}

// Exec executes the coordinator logic using unified completion strategy Strategy: Each input channel must send at least one value. When all connected channels have sent at least one value, the coordinator completes. When a new channel is added, the coordinator automatically re-executes with the new channel.
// Completion is determined by tracking which channels have sent data in the current execution cycle. This simple approach automatically handles topology changes without needing cache resets.
func (c *CoordinatorWant) Exec() bool {
	inCount := c.GetInCount()

	c.StoreLog(fmt.Sprintf("[COORDINATOR] Started"))

	// // CRITICAL: Reset channelsHeard at the start of each execution cycle // This ensures that when a coordinator is retriggered (e.g., to receive a rebooked packet), // it doesn't think it has already received all packets from the previous execution. // Without this reset, a retriggered coordinator would complete immediately.
	// c.channelsHeard = make(map[int]bool)

	// Track which channels we've received data from in this execution cycle This is a local map - NOT persisted to state, only used for completion detection

	timeout := 2000
	// loop while receiving data packets
	for {
		// time.Sleep(1000*time.Millisecond) Try to receive one data packet from any input channel
		channelIndex, data, received := c.ReceiveFromAnyInputChannel(timeout)
		if received {
			// Data received: mark channel as heard and process it
			c.channelsHeard[channelIndex] = true
			c.StoreLog(fmt.Sprintf("[PACKET-RECV] Coordinator received packet from channel %d", channelIndex))
			c.DataHandler.ProcessData(c, channelIndex, data)
		} else {
			// No data available on any channel: exit loop
			break
		}
	}
	return c.tryCompletion(inCount, c.channelsHeard)
}

// tryCompletion checks if all required data has been received and handles completion Uses a timeout-based approach to allow late-arriving packets (e.g., Rebook flights) Strategy: 1. When all channels first send data, record the time
// 2. Wait for the completion timeout to expire (allows delayed packets) 3. Only then mark as completed and reset channelsHeard for potential new packets
func (c *CoordinatorWant) tryCompletion(inCount int, channelsHeard map[int]bool) bool {
	// Apply state updates from data handler
	stateUpdates := c.DataHandler.GetStateUpdates(c)
	if len(stateUpdates) > 0 {
		c.StoreStateMulti(stateUpdates)
	}
	if len(channelsHeard) != inCount {
		return false // Still waiting for data from some channels
	}

	// All channels have sent: record the first time (independent of timeout)
	lastPacketTimeVal, exists := c.GetState("last_packet_time")
	if !exists || lastPacketTimeVal == nil {
		nowUnix := time.Now().Unix()
		c.StoreState("last_packet_time", nowUnix)
	}
	completionTimeout := c.DataHandler.GetCompletionTimeout()

	if completionTimeout > 0 {
		// Timeout is configured: need to wait for it to expire
		lastPacketTimeVal, _ := c.GetState("last_packet_time")
		var lastPacketTime int64
		switch v := lastPacketTimeVal.(type) {
		case int64:
			lastPacketTime = v
		case float64:
			lastPacketTime = int64(v)
		case time.Time:
			// If somehow it was stored as time.Time, convert to Unix
			lastPacketTime = v.Unix()
		default:
			// If somehow still nil/invalid, this shouldn't happen since we just set it above
			nowUnix := time.Now().Unix()
			c.StoreState("last_packet_time", nowUnix)
			return false
		}

		nowUnix := time.Now().Unix()
		elapsed := nowUnix - lastPacketTime

		if elapsed < int64(completionTimeout.Seconds()) {
			return false // Still waiting for timeout
		}
	}
	completionKey := c.DataHandler.GetCompletionKey()
	c.CompletionChecker.OnCompletion(c)
	c.StoreState(completionKey, true)

	return true
}
func (c *CoordinatorWant) checkAllChannelsRepresentedInCache(inCount int) bool {
	// Common logic for all handlers: check data_by_channel
	dataByChannelVal, _ := c.GetState("data_by_channel")
	if dataByChannelVal == nil {
		return false
	}
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
	if dataCount != inCount {
		return false
	}

	// All channels have sent at least one packet Now check if enough time has passed since the last packet (allows for delayed packets like Rebook)
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

// ============================================================================ Approval-Specific Handlers ============================================================================

// ApprovalDataHandler handles approval-specific data processing
type ApprovalDataHandler struct {
	Level int // 1 or 2 for Level1 or Level2 approval
}

func (h *ApprovalDataHandler) ProcessData(want *CoordinatorWant, channelIndex int, data interface{}) bool {
	if approvalData, ok := data.(*ApprovalData); ok {
		dataByChannelVal, _ := want.GetState("data_by_channel")
		dataByChannel, ok := dataByChannelVal.(map[int]interface{})
		if !ok {
			if dataByChannelVal == nil {
				dataByChannel = make(map[int]interface{})
			}
		}
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
	level2Authority := want.GetStringParam("level2_authority", "senior_manager")

	stateUpdates := map[string]interface{}{
		"total_processed":             1,
		"evidence_provider_complete":  true,
		"description_provider_complete": true,
	}
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

// ApprovalCompletionChecker checks if approval data is complete In unified coordinator: completion is handled by checking if all connected channels have sent at least one value. This checker is now optional but kept for backward compatibility.
type ApprovalCompletionChecker struct {
	Level int // 1 or 2
}

func (c *ApprovalCompletionChecker) IsComplete(want *CoordinatorWant, requiredInputs int) bool {
	// In unified coordinator, completion is determined by whether all channels have sent at least one value (handled in Exec). This is kept for compatibility.
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

// ============================================================================ Travel-Specific Handlers ============================================================================

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
	dataByChannelVal, _ := want.GetState("data_by_channel")
	dataByChannel, ok := dataByChannelVal.(map[int]interface{})
	if !ok {
		if dataByChannelVal == nil {
			dataByChannel = make(map[int]interface{})
		}
	}
	dataByChannel[channelIndex] = schedule

	// Track total packets received
	totalPacketsVal, _ := want.GetStateInt("total_packets_received", 0)
	totalPackets := totalPacketsVal + 1
	stateUpdates := map[string]interface{}{
		"data_by_channel":        dataByChannel,
		"total_packets_received": totalPackets,
	}

	// Only set last_packet_time if it hasn't been set yet
	if lastPacketTimeVal, exists := want.GetState("last_packet_time"); !exists || lastPacketTimeVal == nil {
		stateUpdates["last_packet_time"] = time.Now().Unix()
	}

	want.StoreStateMulti(stateUpdates)

	return true
}

func (h *TravelDataHandler) GetStateUpdates(want *CoordinatorWant) map[string]interface{} {
	// For travel coordinator, generate final itinerary from all channels
	dataByChannelVal, _ := want.GetState("data_by_channel")

	stateUpdates := make(map[string]interface{})
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

// TravelCompletionChecker checks if all travel schedules have been collected In unified coordinator: completion is handled by checking if all connected channels have sent at least one value (handled in Exec). This checker is now optional but kept for backward compatibility.
type TravelCompletionChecker struct {
	IsBuffet bool // If true, expect only 1 schedule
}

func (c *TravelCompletionChecker) IsComplete(want *CoordinatorWant, requiredInputs int) bool {
	// In unified coordinator, completion is determined by whether all channels have sent at least one value (handled in Exec). This is kept for compatibility.
	schedulesByChannelVal, _ := want.GetState("schedules_by_channel")
	schedulesByChannel, _ := schedulesByChannelVal.(map[int][]*TravelSchedule)

	if schedulesByChannel == nil {
		return false
	}
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
