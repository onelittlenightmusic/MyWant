package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/src"
	"mywant/engine/src/chain"
	"time"
)

// FlightWant creates flight booking reservations
type FlightWant struct {
	Want
	FlightType          string
	Duration            time.Duration
	DepartureDate       string        // Departure date in YYYY-MM-DD format
	paths               Paths
	monitoringStartTime time.Time
	monitoringDuration  time.Duration // How long to monitor for status changes
	monitoringActive    bool          // Whether monitoring is currently active
	lastLogTime         time.Time     // Track last monitoring log time to reduce spam
}

// NewFlightWant creates a new flight booking want
func NewFlightWant(metadata Metadata, spec WantSpec) *FlightWant {
	flight := &FlightWant{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		FlightType:         "economy",
		Duration:           12 * time.Hour, // Default 12 hour flight
		DepartureDate:      "2024-01-01",   // Default departure date
		monitoringActive:   false,
		monitoringDuration: 60 * time.Second, // Monitor for 60 seconds after flight creation
	}

	if ft, ok := spec.Params["flight_type"]; ok {
		if fts, ok := ft.(string); ok {
			flight.FlightType = fts
		}
	}
	if d, ok := spec.Params["duration_hours"]; ok {
		if df, ok := d.(float64); ok {
			flight.Duration = time.Duration(df * float64(time.Hour))
		}
	}
	if dd, ok := spec.Params["departure_date"]; ok {
		if dds, ok := dd.(string); ok {
			flight.DepartureDate = dds
		}
	}

	return flight
}

// GetConnectivityMetadata returns connectivity requirements
func (f *FlightWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		WantType:        "flight",
		Description:     "Flight booking scheduling want",
	}
}

func (f *FlightWant) InitializePaths(inCount, outCount int) {
	f.paths.In = make([]PathInfo, inCount)
	f.paths.Out = make([]PathInfo, outCount)
}

func (f *FlightWant) GetStats() map[string]interface{} {
	return f.State
}

func (f *FlightWant) Process(paths Paths) bool {
	f.paths = paths
	return false
}

func (f *FlightWant) GetType() string {
	return "flight"
}

func (f *FlightWant) GetWant() *Want {
	return &f.Want
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
		fmt.Printf("[FLIGHT] agent_result is unexpected type: %T\n", result)
		return nil
	}
}

