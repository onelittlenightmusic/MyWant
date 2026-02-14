package types

import (
	"fmt"
	"math/rand"
	. "mywant/engine/core"
	"strings"
	"time"
)

func init() {
	RegisterWantImplementation[RestaurantWant, RestaurantWantLocals]("restaurant")
	RegisterWantImplementation[HotelWant, HotelWantLocals]("hotel")
	RegisterWantImplementation[BuffetWant, BuffetWantLocals]("buffet")
	RegisterWantImplementation[FlightWant, FlightWantLocals]("flight")
}

// RestaurantWantLocals holds type-specific local state for RestaurantWant
type RestaurantWantLocals struct {
	RestaurantType string
	Duration       time.Duration
}

// HotelWantLocals holds type-specific local state for HotelWant
type HotelWantLocals struct {
	HotelType string
	CheckIn   time.Duration
	CheckOut  time.Duration
}

// BuffetWantLocals holds type-specific local state for BuffetWant
type BuffetWantLocals struct {
	BuffetType string
	Duration   time.Duration
}

// TimeSlot represents a time period with start and end times
type TimeSlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Type  string    `json:"type"`
	Name  string    `json:"name"`
}

// TravelSchedule represents a complete travel schedule with multiple events
type TravelSchedule struct {
	Events    []TimeSlot `json:"events"`
	Date      time.Time  `json:"date"`
	Completed bool       `json:"completed"`
}

// ScheduleConflict represents a scheduling conflict that needs resolution
type ScheduleConflict struct {
	Event1   TimeSlot
	Event2   TimeSlot
	Resolved bool
	Attempts int
}

// TravelWantLocalsInterface is a marker interface for all travel want locals
type TravelWantLocalsInterface any

// TravelWantInterface defines methods that specific travel wants must implement
type TravelWantInterface interface {
	tryAgentExecution() any // Returns *RestaurantSchedule, *HotelSchedule, or *BuffetSchedule
	generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule
	SetSchedule(schedule any)
}

// BaseTravelWant provides shared functionality for all travel-related wants
// RestaurantWant, HotelWant, and BuffetWant all embed this base type
type BaseTravelWant struct {
	Want
	executor TravelWantInterface // Reference to concrete type for interface method dispatch
}

// Initialize resets state before execution
func (b *BaseTravelWant) Initialize() {
	// Travel wants don't need state reset
}

// IsAchieved checks if the travel want has been achieved
func (b *BaseTravelWant) IsAchieved() bool {
	completed, _ := b.GetStateBool("completed", false)
	return completed
}

// Progress implements Progressable for all travel wants
func (b *BaseTravelWant) Progress() {
	b.StoreState("completed", true)

	if b.executor == nil {
		b.SetModuleError("executor", "Executor not initialized - Initialize() may not have been called")
		return
	}

	// Try agent execution
	if schedule := b.executor.tryAgentExecution(); schedule != nil {
		b.executor.SetSchedule(schedule)
		return
	}

	// Generate and provide schedule
	locals, ok := b.Locals.(TravelWantLocalsInterface)
	if !ok {
		b.SetModuleErrorAndExit("Locals", "Failed to cast Locals to TravelWantLocalsInterface")
	}
	_, connectionAvailable := b.GetFirstOutputChannel()
	schedule := b.executor.generateSchedule(locals)
	if schedule != nil && connectionAvailable {
		b.Provide(schedule)
		b.ProvideDone()
	} else if schedule == nil {
		b.StoreLog("ERROR: Failed to generate schedule")
	}
}

// CalculateAchievingPercentage returns progress percentage
func (b *BaseTravelWant) CalculateAchievingPercentage() int {
	completed, _ := b.GetStateBool("completed", false)
	if completed {
		return 100
	}
	return 0
}

// RestaurantWant creates dinner restaurant reservations
type RestaurantWant struct {
	BaseTravelWant
}

func (r *RestaurantWant) GetLocals() *RestaurantWantLocals {
	return GetLocals[RestaurantWantLocals](&r.Want)
}

// Initialize prepares the restaurant want for execution
func (r *RestaurantWant) Initialize() {
	r.BaseTravelWant.executor = r
}

