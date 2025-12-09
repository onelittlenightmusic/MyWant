package types

import (
	"context"
	"fmt"
	. "mywant/engine/src"
	"time"
)

// FlightWant creates flight booking reservations
type FlightWant struct {
	Want
	FlightType          string
	Duration            time.Duration
	DepartureDate       string // Departure date in YYYY-MM-DD format
	paths               Paths
	monitoringStartTime time.Time
	monitoringDuration  time.Duration // How long to monitor for status changes
	monitoringActive    bool          // Whether monitoring is currently active
	lastLogTime         time.Time     // Track last monitoring log time to reduce spam
}

// NewFlightWant creates a new flight booking want
func NewFlightWant(metadata Metadata, spec WantSpec) interface{} {
	flight := &FlightWant{
		Want:               Want{},
		FlightType:         "economy",
		Duration:           12 * time.Hour, // Default 12 hour flight
		DepartureDate:      "2024-01-01",   // Default departure date
		monitoringActive:   false,
		// monitoringDuration: 60-second window to monitor flight status for stability
		// After initial booking or rebooking, the system monitors the flight schedule for 60 seconds
		// to ensure it has stabilized before marking completion. This allows detection of immediate
		// status changes (delays, cancellations) that would trigger rebooking. Once the 60-second
		// window expires, if no issues are detected, the FlightWant completes and notifies the
		// parent Target want, allowing the entire travel plan to complete.
		monitoringDuration: 30 * time.Second,
	}

	// Initialize base Want fields
	flight.Init(metadata, spec)

	flight.FlightType = flight.GetStringParam("flight_type", "economy")
	flight.Duration = time.Duration(flight.GetFloatParam("duration_hours", 12.0) * float64(time.Hour))
	flight.DepartureDate = flight.GetStringParam("departure_date", "2024-01-01")

	// Set fields for base Want methods
	flight.WantType = "flight"
	flight.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		WantType:        "flight",
		Description:     "Flight booking scheduling want",
	}

	return flight
}

// extractFlightSchedule converts agent_result from state to FlightSchedule
func (f *FlightWant) extractFlightSchedule(result interface{}) *FlightSchedule {
	// Handle both map[string]interface{} and FlightSchedule types
	var schedule FlightSchedule
	switch v := result.(type) {
	case FlightSchedule:
		return &v
	case *FlightSchedule:
		return v
	case map[string]interface{}:
		// Convert map to FlightSchedule
		if dt, ok := v["departure_time"].(time.Time); ok {
			schedule.DepartureTime = dt
		}
		if at, ok := v["arrival_time"].(time.Time); ok {
			schedule.ArrivalTime = at
		}
		if ft, ok := v["flight_type"].(string); ok {
			schedule.FlightType = ft
		}
		if fn, ok := v["flight_number"].(string); ok {
			schedule.FlightNumber = fn
		}
		if rn, ok := v["reservation_name"].(string); ok {
			schedule.ReservationName = rn
		}
		if pl, ok := v["premium_level"].(string); ok {
			schedule.PremiumLevel = pl
		}
		if st, ok := v["service_tier"].(string); ok {
			schedule.ServiceTier = st
		}
		if amenities, ok := v["premium_amenities"].([]string); ok {
			schedule.PremiumAmenities = amenities
		}
		return &schedule
	default:
		f.StoreLog(fmt.Sprintf("agent_result is unexpected type: %T", result))
		return nil
	}
}

// Flight execution phases (state machine)
const (
	PhaseInitial    = "initial"
	PhaseBooking    = "booking"
	PhaseMonitoring = "monitoring"
	PhaseCanceling  = "canceling"
	PhaseRebooking  = "rebooking"
	PhaseCompleted  = "completed"
)