// Exec creates a flight booking reservation
func (f *FlightWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Handle continuous monitoring phase
	if f.monitoringActive {
		// Continue running monitoring during the monitoring duration
		if time.Since(f.monitoringStartTime) < f.monitoringDuration {
			// Still within monitoring window - check for delays
			// Only log every 30 seconds to reduce spam
			elapsed := time.Since(f.monitoringStartTime)
			now := time.Now()
			if f.lastLogTime.IsZero() || now.Sub(f.lastLogTime) >= 30*time.Second {
				fmt.Printf("[FLIGHT] Monitoring cycle (elapsed: %v/%v)\n",
					elapsed, f.monitoringDuration)
				f.lastLogTime = now
			}

			// Check for delayed flights that need cancellation and rebooking
			// This is checked during monitoring phase so rebooking can happen immediately
			if f.shouldCancelAndRebook() {
				fmt.Printf("[FLIGHT] Flight status is delayed during monitoring, initiating cancellation and rebooking\n")

				// Set flight_action to cancel_flight so the agent executor will handle it
				// Note: Keep flight_id so agent can cancel it
				f.StoreState("flight_action", "cancel_flight")

				// Exit monitoring phase to trigger rebooking immediately
				f.monitoringActive = false

				// Reset attempted flag so agent can execute the cancellation action
				f.StoreState("attempted", false)

				fmt.Printf("[FLIGHT] Set flight_action to cancel_flight during monitoring, waiting for agent cancellation\n")

				// Return false to trigger the rebooking flow in next cycle
				return false
			}

			// The monitoring agent will be triggered through the normal agent execution framework
			// during the reconciliation loop. We just need to stay in the monitoring phase
			// by returning false to keep the want running

			// Return false to keep running through reconciliation cycles
			return false
		} else {
			// Monitoring duration exceeded, complete the monitoring phase
			fmt.Printf("[FLIGHT] Monitoring completed (total duration: %v)\n", time.Since(f.monitoringStartTime))
			f.monitoringActive = false
			return true
		}
	}

	// Read parameters fresh each cycle - enables dynamic changes!
	flightType := "economy"
	if ft, ok := f.Spec.Params["flight_type"]; ok {
		if fts, ok := ft.(string); ok {
			flightType = fts
		}
	}

	duration := 12 * time.Hour // Default 12 hour flight
	if d, ok := f.Spec.Params["duration_hours"]; ok {
		if df, ok := d.(float64); ok {
			duration = time.Duration(df * float64(time.Hour))
		}
	}

	if len(outputs) == 0 {
		return true
	}
	out := outputs[0]

	// Check if already attempted using persistent state
	attemptedVal, _ := f.GetState("attempted")
	attempted, _ := attemptedVal.(bool)

	if attempted {
		// Already booked in this cycle
		return true
	}

	// Mark as attempted in persistent state
	f.StoreState("attempted", true)

	// Try to use agent system if available - agent completely overrides normal execution
	f.tryAgentExecution()

	// Check if agent created a flight result (read from state, not return value)
	agentResult, hasResult := f.GetState("agent_result")
	if hasResult && agentResult != nil {
		fmt.Printf("[FLIGHT] Agent execution completed, processing agent result\n")

		// Convert agent_result to FlightSchedule
		agentSchedule := f.extractFlightSchedule(agentResult)
		if agentSchedule != nil {
			// Use the agent's schedule result
			f.SetSchedule(*agentSchedule)

			// Send the schedule to output channel
			flightEvent := TimeSlot{
				Start: agentSchedule.DepartureTime,
				End:   agentSchedule.ArrivalTime,
				Type:  "flight",
				Name:  agentSchedule.ReservationName,
			}

			travelSchedule := &TravelSchedule{
				Date:   agentSchedule.DepartureTime.Truncate(24 * time.Hour),
				Events: []TimeSlot{flightEvent},
			}

			out <- travelSchedule
			fmt.Printf("[FLIGHT] Sent agent-generated schedule: %s from %s to %s\n",
				agentSchedule.ReservationName,
				agentSchedule.DepartureTime.Format("15:04 Jan 2"),
				agentSchedule.ArrivalTime.Format("15:04 Jan 2"))

			// Start continuous monitoring to capture all status changes
			if !f.monitoringActive {
				f.monitoringActive = true
				f.monitoringStartTime = time.Now()
				fmt.Printf("[FLIGHT] Starting continuous monitoring for status changes (duration: %v)\n", f.monitoringDuration)
			}

			// Continue running to collect more status updates
			// Return false to keep this want running through reconciliation cycles
			return false
		}
	}

	// Check if cancellation just completed (previous_flight_id exists)
	prevFlightID, hasPrevFlight := f.GetState("previous_flight_id")
	if hasPrevFlight && prevFlightID != nil && prevFlightID != "" {
		// Flight was just cancelled, prepare for rebooking
		fmt.Printf("[FLIGHT] Flight cancellation completed, preparing for rebooking\n")

		// Reset attempted flag to allow agent to execute rebooking in this cycle
		// This is critical - without resetting, the "attempted" check above will return true
		f.StoreState("attempted", false)

		// Don't return here - fall through to agent execution for rebooking
		// The agent will see flight_id is empty and attempt rebooking

		// Try rebooking immediately in this same cycle
		f.tryAgentExecution()

		// Check if rebooking created a new flight result (read from state, not return value)
		agentResult, hasResult := f.GetState("agent_result")
		if hasResult && agentResult != nil {
			fmt.Printf("[FLIGHT] Rebooking agent execution completed, processing new flight result\n")

			// Convert agent_result to FlightSchedule
			agentSchedule := f.extractFlightSchedule(agentResult)
			if agentSchedule != nil {
				// Use the agent's schedule result
				f.SetSchedule(*agentSchedule)

				// Send the schedule to output channel
				flightEvent := TimeSlot{
					Start: agentSchedule.DepartureTime,
					End:   agentSchedule.ArrivalTime,
					Type:  "flight",
					Name:  agentSchedule.ReservationName,
				}

				travelSchedule := &TravelSchedule{
					Date:   agentSchedule.DepartureTime.Truncate(24 * time.Hour),
					Events: []TimeSlot{flightEvent},
				}

				out <- travelSchedule
				fmt.Printf("[FLIGHT] Sent rebooked flight schedule: %s from %s to %s\n",
					agentSchedule.ReservationName,
					agentSchedule.DepartureTime.Format("15:04 Jan 2"),
					agentSchedule.ArrivalTime.Format("15:04 Jan 2"))

				// Start continuous monitoring for new flight
				if !f.monitoringActive {
					f.monitoringActive = true
					f.monitoringStartTime = time.Now()
					fmt.Printf("[FLIGHT] Starting continuous monitoring for new booked flight (duration: %v)\n", f.monitoringDuration)
				}

				// Continue monitoring the new flight
				return false
			}
		}
	}

	// Normal flight execution (only runs if agent execution didn't return a result)
	fmt.Printf("[FLIGHT] Agent execution did not return result, using standard flight logic\n")

	// Check for conflicts from input
	var existingSchedule *TravelSchedule
	if len(using) > 0 {
		select {
		case schedData := <-using[0]:
			if schedule, ok := schedData.(*TravelSchedule); ok {
				existingSchedule = schedule
			}
		default:
			// No input data available
		}
	}

	// Generate flight departure time (early morning flights common)
	baseDate := time.Now().AddDate(0, 0, 1) // Tomorrow
	departureTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		6+rand.Intn(6), rand.Intn(60), 0, 0, time.Local) // 6 AM - 12 PM

	newEvent := TimeSlot{
		Start: departureTime,
		End:   departureTime.Add(duration),
		Type:  "flight",
		Name:  fmt.Sprintf("%s %s flight booking", f.Metadata.Name, flightType),
	}

	// Check for conflicts if we have existing schedule
	if existingSchedule != nil {
		for attempt := 0; attempt < 3; attempt++ {
			conflict := false
			for _, event := range existingSchedule.Events {
				if f.hasTimeConflict(newEvent, event) {
					conflict = true
					// Retry with different time
					departureTime = departureTime.Add(2 * time.Hour)
					newEvent.Start = departureTime
					newEvent.End = departureTime.Add(duration)
					fmt.Printf("[FLIGHT] Conflict detected, retrying at %s\n", departureTime.Format("15:04"))
					break
				}
			}
			if !conflict {
				break
			}
		}
	}

	// Create updated schedule
	newSchedule := &TravelSchedule{
		Date:   baseDate,
		Events: []TimeSlot{newEvent},
	}
	if existingSchedule != nil {
		newSchedule.Events = append(existingSchedule.Events, newEvent)
	}

	// Store flight details using thread-safe StoreState (batched to minimize history entries)
	f.StoreState("total_processed", 1)
	f.StoreState("flight_type", flightType)
	f.StoreState("departure_time", newEvent.Start.Format("15:04 Jan 2"))
	f.StoreState("arrival_time", newEvent.End.Format("15:04 Jan 2"))
	f.StoreState("flight_duration_hours", duration.Hours())
	f.StoreState("reservation_name", newEvent.Name)
	f.StoreState("schedule_date", baseDate.Format("2006-01-02"))

	fmt.Printf("[FLIGHT] Scheduled %s from %s to %s\n",
		newEvent.Name, newEvent.Start.Format("15:04 Jan 2"), newEvent.End.Format("15:04 Jan 2"))

	out <- newSchedule
	return true
}