// tryAgentExecution implements TravelWantInterface for RestaurantWant
func (r *RestaurantWant) tryAgentExecution() any {
	if len(r.Spec.Requires) > 0 {
		if err := r.ExecuteAgents(); err != nil {
			r.StoreState("agent_execution_status", "failed")
			r.StoreState("agent_execution_error", err.Error())
			return nil
		}

		r.StoreState("agent_execution_status", "completed")
		r.StoreState("execution_source", "agent")

		if schedule, ok := GetStateAs[RestaurantSchedule](&r.Want, "agent_result"); ok {
			return &schedule
		}

		return nil
	}

	return nil
}

// generateSchedule implements TravelWantInterface for RestaurantWant
func (r *RestaurantWant) generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule {
	restaurantLocals, ok := locals.(*RestaurantWantLocals)
	if !ok {
		return nil
	}

	// Generate restaurant reservation time (evening dinner)
	baseDate := time.Now().AddDate(0, 0, 1) // Tomorrow
	dinnerStart := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		18+rand.Intn(3), rand.Intn(60), 0, 0, time.Local) // 6-9 PM

	// Generate realistic restaurant name for the summary
	restaurantName := generateRealisticRestaurantNameForTravel(restaurantLocals.RestaurantType)
	partySize := r.GetIntParam("party_size", 2)

	newEvent := TimeSlot{
		Start: dinnerStart,
		End:   dinnerStart.Add(restaurantLocals.Duration),
		Type:  "restaurant",
		Name:  fmt.Sprintf("%s - Party of %d at %s restaurant", restaurantName, partySize, restaurantLocals.RestaurantType),
	}
	newSchedule := &TravelSchedule{
		Date:      baseDate,
		Events:    []TimeSlot{newEvent},
		Completed: true,
	}
	r.StoreStateMulti(Dict{
		"total_processed":            1,
		"reservation_type":           restaurantLocals.RestaurantType,
		"reservation_start_time":     newEvent.Start.Format("15:04"),
		"reservation_end_time":       newEvent.End.Format("15:04"),
		"reservation_duration_hours": restaurantLocals.Duration.Hours(),
		"reservation_name":           newEvent.Name,
		"schedule_date":              baseDate.Format("2006-01-02"),
		"achieving_percentage":       100,
	})
	r.StoreLog("📦 Restaurant reservation created: %s", newEvent.Name)
	return newSchedule
}