// Exec creates a flight booking reservation using state machine pattern
// The execution flow follows distinct phases:
// 1. Initial: Setup phase
// 2. Booking: Execute initial flight booking via agents
// 3. Monitoring: Monitor flight status for 60 seconds
// 4. Canceling: Wait for cancellation agent to complete
// 5. Rebooking: Execute rebooking after cancellation
// 6. Completed: Final state, return true to complete want
func (f *FlightWant) Exec() bool {
	out, connectionAvailable := f.GetFirstOutputChannel()
	// f.StoreLog(fmt.Sprintf("[DEBUG-EXEC] GetFirstOutputChannel returned: available=%v, out=%v", connectionAvailable, out != nil))
	if !connectionAvailable {
		f.StoreLog("[DEBUG-EXEC] NO OUTPUT CHANNELS AVAILABLE - Returning complete (true)")
		return true
	}
	// f.StoreLog("[DEBUG-EXEC] Output channels available - Proceeding with execution")

	// Get current phase from state
	phaseVal, _ := f.GetState("flight_phase")
	phase := ""
	if phaseVal != nil {
		phase, _ = phaseVal.(string)
	}
	if phase == "" {
		phase = PhaseInitial
	}

	// State machine: handle each phase
	switch phase {

	// === Phase 1: Initial Setup ===
	case PhaseInitial:
		f.StoreLog("Phase: Initial booking")
		f.StoreState("flight_phase", PhaseBooking)
		return false

	// === Phase 2: Initial Booking ===
	case PhaseBooking:
		f.StoreLog("Executing initial booking")
		f.StoreState("attempted", true)
		f.tryAgentExecution()

		agentResult, hasResult := f.GetState("agent_result")
		if hasResult && agentResult != nil {
			f.StoreLog("Initial booking succeeded")
			agentSchedule := f.extractFlightSchedule(agentResult)
			if agentSchedule != nil {
				f.SetSchedule(*agentSchedule)

				// Send initial flight packet
				f.sendFlightPacket(out, agentSchedule, "Initial")

				// Transition to monitoring phase
				f.monitoringStartTime = time.Now()
				f.StoreState("flight_phase", PhaseMonitoring)
				f.StoreLog("Transitioning to monitoring phase")

				return false
			}
		}

		// Booking failed - complete
		f.StoreLog("Initial booking failed")
		f.StoreState("flight_phase", PhaseCompleted)
		return true

	// === Phase 3: Monitoring ===
	case PhaseMonitoring:
		if time.Since(f.monitoringStartTime) < f.monitoringDuration {
			elapsed := time.Since(f.monitoringStartTime)

			// Check for delay that triggers cancellation
			if f.shouldCancelAndRebook() {
				f.StoreLog(fmt.Sprintf("Delay detected at %v, initiating cancellation", elapsed))
				f.StoreStateMulti(map[string]interface{}{
					"flight_action": "cancel_flight",
					"attempted":     false,
					"flight_phase":  PhaseCanceling,
				})
				return false
			}

			// Log monitoring progress every 30 seconds
			now := time.Now()
			if f.lastLogTime.IsZero() || now.Sub(f.lastLogTime) >= 30*time.Second {
				f.StoreLog(fmt.Sprintf("Monitoring... (elapsed: %v/%v)", elapsed, f.monitoringDuration))
				f.lastLogTime = now
			}

			return false

		} else {
			// Monitoring period expired - flight stable, complete
			f.StoreLog("Monitoring completed successfully")
			f.StoreState("flight_phase", PhaseCompleted)
			return true
		}

	// === Phase 4: Canceling ===
	case PhaseCanceling:
		// Get the flight_id to cancel
		flightIDVal, flightIDExists := f.GetState("flight_id")
		if !flightIDExists || flightIDVal == "" {
			f.StoreStateMulti(map[string]interface{}{
							"flight_phase": PhaseRebooking,
							"attempted":    false,
						})
			return false
		}

		flightID, ok := flightIDVal.(string)
		if !ok {
			f.StoreLog("Invalid flight_id type, transitioning to rebooking")
			f.StoreStateMulti(map[string]interface{}{
				"flight_phase": PhaseRebooking,
				"attempted":    false,
			})
			return false
		}

		// Execute cancel flight action
		f.StoreLog(fmt.Sprintf("Executing cancel_flight action for flight %s", flightID))
		f.tryAgentExecution()

		// Clear flight_id after cancellation to indicate rebooking is next
		f.StoreState("flight_id", "")
		f.StoreLog("Cancelled flight: " + flightID)

		// Transition to rebooking phase
		f.StoreLog("Cancellation completed, transitioning to rebooking phase")
		f.StoreStateMulti(map[string]interface{}{
			"flight_phase": PhaseRebooking,
			"attempted":    false,
		})
		return false

	// === Phase 5: Rebooking ===
	case PhaseRebooking:
		f.StoreLog("Executing rebooking")
		f.tryAgentExecution()

		agentResult, hasResult := f.GetState("agent_result")
		f.StoreLog(fmt.Sprintf("[REBOOK-DEBUG] hasResult=%v, agentResult=%v (type=%T)", hasResult, agentResult, agentResult))

		if hasResult && agentResult != nil {
			f.StoreLog("Rebooking succeeded")
			agentSchedule := f.extractFlightSchedule(agentResult)
			f.StoreLog(fmt.Sprintf("[REBOOK-DEBUG] Extracted schedule: %+v", agentSchedule))

			if agentSchedule != nil {
				f.SetSchedule(*agentSchedule)

				// Send rebooked flight packet
				f.StoreLog("[REBOOK-DEBUG] About to send rebooked packet")
				f.sendFlightPacket(out, agentSchedule, "Rebooked")

				// Restart monitoring for new flight
				f.monitoringStartTime = time.Now()
				f.StoreState("flight_phase", PhaseMonitoring)
				f.StoreLog("Transitioning back to monitoring phase for rebooked flight")

				return false
			}
		}

		// Rebooking failed - complete
		f.StoreLog("Rebooking failed")
		f.StoreState("flight_phase", PhaseCompleted)
		return true

	// === Phase 6: Completed ===
	case PhaseCompleted:
		// Clear agent_result to prevent reuse in next execution cycle
		f.StoreState("agent_result", nil)
		return true

	default:
		f.StoreLog("Unknown phase: " + phase)
		f.StoreState("flight_phase", PhaseCompleted)
		return true
	}
}

