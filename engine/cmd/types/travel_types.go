package types

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
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

// ============================================================================
// FlightWant Implementation (migrated from flight_types.go)
// ============================================================================

// FlightMonitoringAgent implements BackgroundAgent for continuous flight status monitoring
type FlightMonitoringAgent struct {
	id       string
	monitor  *MonitorFlightAPI
	ticker   *time.Ticker
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	want     *Want
}

// ID returns the agent's unique identifier
func (f *FlightMonitoringAgent) ID() string {
	return f.id
}

// Start begins the flight monitoring goroutine
func (f *FlightMonitoringAgent) Start(ctx context.Context, w *Want) error {
	f.want = w
	f.ctx, f.cancel = context.WithCancel(ctx)
	f.ticker = time.NewTicker(10 * time.Second)
	f.done = make(chan struct{})

	go func() {
		defer f.ticker.Stop()
		defer close(f.done)

		for {
			select {
			case <-f.ctx.Done():
				return
			case <-f.ticker.C:
				// Monitor flight status
				f.BeginProgressCycle()
				f.monitor.Exec(f.ctx, f.want)
				f.EndProgressCycle()
			}
		}
	}()

	return nil
}

// Stop gracefully stops the flight monitoring
func (f *FlightMonitoringAgent) Stop() error {
	if f.cancel != nil {
		f.cancel()
	}
	if f.done != nil {
		select {
		case <-f.done:
			// Already done
		case <-time.After(1 * time.Second):
			// Timeout waiting for goroutine to stop
		}
	}
	return nil
}

// BeginProgressCycle wraps want execution for proper state management
func (f *FlightMonitoringAgent) BeginProgressCycle() {
	if f.want != nil {
		f.want.BeginProgressCycle()
	}
}

// EndProgressCycle completes the progress cycle
func (f *FlightMonitoringAgent) EndProgressCycle() {
	if f.want != nil {
		f.want.EndProgressCycle()
	}
}

// NewFlightMonitoringAgent creates a new flight monitoring background agent
// Initializes MonitorFlightAPI internally with proper configuration
func NewFlightMonitoringAgent(flightID, serverURL string) *FlightMonitoringAgent {
	agentID := "flight-monitor-" + flightID
	// Initialize MonitorFlightAPI with agent configuration
	monitor := &MonitorFlightAPI{
		MonitorAgent: MonitorAgent{
			BaseAgent: BaseAgent{
				Name:         agentID,
				Capabilities: []string{},
				Uses:         []string{},
				Type:         MonitorAgentType,
			},
		},
		ServerURL:           serverURL,
		PollInterval:        10 * time.Second,
		StatusChangeHistory: make([]StatusChange, 0),
	}

	return &FlightMonitoringAgent{
		id:      agentID,
		monitor: monitor,
	}
}

