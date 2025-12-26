package types

import (
	"fmt"
	. "mywant/engine/src"
	"time"
)

// DataHandler defines the interface for processing received coordinator data
// Each handler processes only its specific data type, no type-checking logic
type DataHandler interface {
	ProcessData(want *CoordinatorWant, channelIndex int, data any) bool
	GetStateUpdates(want *CoordinatorWant) map[string]any
	GetCompletionKey() string
	GetCompletionTimeout() time.Duration
}

// DataHandlerDispatcher routes received data to the appropriate handler based on data type
// This centralizes type-based routing logic using a handler registry instead of hardcoded switch statements
// Handlers are registered by type name, making it easy to add new types without modifying the dispatcher
type DataHandlerDispatcher struct {
	handlers       map[string]DataHandler // Maps type name to handler
	defaultHandler DataHandler            // Fallback handler for unknown types
}

// NewDataHandlerDispatcher creates a new dispatcher with the appropriate handlers
func NewDataHandlerDispatcher(approvalHandler *ApprovalDataHandler, travelHandler *TravelDataHandler) *DataHandlerDispatcher {
	handlers := make(map[string]DataHandler)

	// Register handlers by their data type names
	handlers["*types.ApprovalData"] = approvalHandler
	handlers["*types.TravelSchedule"] = travelHandler

	return &DataHandlerDispatcher{
		handlers:       handlers,
		defaultHandler: &DefaultDataHandler{},
	}
}

// RegisterHandler adds or updates a handler for a specific data type
// typeName should be the fully qualified type name (e.g., "*types.CustomData")
func (d *DataHandlerDispatcher) RegisterHandler(typeName string, handler DataHandler) {
	d.handlers[typeName] = handler
}