// RestaurantSchedule represents a complete restaurant reservation schedule
type RestaurantSchedule struct {
	TravelSchedule
	ReservationTime  time.Time `json:"reservation_time"`
	DurationHours    float64   `json:"duration_hours"`
	RestaurantType   string    `json:"restaurant_type"`
	ReservationName  string    `json:"reservation_name"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

// SetSchedule implements TravelWantInterface for RestaurantWant
func (r *RestaurantWant) SetSchedule(schedule any) {
	s, ok := schedule.(RestaurantSchedule)
	if !ok {
		if sPtr, ok := schedule.(*RestaurantSchedule); ok {
			s = *sPtr
		} else {
			r.StoreLog("ERROR: Failed to cast schedule to RestaurantSchedule")
			return
		}
	}

	stateUpdates := Dict{
		"completed":                  true,
		"reservation_start_time":     s.ReservationTime.Format("15:04"),
		"reservation_end_time":       s.ReservationTime.Add(time.Duration(s.DurationHours * float64(time.Hour))).Format("15:04"),
		"restaurant_type":            s.RestaurantType,
		"reservation_duration_hours": s.DurationHours,
		"reservation_name":           s.ReservationName,
		"total_processed":            1,
		"schedule_date":              s.ReservationTime.Format("2006-01-02"),
	}
	if s.PremiumLevel != "" {
		stateUpdates["premium_processed"] = true
		stateUpdates["premium_level"] = s.PremiumLevel
	}
	if s.ServiceTier != "" {
		stateUpdates["service_tier"] = s.ServiceTier
	}
	if len(s.PremiumAmenities) > 0 {
		stateUpdates["premium_amenities"] = s.PremiumAmenities
	}

	r.Want.StoreStateMulti(stateUpdates)
	r.ProvideDone()
}

// generateRealisticRestaurantNameForTravel generates realistic restaurant names for travel summaries
func generateRealisticRestaurantNameForTravel(cuisineType string) string {
	var names map[string][]string = map[string][]string{
		"fine dining": {
			"L'Élégance", "The Michelin House", "Le Bernardin", "Per Se", "The French Laundry",
			"Alinea", "The Ledbury", "Noma", "Chef's Table", "Sous Vide",
		},
		"casual": {
			"The Garden Bistro", "Rustic Table", "Harvest Kitchen", "Homestead",
			"The Local Taste", "Farm to Fork", "Urban Eats", "Downtown Cafe",
		},
		"buffet": {
			"The Buffet House", "All You Can Eat Palace", "Golden Buffet", "Celebration Buffet",
		},
		"steakhouse": {
			"The Prime Cut", "Bone & Barrel", "Wagyu House", "The Smokehouse",
			"Texas Grill", "The Cattleman's", "Ribeye Room",
		},
		"seafood": {
			"The Lobster Cove", "Catch of the Day", "The Oyster House", "Sea Pearl",
			"Harbor View", "Dockside Grille", "The Fish House", "Captain's Table",
		},
	}

	category := strings.ToLower(cuisineType)
	if list, exists := names[category]; exists && len(list) > 0 {
		return list[rand.Intn(len(list))]
	}

	// Default to fine dining if unknown
	return names["fine dining"][rand.Intn(len(names["fine dining"]))]
}

// generateRealisticHotelNameForTravel generates realistic hotel names for travel summaries
func generateRealisticHotelNameForTravel(hotelType string) string {
	var names map[string][]string = map[string][]string{
		"luxury": {
			"The Grand Plaza", "Royal Palace Hotel", "Platinum Suites", "Signature Towers",
			"The Ritz Collection", "Prestige Residences", "Crown Jewel Hotel", "Luxury Haven",
		},
		"standard": {
			"City Central Hotel", "Comfort Inn Express", "Downtown Plaza", "Urban Oasis",
			"Gateway Hotel", "The Meridian", "Riverside Inn", "Sunrise Hotel",
		},
		"budget": {
			"Budget Stay Inn", "Economy Hotel", "Value Lodge", "Basic Inn",
			"The Affordable", "Quick Stop Hotel", "Smart Stay", "City Budget",
		},
		"boutique": {
			"Artistic House", "Heritage Inn", "Boutique Noir", "Cultural Quarters",
			"The Artisan Hotel", "Signature Boutique", "Modern Spaces", "Unique Stay",
		},
	}

	category := strings.ToLower(hotelType)
	if list, exists := names[category]; exists && len(list) > 0 {
		return list[rand.Intn(len(list))]
	}

	// Default to standard if unknown
	return names["standard"][rand.Intn(len(names["standard"]))]
}

// generateRealisticBuffetNameForTravel generates realistic buffet names for travel summaries
func generateRealisticBuffetNameForTravel(buffetType string) string {
	var names map[string][]string = map[string][]string{
		"continental": {
			"The Continental Breakfast", "Morning Spread Buffet", "Continental Sunrise",
			"Classic Breakfast House", "The Morning Table",
		},
		"international": {
			"World Flavors Buffet", "Global Taste", "International Feast",
			"The Passport Cafe", "Around the World Dining",
		},
		"asian": {
			"Asian Cuisine Buffet", "Dragon's Feast", "The Orient Express",
			"Asian Spice Buffet", "East Meets Plate",
		},
		"mediterranean": {
			"Mediterranean Buffet", "The Greek Table", "Olive & Vine",
			"Mediterranean Feast", "The Coastline",
		},
	}

	category := strings.ToLower(buffetType)
	if list, exists := names[category]; exists && len(list) > 0 {
		return list[rand.Intn(len(list))]
	}

	// Default to continental if unknown
	return names["continental"][rand.Intn(len(names["continental"]))]
}

// HotelWant creates hotel stay reservations
type HotelWant struct {
	BaseTravelWant
}

func (h *HotelWant) GetLocals() *HotelWantLocals {
	return GetLocals[HotelWantLocals](&h.Want)
}

// NewHotelWant creates a new hotel reservation want
// Initialize prepares the hotel want for execution
func (h *HotelWant) Initialize() {
	h.BaseTravelWant.executor = h
}

// tryAgentExecution implements TravelWantInterface for HotelWant
func (h *HotelWant) tryAgentExecution() any {
	if len(h.Spec.Requires) > 0 {
		// Use dynamic agent execution based on requirements
		if err := h.ExecuteAgents(); err != nil {
			h.StoreState("agent_execution_status", "failed")
			h.StoreState("agent_execution_error", err.Error())
			return nil
		}

		h.StoreState("agent_execution_status", "completed")

		// Wait for agent to complete and retrieve result Check for agent_result in state
		if schedule, ok := GetStateAs[HotelSchedule](&h.Want, "agent_result"); ok {
			return &schedule
		}

		return nil
	}

	return nil
}

// generateSchedule implements TravelWantInterface for HotelWant
func (h *HotelWant) generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule {
	hotelLocals, ok := locals.(*HotelWantLocals)
	if !ok {
		return nil
	}

	baseDate := time.Now().AddDate(0, 0, 1) // Tomorrow
	checkInTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		20+rand.Intn(4), rand.Intn(60), 0, 0, time.Local) // 8 PM - midnight

	nextDay := baseDate.AddDate(0, 0, 1)
	checkOutTime := time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(),
		7+rand.Intn(3), rand.Intn(60), 0, 0, time.Local) // 7-10 AM next day

	// Generate realistic hotel name for the summary
	hotelName := generateRealisticHotelNameForTravel(hotelLocals.HotelType)

	newEvent := TimeSlot{
		Start: checkInTime,
		End:   checkOutTime,
		Type:  "hotel",
		Name:  fmt.Sprintf("%s (%s hotel)", hotelName, hotelLocals.HotelType),
	}

	newSchedule := &TravelSchedule{
		Date:      baseDate,
		Events:    []TimeSlot{newEvent},
		Completed: true,
	}
	h.StoreStateMulti(Dict{
		"total_processed":      1,
		"hotel_type":           hotelLocals.HotelType,
		"check_in_time":        newEvent.Start.Format("15:04 Jan 2"),
		"check_out_time":       newEvent.End.Format("15:04 Jan 2"),
		"stay_duration_hours":  newEvent.End.Sub(newEvent.Start).Hours(),
		"reservation_name":     newEvent.Name,
		"achieving_percentage": 100,
	})
	h.StoreLog("📦 Hotel reservation created: %s", newEvent.Name)
	return newSchedule
}

// BuffetWant creates breakfast buffet reservations
type BuffetWant struct {
	BaseTravelWant
}

func (b *BuffetWant) GetLocals() *BuffetWantLocals {
	return GetLocals[BuffetWantLocals](&b.Want)
}

// Initialize prepares the buffet want for execution
func (b *BuffetWant) Initialize() {
	b.BaseTravelWant.executor = b
}

// tryAgentExecution implements TravelWantInterface for BuffetWant
func (b *BuffetWant) tryAgentExecution() any {
	if len(b.Spec.Requires) > 0 {
		// Use dynamic agent execution based on requirements
		if err := b.ExecuteAgents(); err != nil {
			b.StoreState("agent_execution_status", "failed")
			b.StoreState("agent_execution_error", err.Error())
			return nil
		}

		b.StoreState("agent_execution_status", "completed")

		// Wait for agent to complete and retrieve result Check for agent_result in state
		if schedule, ok := GetStateAs[BuffetSchedule](&b.Want, "agent_result"); ok {
			return &schedule
		}

		return nil
	}

	return nil
}

// generateSchedule implements TravelWantInterface for BuffetWant
func (b *BuffetWant) generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule {
	buffetLocals, ok := locals.(*BuffetWantLocals)
	if !ok {
		return nil
	}

	// Next day morning buffet
	nextDay := time.Now().AddDate(0, 0, 2) // Day after tomorrow
	buffetStart := time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(),
		8+rand.Intn(2), rand.Intn(30), 0, 0, time.Local) // 8-10 AM

	// Generate realistic buffet name for the summary
	buffetName := generateRealisticBuffetNameForTravel(buffetLocals.BuffetType)

	newEvent := TimeSlot{
		Start: buffetStart,
		End:   buffetStart.Add(buffetLocals.Duration),
		Type:  "buffet",
		Name:  fmt.Sprintf("%s (%s buffet)", buffetName, buffetLocals.BuffetType),
	}

	newSchedule := &TravelSchedule{
		Date:      nextDay,
		Events:    []TimeSlot{newEvent},
		Completed: true,
	}
	b.StoreStateMulti(Dict{
		"total_processed":       1,
		"buffet_type":           buffetLocals.BuffetType,
		"buffet_start_time":     newEvent.Start.Format("15:04 Jan 2"),
		"buffet_end_time":       newEvent.End.Format("15:04 Jan 2"),
		"buffet_duration_hours": buffetLocals.Duration.Hours(),
		"reservation_name":      newEvent.Name,
		"achieving_percentage":  100,
	})
	b.StoreLog("📦 Buffet reservation created: %s", newEvent.Name)
	return newSchedule
}

// BuffetSchedule represents a complete buffet reservation schedule
type BuffetSchedule struct {
	TravelSchedule
	ReservationTime  time.Time `json:"reservation_time"`
	DurationHours    float64   `json:"duration_hours"`
	BuffetType       string    `json:"buffet_type"`
	ReservationName  string    `json:"reservation_name"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

// SetSchedule implements TravelWantInterface for BuffetWant
func (b *BuffetWant) SetSchedule(schedule any) {
	s, ok := schedule.(BuffetSchedule)
	if !ok {
		if sPtr, ok := schedule.(*BuffetSchedule); ok {
			s = *sPtr
		} else {
			b.StoreLog("ERROR: Failed to cast schedule to BuffetSchedule")
			return
		}
	}

	stateUpdates := Dict{
		"completed":             true,
		"buffet_start_time":     s.ReservationTime.Format("15:04 Jan 2"),
		"buffet_end_time":       s.ReservationTime.Add(time.Duration(s.DurationHours * float64(time.Hour))).Format("15:04 Jan 2"),
		"buffet_type":           s.BuffetType,
		"buffet_duration_hours": s.DurationHours,
		"reservation_name":      s.ReservationName,
		"total_processed":       1,
	}
	if s.PremiumLevel != "" {
		stateUpdates["premium_processed"] = true
		stateUpdates["premium_level"] = s.PremiumLevel
	}
	if s.ServiceTier != "" {
		stateUpdates["service_tier"] = s.ServiceTier
	}
	if len(s.PremiumAmenities) > 0 {
		stateUpdates["premium_amenities"] = s.PremiumAmenities
	}

	b.Want.StoreStateMulti(stateUpdates)
	b.ProvideDone()
}

// ============================================================================
// FlightWant Implementation (migrated from flight_types.go)
// ============================================================================

// FlightWantLocals holds type-specific local state for FlightWant
type FlightWantLocals struct {
	FlightType          string
	Duration            time.Duration
	DepartureDate       string // Departure date in YYYY-MM-DD format
	monitoringStartTime time.Time
	monitoringDuration  time.Duration // How long to monitor for status changes
	lastLogTime         time.Time     // Track last monitoring log time to reduce spam
	monitoringDone      chan struct{} // Signal to stop monitoring goroutine
}

// StatusChange represents a status change event
type StatusChange struct {
	Timestamp time.Time
	OldStatus string
	NewStatus string
	Details   string
}

// Flight execution phases (state machine)
const (
	PhaseInitial    = "initial"
	PhaseBooking    = "booking"
	PhaseMonitoring = "monitoring"
	PhaseCanceling  = "canceling"
	PhaseCompleted  = "completed"
)

// FlightWant creates flight booking reservations
type FlightWant struct {
	BaseTravelWant
}

// Initialize prepares the flight want for execution
func (f *FlightWant) Initialize() {
	f.BaseTravelWant.executor = f

	// Get or initialize locals
	locals, ok := f.Locals.(*FlightWantLocals)
	if !ok {
		locals = &FlightWantLocals{}
		f.Locals = locals
	}

	locals.monitoringDone = make(chan struct{})
}

// IsAchieved checks if flight booking is complete (all phases finished)
func (f *FlightWant) IsAchieved() bool {
	phase, _ := f.GetStateString("_flight_phase", "")
	return phase == PhaseCompleted
}

// GetLocals returns the FlightWantLocals from this want
func (f *FlightWant) GetLocals() *FlightWantLocals {
	return GetLocals[FlightWantLocals](&f.Want)
}

// extractFlightSchedule converts agent_result from state to FlightSchedule
func (f *FlightWant) extractFlightSchedule(result any) *FlightSchedule {
	var schedule FlightSchedule
	switch v := result.(type) {
	case FlightSchedule:
		return &v
	case *FlightSchedule:
		return v
	case map[string]any:
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
		f.StoreLog("agent_result is unexpected type: %T", result)
		return nil
	}
}

// Progress creates a flight booking reservation using state machine pattern
// The execution flow follows distinct phases:
// 1. Initial: Setup phase, transitions immediately to Booking
// 2. Booking: Execute initial flight booking via agents
// 3. Monitoring: Monitor flight status, cancel and rebook if delayed
// 4. Canceling: Cancel the flight, reset state, return to Booking for rebooking
// 5. Completed: Final state after successful completion
func (f *FlightWant) Progress() {
	locals := CheckLocalsInitialized[FlightWantLocals](&f.Want)

	_, connectionAvailable := f.GetFirstOutputChannel()

	phase, _ := f.GetStateString("_flight_phase", PhaseInitial)

	// Only log phase transition to avoid spam
	lastLoggedPhase, _ := f.GetStateString("last_logged_phase", "")
	if lastLoggedPhase != phase {
		f.StoreLog("[FLIGHT] Transitioned to phase: %s", phase)
		f.StoreState("last_logged_phase", phase)
	}

	// State machine: handle each phase
	switch phase {

	// === Phase 1: Initial Setup ===
	case PhaseInitial:
		f.StoreState("_flight_phase", PhaseBooking)
		return

	// === Phase 2: Initial Booking ===
	case PhaseBooking:
		// Initialize monitoring duration on first booking transition
		if locals.monitoringDuration == 0 {
			completionTimeoutSeconds := f.GetIntParam("completion_timeout", 60)
			locals.monitoringDuration = time.Duration(completionTimeoutSeconds) * time.Second
		}

		f.StoreState("completed", true)
		f.tryAgentExecution()

		agentResult, hasResult := f.GetState("agent_result")
		if hasResult && agentResult != nil {
			agentSchedule := f.extractFlightSchedule(agentResult)
			if agentSchedule != nil {
				f.SetSchedule(*agentSchedule)
				if connectionAvailable {
					out, _ := f.GetFirstOutputChannel()
					f.sendFlightPacket(out, agentSchedule, "Initial")
				}

				// Transition to monitoring phase
				locals.monitoringStartTime = time.Now()
				f.StoreState("_flight_phase", PhaseMonitoring)
				return
			}
		}

		// Booking failed - don't complete, let it retry or stay in booking phase
		return

	// === Phase 3: Monitoring ===
	case PhaseMonitoring:
		if time.Since(locals.monitoringStartTime) < locals.monitoringDuration {
			elapsed := time.Since(locals.monitoringStartTime)
			if f.shouldCancelAndRebook() {
				f.StoreLog("📦 Delay detected at %v, initiating cancellation", elapsed)
				f.StoreStateMulti(Dict{
					"flight_action": "cancel_flight",
					"completed":     false,
					"_flight_phase": PhaseCanceling,
				})
				return
			}

			// Log monitoring progress every 30 seconds
			now := time.Now()
			if locals.lastLogTime.IsZero() || now.Sub(locals.lastLogTime) >= 30*time.Second {
				f.StoreLog("Monitoring... (elapsed: %v/%v)", elapsed, locals.monitoringDuration)
				locals.lastLogTime = now
			}

			return

		} else {
			// Monitoring period expired - flight stable, complete
			f.StoreLog("📦 Flight monitoring completed successfully")
			f.StoreStateMulti(Dict{
				"_flight_phase":        PhaseCompleted,
				"achieving_percentage": 100,
			})
			f.ProvideDone()
			return
		}

	// === Phase 4: Canceling ===
	case PhaseCanceling:
		flightID, ok := f.GetStateString("flight_id", "")
		if !ok || flightID == "" {
			f.ResetFlightState()
			f.StoreStateMulti(Dict{
				"_flight_phase": PhaseBooking,
				"completed":     false,
			})
			return
		}

		// Execute cancel flight action
		f.tryAgentExecution()

		f.StoreLog("📦 Cancelled flight: %s", flightID)

		// Reset flight state and transition back to booking phase for rebooking
		f.ResetFlightState()
		f.StoreStateMulti(Dict{
			"_flight_phase": PhaseBooking,
			"completed":     false,
		})
		return

	// === Phase 5: Completed ===
	case PhaseCompleted:
		// Clear agent_result to prevent reuse in next execution cycle
		f.StoreState("agent_result", nil)
		return

	default:
		f.SetModuleError("Phase", fmt.Sprintf("Unknown phase: %s", phase))
		return
	}
}

func (f *FlightWant) sendFlightPacket(out any, schedule *FlightSchedule, label string) {
	flightEvent := TimeSlot{
		Start: schedule.DepartureTime,
		End:   schedule.ArrivalTime,
		Type:  "flight",
		Name:  schedule.ReservationName,
	}

	travelSchedule := &TravelSchedule{
		Date:      schedule.DepartureTime.Truncate(24 * time.Hour),
		Events:    []TimeSlot{flightEvent},
		Completed: true,
	}
	f.Provide(travelSchedule)

	f.StoreLog("[PACKET-SEND] %s flight: %s (%s to %s) | TravelSchedule: Date=%s, Events=%d",
		label,
		schedule.ReservationName,
		schedule.DepartureTime.Format("15:04 Jan 2"),
		schedule.ArrivalTime.Format("15:04 Jan 2"),
		travelSchedule.Date.Format("2006-01-02"),
		len(travelSchedule.Events))
}

// tryAgentExecution implements TravelWantInterface for FlightWant
// Attempts to execute flight booking using the agent system
func (f *FlightWant) tryAgentExecution() any {
	if len(f.Spec.Requires) > 0 {
		// Execute agents via ExecuteAgents() which properly tracks agent history
		if err := f.ExecuteAgents(); err != nil {
			f.StoreStateMulti(Dict{
				"agent_execution_status": "failed",
				"agent_execution_error":  err.Error(),
			})
			return nil
		}

		f.StoreState("agent_execution_status", "completed")
		if result, exists := f.GetState("agent_result"); exists && result != nil {
			f.StoreState("execution_source", "agent")

			// Start background monitoring for this flight using registered MonitorAgent
			flightID, _ := f.GetStateString("flight_id", "")
			agentName := "flight-monitor-" + flightID

			if agent, ok := f.GetAgentRegistry().GetAgent(flightMonitorAgentName); ok {
				if err := f.AddMonitoringAgent(agentName, 10*time.Second, agent.Exec); err != nil {
					f.StoreLog("ERROR: Failed to start background monitoring: %v", err)
				}
			} else {
				f.StoreLog("ERROR: Monitor agent %s not found in registry", flightMonitorAgentName)
			}

			// Extract and return FlightSchedule
			return f.extractFlightSchedule(result)
		}
	}

	return nil
}

// generateSchedule implements TravelWantInterface for FlightWant
// FlightWant always uses agents for scheduling, so this returns nil
func (f *FlightWant) generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule {
	return nil
}

// FlightSchedule represents a complete flight booking schedule
type FlightSchedule struct {
	TravelSchedule
	DepartureTime    time.Time `json:"departure_time"`
	ArrivalTime      time.Time `json:"arrival_time"`
	FlightType       string    `json:"flight_type"`
	FlightNumber     string    `json:"flight_number"`
	ReservationName  string    `json:"reservation_name"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

// SetSchedule implements TravelWantInterface for FlightWant
func (f *FlightWant) SetSchedule(schedule any) {
	s, ok := schedule.(FlightSchedule)
	if !ok {
		if sPtr, ok := schedule.(*FlightSchedule); ok {
			s = *sPtr
		} else {
			f.StoreLog("ERROR: Failed to cast schedule to FlightSchedule")
			return
		}
	}

	stateUpdates := Dict{
		"completed":             true,
		"departure_time":        s.DepartureTime.Format("15:04 Jan 2"),
		"arrival_time":          s.ArrivalTime.Format("15:04 Jan 2"),
		"flight_type":           s.FlightType,
		"flight_duration_hours": s.ArrivalTime.Sub(s.DepartureTime).Hours(),
		"flight_number":         s.FlightNumber,
		"reservation_name":      s.ReservationName,
		"total_processed":       1,
		"schedule_date":         s.DepartureTime.Format("2006-01-02"),
		"achieving_percentage":  100,
	}
	if s.PremiumLevel != "" {
		stateUpdates["premium_processed"] = true
		stateUpdates["premium_level"] = s.PremiumLevel
	}
	if s.ServiceTier != "" {
		stateUpdates["service_tier"] = s.ServiceTier
	}
	if len(s.PremiumAmenities) > 0 {
		stateUpdates["premium_amenities"] = s.PremiumAmenities
	}

	f.StoreStateMulti(stateUpdates)
	f.ProvideDone()
}

// ResetFlightState clears all flight-specific state information
// Used after cancellation to prepare for rebooking attempt
func (f *FlightWant) ResetFlightState() {
	resetKeys := []string{
		"flight_id",
		"flight_status",
		"flight_number",
		"from",
		"to",
		"departure_time",
		"arrival_time",
		"status_message",
		"updated_at",
		"status_changed",
		"status_changed_at",
		"status_change_history_count",
		"status_history",
		"agent_result",
		"agent_execution_status",
		"agent_execution_error",
		"execution_source",
		"premium_level",
		"service_tier",
		"premium_amenities",
		"premium_processed",
		"flight_duration_hours",
		"total_processed",
		"schedule_date",
		"canceled_at",
		"_previous_flight_status",
		"_monitor_state_hash",
		// NOTE: _previous_flight_id is NOT reset - it's used to detect rebooking state
	}

	for _, key := range resetKeys {
		f.StoreState(key, nil)
	}

	f.StoreLog("Flight state reset for rebooking attempt")
}

// shouldCancelAndRebook checks if the current flight should be cancelled due to delay
func (f *FlightWant) shouldCancelAndRebook() bool {
	flightID, ok := f.GetStateString("flight_id", "")
	if !ok || flightID == "" {
		return false
	}
	status, ok := f.GetStateString("flight_status", "")
	if !ok {
		return false
	}

	// Cancel and rebook if delayed
	if status == "delayed_one_day" {
		return true
	}

	return false
}

// Helper function to check time conflicts
// HotelSchedule represents a complete hotel booking schedule
type HotelSchedule struct {
	TravelSchedule
	CheckInTime       time.Time `json:"check_in_time"`
	CheckOutTime      time.Time `json:"check_out_time"`
	HotelType         string    `json:"hotel_type"`
	StayDurationHours float64   `json:"stay_duration_hours"`
	ReservationName   string    `json:"reservation_name"`
	PremiumLevel      string    `json:"premium_level,omitempty"`
	ServiceTier       string    `json:"service_tier,omitempty"`
	PremiumAmenities  []string  `json:"premium_amenities,omitempty"`
}

// SetSchedule implements TravelWantInterface for HotelWant
func (h *HotelWant) SetSchedule(schedule any) {
	s, ok := schedule.(HotelSchedule)
	if !ok {
		if sPtr, ok := schedule.(*HotelSchedule); ok {
			s = *sPtr
		} else {
			h.StoreLog("ERROR: Failed to cast schedule to HotelSchedule")
			return
		}
	}

	stateUpdates := Dict{
		"completed":           true,
		"check_in_time":       s.CheckInTime.Format("15:04 Jan 2"),
		"check_out_time":      s.CheckOutTime.Format("15:04 Jan 2"),
		"hotel_type":          s.HotelType,
		"stay_duration_hours": s.StayDurationHours,
		"reservation_name":    s.ReservationName,
		"total_processed":     1,
	}
	if s.PremiumLevel != "" {
		stateUpdates["premium_processed"] = true
		stateUpdates["premium_level"] = s.PremiumLevel
	}
	if s.ServiceTier != "" {
		stateUpdates["service_tier"] = s.ServiceTier
	}
	if len(s.PremiumAmenities) > 0 {
		stateUpdates["premium_amenities"] = s.PremiumAmenities
	}

	h.Want.StoreStateMulti(stateUpdates)
	h.ProvideDone()

	// No need to send output here anymore - handled directly in Exec method
}

// generateTravelTimeline creates a human-readable timeline from all events
func generateTravelTimeline(events []TimeSlot) string {
	if len(events) == 0 {
		return "No events scheduled"
	}

	timeline := ""
	for _, event := range events {
		startTime := event.Start.Format("15:04")
		endTime := event.End.Format("15:04")

		// Map event type to readable names
		eventName := event.Name
		if event.Type != "" {
			switch event.Type {
			case "restaurant":
				eventName = "Restaurant: " + eventName
			case "hotel":
				eventName = "Hotel: " + eventName
			case "buffet":
				eventName = "Buffet: " + eventName
			case "flight":
				eventName = "Flight: " + eventName
			}
		}

		timeline += fmt.Sprintf("%s, %s to %s\n", eventName, startTime, endTime)
	}

	return timeline
}