// tryAgentExecution attempts to execute flight booking using the agent system
// Returns the FlightSchedule if successful, nil if no agent execution
func (f *FlightWant) tryAgentExecution() {
	// Check if this want has agent requirements
	if len(f.Spec.Requires) > 0 {
		fmt.Printf("[FLIGHT] Want has agent requirements: %v\n", f.Spec.Requires)

		// Store the requirements in want state for tracking
		f.StoreState("agent_requirements", f.Spec.Requires)

		// Execute agents via ExecuteAgents() which properly tracks agent history
		fmt.Printf("[FLIGHT] Executing agents via ExecuteAgents() for proper tracking\n")
		if err := f.ExecuteAgents(); err != nil {
			fmt.Printf("[FLIGHT] Dynamic agent execution failed: %v\n", err)
			f.StoreState("agent_execution_status", "failed")
			f.StoreState("agent_execution_error", err.Error())
			return
		}

		fmt.Printf("[FLIGHT] Dynamic agent execution completed successfully\n")
		f.StoreState("agent_execution_status", "completed")

		// Check for agent_result in state
		if result, exists := f.GetState("agent_result"); exists && result != nil {
			fmt.Printf("[FLIGHT] Found agent_result in state: %+v\n", result)
			f.StoreState("execution_source", "agent")

			// Start continuous monitoring for this flight
			f.StartContinuousMonitoring()

			return
		}

		fmt.Printf("[FLIGHT] Warning: Agent completed but no result found in state\n")
		return
	}

	fmt.Printf("[FLIGHT] No agent requirements specified\n")
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
	// Store basic flight booking information
	f.Want.StoreState("attempted", true)
	f.Want.StoreState("departure_time", schedule.DepartureTime.Format("15:04 Jan 2"))
	f.Want.StoreState("arrival_time", schedule.ArrivalTime.Format("15:04 Jan 2"))
	f.Want.StoreState("flight_type", schedule.FlightType)
	f.Want.StoreState("flight_duration_hours", schedule.ArrivalTime.Sub(schedule.DepartureTime).Hours())
	f.Want.StoreState("flight_number", schedule.FlightNumber)
	f.Want.StoreState("reservation_name", schedule.ReservationName)
	f.Want.StoreState("total_processed", 1)
	f.Want.StoreState("schedule_date", schedule.DepartureTime.Format("2006-01-02"))

	// Store premium information if provided
	if schedule.PremiumLevel != "" {
		f.Want.StoreState("premium_processed", true)
		f.Want.StoreState("premium_level", schedule.PremiumLevel)
	}
	if schedule.ServiceTier != "" {
		f.Want.StoreState("service_tier", schedule.ServiceTier)
	}
	if len(schedule.PremiumAmenities) > 0 {
		f.Want.StoreState("premium_amenities", schedule.PremiumAmenities)
	}
}