// FlightWantLocals holds type-specific local state for FlightWant
type FlightWantLocals struct {
	FlightType          string
	Duration            time.Duration
	DepartureDate       string        // Departure date in YYYY-MM-DD format
	monitoringStartTime time.Time
	monitoringDuration  time.Duration // How long to monitor for status changes
	monitoringActive    bool          // Whether monitoring is currently active
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

// MonitorFlightAPI extends MonitorAgent to poll flight status from mock server
type MonitorFlightAPI struct {
	MonitorAgent
	ServerURL             string
	PollInterval          time.Duration
	LastPollTime          time.Time
	LastKnownStatus       string
	StatusChangeHistory   []StatusChange
	LastRecordedStateHash string // Track last recorded state to avoid duplicate history entries
}

// Exec polls the mock server for flight status updates NOTE: This agent runs ONE TIME per ExecuteAgents() call The continuous polling loop is handled by the Want's Progress method (FlightWant) Individual agents should NOT implement their own polling loops
func (m *MonitorFlightAPI) Exec(ctx context.Context, want *Want) error {
	flightID, exists := want.GetState("flight_id")
	if !exists {
		return fmt.Errorf("no flight_id found in state - flight not created yet")
	}

	flightIDStr, ok := flightID.(string)
	if !ok {
		return fmt.Errorf("flight_id is not a string")
	}

	// Skip monitoring if flight_id is empty (flight cancellation/rebooking in progress)
	if flightIDStr == "" {
		want.StoreLog("Skipping monitoring: flight_id is empty (cancellation/rebooking in progress)")
		return nil
	}
	now := time.Now()
	if !m.LastPollTime.IsZero() && now.Sub(m.LastPollTime) < m.PollInterval {
		// Skip this polling cycle - wait for PollInterval to elapse
		return nil
	}

	// Record this poll time for next interval check
	m.LastPollTime = now

	// Restore last known status from want state for persistence across execution cycles
	if lastStatus, exists := want.GetState("flight_status"); exists {
		if lastStatusStr, ok := lastStatus.(string); ok {
			m.LastKnownStatus = lastStatusStr
		}
	} else {
		m.LastKnownStatus = "unknown" // Default if not found in state
	}

	// Restore status history from want state for persistence Do NOT clear history - it accumulates across multiple monitoring executions
	if historyI, exists := want.GetState("status_history"); exists {
		if historyStrs, ok := historyI.([]any); ok {
			for _, entryI := range historyStrs {
				if entry, ok := entryI.(string); ok {
					if parsed, ok := parseStatusHistoryEntry(entry); ok {
						// Only add if not already in history
						found := false
						for _, existing := range m.StatusChangeHistory {
							if existing.OldStatus == parsed.OldStatus && existing.NewStatus == parsed.NewStatus && existing.Details == parsed.Details {
								found = true
								break
							}
						}
						if !found {
							m.StatusChangeHistory = append(m.StatusChangeHistory, parsed)
						}
					}
				}
			}
		} else if historyStrs, ok := historyI.([]string); ok {
			for _, entry := range historyStrs {
				if parsed, ok := parseStatusHistoryEntry(entry); ok {
					// Only add if not already in history
					found := false
					for _, existing := range m.StatusChangeHistory {
						if existing.OldStatus == parsed.OldStatus && existing.NewStatus == parsed.NewStatus && existing.Details == parsed.Details {
							found = true
							break
						}
					}
					if !found {
						m.StatusChangeHistory = append(m.StatusChangeHistory, parsed)
					}
				}
			}
		}
	}
	url := fmt.Sprintf("%s/api/flights/%s", m.ServerURL, flightIDStr)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get flight status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get flight: status %d, body: %s", resp.StatusCode, string(body))
	}
	var reservation FlightReservation
	if err := json.NewDecoder(resp.Body).Decode(&reservation); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}
	newStatus := reservation.Status
	oldStatus := m.LastKnownStatus
	hasStateChange := newStatus != oldStatus

	// Calculate hash of current reservation data for differential history
	currentStateJSON, _ := json.Marshal(reservation)
	currentStateHash := fmt.Sprintf("%x", md5.Sum(currentStateJSON))

	// Only update state if state has actually changed (differential history) NOTE: Exec cycle wrapping is handled by the agent execution framework in want_agent.go Individual agents should NOT call BeginExecCycle/EndExecCycle
	if hasStateChange || currentStateHash != m.LastRecordedStateHash {
		updates := map[string]any{
			"flight_id":      reservation.ID,
			"flight_number":  reservation.FlightNumber,
			"from":           reservation.From,
			"to":             reservation.To,
			"departure_time": reservation.DepartureTime.Format(time.RFC3339),
			"arrival_time":   reservation.ArrivalTime.Format(time.RFC3339),
			"status_message": reservation.StatusMessage,
			"updated_at":     reservation.UpdatedAt.Format(time.RFC3339),
		}

		if hasStateChange {
			want.StoreLog(fmt.Sprintf("Status changed: %s -> %s", oldStatus, newStatus))

			// Record status change
			statusChange := StatusChange{
				Timestamp: time.Now(),
				OldStatus: oldStatus,
				NewStatus: newStatus,
				Details:   reservation.StatusMessage,
			}
			m.StatusChangeHistory = append(m.StatusChangeHistory, statusChange)

			updates["flight_status"] = newStatus
			updates["status_changed"] = true
			updates["status_changed_at"] = time.Now().Format(time.RFC3339)
			updates["status_change_history_count"] = len(m.StatusChangeHistory)

			// Record activity description for agent history
			activity := fmt.Sprintf("Flight status updated: %s → %s for flight %s (%s)",
				oldStatus, newStatus, reservation.FlightNumber, reservation.StatusMessage)
			want.SetAgentActivity(m.Name, activity)
			schedule := FlightSchedule{
				DepartureTime:   reservation.DepartureTime,
				ArrivalTime:     reservation.ArrivalTime,
				FlightNumber:    reservation.FlightNumber,
				ReservationName: fmt.Sprintf("Flight %s from %s to %s", reservation.FlightNumber, reservation.From, reservation.To),
			}
			updates["agent_result"] = schedule
			want.StoreLog(fmt.Sprintf("[PACKET-SEND] Flight schedule packet: FlightNumber=%s, From=%s, To=%s, Status=%s",
				schedule.FlightNumber, reservation.From, reservation.To, newStatus))
			statusHistoryStrs := make([]string, 0)
			for _, change := range m.StatusChangeHistory {
				historyEntry := fmt.Sprintf("%s: %s -> %s (%s)",
					change.Timestamp.Format("15:04:05"),
					change.OldStatus,
					change.NewStatus,
					change.Details)
				statusHistoryStrs = append(statusHistoryStrs, historyEntry)
			}
			updates["status_history"] = statusHistoryStrs

			m.LastKnownStatus = newStatus

			// Print status progression
			want.StoreLog(fmt.Sprintf("FLIGHT %s STATUS PROGRESSION: %s (at %s)",
				reservation.ID, newStatus, time.Now().Format("15:04:05")))

			// Update hash after successful commit
			m.LastRecordedStateHash = currentStateHash
			want.StoreLog(fmt.Sprintf("State recorded (hash: %s)", currentStateHash[:8]))
		} else {
			// No status change - don't create history entry, but still update other flight details Removed verbose log: "Flight details changed but status is still: ..."
			m.LastRecordedStateHash = currentStateHash
		}
		// Use StoreStateForAgent for background agent updates (separate from Want progress cycle)
		for key, value := range updates {
			want.StoreStateForAgent(key, value)
		}
	}

	return nil
}

