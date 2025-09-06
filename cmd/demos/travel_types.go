package main

import (
	"fmt"
	"math/rand"
	"time"
	. "mywant/src"
	"mywant/src/chain"
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
	Event1    TimeSlot
	Event2    TimeSlot
	Resolved  bool
	Attempts  int
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
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		RestaurantType: "casual",
		Duration:       2 * time.Hour, // Default 2 hour dinner
	}

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

	return restaurant
}

// GetConnectivityMetadata returns connectivity requirements
func (r *RestaurantWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		WantType:        "restaurant",
		Description:     "Restaurant reservation scheduling want",
	}
}

func (r *RestaurantWant) InitializePaths(inCount, outCount int) {
	r.paths.In = make([]PathInfo, inCount)
	r.paths.Out = make([]PathInfo, outCount)
}

func (r *RestaurantWant) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return r.Stats
}

func (r *RestaurantWant) Process(paths Paths) bool {
	r.paths = paths
	return false
}

func (r *RestaurantWant) GetType() string {
	return "restaurant"
}

func (r *RestaurantWant) GetWant() *Want {
	return &r.Want
}

// CreateFunction creates a restaurant reservation
func (r *RestaurantWant) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	attempted := false

	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]

		if attempted {
			return true
		}
		attempted = true

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
			End:   dinnerStart.Add(r.Duration),
			Type:  "restaurant",
			Name:  fmt.Sprintf("%s dinner at %s restaurant", r.Metadata.Name, r.RestaurantType),
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
						newEvent.End = dinnerStart.Add(r.Duration)
						fmt.Printf("[RESTAURANT] Conflict detected, retrying at %s\n", dinnerStart.Format("15:04"))
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

		// Initialize stats map if not exists
		if r.Stats == nil {
			r.Stats = make(WantStats)
		}
		r.Stats["total_processed"] = 1
		fmt.Printf("[RESTAURANT] Scheduled %s from %s to %s\n",
			newEvent.Name, newEvent.Start.Format("15:04"), newEvent.End.Format("15:04"))

		out <- newSchedule
		return true
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
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		HotelType: "standard",
		CheckIn:   22 * time.Hour, // 10 PM
		CheckOut:  8 * time.Hour,  // 8 AM next day
	}

	if ht, ok := spec.Params["hotel_type"]; ok {
		if hts, ok := ht.(string); ok {
			hotel.HotelType = hts
		}
	}

	return hotel
}

func (h *HotelWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		WantType:        "hotel",
		Description:     "Hotel reservation scheduling want",
	}
}

func (h *HotelWant) InitializePaths(inCount, outCount int) {
	h.paths.In = make([]PathInfo, inCount)
	h.paths.Out = make([]PathInfo, outCount)
}

func (h *HotelWant) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return h.Stats
}

func (h *HotelWant) Process(paths Paths) bool {
	h.paths = paths
	return false
}

func (h *HotelWant) GetType() string {
	return "hotel"
}

func (h *HotelWant) GetWant() *Want {
	return &h.Want
}

func (h *HotelWant) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	attempted := false

	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]

		if attempted {
			return true
		}
		attempted = true

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
			Name:  fmt.Sprintf("%s stay at %s hotel", h.Metadata.Name, h.HotelType),
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
						fmt.Printf("[HOTEL] Conflict detected, retrying check-in at %s\n", checkInTime.Format("15:04"))
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

		// Initialize stats map if not exists
		if h.Stats == nil {
			h.Stats = make(WantStats)
		}
		h.Stats["total_processed"] = 1
		fmt.Printf("[HOTEL] Scheduled %s from %s to %s\n",
			newEvent.Name, newEvent.Start.Format("15:04 Jan 2"), newEvent.End.Format("15:04 Jan 2"))

		out <- newSchedule
		return true
	}
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
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		BuffetType: "continental",
		Duration:   1*time.Hour + 30*time.Minute, // 1.5 hour breakfast
	}

	if bt, ok := spec.Params["buffet_type"]; ok {
		if bts, ok := bt.(string); ok {
			buffet.BuffetType = bts
		}
	}

	return buffet
}

func (b *BuffetWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		WantType:        "buffet",
		Description:     "Breakfast buffet scheduling want",
	}
}