// SelectHandler returns the appropriate handler for the given data type
// First checks the type registry, then falls back to the default handler
func (d *DataHandlerDispatcher) SelectHandler(data any) DataHandler {
	// Get the fully qualified type name
	typeName := fmt.Sprintf("%T", data)

	// Check if we have a registered handler for this type
	if handler, exists := d.handlers[typeName]; exists {
		return handler
	}

	// Fall back to default handler
	return d.defaultHandler
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
	DataHandler           DataHandler            // The primary data handler (delegated to dispatcher)
	DataHandlerDispatcher *DataHandlerDispatcher // Dispatcher for routing to type-specific handlers
	CompletionChecker     CompletionChecker
	CoordinatorType       string
	paths                 Paths
	channelsHeard         map[int]bool
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

	// Create dispatcher with approval and travel handlers
	approvalHandler := &ApprovalDataHandler{Level: 1}
	travelHandler := &TravelDataHandler{IsBuffet: false, CompletionTimeout: 0}
	dispatcher := NewDataHandlerDispatcher(approvalHandler, travelHandler)

	coordinator := &CoordinatorWant{
		Want:                  *want,
		DataHandler:           dataHandler,
		DataHandlerDispatcher: dispatcher,
		CompletionChecker:     completionChecker,
		CoordinatorType:       coordinatorType,
		channelsHeard:         make(map[int]bool),
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

// Initialize resets state before execution begins
func (c *CoordinatorWant) Initialize() {
	// No state reset needed for coordinator wants
}

// IsAchieved checks if coordinator has collected all required data and timeout has expired
func (c *CoordinatorWant) IsAchieved() bool {
	completionKey := c.DataHandler.GetCompletionKey()
	completed, _ := c.GetStateBool(completionKey, false)

	// Even if completion flag is set, check if there are unused packets remaining
	// This allows coordinator to retrigger when new packets arrive (e.g., during rebooking)
	// Wait up to 5000ms for new packets to arrive before declaring completion
	if completed {
		// Check if unused packets exist (with reflect.Select monitoring)
		// Wait up to 5000ms for rebooked packets to arrive (e.g., when Flight sends rebooked packet after delay detection)
		// This gives the packet time to traverse the channel after retrigger
		beforeCheck := time.Now()
		hasUnused := c.UnusedExists(1000)
		afterCheck := time.Now()
		elapsed := afterCheck.Sub(beforeCheck).Milliseconds()
		c.StoreLog(fmt.Sprintf("[IsAchieved] completed=true, hasUnused=%v, returning=%v (waited %dms for packet detection)", hasUnused, !hasUnused, elapsed))
		// Return false if unused packets exist, allowing further processing
		return !hasUnused
	}

	c.StoreLog(fmt.Sprintf("[IsAchieved] completed=false, waiting for initial packets"))
	return false
}

// Progress executes the coordinator logic using unified completion strategy Strategy: Each input channel must send at least one value. When all connected channels have sent at least one value, the coordinator completes. When a new channel is added, the coordinator automatically re-executes with the new channel.
// Completion is determined by tracking which channels have sent data in the current execution cycle. This simple approach automatically handles topology changes without needing cache resets.
// Uses dispatcher to route data to the appropriate handler based on data type.
func (c *CoordinatorWant) Progress() {
	inCount := c.GetInCount()
	heardsCount := len(c.channelsHeard)
	c.StoreLog(fmt.Sprintf("[Progress] Started - InCount=%d, ChannelsHeard=%d", inCount, heardsCount))

	// Track which channels we've received data from in this execution cycle This is a local map - NOT persisted to state, only used for completion detection

	timeout := 2000
	// time.Sleep(1000*time.Millisecond) Try to receive one data packet from any input channel
	channelIndex, data, received := c.Use(timeout)
	if received {
		// Data received: mark channel as heard and process it
		c.channelsHeard[channelIndex] = true
		c.StoreLog(fmt.Sprintf("[Progress] Received packet from channel %d (total heard: %d/%d), data type: %T", channelIndex, len(c.channelsHeard), inCount, data))

		// Use dispatcher to select appropriate handler based on data type
		handler := c.DataHandlerDispatcher.SelectHandler(data)
		c.StoreLog(fmt.Sprintf("[Progress] Selected handler: %T for data type %T", handler, data))
		handler.ProcessData(c, channelIndex, data)
	} else {
		c.StoreLog(fmt.Sprintf("[Progress] No packet received within timeout (heard: %d/%d)", len(c.channelsHeard), inCount))
	}

	// Calculate and store achieving_percentage based on actual received data
	// Use data_by_channel to get the actual count of received packets
	dataByChannelVal, _ := c.GetState("data_by_channel")
	receivedCount := 0
	if dataByChannel, ok := dataByChannelVal.(map[int]any); ok {
		receivedCount = len(dataByChannel)
	} else if dataByChannel, ok := dataByChannelVal.(map[string]any); ok {
		receivedCount = len(dataByChannel)
	}

	achievingPercentage := 0
	if inCount > 0 {
		achievingPercentage = (receivedCount * 100) / inCount
		if achievingPercentage > 100 {
			achievingPercentage = 100
		}
	}
	c.StoreState("achieving_percentage", achievingPercentage)

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

	inCount := c.GetInCount()
	if len(channelsHeard) != inCount {
		c.StoreLog(fmt.Sprintf("[tryCompletion] Waiting for all channels - heard %d/%d", len(channelsHeard), inCount))
		return // Still waiting for data from some channels
	}

	c.StoreLog(fmt.Sprintf("[tryCompletion] All channels heard (%d/%d), checking timeout", len(channelsHeard), inCount))

	// All channels have sent: record the first time (independent of timeout)
	lastPacketTimeVal, exists := c.GetState("last_packet_time")
	if !exists || lastPacketTimeVal == nil {
		nowUnix := time.Now().Unix()
		c.StoreState("last_packet_time", nowUnix)
		c.StoreLog("[tryCompletion] Recording first packet time")
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
			c.StoreLog("[tryCompletion] Reset packet time due to invalid state")
			return
		}

		nowUnix := time.Now().Unix()
		elapsed := nowUnix - lastPacketTime

		if elapsed < int64(completionTimeout.Seconds()) {
			c.StoreLog(fmt.Sprintf("[tryCompletion] Waiting for timeout - elapsed %d/%d seconds", elapsed, int64(completionTimeout.Seconds())))
			return // Still waiting for timeout
		}
		c.StoreLog("[tryCompletion] Timeout expired, marking complete")
	}
	completionKey := c.DataHandler.GetCompletionKey()
	c.CompletionChecker.OnCompletion(c)
	c.StoreState(completionKey, true)
	c.StoreState("achieving_percentage", 100)
	c.StoreLog("[tryCompletion] Marked as complete, resetting channelsHeard for potential retrigger")
	c.channelsHeard = make(map[int]bool)
}

// ============================================================================ Default Handler for Generic Map Data ============================================================================

// DefaultDataHandler processes generic map[string]any data
// This is the fallback handler for any data type that doesn't have a specialized handler
type DefaultDataHandler struct{}

func (h *DefaultDataHandler) ProcessData(want *CoordinatorWant, channelIndex int, data any) bool {
	packetMap, ok := data.(map[string]any)
	if !ok {
		want.StoreLog(fmt.Sprintf("[DefaultDataHandler] Expected map[string]any, got %T", data))
		return false
	}
	return processDefaultMapData(want, channelIndex, packetMap)
}

func (h *DefaultDataHandler) GetStateUpdates(want *CoordinatorWant) map[string]any {
	// Default handler returns empty state updates - no special processing needed
	return make(map[string]any)
}

func (h *DefaultDataHandler) GetCompletionKey() string {
	return "coordinator_completed"
}

func (h *DefaultDataHandler) GetCompletionTimeout() time.Duration {
	return 0
}

// ============================================================================ Common Handler Utilities ============================================================================

// processDefaultMapData is a helper function for processing generic map[string]any data
func processDefaultMapData(want *CoordinatorWant, channelIndex int, packetMap map[string]any) bool {
	dataByChannelVal, _ := want.GetState("data_by_channel")
	dataByChannel, ok := dataByChannelVal.(map[int]any)
	if !ok {
		if dataByChannelVal == nil {
			dataByChannel = make(map[int]any)
		}
	}

	dataByChannel[channelIndex] = packetMap

	// Track total packets received
	totalPacketsVal, _ := want.GetStateInt("total_packets_received", 0)
	want.StoreStateMulti(Dict{
		"data_by_channel":        dataByChannel,
		"total_packets_received": totalPacketsVal + 1,
		"last_packet_time":       time.Now(),
	})

	// Extract and store common fields if present
	commonFields := make(map[string]any)
	if status, exists := packetMap["status"]; exists {
		commonFields["status"] = status
	}
	if name, exists := packetMap["name"]; exists {
		commonFields["name"] = name
	}
	if typeVal, exists := packetMap["type"]; exists {
		commonFields["type"] = typeVal
	}

	if len(commonFields) > 0 {
		want.StoreStateMulti(commonFields)
	}

	return true
}

// ============================================================================ Approval-Specific Handlers ============================================================================

// ApprovalDataHandler handles approval-specific data processing
type ApprovalDataHandler struct {
	Level int // 1 or 2 for Level1 or Level2 approval
}

func (h *ApprovalDataHandler) ProcessData(want *CoordinatorWant, channelIndex int, data any) bool {
	// ApprovalDataHandler only processes *ApprovalData
	// Type-checking and routing to other handlers is handled by the dispatcher
	approvalData, ok := data.(*ApprovalData)
	if !ok {
		want.StoreLog(fmt.Sprintf("[ApprovalDataHandler] Expected *ApprovalData, got %T. Dispatcher should have routed to appropriate handler.", data))
		return false
	}

	dataByChannelVal, _ := want.GetState("data_by_channel")
	dataByChannel, ok := dataByChannelVal.(map[int]any)
	if !ok {
		if dataByChannelVal == nil {
			dataByChannel = make(map[int]any)
		}
	}

	dataByChannel[channelIndex] = approvalData

	// Store packet data and tracking
	totalPacketsVal, _ := want.GetStateInt("total_packets_received", 0)
	want.StoreStateMulti(Dict{
		"data_by_channel":        dataByChannel,
		"total_packets_received": totalPacketsVal + 1,
		"last_packet_time":       time.Now(),
	})

	// Store evidence data if present
	if approvalData.Evidence != nil {
		want.StoreStateMulti(Dict{
			"evidence_received":    true,
			"evidence_type":        approvalData.Evidence,
			"evidence_provided":    true,
			"evidence_provided_at": approvalData.Timestamp.Format(time.RFC3339),
		})
		if !approvalData.Timestamp.IsZero() {
			want.StoreState("evidence_received_at", approvalData.Timestamp.Format(time.RFC3339))
		}
	}

	// Store description data if present
	if approvalData.Description != "" {
		want.StoreStateMulti(Dict{
			"description_received":    true,
			"description_text":        approvalData.Description,
			"description_provided":    true,
			"description_provided_at": approvalData.Timestamp.Format(time.RFC3339),
		})
		if !approvalData.Timestamp.IsZero() {
			want.StoreState("description_received_at", approvalData.Timestamp.Format(time.RFC3339))
		}
	}

	return true
}

func (h *ApprovalDataHandler) GetStateUpdates(want *CoordinatorWant) map[string]any {
	level2Authority := want.GetStringParam("level2_authority", "senior_manager")

	stateUpdates := Dict{
		"total_processed":               1,
		"evidence_provider_complete":    true,
		"description_provider_complete": true,
		"approval_level":                h.Level,
		"approval_status":               "approved",
		"approval_time":                 time.Now().Format(time.RFC3339),
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
	coordinatorResult := Dict{
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

func (h *TravelDataHandler) ProcessData(want *CoordinatorWant, channelIndex int, data any) bool {
	// TravelDataHandler only processes *TravelSchedule
	// Type-checking and routing to other handlers is handled by the dispatcher
	schedule, ok := data.(*TravelSchedule)
	if !ok {
		want.StoreLog(fmt.Sprintf("[TravelDataHandler] Expected *TravelSchedule, got %T. Dispatcher should have routed to appropriate handler.", data))
		return false
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
	dataByChannel, ok := dataByChannelVal.(map[int]any)
	if !ok {
		if dataByChannelVal == nil {
			dataByChannel = make(map[int]any)
		}
	}
	dataByChannel[channelIndex] = schedule

	// Track total packets received
	totalPacketsVal, _ := want.GetStateInt("total_packets_received", 0)
	totalPackets := totalPacketsVal + 1

	want.StoreStateMulti(Dict{
		"data_by_channel":        dataByChannel,
		"total_packets_received": totalPackets,
	})

	// Only set last_packet_time if it hasn't been set yet
	if lastPacketTimeVal, exists := want.GetState("last_packet_time"); !exists || lastPacketTimeVal == nil {
		want.StoreState("last_packet_time", time.Now().Unix())
	}

	return true
}

func (h *TravelDataHandler) GetStateUpdates(want *CoordinatorWant) map[string]any {
	// For travel coordinator, generate final itinerary from all channels
	dataByChannelVal, _ := want.GetState("data_by_channel")

	stateUpdates := make(map[string]any)
	allEvents := make([]TimeSlot, 0)
	switch v := dataByChannelVal.(type) {
	case map[int]any:
		for _, data := range v {
			if schedule, ok := data.(*TravelSchedule); ok {
				allEvents = append(allEvents, schedule.Events...)
			} else {
				want.StoreLog(fmt.Sprintf("[ERROR] TravelDataHandler.GetStateUpdates: type assertion failed for channel data. Expected *TravelSchedule, got %T", data))
			}
		}
	case map[string]any:
		for _, data := range v {
			if schedule, ok := data.(*TravelSchedule); ok {
				allEvents = append(allEvents, schedule.Events...)
			} else {
				want.StoreLog(fmt.Sprintf("[ERROR] TravelDataHandler.GetStateUpdates: type assertion failed for channel data. Expected *TravelSchedule, got %T", data))
			}
		}
	default:
		if dataByChannelVal != nil {
			want.StoreLog(fmt.Sprintf("[ERROR] TravelDataHandler.GetStateUpdates: type assertion failed for data_by_channel. Expected map[int]any or map[string]any, got %T", dataByChannelVal))
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
		stateUpdates["final_result"] = timeline
	}

	// Build travel inputs list
	travelInputs := []string{"restaurant", "hotel", "buffet"}
	coordinatorResult := Dict{
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
