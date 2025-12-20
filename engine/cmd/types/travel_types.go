package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/src"
	"strings"
	"time"
)

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

// TravelWantLocalsInterface is a marker interface for all travel want locals
type TravelWantLocalsInterface interface{}

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

// IsAchieved checks if the travel want has been achieved
func (b *BaseTravelWant) IsAchieved() bool {
	attempted, _ := b.GetStateBool("attempted", false)
	return attempted
}

// Progress implements Progressable for all travel wants
func (b *BaseTravelWant) Progress() {
	b.StoreState("attempted", true)

	if b.executor == nil {
		b.StoreLog("ERROR: executor not initialized")
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
		b.StoreLog("ERROR: Failed to access TravelWantLocalsInterface from Want.Locals")
		return
	}
	_, connectionAvailable := b.GetFirstOutputChannel()
	schedule := b.executor.generateSchedule(locals)
	if schedule != nil && connectionAvailable {
		b.Provide(schedule)
	} else if schedule == nil {
		b.StoreLog("ERROR: Failed to generate schedule")
	} else if !connectionAvailable {
		b.StoreLog("WARNING: Output channel not available, schedule not sent")
	}
}

// CalculateAchievingPercentage returns progress percentage
func (b *BaseTravelWant) CalculateAchievingPercentage() int {
	attempted, _ := b.GetStateBool("attempted", false)
	if attempted {
		return 100
	}
	return 0
}

// RestaurantWant creates dinner restaurant reservations
type RestaurantWant struct {
	BaseTravelWant
}

// NewRestaurantWant creates a new restaurant reservation want
func NewRestaurantWant(metadata Metadata, spec WantSpec) Progressable {
	locals := &RestaurantWantLocals{}
	want := NewWantWithLocals(
		metadata,
		spec,
		locals,
		"restaurant",
	)
	restaurantWant := &RestaurantWant{
		BaseTravelWant: BaseTravelWant{Want: *want},
	}
	// Set executor to self for interface method dispatch
	restaurantWant.BaseTravelWant.executor = restaurantWant
	return restaurantWant
}

