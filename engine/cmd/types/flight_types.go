package types

import (
	"context"
	"fmt"
	. "mywant/engine/src"
	"time"
)

// FlightWantLocals holds type-specific local state for FlightWant
type FlightWantLocals struct {
	FlightType         string
	Duration           time.Duration
	DepartureDate      string // Departure date in YYYY-MM-DD format
	monitoringStartTime time.Time
	monitoringDuration time.Duration // How long to monitor for status changes
	monitoringActive   bool          // Whether monitoring is currently active
	lastLogTime        time.Time     // Track last monitoring log time to reduce spam
}

// FlightWant creates flight booking reservations
type FlightWant struct {
	Want
}

// NewFlightWant creates a new flight booking want
func NewFlightWant(metadata Metadata, spec WantSpec) Progressable {
	return &FlightWant{*NewWantWithLocals(
		metadata,
		spec,
		&FlightWantLocals{},
		"flight",
	)}
}

// IsAchieved checks if flight booking is complete (all phases finished)
func (f *FlightWant) IsAchieved() bool {
	phaseVal, _ := f.GetState("flight_phase")
	phase, _ := phaseVal.(string)
	return phase == PhaseCompleted
}

// extractFlightSchedule converts agent_result from state to FlightSchedule
func (f *FlightWant) extractFlightSchedule(result interface{}) *FlightSchedule {
	var schedule FlightSchedule
	switch v := result.(type) {
	case FlightSchedule:
		return &v
	case *FlightSchedule:
		return v
	case map[string]interface{}:
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

// Progress creates a flight booking reservation using state machine pattern The execution flow follows distinct phases: 1. Initial: Setup phase 2. Booking: Execute initial flight booking via agents
// 3. Monitoring: Monitor flight status for 60 seconds 4. Canceling: Wait for cancellation agent to complete 5. Rebooking: Execute rebooking after cancellation 6. Completed: Final state
func (f *FlightWant) Progress() {
	locals, ok := f.Locals.(*FlightWantLocals)
	if !ok {
		f.StoreLog("ERROR: Failed to access FlightWantLocals from Want.Locals")
		return
	}

	out, connectionAvailable := f.GetFirstOutputChannel()
	if !connectionAvailable {
		f.StoreLog("[DEBUG-EXEC] NO OUTPUT CHANNELS AVAILABLE - Returning complete")
		return
	}

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
		return

	// === Phase 2: Initial Booking ===
	case PhaseBooking:
		// Initialize monitoring duration on first booking transition
		if locals.monitoringDuration == 0 {
			completionTimeoutSeconds := f.GetIntParam("completion_timeout", 60)
			locals.monitoringDuration = time.Duration(completionTimeoutSeconds) * time.Second
			f.StoreLog(fmt.Sprintf("Monitoring duration initialized: %v seconds", completionTimeoutSeconds))
		}

		f.StoreLog("Executing initial booking")
		f.StoreState("attempted", true)
		f.tryAgentExecution()

		agentResult, hasResult := f.GetState("agent_result")
		if hasResult && agentResult != nil {
			f.StoreLog("Initial booking succeeded")
			agentSchedule := f.extractFlightSchedule(agentResult)
			if agentSchedule != nil {
				f.SetSchedule(*agentSchedule)
				f.sendFlightPacket(out, agentSchedule, "Initial")

				// Transition to monitoring phase
				locals.monitoringStartTime = time.Now()
				f.StoreState("flight_phase", PhaseMonitoring)
				f.StoreLog("Transitioning to monitoring phase")

				return
			}
		}

		// Booking failed - complete
		f.StoreLog("Initial booking failed")
		f.StoreState("flight_phase", PhaseCompleted)
		return

	// === Phase 3: Monitoring ===
	case PhaseMonitoring:
		if time.Since(locals.monitoringStartTime) < locals.monitoringDuration {
			elapsed := time.Since(locals.monitoringStartTime)
			if f.shouldCancelAndRebook() {
				f.StoreLog(fmt.Sprintf("Delay detected at %v, initiating cancellation", elapsed))
				f.StoreStateMulti(map[string]interface{}{
					"flight_action": "cancel_flight",
					"attempted":     false,
					"flight_phase":  PhaseCanceling,
				})
				return
			}

			// Log monitoring progress every 30 seconds
			now := time.Now()
			if locals.lastLogTime.IsZero() || now.Sub(locals.lastLogTime) >= 30*time.Second {
				f.StoreLog(fmt.Sprintf("Monitoring... (elapsed: %v/%v)", elapsed, locals.monitoringDuration))
				locals.lastLogTime = now
			}

			return

		} else {
			// Monitoring period expired - flight stable, complete
			f.StoreLog("Monitoring completed successfully")
			f.StoreState("flight_phase", PhaseCompleted)
			return
		}

	// === Phase 4: Canceling ===
	case PhaseCanceling:
		flightIDVal, flightIDExists := f.GetState("flight_id")
		if !flightIDExists || flightIDVal == "" {
			f.StoreStateMulti(map[string]interface{}{
							"flight_phase": PhaseRebooking,
							"attempted":    false,
						})
			return
		}

		flightID, ok := flightIDVal.(string)
		if !ok {
			f.StoreLog("Invalid flight_id type, transitioning to rebooking")
			f.StoreStateMulti(map[string]interface{}{
				"flight_phase": PhaseRebooking,
				"attempted":    false,
			})
			return
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
		return

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
				f.StoreLog("[REBOOK-DEBUG] About to send rebooked packet")
				f.sendFlightPacket(out, agentSchedule, "Rebooked")

				// Restart monitoring for new flight
				locals.monitoringStartTime = time.Now()
				f.StoreState("flight_phase", PhaseMonitoring)
				f.StoreLog("Transitioning back to monitoring phase for rebooked flight")

				return
			}
		}

		// Rebooking failed - complete
		f.StoreLog("Rebooking failed")
		f.StoreState("flight_phase", PhaseCompleted)
		return

	// === Phase 6: Completed ===
	case PhaseCompleted:
		// Clear agent_result to prevent reuse in next execution cycle
		f.StoreState("agent_result", nil)
		return

	default:
		f.StoreLog("Unknown phase: " + phase)
		f.StoreState("flight_phase", PhaseCompleted)
		return
	}
}
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
	f.SendPacketMulti(travelSchedule)

	f.StoreLog(fmt.Sprintf("[PACKET-SEND] %s flight: %s (%s to %s) | TravelSchedule: Date=%s, Events=%d",
		label,
		schedule.ReservationName,
		schedule.DepartureTime.Format("15:04 Jan 2"),
		schedule.ArrivalTime.Format("15:04 Jan 2"),
		travelSchedule.Date.Format("2006-01-02"),
		len(travelSchedule.Events)))
}

