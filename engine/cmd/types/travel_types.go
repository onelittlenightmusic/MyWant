package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/src"
	"time"
)

// TimeSlot represents a time period with start and end times
type TimeSlot struct {
	Start time.Time
	End   time.Time
	Type  string
	Name  string
}

// TravelSchedule represents a complete travel schedule with multiple events
type TravelSchedule struct {
	Events []TimeSlot
	Date   time.Time
}

// ScheduleConflict represents a scheduling conflict that needs resolution
type ScheduleConflict struct {
	Event1   TimeSlot
	Event2   TimeSlot
	Resolved bool
	Attempts int
}

// RestaurantWant creates dinner restaurant reservations
type RestaurantWant struct {
	Want
	RestaurantType string
	Duration       time.Duration
}

// NewRestaurantWant creates a new restaurant reservation want
func NewRestaurantWant(metadata Metadata, spec WantSpec) interface{} {
	restaurant := &RestaurantWant{
		Want:           Want{},
		RestaurantType: "casual",
		Duration:       2 * time.Hour, // Default 2 hour dinner
	}

	// Initialize base Want fields
	restaurant.Init(metadata, spec)

	restaurant.RestaurantType = restaurant.GetStringParam("restaurant_type", "casual")
	restaurant.Duration = time.Duration(restaurant.GetFloatParam("duration_hours", 2.0) * float64(time.Hour))

	// Set fields for base Want methods
	restaurant.WantType = "restaurant"
	restaurant.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		WantType:        "restaurant",
		Description:     "Restaurant reservation scheduling want",
	}

	return restaurant
}

func (r *RestaurantWant) GetWant() *Want {
	return &r.Want
}