// sendFlightPacket sends a flight schedule packet to the output channel and logs it
// Uses SendPacketMulti to send with retrigger logic for achieved receivers
func (f *FlightWant) sendFlightPacket(out interface{}, schedule *FlightSchedule, label string) {
	flightEvent := TimeSlot{
		Start: schedule.DepartureTime,
		End:   schedule.ArrivalTime,
		Type:  "flight",
		Name:  schedule.ReservationName,
	}

	travelSchedule := &TravelSchedule{
		Date:   schedule.DepartureTime.Truncate(24 * time.Hour),
		Events: []TimeSlot{flightEvent},
	}

	// Send via SendPacketMulti which uses paths from want setup
	// Paths.Out contains the channels configured during want initialization
	f.SendPacketMulti(travelSchedule)

	f.StoreLog(fmt.Sprintf("Sent %s flight schedule: %s from %s to %s",
		label,
		schedule.ReservationName,
		schedule.DepartureTime.Format("15:04 Jan 2"),
		schedule.ArrivalTime.Format("15:04 Jan 2")))

	f.StoreLog(fmt.Sprintf("[PACKET-SEND] Flight sent %s TravelSchedule: Date=%s, Events=%d (name=%s, start=%s, end=%s)",
		label,
		travelSchedule.Date.Format("2006-01-02"),
		len(travelSchedule.Events),
		flightEvent.Name,
		flightEvent.Start.Format("15:04:05"),
		flightEvent.End.Format("15:04:05")))
}

// tryAgentExecution attempts to execute flight booking using the agent system
// Returns the FlightSchedule if successful, nil if no agent execution
func (f *FlightWant) tryAgentExecution() {
	// Check if this want has agent requirements
	if len(f.Spec.Requires) > 0 {
		f.StoreLog(fmt.Sprintf("Want has agent requirements: %v", f.Spec.Requires))

		// Store the requirements in want state for tracking
		f.StoreState("agent_requirements", f.Spec.Requires)

		// Execute agents via ExecuteAgents() which properly tracks agent history
		f.StoreLog("Executing agents via ExecuteAgents() for proper tracking")
		if err := f.ExecuteAgents(); err != nil {
			f.StoreLog(fmt.Sprintf("Dynamic agent execution failed: %v", err))
			f.StoreStateMulti(map[string]interface{}{
				"agent_execution_status": "failed",
				"agent_execution_error":  err.Error(),
			})
			return
		}

		f.StoreLog("Dynamic agent execution completed successfully")
		f.StoreState("agent_execution_status", "completed")

		// Check for agent_result in state
		if result, exists := f.GetState("agent_result"); exists && result != nil {
			f.StoreLog(fmt.Sprintf("Found agent_result in state: %+v", result))
			f.StoreState("execution_source", "agent")

			// Start continuous monitoring for this flight
			f.StartContinuousMonitoring()

			return
		}

		f.StoreLog("Warning: Agent completed but no result found in state")
		return
	}

	f.StoreLog("No agent requirements specified")
}