// tryAgentExecution attempts to execute flight booking using the agent system Returns the FlightSchedule if successful, nil if no agent execution
func (f *FlightWant) tryAgentExecution() {
	if len(f.Spec.Requires) > 0 {
		f.StoreState("agent_requirements", f.Spec.Requires)

		// Execute agents via ExecuteAgents() which properly tracks agent history
		if err := f.ExecuteAgents(); err != nil {
			f.StoreStateMulti(map[string]interface{}{
				"agent_execution_status": "failed",
				"agent_execution_error":  err.Error(),
			})
			return
		}

		f.StoreState("agent_execution_status", "completed")
		if result, exists := f.GetState("agent_result"); exists && result != nil {
			f.StoreState("execution_source", "agent")

			// Start continuous monitoring for this flight
			f.StartContinuousMonitoring()

			return
		}

		return
	}

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

// shouldCancelAndRebook checks if the current flight should be cancelled due to delay
func (f *FlightWant) shouldCancelAndRebook() bool {
	flightIDVal, exists := f.GetState("flight_id")
	if !exists || flightIDVal == "" {
		return false
	}
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
func (f *FlightWant) GetStateValue(key string) interface{} {
	val, _ := f.GetState(key)
	return val
}

// StartContinuousMonitoring starts a background goroutine to continuously poll flight status This is called after the flight is successfully booked via agents
func (f *FlightWant) StartContinuousMonitoring() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			flightIDVal, exists := f.GetState("flight_id")
			if !exists || flightIDVal == "" {
				return
			}

			flightID, ok := flightIDVal.(string)
			if !ok || flightID == "" {
				return
			}
			params := f.Spec.Params
			serverURL, ok := params["server_url"].(string)
			if !ok || serverURL == "" {
				serverURL = "http://localhost:8081"
			}
			monitor := NewMonitorFlightAPI("flight-monitor-"+flightID, []string{}, []string{}, serverURL)

			// AGGREGATION: Wrap monitor.Progress() in exec cycle to batch all StoreState calls This prevents lock contention when multiple monitoring goroutines call StoreState
			f.BeginExecCycle()
			monitor.Progress(context.Background(), &f.Want)
			f.EndExecCycle()
		}
	}()
}