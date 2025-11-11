package types

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	. "mywant/engine/src"
	"mywant/engine/src/chain"
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
	paths          Paths
}

// NewRestaurantWant creates a new restaurant reservation want
func NewRestaurantWant(metadata Metadata, spec WantSpec) *RestaurantWant {
	restaurant := &RestaurantWant{
		Want:           Want{},
		RestaurantType: "casual",
		Duration:       2 * time.Hour, // Default 2 hour dinner
	}

	// Initialize base Want fields
	restaurant.Init(metadata, spec)

	if rt, ok := spec.Params["restaurant_type"]; ok {
		if rts, ok := rt.(string); ok {
			restaurant.RestaurantType = rts
		}
	}
	if d, ok := spec.Params["duration_hours"]; ok {
		if df, ok := d.(float64); ok {
			restaurant.Duration = time.Duration(df * float64(time.Hour))
		}
	}

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
func (r *RestaurantWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	restaurantType := "casual"
	if rt, ok := r.Spec.Params["restaurant_type"]; ok {
		if rts, ok := rt.(string); ok {
			restaurantType = rts
		}
	}

	duration := 2 * time.Hour // Default 2 hour dinner
	if d, ok := r.Spec.Params["duration_hours"]; ok {
		if df, ok := d.(float64); ok {
			duration = time.Duration(df * float64(time.Hour))
		}
	}

	// Check if already attempted using persistent state
	attemptedVal, _ := r.GetState("attempted")
	attempted, _ := attemptedVal.(bool)

	if len(outputs) == 0 {
		return true
	}
	out := outputs[0]

	if attempted {
		return true
	}

	// Mark as attempted in persistent state
	r.StoreState("attempted", true)

	// Try to use agent system if available - agent completely overrides normal execution
	if agentSchedule := r.tryAgentExecution(); agentSchedule != nil {
		log.Printf("[RESTAURANT] Agent execution completed, processing agent result\n")

		// Use the agent's schedule result
		r.SetSchedule(*agentSchedule)

		// Send the schedule to output channel
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
		log.Printf("[RESTAURANT] Sent agent-generated schedule: %s from %s to %s\n",
			agentSchedule.ReservationName,
			agentSchedule.ReservationTime.Format("15:04 Jan 2"),
			restaurantEvent.End.Format("15:04 Jan 2"))

		return true
	}

	// Normal restaurant execution (only runs if agent execution didn't return a result)
	log.Printf("[RESTAURANT] Agent execution did not return result, using standard restaurant logic\n")

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
					log.Printf("[RESTAURANT] Conflict detected, retrying at %s\n", dinnerStart.Format("15:04"))
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
	r.StoreState("total_processed", 1)

	// Store live state with reservation details
	r.StoreState("total_processed", 1)
	r.StoreState("reservation_type", restaurantType)
	r.StoreState("reservation_start_time", newEvent.Start.Format("15:04"))
	r.StoreState("reservation_end_time", newEvent.End.Format("15:04"))
	r.StoreState("reservation_duration_hours", duration.Hours())
	r.StoreState("reservation_name", newEvent.Name)
	r.StoreState("schedule_date", baseDate.Format("2006-01-02"))

	log.Printf("[RESTAURANT] Scheduled %s from %s to %s\n",
		newEvent.Name, newEvent.Start.Format("15:04"), newEvent.End.Format("15:04"))

	out <- newSchedule
	return true
}

// tryAgentExecution attempts to execute restaurant reservation using the agent system
// Returns the RestaurantSchedule if successful, nil if no agent execution
func (r *RestaurantWant) tryAgentExecution() *RestaurantSchedule {
	// Check if this want has agent requirements
	if len(r.Spec.Requires) > 0 {
		log.Printf("[RESTAURANT] Want has agent requirements: %v\n", r.Spec.Requires)

		// Store the requirements in want state for tracking
		r.StoreState("agent_requirements", r.Spec.Requires)

		// Step 1: Execute MonitorRestaurant first to check for existing state
		log.Printf("[RESTAURANT] Step 1: Executing MonitorRestaurant to check existing state\n")

		// Create and execute MonitorRestaurant directly inline to avoid context issues
		monitorAgent := NewMonitorRestaurant(
			"restaurant_monitor",
			[]string{"restaurant_agency"},
			[]string{"xxx"},
		)

		ctx := context.Background()
		if err := monitorAgent.Exec(ctx, &r.Want); err != nil {
			log.Printf("[RESTAURANT] MonitorRestaurant execution failed: %v\n", err)
		}

		r.AggregateChanges()

		// Check if MonitorRestaurant found an existing schedule
		if result, exists := r.GetState("agent_result"); exists && result != nil {
			log.Printf("[RESTAURANT] DEBUG: Found agent_result in state, type: %T, value: %+v\n", result, result)
			if schedule, ok := result.(RestaurantSchedule); ok {
				log.Printf("[RESTAURANT] MonitorRestaurant found existing schedule: %+v\n", schedule)
				r.StoreState("execution_source", "monitor")

				// Immediately set the schedule and complete the cycle
				r.SetSchedule(schedule)
				log.Printf("[RESTAURANT] MonitorRestaurant cycle completed - want finished\n")
				return &schedule
			} else {
				log.Printf("[RESTAURANT] DEBUG: agent_result is not RestaurantSchedule type, it's: %T\n", result)
			}
		} else {
			log.Printf("[RESTAURANT] DEBUG: No agent_result found in state - exists: %v, result: %v\n", exists, result)
		}

		// Step 2: No existing schedule found, execute AgentRestaurant
		log.Printf("[RESTAURANT] Step 2: No existing schedule found, executing AgentRestaurant\n")
		if err := r.ExecuteAgents(); err != nil {
			log.Printf("[RESTAURANT] Dynamic agent execution failed: %v, falling back to direct execution\n", err)
			r.StoreState("agent_execution_status", "failed")
			r.StoreState("agent_execution_error", err.Error())
			return nil
		}

		log.Printf("[RESTAURANT] Dynamic agent execution completed successfully\n")
		r.StoreState("agent_execution_status", "completed")
		r.StoreState("execution_source", "agent")

		// Wait for agent to complete and retrieve result
		// Check for agent_result in state
		log.Printf("[RESTAURANT] Checking state for agent_result\n")
		if result, exists := r.GetState("agent_result"); exists && result != nil {
			log.Printf("[RESTAURANT] Found agent_result in state: %+v\n", result)
			if schedule, ok := result.(RestaurantSchedule); ok {
				log.Printf("[RESTAURANT] Successfully retrieved agent result: %+v\n", schedule)
				return &schedule
			} else {
				log.Printf("[RESTAURANT] agent_result is not RestaurantSchedule type: %T\n", result)
			}
		}

		log.Printf("[RESTAURANT] Warning: Agent completed but no result found in state\n")
		return nil
	}

	log.Printf("[RESTAURANT] No agent requirements specified\n")
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

	// Debug: Check if state was stored
	if result, exists := r.GetState("agent_result"); exists {
		log.Printf("[RESTAURANT] DEBUG: MonitorRestaurant stored agent_result: %+v\n", result)
	} else {
		log.Printf("[RESTAURANT] DEBUG: MonitorRestaurant did not store agent_result\n")
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
	paths     Paths
}

// NewHotelWant creates a new hotel reservation want
func NewHotelWant(metadata Metadata, spec WantSpec) *HotelWant {
	hotel := &HotelWant{
		Want:      Want{},
		HotelType: "standard",
		CheckIn:   22 * time.Hour, // 10 PM
		CheckOut:  8 * time.Hour,  // 8 AM next day
	}

	// Initialize base Want fields
	hotel.Init(metadata, spec)

	if ht, ok := spec.Params["hotel_type"]; ok {
		if hts, ok := ht.(string); ok {
			hotel.HotelType = hts
		}
	}

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

func (h *HotelWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	hotelType := "standard"
	if ht, ok := h.Spec.Params["hotel_type"]; ok {
		if hts, ok := ht.(string); ok {
			hotelType = hts
		}
	}

	// Check if already attempted using persistent state
	attemptedVal, _ := h.GetState("attempted")
	attempted, _ := attemptedVal.(bool)

	if len(outputs) == 0 {
		return true
	}
	out := outputs[0]

	if attempted {
		return true
	}

	// Mark as attempted in persistent state
	h.StoreState("attempted", true)

	// Try to use agent system if available - agent completely overrides normal execution
	if agentSchedule := h.tryAgentExecution(); agentSchedule != nil {
		log.Printf("[HOTEL] Agent execution completed, processing agent result\n")

		// Use the agent's schedule result
		h.SetSchedule(*agentSchedule)

		// Send the schedule to output channel
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
		log.Printf("[HOTEL] Sent agent-generated schedule: %s from %s to %s\n",
			agentSchedule.ReservationName,
			agentSchedule.CheckInTime.Format("15:04 Jan 2"),
			agentSchedule.CheckOutTime.Format("15:04 Jan 2"))

		return true
	}

	// Normal hotel execution (only runs if agent execution didn't return a result)
	log.Printf("[HOTEL] Agent execution did not return result, using standard hotel logic\n")

	// Check for existing schedule
	var existingSchedule *TravelSchedule
	if len(using) > 0 {
		select {
		case schedData := <-using[0]:
			if schedule, ok := schedData.(*TravelSchedule); ok {
				existingSchedule = schedule
			}
		default:
			// No input data
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
					log.Printf("[HOTEL] Conflict detected, retrying check-in at %s\n", checkInTime.Format("15:04"))
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
	h.StoreState("total_processed", 1)
	h.StoreState("hotel_type", hotelType)
	h.StoreState("check_in_time", newEvent.Start.Format("15:04 Jan 2"))
	h.StoreState("check_out_time", newEvent.End.Format("15:04 Jan 2"))
	h.StoreState("stay_duration_hours", newEvent.End.Sub(newEvent.Start).Hours())
	h.StoreState("reservation_name", newEvent.Name)

	log.Printf("[HOTEL] Scheduled %s from %s to %s\n",
		newEvent.Name, newEvent.Start.Format("15:04 Jan 2"), newEvent.End.Format("15:04 Jan 2"))

	out <- newSchedule
	return true
}

// tryAgentExecution attempts to execute hotel reservation using the agent system
// Returns the HotelSchedule if successful, nil if no agent execution
func (h *HotelWant) tryAgentExecution() *HotelSchedule {
	// Check if this want has agent requirements
	if len(h.Spec.Requires) > 0 {
		log.Printf("[HOTEL] Want has agent requirements: %v\n", h.Spec.Requires)

		// Store the requirements in want state for tracking
		h.StoreState("agent_requirements", h.Spec.Requires)

		// Use dynamic agent execution based on requirements
		if err := h.ExecuteAgents(); err != nil {
			log.Printf("[HOTEL] Dynamic agent execution failed: %v, falling back to direct execution\n", err)
			h.StoreState("agent_execution_status", "failed")
			h.StoreState("agent_execution_error", err.Error())
			return nil
		}

		log.Printf("[HOTEL] Dynamic agent execution completed successfully\n")
		h.StoreState("agent_execution_status", "completed")

		// Wait for agent to complete and retrieve result
		// Check for agent_result in state
		log.Printf("[HOTEL] Checking state for agent_result\n")
		if result, exists := h.GetState("agent_result"); exists {
			log.Printf("[HOTEL] Found agent_result in state: %+v\n", result)
			if schedule, ok := result.(HotelSchedule); ok {
				log.Printf("[HOTEL] Successfully retrieved agent result: %+v\n", schedule)
				return &schedule
			} else {
				log.Printf("[HOTEL] agent_result is not HotelSchedule type: %T\n", result)
			}
		}

		log.Printf("[HOTEL] Warning: Agent completed but no result found in state\n")
		return nil
	}

	log.Printf("[HOTEL] No agent requirements specified\n")
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
	paths      Paths
}

func NewBuffetWant(metadata Metadata, spec WantSpec) *BuffetWant {
	buffet := &BuffetWant{
		Want:       Want{},
		BuffetType: "continental",
		Duration:   1*time.Hour + 30*time.Minute, // 1.5 hour breakfast
	}

	// Initialize base Want fields
	buffet.Init(metadata, spec)

	if bt, ok := spec.Params["buffet_type"]; ok {
		if bts, ok := bt.(string); ok {
			buffet.BuffetType = bts
		}
	}

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

func (b *BuffetWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	buffetType := "continental"
	if bt, ok := b.Spec.Params["buffet_type"]; ok {
		if bts, ok := bt.(string); ok {
			buffetType = bts
		}
	}

	duration := 1*time.Hour + 30*time.Minute // Default 1.5 hour breakfast

	// Check if already attempted using persistent state
	attemptedVal, _ := b.GetState("attempted")
	attempted, _ := attemptedVal.(bool)

	if len(outputs) == 0 {
		return true
	}
	out := outputs[0]

	if attempted {
		return true
	}

	// Mark as attempted in persistent state
	b.StoreState("attempted", true)

	// Try to use agent system if available - agent completely overrides normal execution
	if agentSchedule := b.tryAgentExecution(); agentSchedule != nil {
		log.Printf("[BUFFET] Agent execution completed, processing agent result\n")

		// Use the agent's schedule result
		b.SetSchedule(*agentSchedule)

		// Send the schedule to output channel
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
		log.Printf("[BUFFET] Sent agent-generated schedule: %s from %s to %s\n",
			agentSchedule.ReservationName,
			agentSchedule.ReservationTime.Format("15:04 Jan 2"),
			buffetEvent.End.Format("15:04 Jan 2"))

		return true
	}

	// Normal buffet execution (only runs if agent execution didn't return a result)
	log.Printf("[BUFFET] Agent execution did not return result, using standard buffet logic\n")

	var existingSchedule *TravelSchedule
	if len(using) > 0 {
		select {
		case schedData := <-using[0]:
			if schedule, ok := schedData.(*TravelSchedule); ok {
				existingSchedule = schedule
			}
		default:
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
					log.Printf("[BUFFET] Conflict detected, retrying at %s\n", buffetStart.Format("15:04"))
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
	b.StoreState("total_processed", 1)

	// Store live state with reservation details
	b.StoreState("total_processed", 1)
	b.StoreState("buffet_type", buffetType)
	b.StoreState("buffet_start_time", newEvent.Start.Format("15:04 Jan 2"))
	b.StoreState("buffet_end_time", newEvent.End.Format("15:04 Jan 2"))
	b.StoreState("buffet_duration_hours", duration.Hours())
	b.StoreState("reservation_name", newEvent.Name)

	log.Printf("[BUFFET] Scheduled %s from %s to %s\n",
		newEvent.Name, newEvent.Start.Format("15:04 Jan 2"), newEvent.End.Format("15:04 Jan 2"))

	out <- newSchedule
	return true
}

// tryAgentExecution attempts to execute buffet reservation using the agent system
// Returns the BuffetSchedule if successful, nil if no agent execution
func (b *BuffetWant) tryAgentExecution() *BuffetSchedule {
	// Check if this want has agent requirements
	if len(b.Spec.Requires) > 0 {
		log.Printf("[BUFFET] Want has agent requirements: %v\n", b.Spec.Requires)

		// Store the requirements in want state for tracking
		b.StoreState("agent_requirements", b.Spec.Requires)

		// Use dynamic agent execution based on requirements
		if err := b.ExecuteAgents(); err != nil {
			log.Printf("[BUFFET] Dynamic agent execution failed: %v, falling back to direct execution\n", err)
			b.StoreState("agent_execution_status", "failed")
			b.StoreState("agent_execution_error", err.Error())
			return nil
		}

		log.Printf("[BUFFET] Dynamic agent execution completed successfully\n")
		b.StoreState("agent_execution_status", "completed")

		// Wait for agent to complete and retrieve result
		// Check for agent_result in state
		log.Printf("[BUFFET] Checking state for agent_result\n")
		if result, exists := b.GetState("agent_result"); exists {
			log.Printf("[BUFFET] Found agent_result in state: %+v\n", result)
			if schedule, ok := result.(BuffetSchedule); ok {
				log.Printf("[BUFFET] Successfully retrieved agent result: %+v\n", schedule)
				return &schedule
			} else {
				log.Printf("[BUFFET] agent_result is not BuffetSchedule type: %T\n", result)
			}
		}

		log.Printf("[BUFFET] Warning: Agent completed but no result found in state\n")
		return nil
	}

	log.Printf("[BUFFET] No agent requirements specified\n")
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

// TravelCoordinatorWant orchestrates the entire travel itinerary
type TravelCoordinatorWant struct {
	Want
	Template string
	paths    Paths
}

func NewTravelCoordinatorWant(metadata Metadata, spec WantSpec) *TravelCoordinatorWant {
	coordinator := &TravelCoordinatorWant{
		Want:     Want{},
		Template: "travel itinerary",
	}

	// Initialize base Want fields
	coordinator.Init(metadata, spec)

	if tmpl, ok := spec.Params["template"]; ok {
		if tmpls, ok := tmpl.(string); ok {
			coordinator.Template = tmpls
		}
	}

	// Set fields for base Want methods
	coordinator.WantType = "travel_coordinator"
	coordinator.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  3,
		RequiredOutputs: 0,
		MaxInputs:       3,
		MaxOutputs:      0,
		WantType:        "travel_coordinator",
		Description:     "Travel itinerary coordinator want",
	}

	return coordinator
}

func (t *TravelCoordinatorWant) GetWant() *Want {
	return &t.Want
}

func (t *TravelCoordinatorWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	if len(using) < 3 {
		return true
	}

	// Use persistent state to track schedules
	schedulesVal, _ := t.GetState("schedules")
	schedules, _ := schedulesVal.([]*TravelSchedule)
	if schedules == nil {
		schedules = make([]*TravelSchedule, 0)
		// Batch initial schedules update
		{
			t.BeginExecCycle()
			t.StoreState("schedules", schedules)
			t.EndExecCycle()
		}
	}

	// Collect all schedules from child wants
	for _, input := range using {
		select {
		case schedData := <-input:
			if schedule, ok := schedData.(*TravelSchedule); ok {
				schedules = append(schedules, schedule)
			}
		default:
			// No more data on this channel
		}
	}

	// Update persistent state with collected schedules
	{
		t.BeginExecCycle()
		t.StoreState("schedules", schedules)
		t.EndExecCycle()
	}

	// When we have all schedules, create final itinerary
	if len(schedules) >= 3 {
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

		// Batch final coordinator state update
		{
			t.BeginExecCycle()
			t.StoreState("total_processed", len(allEvents))
			t.EndExecCycle()
		}
		return true
	}

	return false // Continue waiting for more schedules
}

// RegisterTravelWantTypes registers all travel-related want types
func RegisterTravelWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("flight", func(metadata Metadata, spec WantSpec) interface{} {
		return NewFlightWant(metadata, spec)
	})

	builder.RegisterWantType("restaurant", func(metadata Metadata, spec WantSpec) interface{} {
		return NewRestaurantWant(metadata, spec)
	})

	builder.RegisterWantType("hotel", func(metadata Metadata, spec WantSpec) interface{} {
		return NewHotelWant(metadata, spec)
	})

	builder.RegisterWantType("buffet", func(metadata Metadata, spec WantSpec) interface{} {
		return NewBuffetWant(metadata, spec)
	})

	builder.RegisterWantType("travel_coordinator", func(metadata Metadata, spec WantSpec) interface{} {
		return NewTravelCoordinatorWant(metadata, spec)
	})
}

// RegisterTravelWantTypesWithAgents registers travel want types with agent system support
func RegisterTravelWantTypesWithAgents(builder *ChainBuilder, agentRegistry *AgentRegistry) {
	builder.RegisterWantType("flight", func(metadata Metadata, spec WantSpec) interface{} {
		flight := NewFlightWant(metadata, spec)
		flight.SetAgentRegistry(agentRegistry)
		return flight
	})

	builder.RegisterWantType("restaurant", func(metadata Metadata, spec WantSpec) interface{} {
		restaurant := NewRestaurantWant(metadata, spec)
		restaurant.SetAgentRegistry(agentRegistry)
		return restaurant
	})

	builder.RegisterWantType("hotel", func(metadata Metadata, spec WantSpec) interface{} {
		hotel := NewHotelWant(metadata, spec)
		hotel.SetAgentRegistry(agentRegistry)
		return hotel
	})

	builder.RegisterWantType("buffet", func(metadata Metadata, spec WantSpec) interface{} {
		buffet := NewBuffetWant(metadata, spec)
		buffet.SetAgentRegistry(agentRegistry)
		return buffet
	})

	builder.RegisterWantType("travel_coordinator", func(metadata Metadata, spec WantSpec) interface{} {
		return NewTravelCoordinatorWant(metadata, spec)
	})
}