// FlightSchedule represents a complete flight booking schedule
type FlightSchedule struct {
	DepartureTime    time.Time `json:"departure_time"`
	ArrivalTime      time.Time `json:"arrival_time"`
	FlightType       string    `json:"flight_type"`
	FlightNumber     string    `json:"flight_number"`
	ReservationName  string    `json:"reservation_name"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

// SetSchedule sets the flight booking schedule and updates all related state
func (f *FlightWant) SetSchedule(schedule FlightSchedule) {
	stateUpdates := map[string]interface{}{
		"attempted":             true,
		"departure_time":        schedule.DepartureTime.Format("15:04 Jan 2"),
		"arrival_time":          schedule.ArrivalTime.Format("15:04 Jan 2"),
		"flight_type":           schedule.FlightType,
		"flight_duration_hours": schedule.ArrivalTime.Sub(schedule.DepartureTime).Hours(),
		"flight_number":         schedule.FlightNumber,
		"reservation_name":      schedule.ReservationName,
		"total_processed":       1,
		"schedule_date":         schedule.DepartureTime.Format("2006-01-02"),
	}

	// Store premium information if provided
	if schedule.PremiumLevel != "" {
		stateUpdates["premium_processed"] = true
		stateUpdates["premium_level"] = schedule.PremiumLevel
	}
	if schedule.ServiceTier != "" {
		stateUpdates["service_tier"] = schedule.ServiceTier
	}
	if len(schedule.PremiumAmenities) > 0 {
		stateUpdates["premium_amenities"] = schedule.PremiumAmenities
	}

	f.Want.StoreStateMulti(stateUpdates)
}

// Helper function to check time conflicts
func (f *FlightWant) hasTimeConflict(event1, event2 TimeSlot) bool {
	return event1.Start.Before(event2.End) && event2.Start.Before(event1.End)
}

// shouldCancelAndRebook checks if the current flight should be cancelled due to delay
func (f *FlightWant) shouldCancelAndRebook() bool {
	// Check if flight has been created
	flightIDVal, exists := f.GetState("flight_id")
	if !exists || flightIDVal == "" {
		return false
	}

	// Check current flight status
	statusVal, exists := f.GetState("flight_status")
	if !exists {
		return false
	}

	status, ok := statusVal.(string)
	if !ok {
		return false
	}

	// Cancel and rebook if delayed
	if status == "delayed_one_day" {
		return true
	}

	return false
}

// GetStateValue is a helper to safely get state value
func (f *FlightWant) GetStateValue(key string) interface{} {
	val, _ := f.GetState(key)
	return val
}

// StartContinuousMonitoring starts a background goroutine to continuously poll flight status
// This is called after the flight is successfully booked via agents
func (f *FlightWant) StartContinuousMonitoring() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// Check if flight has been booked
			flightIDVal, exists := f.GetState("flight_id")
			if !exists || flightIDVal == "" {
				return
			}

			flightID, ok := flightIDVal.(string)
			if !ok || flightID == "" {
				return
			}

			// Get server URL from params
			params := f.Spec.Params
			serverURL, ok := params["server_url"].(string)
			if !ok || serverURL == "" {
				serverURL = "http://localhost:8081"
			}

			// Create monitor agent and poll
			monitor := NewMonitorFlightAPI("flight-monitor-"+flightID, []string{}, []string{}, serverURL)

			// AGGREGATION: Wrap monitor.Exec() in exec cycle to batch all StoreState calls
			// This prevents lock contention when multiple monitoring goroutines call StoreState
			f.BeginExecCycle()
			monitor.Exec(context.Background(), &f.Want)
			f.EndExecCycle()
		}
	}()
}