// tryAgentExecution implements TravelWantInterface for RestaurantWant
func (r *RestaurantWant) tryAgentExecution() any {
	if len(r.Spec.Requires) > 0 {
		r.StoreState("agent_requirements", r.Spec.Requires)

		// Step 1: Execute MonitorRestaurant first to check for existing state
		monitorAgent := NewMonitorRestaurant(
			"restaurant_monitor",
			[]string{"restaurant_agency"},
			[]string{"xxx"},
		)

		ctx := context.Background()
		if err := monitorAgent.Exec(ctx, &r.Want); err != nil {
		}

		r.AggregateChanges()
		if schedule, ok := GetStateAs[RestaurantSchedule](&r.Want, "agent_result"); ok {
			r.StoreState("execution_source", "monitor")

			// Immediately set the schedule and complete the cycle
			r.SetSchedule(schedule)
			return &schedule
		}

		// Step 2: No existing schedule found, execute AgentRestaurant
		if err := r.ExecuteAgents(); err != nil {
			r.StoreState("agent_execution_status", "failed")
			r.StoreState("agent_execution_error", err.Error())
			return nil
		}

		r.StoreState("agent_execution_status", "completed")
		r.StoreState("execution_source", "agent")

		// Wait for agent to complete and retrieve result Check for agent_result in state
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
		Date:   baseDate,
		Events: []TimeSlot{newEvent},
	}
	r.StoreStateMulti(map[string]any{
		"total_processed":            1,
		"reservation_type":           restaurantLocals.RestaurantType,
		"reservation_start_time":     newEvent.Start.Format("15:04"),
		"reservation_end_time":       newEvent.End.Format("15:04"),
		"reservation_duration_hours": restaurantLocals.Duration.Hours(),
		"reservation_name":           newEvent.Name,
		"schedule_date":              baseDate.Format("2006-01-02"),
		"achieving_percentage":       100,
		"finalResult":                newEvent.Name,
	})
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

	stateUpdates := map[string]any{
		"attempted":                  true,
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

// NewHotelWant creates a new hotel reservation want
func NewHotelWant(metadata Metadata, spec WantSpec) Progressable {
	locals := &HotelWantLocals{}
	want := NewWantWithLocals(
		metadata,
		spec,
		locals,
		"hotel",
	)
	hotelWant := &HotelWant{
		BaseTravelWant: BaseTravelWant{Want: *want},
	}
	// Set executor to self for interface method dispatch
	hotelWant.BaseTravelWant.executor = hotelWant
	return hotelWant
}

// tryAgentExecution implements TravelWantInterface for HotelWant
func (h *HotelWant) tryAgentExecution() any {
	if len(h.Spec.Requires) > 0 {
	h.StoreState("agent_requirements", h.Spec.Requires)

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
		Date:   baseDate,
		Events: []TimeSlot{newEvent},
	}
	h.StoreStateMulti(map[string]any{
		"total_processed":      1,
		"hotel_type":           hotelLocals.HotelType,
		"check_in_time":        newEvent.Start.Format("15:04 Jan 2"),
		"check_out_time":       newEvent.End.Format("15:04 Jan 2"),
		"stay_duration_hours":  newEvent.End.Sub(newEvent.Start).Hours(),
		"reservation_name":     newEvent.Name,
		"achieving_percentage": 100,
	})
	return newSchedule
}

// BuffetWant creates breakfast buffet reservations
type BuffetWant struct {
	BaseTravelWant
}

func NewBuffetWant(metadata Metadata, spec WantSpec) Progressable {
	locals := &BuffetWantLocals{}
	want := NewWantWithLocals(
		metadata,
		spec,
		locals,
		"buffet",
	)
	buffetWant := &BuffetWant{
		BaseTravelWant: BaseTravelWant{Want: *want},
	}
	// Set executor to self for interface method dispatch
	buffetWant.BaseTravelWant.executor = buffetWant
	return buffetWant
}

// tryAgentExecution implements TravelWantInterface for BuffetWant
func (b *BuffetWant) tryAgentExecution() any {
	if len(b.Spec.Requires) > 0 {
	b.StoreState("agent_requirements", b.Spec.Requires)

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
		Date:   nextDay,
		Events: []TimeSlot{newEvent},
	}
	b.StoreStateMulti(map[string]any{
		"total_processed":        1,
		"buffet_type":            buffetLocals.BuffetType,
		"buffet_start_time":      newEvent.Start.Format("15:04 Jan 2"),
		"buffet_end_time":        newEvent.End.Format("15:04 Jan 2"),
		"buffet_duration_hours":  buffetLocals.Duration.Hours(),
		"reservation_name":       newEvent.Name,
		"achieving_percentage":   100,
	})
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

	stateUpdates := map[string]any{
		"attempted":               true,
		"buffet_start_time":       s.ReservationTime.Format("15:04 Jan 2"),
		"buffet_end_time":         s.ReservationTime.Add(time.Duration(s.DurationHours * float64(time.Hour))).Format("15:04 Jan 2"),
		"buffet_type":             s.BuffetType,
		"buffet_duration_hours":   s.DurationHours,
		"reservation_name":        s.ReservationName,
		"total_processed":         1,
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

	stateUpdates := map[string]any{
		"attempted":           true,
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

// TravelCoordinatorWant orchestrates the entire travel itinerary RegisterTravelWantTypes registers all travel-related want types Note: All coordinators now use the unified "coordinator" type Configuration is determined by parameters (is_buffet, required_inputs, etc.)
func RegisterTravelWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("flight", NewFlightWant)
	builder.RegisterWantType("restaurant", NewRestaurantWant)
	builder.RegisterWantType("hotel", NewHotelWant)
	builder.RegisterWantType("buffet", NewBuffetWant)
	builder.RegisterWantType("coordinator", NewCoordinatorWant)
}

// RegisterTravelWantTypesWithAgents registers travel want types with agent system support
func RegisterTravelWantTypesWithAgents(builder *ChainBuilder, agentRegistry *AgentRegistry) {
	builder.RegisterWantType("flight", func(metadata Metadata, spec WantSpec) Progressable {
		result := NewFlightWant(metadata, spec)
		fw := result.(*FlightWant)
		fw.SetAgentRegistry(agentRegistry)
		return result
	})

	builder.RegisterWantType("restaurant", func(metadata Metadata, spec WantSpec) Progressable {
		result := NewRestaurantWant(metadata, spec)
		rw := result.(*RestaurantWant)
		rw.SetAgentRegistry(agentRegistry)
		return result
	})

	builder.RegisterWantType("hotel", func(metadata Metadata, spec WantSpec) Progressable {
		result := NewHotelWant(metadata, spec)
		hw := result.(*HotelWant)
		hw.SetAgentRegistry(agentRegistry)
		return result
	})

	builder.RegisterWantType("buffet", func(metadata Metadata, spec WantSpec) Progressable {
		result := NewBuffetWant(metadata, spec)
		bw := result.(*BuffetWant)
		bw.SetAgentRegistry(agentRegistry)
		return result
	})

	builder.RegisterWantType("coordinator", NewCoordinatorWant)
}