// Helper function to check time conflicts
func (f *FlightWant) hasTimeConflict(event1, event2 TimeSlot) bool {
	return event1.Start.Before(event2.End) && event2.Start.Before(event1.End)
}

// shouldCancelAndRebook checks if the current flight should be cancelled due to delay
func (f *FlightWant) shouldCancelAndRebook() bool {
	// Check if flight has been created
	flightID, exists := f.GetState("flight_id")
	if !exists || flightID == "" {
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
		fmt.Printf("[FLIGHT] Detected delayed_one_day status, will cancel and rebook\n")
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
			if !exists {
				fmt.Printf("[FLIGHT-MONITOR] No flight_id found, stopping monitoring\n")
				return
			}

			flightID, ok := flightIDVal.(string)
			if !ok || flightID == "" {
				fmt.Printf("[FLIGHT-MONITOR] Invalid flight_id, stopping monitoring\n")
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
			err := monitor.Exec(context.Background(), &f.Want)
			f.EndExecCycle()

			if err != nil {
				fmt.Printf("[FLIGHT-MONITOR] Polling error: %v\n", err)
			} else {
				// Log the current status
				if status, exists := f.GetState("flight_status"); exists {
					fmt.Printf("[FLIGHT-MONITOR] Flight %s status: %v (polled at %s)\n",
						flightID, status, time.Now().Format("15:04:05"))
				}
			}
		}
	}()

	fmt.Printf("[FLIGHT] Started continuous monitoring for flight %s\n", f.Metadata.Name)
}
