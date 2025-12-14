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
) Progressable {
	coordinatorType := metadata.Type

	want := NewWantWithLocals(
		metadata,
		spec,
		nil,
		coordinatorType,
	)

	// Determine coordinator configuration based on want type
	_, dataHandler, completionChecker := getCoordinatorConfig(coordinatorType, want)

	coordinator := &CoordinatorWant{
		Want:               *want,
		DataHandler:        dataHandler,
		CompletionChecker:  completionChecker,
		CoordinatorType:    coordinatorType,
		channelsHeard:      make(map[int]bool),
	}

	return coordinator
}
// This enables full customization of coordinator behavior through parameters
func getCoordinatorConfig(coordinatorType string, want *Want) (int, DataHandler, CompletionChecker) {
	coordinatorLevel := want.GetIntParam("coordinator_level", -1)
	coordinatorTypeParam := want.GetStringParam("coordinator_type", "")

	// Approval coordinator configuration
	if coordinatorTypeParam == "approval" || coordinatorLevel > 0 {
		approvalLevel := coordinatorLevel
		if approvalLevel <= 0 {
			approvalLevel = 1
		}
		return 0,
			&ApprovalDataHandler{Level: approvalLevel},
			&ApprovalCompletionChecker{Level: approvalLevel}
	}

	// Travel coordinator configuration
	isBuffetParam := want.GetBoolParam("is_buffet", false)
	completionTimeoutSeconds := want.GetIntParam("completion_timeout", 0)
	completionTimeout := time.Duration(completionTimeoutSeconds) * time.Second

	return 0,
		&TravelDataHandler{
			IsBuffet:          isBuffetParam || coordinatorType == "buffet coordinator",
			CompletionTimeout: completionTimeout,
		},
		&TravelCompletionChecker{IsBuffet: isBuffetParam || coordinatorType == "buffet coordinator"}
}

// IsAchieved checks if coordinator has collected all required data and timeout has expired
func (c *CoordinatorWant) IsAchieved() bool {
	completionKey := c.DataHandler.GetCompletionKey()
	completed, _ := c.GetStateBool(completionKey, false)
	return completed
}

// Progress executes the coordinator logic using unified completion strategy Strategy: Each input channel must send at least one value. When all connected channels have sent at least one value, the coordinator completes. When a new channel is added, the coordinator automatically re-executes with the new channel.
// Completion is determined by tracking which channels have sent data in the current execution cycle. This simple approach automatically handles topology changes without needing cache resets.
func (c *CoordinatorWant) Progress() {
	c.StoreLog(fmt.Sprintf("[COORDINATOR] Started"))

	// Track which channels we've received data from in this execution cycle This is a local map - NOT persisted to state, only used for completion detection

	timeout := 2000
	// time.Sleep(1000*time.Millisecond) Try to receive one data packet from any input channel
	channelIndex, data, received := c.Use(timeout)
	if received {
		// Data received: mark channel as heard and process it
		c.channelsHeard[channelIndex] = true
		c.StoreLog(fmt.Sprintf("[PACKET-RECV] Coordinator received packet from channel %d", channelIndex))
		c.DataHandler.ProcessData(c, channelIndex, data)
	}
	c.tryCompletion(c.channelsHeard)
}

// tryCompletion checks if all required data has been received and handles completion Uses a timeout-based approach to allow late-arriving packets (e.g., Rebook flights) Strategy: 1. When all channels first send data, record the time
// 2. Wait for the completion timeout to expire (allows delayed packets) 3. Only then mark as completed and reset channelsHeard for potential new packets
func (c *CoordinatorWant) tryCompletion(channelsHeard map[int]bool) {
	// Apply state updates from data handler
	stateUpdates := c.DataHandler.GetStateUpdates(c)
	if len(stateUpdates) > 0 {
		c.StoreStateMulti(stateUpdates)
	}
	if len(channelsHeard) != c.GetInCount() {
		return // Still waiting for data from some channels
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
			return
		}

		nowUnix := time.Now().Unix()
		elapsed := nowUnix - lastPacketTime

		if elapsed < int64(completionTimeout.Seconds()) {
			return // Still waiting for timeout
		}
	}
	completionKey := c.DataHandler.GetCompletionKey()
	c.CompletionChecker.OnCompletion(c)
	c.StoreState(completionKey, true)
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
		"total_processed":              1,
		"evidence_provider_complete":   true,
		"description_provider_complete": true,
		"approval_level":               h.Level,
		"approval_status":              "approved",
		"approval_time":                time.Now().Format(time.RFC3339),
	}

	// Build approval input details
	approvalInputs := []string{"evidence", "description"}
	if h.Level == 2 {
		approvalInputs = append(approvalInputs, "level 2 authority")
		stateUpdates["level2_authority"] = level2Authority
		stateUpdates["approver_id"] = level2Authority
		stateUpdates["comments"] = "Level 2 final approval granted"
	} else {
		stateUpdates["approver_id"] = "level1-manager"
		stateUpdates["comments"] = "Level 1 approval granted"
	}

	// Build coordinator result with nested structure
	coordinatorResult := map[string]interface{}{
		"approval_input": approvalInputs,
	}
	stateUpdates["coordinator_result"] = coordinatorResult

	return stateUpdates
}

func (h *ApprovalDataHandler) GetCompletionKey() string {
	return "coordinator_completed"
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

	statusVal, _ := want.GetState("approval_status")
	status := "approved"
	if statusVal != nil {
		status = fmt.Sprintf("%v", statusVal)
	}

	approverVal, _ := want.GetState("approver_id")
	approverID := "level1-manager"
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

	// Build travel inputs list
	travelInputs := []string{"restaurant", "hotel", "buffet"}
	coordinatorResult := map[string]interface{}{
		"travel_input": travelInputs,
	}
	stateUpdates["coordinator_result"] = coordinatorResult

	return stateUpdates
}

func (h *TravelDataHandler) GetCompletionKey() string {
	return "coordinator_completed"
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