// GetStatusChangeHistory returns the status change history
func (m *MonitorFlightAPI) GetStatusChangeHistory() []StatusChange {
	return m.StatusChangeHistory
}

// WasStatusChanged checks if status has changed since last check
func (m *MonitorFlightAPI) WasStatusChanged() bool {
	return len(m.StatusChangeHistory) > 0
}

// parseStatusHistoryEntry parses a status history entry string
func parseStatusHistoryEntry(entry string) (StatusChange, bool) {
	colonIdx := findFirstColon(entry)
	if colonIdx < 0 || colonIdx+2 >= len(entry) {
		return StatusChange{}, false
	}

	// Extract timestamp part (before first colon)
	timestampStr := entry[:colonIdx]
	rest := strings.TrimSpace(entry[colonIdx+1:])
	arrowIdx := strings.Index(rest, " -> ")
	if arrowIdx < 0 {
		return StatusChange{}, false
	}

	// Extract old status (after colon, before arrow)
	oldStatus := strings.TrimSpace(rest[:arrowIdx])
	afterArrow := strings.TrimSpace(rest[arrowIdx+4:])
	parenIdx := strings.Index(afterArrow, "(")
	if parenIdx < 0 {
		return StatusChange{}, false
	}

	// Extract new status (after arrow, before parenthesis)
	newStatus := strings.TrimSpace(afterArrow[:parenIdx])

	// Extract details (inside parentheses)
	detailsPart := strings.TrimSpace(afterArrow[parenIdx:])
	if len(detailsPart) < 2 || !strings.HasPrefix(detailsPart, "(") || !strings.HasSuffix(detailsPart, ")") {
		return StatusChange{}, false
	}
	details := strings.TrimSpace(detailsPart[1 : len(detailsPart)-1])
	parsedTime, err := time.Parse("15:04:05", timestampStr)
	if err != nil {
		parsedTime = time.Now() // Fallback
	}

	return StatusChange{
		Timestamp: parsedTime,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Details:   details,
	}, true
}

// findFirstColon finds the first colon in a string
func findFirstColon(s string) int {
	for i, ch := range s {
		if ch == ':' {
			return i
		}
	}
	return -1
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

// FlightWant creates flight booking reservations
type FlightWant struct {
	BaseTravelWant
}

// NewFlightWant creates a new flight booking want
func NewFlightWant(metadata Metadata, spec WantSpec) Progressable {
	locals := &FlightWantLocals{
		monitoringDone: make(chan struct{}),
	}
	want := NewWantWithLocals(metadata, spec, locals, "flight")
	flightWant := &FlightWant{
		BaseTravelWant: BaseTravelWant{Want: *want},
	}
	flightWant.BaseTravelWant.executor = flightWant
	return flightWant
}

// IsAchieved checks if flight booking is complete (all phases finished)
func (f *FlightWant) IsAchieved() bool {
	phaseVal, _ := f.GetState("flight_phase")
	phase, _ := phaseVal.(string)
	return phase == PhaseCompleted
}

// GetLocals returns the FlightWantLocals from this want
func (f *FlightWant) GetLocals() *FlightWantLocals {
	return f.BaseTravelWant.Want.GetLocals().(*FlightWantLocals)
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
		f.StoreLog(fmt.Sprintf("agent_result is unexpected type: %T", result))
		return nil
	}
}