func (b *BuffetWant) InitializePaths(inCount, outCount int) {
	b.paths.In = make([]PathInfo, inCount)
	b.paths.Out = make([]PathInfo, outCount)
}

func (b *BuffetWant) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return b.Stats
}

func (b *BuffetWant) Process(paths Paths) bool {
	b.paths = paths
	return false
}

func (b *BuffetWant) GetType() string {
	return "buffet"
}

func (b *BuffetWant) GetWant() *Want {
	return &b.Want
}

func (b *BuffetWant) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	attempted := false

	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]

		if attempted {
			return true
		}
		attempted = true

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
			End:   buffetStart.Add(b.Duration),
			Type:  "buffet",
			Name:  fmt.Sprintf("%s %s breakfast buffet", b.Metadata.Name, b.BuffetType),
		}

		if existingSchedule != nil {
			for attempt := 0; attempt < 3; attempt++ {
				conflict := false
				for _, event := range existingSchedule.Events {
					if b.hasTimeConflict(newEvent, event) {
						conflict = true
						buffetStart = buffetStart.Add(30 * time.Minute)
						newEvent.Start = buffetStart
						newEvent.End = buffetStart.Add(b.Duration)
						fmt.Printf("[BUFFET] Conflict detected, retrying at %s\n", buffetStart.Format("15:04"))
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

		// Initialize stats map if not exists
		if b.Stats == nil {
			b.Stats = make(WantStats)
		}
		b.Stats["total_processed"] = 1
		fmt.Printf("[BUFFET] Scheduled %s from %s to %s\n",
			newEvent.Name, newEvent.Start.Format("15:04 Jan 2"), newEvent.End.Format("15:04 Jan 2"))

		out <- newSchedule
		return true
	}
}

// Helper function to check time conflicts
func (r *RestaurantWant) hasTimeConflict(event1, event2 TimeSlot) bool {
	return event1.Start.Before(event2.End) && event2.Start.Before(event1.End)
}

func (h *HotelWant) hasTimeConflict(event1, event2 TimeSlot) bool {
	return event1.Start.Before(event2.End) && event2.Start.Before(event1.End)
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
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		Template: "travel itinerary",
	}

	if tmpl, ok := spec.Params["template"]; ok {
		if tmpls, ok := tmpl.(string); ok {
			coordinator.Template = tmpls
		}
	}

	return coordinator
}

func (t *TravelCoordinatorWant) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  3, // restaurant, hotel, buffet
		RequiredOutputs: 0, // final output
		MaxInputs:       3,
		MaxOutputs:      0,
		WantType:        "travel_coordinator",
		Description:     "Travel itinerary coordinator want",
	}
}

func (t *TravelCoordinatorWant) InitializePaths(inCount, outCount int) {
	t.paths.In = make([]PathInfo, inCount)
	t.paths.Out = make([]PathInfo, outCount)
}

func (t *TravelCoordinatorWant) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return t.Stats
}

func (t *TravelCoordinatorWant) Process(paths Paths) bool {
	t.paths = paths
	return false
}

func (t *TravelCoordinatorWant) GetType() string {
	return "travel_coordinator"
}

func (t *TravelCoordinatorWant) GetWant() *Want {
	return &t.Want
}

func (t *TravelCoordinatorWant) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	schedules := make([]*TravelSchedule, 0)

	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(using) < 3 {
			return true
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

		// When we have all schedules, create final itinerary
		if len(schedules) >= 3 {
			fmt.Printf("\n🗓️  Final %s:\n", t.Template)
			fmt.Printf("=================================\n")

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

			for _, event := range allEvents {
				fmt.Printf("📅 %s: %s - %s\n",
					event.Type, event.Start.Format("Mon 15:04"), event.End.Format("15:04"))
				fmt.Printf("   %s\n", event.Name)
			}

			// Initialize stats map if not exists
			if t.Stats == nil {
				t.Stats = make(WantStats)
			}
			t.Stats["total_processed"] = len(allEvents)
			fmt.Printf("\n✅ Travel itinerary completed with %d events!\n", len(allEvents))
			return true
		}

		return false // Continue waiting for more schedules
	}
}

// RegisterTravelWantTypes registers all travel-related want types
func RegisterTravelWantTypes(builder *ChainBuilder) {
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