// Exec creates a restaurant reservation
func (r *RestaurantWant) Exec() bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	restaurantType := r.GetStringParam("restaurant_type", "casual")
	duration := time.Duration(r.GetFloatParam("duration_hours", 2.0) * float64(time.Hour))

	// Check if already attempted using persistent state
	attemptedVal, _ := r.GetState("attempted")
	attempted, _ := attemptedVal.(bool)

	// Get output channel
	out, connectionAvailable := r.GetFirstOutputChannel()

	if attempted {
		return true
	}

	// Mark as attempted in persistent state
	r.StoreState("attempted", true)

	// Try to use agent system if available - agent completely overrides normal execution
	if agentSchedule := r.tryAgentExecution(); agentSchedule != nil {
		// Use the agent's schedule result
		r.SetSchedule(*agentSchedule)

		// Send the schedule to output channel only if available
		if connectionAvailable {
			restaurantEvent := TimeSlot{
				Start: agentSchedule.ReservationTime,
				End:   agentSchedule.ReservationTime.Add(time.Duration(agentSchedule.DurationHours * float64(time.Hour))),
				Type:  "restaurant",
				Name:  agentSchedule.ReservationName,
			}

			travelSchedule := &TravelSchedule{
				Date:   agentSchedule.ReservationTime.Truncate(24 * time.Hour),
				Events: []TimeSlot{restaurantEvent},
			}

			out <- travelSchedule
		}

		return true
	}

	// Check for conflicts from input
	var existingSchedule *TravelSchedule
	if r.GetInCount() > 0 {
		in, connectionAvailable := r.GetInputChannel(0)
		if connectionAvailable {
			select {
			case schedData := <-in:
				if schedule, ok := schedData.(*TravelSchedule); ok {
					existingSchedule = schedule
				}
			default:
				// No input data available
			}
		}
	}

	// Generate restaurant reservation time (evening dinner)
	baseDate := time.Now().AddDate(0, 0, 1) // Tomorrow
	dinnerStart := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		18+rand.Intn(3), rand.Intn(60), 0, 0, time.Local) // 6-9 PM

	newEvent := TimeSlot{
		Start: dinnerStart,
		End:   dinnerStart.Add(duration),
		Type:  "restaurant",
		Name:  fmt.Sprintf("%s dinner at %s restaurant", r.Metadata.Name, restaurantType),
	}

	// Check for conflicts if we have existing schedule
	if existingSchedule != nil {
		for attempt := 0; attempt < 3; attempt++ {
			conflict := false
			for _, event := range existingSchedule.Events {
				if r.hasTimeConflict(newEvent, event) {
					conflict = true
					// Retry with different time
				dinnerStart = dinnerStart.Add(time.Hour)
					newEvent.Start = dinnerStart
					newEvent.End = dinnerStart.Add(duration)
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

	// Store stats using thread-safe StoreState
	r.StoreStateMulti(map[string]interface{}{
		"total_processed":            1,
		"reservation_type":           restaurantType,
		"reservation_start_time":     newEvent.Start.Format("15:04"),
		"reservation_end_time":       newEvent.End.Format("15:04"),
		"reservation_duration_hours": duration.Hours(),
		"reservation_name":           newEvent.Name,
		"schedule_date":              baseDate.Format("2006-01-02"),
	})

	// Send to output channel only if available
	if connectionAvailable {
		out <- newSchedule
	}

	return true
}

// tryAgentExecution attempts to execute restaurant reservation using the agent system
// Returns the RestaurantSchedule if successful, nil if no agent execution
func (r *RestaurantWant) tryAgentExecution() *RestaurantSchedule {
	// Check if this want has agent requirements
	if len(r.Spec.Requires) > 0 {
		// Store the requirements in want state for tracking
		r.StoreState("agent_requirements", r.Spec.Requires)

		// Step 1: Execute MonitorRestaurant first to check for existing state

		// Create and execute MonitorRestaurant directly inline to avoid context issues
		monitorAgent := NewMonitorRestaurant(
			"restaurant_monitor",
			[]string{"restaurant_agency"},
			[]string{"xxx"},
		)

		ctx := context.Background()
		if err := monitorAgent.Exec(ctx, &r.Want); err != nil {
		}

		r.AggregateChanges()

		// Check if MonitorRestaurant found an existing schedule
		if result, exists := r.GetState("agent_result"); exists && result != nil {
			if schedule, ok := result.(RestaurantSchedule); ok {
				r.StoreState("execution_source", "monitor")

				// Immediately set the schedule and complete the cycle
			r.SetSchedule(schedule)
				return &schedule
			}
		}

		// Step 2: No existing schedule found, execute AgentRestaurant
		if err := r.ExecuteAgents(); err != nil {
			r.StoreState("agent_execution_status", "failed")
			r.StoreState("agent_execution_error", err.Error())
			return nil
		}

		r.StoreState("agent_execution_status", "completed")
		r.StoreState("execution_source", "agent")

		// Wait for agent to complete and retrieve result
		// Check for agent_result in state
		if result, exists := r.GetState("agent_result"); exists && result != nil {
			if schedule, ok := result.(RestaurantSchedule); ok {
				return &schedule
			}
		}

		return nil
	}

	return nil
}

// executeMonitorRestaurant executes the MonitorRestaurant agent to check for existing state
func (r *RestaurantWant) executeMonitorRestaurant() error {
	// Create a MonitorRestaurant instance
	monitorAgent := NewMonitorRestaurant(
		"restaurant_monitor",
		[]string{"restaurant_agency"},
		[]string{"xxx"},
	)

	// Execute the monitor agent on the want
	ctx := context.Background()
	if err := monitorAgent.Exec(ctx, &r.Want); err != nil {
		return err
	}

	return nil
}

// RestaurantSchedule represents a complete restaurant reservation schedule
type RestaurantSchedule struct {
	ReservationTime  time.Time `json:"reservation_time"`
	DurationHours    float64   `json:"duration_hours"`
	RestaurantType   string    `json:"restaurant_type"`
	ReservationName  string    `json:"reservation_name"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

// SetSchedule sets the restaurant reservation schedule and updates all related state
func (r *RestaurantWant) SetSchedule(schedule RestaurantSchedule) {
	// Store basic restaurant booking information
	r.Want.StoreState("attempted", true)
	r.Want.StoreState("reservation_start_time", schedule.ReservationTime.Format("15:04"))
	r.Want.StoreState("reservation_end_time", schedule.ReservationTime.Add(time.Duration(schedule.DurationHours*float64(time.Hour))).Format("15:04"))
	r.Want.StoreState("restaurant_type", schedule.RestaurantType)
	r.Want.StoreState("reservation_duration_hours", schedule.DurationHours)
	r.Want.StoreState("reservation_name", schedule.ReservationName)
	r.Want.StoreState("total_processed", 1)
	r.Want.StoreState("schedule_date", schedule.ReservationTime.Format("2006-01-02"))

	// Store premium information if provided
	if schedule.PremiumLevel != "" {
		r.Want.StoreState("premium_processed", true)
		r.Want.StoreState("premium_level", schedule.PremiumLevel)
	}
	if schedule.ServiceTier != "" {
		r.Want.StoreState("service_tier", schedule.ServiceTier)
	}
	if len(schedule.PremiumAmenities) > 0 {
		r.Want.StoreState("premium_amenities", schedule.PremiumAmenities)
	}
}

// HotelWant creates hotel stay reservations
type HotelWant struct {
	Want
	HotelType string
	CheckIn   time.Duration
	CheckOut  time.Duration
}

// NewHotelWant creates a new hotel reservation want
func NewHotelWant(metadata Metadata, spec WantSpec) interface{} {
	hotel := &HotelWant{
		Want:      Want{},
		HotelType: "standard",
		CheckIn:   22 * time.Hour, // 10 PM
		CheckOut:  8 * time.Hour,  // 8 AM next day
	}

	// Initialize base Want fields
	hotel.Init(metadata, spec)

	hotel.HotelType = hotel.GetStringParam("hotel_type", "standard")

	// Set fields for base Want methods
	hotel.WantType = "hotel"
	hotel.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		WantType:        "hotel",
		Description:     "Hotel reservation scheduling want",
	}

	return hotel
}

func (h *HotelWant) GetWant() *Want {
	return &h.Want
}

func (h *HotelWant) Exec() bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	hotelType := h.GetStringParam("hotel_type", "standard")

	// Check if already attempted using persistent state
	attemptedVal, _ := h.GetState("attempted")
	attempted, _ := attemptedVal.(bool)

	// Get output channel
	out, connectionAvailable := h.GetFirstOutputChannel()

	if attempted {
		return true
	}

	// Mark as attempted in persistent state
	h.StoreState("attempted", true)

	// Try to use agent system if available - agent completely overrides normal execution
	if agentSchedule := h.tryAgentExecution(); agentSchedule != nil {
		// Use the agent's schedule result
		h.SetSchedule(*agentSchedule)

		// Send the schedule to output channel only if available
		if connectionAvailable {
			hotelEvent := TimeSlot{
				Start: agentSchedule.CheckInTime,
				End:   agentSchedule.CheckOutTime,
				Type:  "hotel",
				Name:  agentSchedule.ReservationName,
			}

			travelSchedule := &TravelSchedule{
				Date:   agentSchedule.CheckInTime.Truncate(24 * time.Hour),
				Events: []TimeSlot{hotelEvent},
			}

			out <- travelSchedule
		}

		return true
	}

	// Normal hotel execution (only runs if agent execution didn't return a result)

	// Check for existing schedule
	var existingSchedule *TravelSchedule
	if h.GetInCount() > 0 {
		in, connectionAvailable := h.GetInputChannel(0)
		if connectionAvailable {
			select {
			case schedData := <-in:
				if schedule, ok := schedData.(*TravelSchedule); ok {
					existingSchedule = schedule
				}
			default:
				// No input data
			}
		}
	}

	baseDate := time.Now().AddDate(0, 0, 1) // Tomorrow
	checkInTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		20+rand.Intn(4), rand.Intn(60), 0, 0, time.Local) // 8 PM - midnight

	nextDay := baseDate.AddDate(0, 0, 1)
	checkOutTime := time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(),
		7+rand.Intn(3), rand.Intn(60), 0, 0, time.Local) // 7-10 AM next day

	newEvent := TimeSlot{
		Start: checkInTime,
		End:   checkOutTime,
		Type:  "hotel",
		Name:  fmt.Sprintf("%s stay at %s hotel", h.Metadata.Name, hotelType),
	}

	// Check conflicts and retry if needed
	if existingSchedule != nil {
		for attempt := 0; attempt < 3; attempt++ {
			conflict := false
			for _, event := range existingSchedule.Events {
				if h.hasTimeConflict(newEvent, event) {
					conflict = true
					// Adjust check-in time
					checkInTime = checkInTime.Add(30 * time.Minute)
					newEvent.Start = checkInTime
					break
				}
			}
			if !conflict {
				break
			}
		}
	}

	newSchedule := &TravelSchedule{
		Date:   baseDate,
		Events: []TimeSlot{newEvent},
	}
	if existingSchedule != nil {
		newSchedule.Events = append(existingSchedule.Events, newEvent)
	}

	// Store stats using thread-safe StoreState
	h.StoreStateMulti(map[string]interface{}{
		"total_processed":     1,
		"hotel_type":          hotelType,
		"check_in_time":       newEvent.Start.Format("15:04 Jan 2"),
		"check_out_time":      newEvent.End.Format("15:04 Jan 2"),
		"stay_duration_hours": newEvent.End.Sub(newEvent.Start).Hours(),
		"reservation_name":    newEvent.Name,
	})

	// Send to output channel only if available
	if connectionAvailable {
		out <- newSchedule
	}

	return true
}

// tryAgentExecution attempts to execute hotel reservation using the agent system
// Returns the HotelSchedule if successful, nil if no agent execution
func (h *HotelWant) tryAgentExecution() *HotelSchedule {
	// Check if this want has agent requirements
	if len(h.Spec.Requires) > 0 {
		// Store the requirements in want state for tracking
	h.StoreState("agent_requirements", h.Spec.Requires)

		// Use dynamic agent execution based on requirements
		if err := h.ExecuteAgents(); err != nil {
			h.StoreState("agent_execution_status", "failed")
			h.StoreState("agent_execution_error", err.Error())
			return nil
		}

		h.StoreState("agent_execution_status", "completed")

		// Wait for agent to complete and retrieve result
		// Check for agent_result in state
		if result, exists := h.GetState("agent_result"); exists {
			if schedule, ok := result.(HotelSchedule); ok {
				return &schedule
			}
		}

		return nil
	}

	return nil
}

// Helper function to get state keys for debugging
func getStateKeys(state map[string]interface{}) []string {
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	return keys
}

// BuffetWant creates breakfast buffet reservations
type BuffetWant struct {
	Want
	BuffetType string
	Duration   time.Duration
}

func NewBuffetWant(metadata Metadata, spec WantSpec) interface{} {
	buffet := &BuffetWant{
		Want:       Want{},
		BuffetType: "continental",
		Duration:   1*time.Hour + 30*time.Minute, // 1.5 hour breakfast
	}

	// Initialize base Want fields
	buffet.Init(metadata, spec)

	buffet.BuffetType = buffet.GetStringParam("buffet_type", "continental")

	// Set fields for base Want methods
	buffet.WantType = "buffet"
	buffet.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		WantType:        "buffet",
		Description:     "Breakfast buffet scheduling want",
	}

	return buffet
}

func (b *BuffetWant) GetWant() *Want {
	return &b.Want
}

func (b *BuffetWant) Exec() bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	buffetType := b.GetStringParam("buffet_type", "continental")

	duration := 1*time.Hour + 30*time.Minute // Default 1.5 hour breakfast

	// Check if already attempted using persistent state
	attemptedVal, _ := b.GetState("attempted")
	attempted, _ := attemptedVal.(bool)

	// Get output channel
	out, connectionAvailable := b.GetFirstOutputChannel()

	if attempted {
		return true
	}

	// Mark as attempted in persistent state
	b.StoreState("attempted", true)

	// Try to use agent system if available - agent completely overrides normal execution
	if agentSchedule := b.tryAgentExecution(); agentSchedule != nil {
		// Use the agent's schedule result
		b.SetSchedule(*agentSchedule)

		// Send the schedule to output channel only if available
		if connectionAvailable {
			buffetEvent := TimeSlot{
				Start: agentSchedule.ReservationTime,
				End:   agentSchedule.ReservationTime.Add(time.Duration(agentSchedule.DurationHours * float64(time.Hour))),
				Type:  "buffet",
				Name:  agentSchedule.ReservationName,
			}

			travelSchedule := &TravelSchedule{
				Date:   agentSchedule.ReservationTime.Truncate(24 * time.Hour),
				Events: []TimeSlot{buffetEvent},
			}

			out <- travelSchedule
		}

		return true
	}

	// Normal buffet execution (only runs if agent execution didn't return a result)

	var existingSchedule *TravelSchedule
	if b.GetInCount() > 0 {
		in, connectionAvailable := b.GetInputChannel(0)
		if connectionAvailable {
			select {
			case schedData := <-in:
				if schedule, ok := schedData.(*TravelSchedule); ok {
					existingSchedule = schedule
				}
			default:
			}
		}
	}

	// Next day morning buffet
	nextDay := time.Now().AddDate(0, 0, 2) // Day after tomorrow
	buffetStart := time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(),
		8+rand.Intn(2), rand.Intn(30), 0, 0, time.Local) // 8-10 AM

	newEvent := TimeSlot{
		Start: buffetStart,
		End:   buffetStart.Add(duration),
		Type:  "buffet",
		Name:  fmt.Sprintf("%s %s breakfast buffet", b.Metadata.Name, buffetType),
	}

	if existingSchedule != nil {
		for attempt := 0; attempt < 3; attempt++ {
			conflict := false
			for _, event := range existingSchedule.Events {
				if b.hasTimeConflict(newEvent, event) {
					conflict = true
				buffetStart = buffetStart.Add(30 * time.Minute)
					newEvent.Start = buffetStart
					newEvent.End = buffetStart.Add(duration)
					break
				}
			}
			if !conflict {
				break
			}
		}
	}

	newSchedule := &TravelSchedule{
		Date:   nextDay,
		Events: []TimeSlot{newEvent},
	}
	if existingSchedule != nil {
		newSchedule.Events = append(existingSchedule.Events, newEvent)
	}

	// Store stats using thread-safe StoreState
	b.StoreStateMulti(map[string]interface{}{
		"total_processed":       1,
		"buffet_type":           buffetType,
		"buffet_start_time":     newEvent.Start.Format("15:04 Jan 2"),
		"buffet_end_time":       newEvent.End.Format("15:04 Jan 2"),
		"buffet_duration_hours": duration.Hours(),
		"reservation_name":      newEvent.Name,
	})

	// Send to output channel only if available
	if connectionAvailable {
		out <- newSchedule
	}

	return true
}

// tryAgentExecution attempts to execute buffet reservation using the agent system
// Returns the BuffetSchedule if successful, nil if no agent execution
func (b *BuffetWant) tryAgentExecution() *BuffetSchedule {
	// Check if this want has agent requirements
	if len(b.Spec.Requires) > 0 {
		// Store the requirements in want state for tracking
	b.StoreState("agent_requirements", b.Spec.Requires)

		// Use dynamic agent execution based on requirements
		if err := b.ExecuteAgents(); err != nil {
			b.StoreState("agent_execution_status", "failed")
			b.StoreState("agent_execution_error", err.Error())
			return nil
		}

		b.StoreState("agent_execution_status", "completed")

		// Wait for agent to complete and retrieve result
		// Check for agent_result in state
		if result, exists := b.GetState("agent_result"); exists {
			if schedule, ok := result.(BuffetSchedule); ok {
				return &schedule
			}
		}

		return nil
	}

	return nil
}

// BuffetSchedule represents a complete buffet reservation schedule
type BuffetSchedule struct {
	ReservationTime  time.Time `json:"reservation_time"`
	DurationHours    float64   `json:"duration_hours"`
	BuffetType       string    `json:"buffet_type"`
	ReservationName  string    `json:"reservation_name"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

// SetSchedule sets the buffet reservation schedule and updates all related state
func (b *BuffetWant) SetSchedule(schedule BuffetSchedule) {
	// Store basic buffet booking information
	b.Want.StoreState("attempted", true)
	b.Want.StoreState("buffet_start_time", schedule.ReservationTime.Format("15:04 Jan 2"))
	b.Want.StoreState("buffet_end_time", schedule.ReservationTime.Add(time.Duration(schedule.DurationHours*float64(time.Hour))).Format("15:04 Jan 2"))
	b.Want.StoreState("buffet_type", schedule.BuffetType)
	b.Want.StoreState("buffet_duration_hours", schedule.DurationHours)
	b.Want.StoreState("reservation_name", schedule.ReservationName)
	b.Want.StoreState("total_processed", 1)

	// Store premium information if provided
	if schedule.PremiumLevel != "" {
		b.Want.StoreState("premium_processed", true)
		b.Want.StoreState("premium_level", schedule.PremiumLevel)
	}
	if schedule.ServiceTier != "" {
		b.Want.StoreState("service_tier", schedule.ServiceTier)
	}
	if len(schedule.PremiumAmenities) > 0 {
		b.Want.StoreState("premium_amenities", schedule.PremiumAmenities)
	}
}

// Helper function to check time conflicts
func (r *RestaurantWant) hasTimeConflict(event1, event2 TimeSlot) bool {
	return event1.Start.Before(event2.End) && event2.Start.Before(event1.End)
}

func (h *HotelWant) hasTimeConflict(event1, event2 TimeSlot) bool {
	return event1.Start.Before(event2.End) && event2.Start.Before(event1.End)
}

// HotelSchedule represents a complete hotel booking schedule
type HotelSchedule struct {
	CheckInTime       time.Time `json:"check_in_time"`
	CheckOutTime      time.Time `json:"check_out_time"`
	HotelType         string    `json:"hotel_type"`
	StayDurationHours float64   `json:"stay_duration_hours"`
	ReservationName   string    `json:"reservation_name"`
	PremiumLevel      string    `json:"premium_level,omitempty"`
	ServiceTier       string    `json:"service_tier,omitempty"`
	PremiumAmenities  []string  `json:"premium_amenities,omitempty"`
}

// SetSchedule sets the hotel booking schedule and updates all related state
func (h *HotelWant) SetSchedule(schedule HotelSchedule) {
	// Store basic hotel booking information
	h.Want.StoreState("attempted", true)
	h.Want.StoreState("check_in_time", schedule.CheckInTime.Format("15:04 Jan 2"))
	h.Want.StoreState("check_out_time", schedule.CheckOutTime.Format("15:04 Jan 2"))
	h.Want.StoreState("hotel_type", schedule.HotelType)
	h.Want.StoreState("stay_duration_hours", schedule.StayDurationHours)
	h.Want.StoreState("reservation_name", schedule.ReservationName)
	h.Want.StoreState("total_processed", 1)

	// Store premium information if provided
	if schedule.PremiumLevel != "" {
		h.Want.StoreState("premium_processed", true)
		h.Want.StoreState("premium_level", schedule.PremiumLevel)
	}
	if schedule.ServiceTier != "" {
		h.Want.StoreState("service_tier", schedule.ServiceTier)
	}
	if len(schedule.PremiumAmenities) > 0 {
		h.Want.StoreState("premium_amenities", schedule.PremiumAmenities)
	}

	// No need to send output here anymore - handled directly in Exec method
}

func (b *BuffetWant) hasTimeConflict(event1, event2 TimeSlot) bool {
	return event1.Start.Before(event2.End) && event2.Start.Before(event1.End)
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

// TravelCoordinatorWant orchestrates the entire travel itinerary
// TravelCoordinatorWant now uses the generic CoordinatorWant with TravelDataHandler and TravelCompletionChecker

// NewTravelCoordinatorWant creates a new Travel coordinator using generic pattern
func NewTravelCoordinatorWant(metadata Metadata, spec WantSpec) interface{} {
	coordinator := NewCoordinatorWant(
		metadata,
		spec,
		3, // Requires 3 inputs (restaurant, hotel, buffet schedules)
		&TravelDataHandler{IsBuffet: false},
		&TravelCompletionChecker{IsBuffet: false},
		"travel coordinator",
	)
	return coordinator
}

// BuffetCoordinatorWant is a minimal coordinator for standalone buffet deployment
// It simply collects the buffet schedule from the BuffetWant and marks completion
// Now uses the generic CoordinatorWant with TravelDataHandler and TravelCompletionChecker

// NewBuffetCoordinatorWant creates a new Buffet coordinator using generic pattern
func NewBuffetCoordinatorWant(metadata Metadata, spec WantSpec) interface{} {
	coordinator := NewCoordinatorWant(
		metadata,
		spec,
		1, // Requires 1 input (buffet schedule)
		&TravelDataHandler{IsBuffet: true},
		&TravelCompletionChecker{IsBuffet: true},
		"buffet coordinator",
	)
	return coordinator
}

// RegisterTravelWantTypes registers all travel-related want types
func RegisterTravelWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("flight", NewFlightWant)
	builder.RegisterWantType("restaurant", NewRestaurantWant)
	builder.RegisterWantType("hotel", NewHotelWant)
	builder.RegisterWantType("buffet", NewBuffetWant)
	builder.RegisterWantType("travel coordinator", NewTravelCoordinatorWant)
	builder.RegisterWantType("buffet coordinator", NewBuffetCoordinatorWant)
}

// RegisterTravelWantTypesWithAgents registers travel want types with agent system support
func RegisterTravelWantTypesWithAgents(builder *ChainBuilder, agentRegistry *AgentRegistry) {
	builder.RegisterWantType("flight", func(metadata Metadata, spec WantSpec) interface{} {
		flight := NewFlightWant(metadata, spec).(*FlightWant)
		flight.SetAgentRegistry(agentRegistry)
		return flight
	})

	builder.RegisterWantType("restaurant", func(metadata Metadata, spec WantSpec) interface{} {
		restaurant := NewRestaurantWant(metadata, spec).(*RestaurantWant)
		restaurant.SetAgentRegistry(agentRegistry)
		return restaurant
	})

	builder.RegisterWantType("hotel", func(metadata Metadata, spec WantSpec) interface{} {
		hotel := NewHotelWant(metadata, spec).(*HotelWant)
		hotel.SetAgentRegistry(agentRegistry)
		return hotel
	})

	builder.RegisterWantType("buffet", func(metadata Metadata, spec WantSpec) interface{} {
		buffet := NewBuffetWant(metadata, spec).(*BuffetWant)
		buffet.SetAgentRegistry(agentRegistry)
		return buffet
	})

	builder.RegisterWantType("travel_coordinator", func(metadata Metadata, spec WantSpec) interface{} {
		return NewTravelCoordinatorWant(metadata, spec)
	})

	builder.RegisterWantType("buffet coordinator", func(metadata Metadata, spec WantSpec) interface{} {
		return NewBuffetCoordinatorWant(metadata, spec)
	})
}