// Progress creates a flight booking reservation using state machine pattern
// The execution flow follows distinct phases: 1. Initial: Setup phase
// 2. Booking: Execute initial flight booking via agents
// 3. Monitoring: Monitor flight status for 60 seconds
// 4. Canceling: Wait for cancellation agent to complete
// 5. Rebooking: Execute rebooking after cancellation
// 6. Completed: Final state
func (f *FlightWant) Progress() {
	locals := f.GetLocals()
	if locals == nil {
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
				f.StoreStateMulti(map[string]any{
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
			f.StoreStateMulti(map[string]any{
				"flight_phase": PhaseRebooking,
				"attempted":    false,
			})
			return
		}

		flightID, ok := flightIDVal.(string)
		if !ok {
			f.StoreLog("Invalid flight_id type, transitioning to rebooking")
			f.StoreStateMulti(map[string]any{
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
		f.StoreStateMulti(map[string]any{
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
				// Batch all phase transition state changes together
				f.StoreStateMulti(map[string]any{
					"flight_phase": PhaseMonitoring,
				})
				f.StoreLog("Transitioning back to monitoring phase for rebooked flight")

				return
			}
		}

		// Rebooking failed - complete
		f.StoreLog("Rebooking failed")
		f.StoreStateMulti(map[string]any{
			"flight_phase": PhaseCompleted,
		})
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

func (f *FlightWant) sendFlightPacket(out any, schedule *FlightSchedule, label string) {
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
	f.Provide(travelSchedule)

	f.StoreLog(fmt.Sprintf("[PACKET-SEND] %s flight: %s (%s to %s) | TravelSchedule: Date=%s, Events=%d",
		label,
		schedule.ReservationName,
		schedule.DepartureTime.Format("15:04 Jan 2"),
		schedule.ArrivalTime.Format("15:04 Jan 2"),
		travelSchedule.Date.Format("2006-01-02"),
		len(travelSchedule.Events)))
}

// tryAgentExecution implements TravelWantInterface for FlightWant
// Attempts to execute flight booking using the agent system
func (f *FlightWant) tryAgentExecution() any {
	if len(f.Spec.Requires) > 0 {
		f.StoreState("agent_requirements", f.Spec.Requires)

		// Execute agents via ExecuteAgents() which properly tracks agent history
		if err := f.ExecuteAgents(); err != nil {
			f.StoreStateMulti(map[string]any{
				"agent_execution_status": "failed",
				"agent_execution_error":  err.Error(),
			})
			return nil
		}

		f.StoreState("agent_execution_status", "completed")
		if result, exists := f.GetState("agent_result"); exists && result != nil {
			f.StoreState("execution_source", "agent")

			// Start background monitoring for this flight
			flightIDVal, _ := f.GetState("flight_id")
			flightID, _ := flightIDVal.(string)

			params := f.Spec.Params
			serverURL, ok := params["server_url"].(string)
			if !ok || serverURL == "" {
				serverURL = "http://localhost:8081"
			}

			// Create and add background monitoring agent
			monitorAgent := NewFlightMonitoringAgent(flightID, serverURL)
			if err := f.AddBackgroundAgent(monitorAgent); err != nil {
				f.StoreLog(fmt.Sprintf("ERROR: Failed to start background monitoring: %v", err))
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

	stateUpdates := map[string]any{
		"attempted":             true,
		"departure_time":        s.DepartureTime.Format("15:04 Jan 2"),
		"arrival_time":          s.ArrivalTime.Format("15:04 Jan 2"),
		"flight_type":           s.FlightType,
		"flight_duration_hours": s.ArrivalTime.Sub(s.DepartureTime).Hours(),
		"flight_number":         s.FlightNumber,
		"reservation_name":      s.ReservationName,
		"total_processed":       1,
		"schedule_date":         s.DepartureTime.Format("2006-01-02"),
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