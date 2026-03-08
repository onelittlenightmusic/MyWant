package types
import (
	"encoding/json"
	"fmt"
	"math/rand"
	. "mywant/engine/core"
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

type BaseTravelWantLocals struct {
	// Common fields for travel want locals
}

// TravelWantInterface defines methods that specific travel wants must implement
type TravelWantInterface interface {
	tryAgentExecution() any // Returns *RestaurantSchedule, *HotelSchedule, or *BuffetSchedule
	generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule
	SetSchedule(schedule any)
	formatResult(result any) string
}

// BaseTravelWant provides shared functionality for all travel-related wants
type BaseTravelWant struct {
	Want
	executor TravelWantInterface
}

// Initialize resets state before execution.
func (b *BaseTravelWant) Initialize() {
	if cancelReq, _ := b.GetStateBool("_cancel_requested", false); cancelReq {
		b.SetCurrent("completed", false)
		return
	}
	thinkerID := conditionThinkerAgentName + "-" + b.Want.Metadata.ID
	if _, running := b.Want.GetBackgroundAgent(thinkerID); !running {
		b.SetCurrent("good_to_reserve", true)
	}
}

// IsAchieved checks if the travel want has been achieved
func (b *BaseTravelWant) IsAchieved() bool {
	completed, _ := b.GetStateBool("completed", false)
	return completed
}

// Progress implements Progressable for all travel wants
func (b *BaseTravelWant) Progress() {
	if cancelReq, _ := b.GetStateBool("_cancel_requested", false); cancelReq {
		b.Want.MergeParentState(map[string]any{
			"costs": map[string]any{b.Want.Metadata.Name: 0.0},
		})
		b.SetInternal("_cancel_requested", false)
		b.SetCurrent("cancelled", true)
		b.StoreLog("[CANCEL] Self-cancelling as requested by itinerary")
		b.Want.SetStatus(WantStatusCancelled)
		return
	}

	goodToReserve, _ := b.GetStateBool("good_to_reserve", false)
	if !goodToReserve {
		return
	}

	if b.executor == nil {
		b.SetModuleError("executor", "Executor not initialized")
		return
	}

	// Try agent execution
	if schedule := b.executor.tryAgentExecution(); schedule != nil {
		b.SetCurrent("completed", true)
		b.SetCurrent("final_result", b.executor.formatResult(schedule))
		b.executor.SetSchedule(schedule)
		return
	}

	// Generate and provide schedule (fallback)
	locals := b.Locals
	_, connectionAvailable := b.GetFirstOutputChannel()
	schedule := b.executor.generateSchedule(locals)
	if schedule != nil {
		b.SetCurrent("completed", true)
		b.SetCurrent("final_result", b.executor.formatResult(schedule))
		if connectionAvailable {
			b.Provide(schedule)
			b.ProvideDone()
		}
	} else {
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
	return CheckLocalsInitialized[RestaurantWantLocals](&r.Want)
}

func (r *RestaurantWant) Initialize() {
	r.BaseTravelWant.Initialize()
	r.BaseTravelWant.executor = r
}

type RestaurantSchedule struct {
	TravelSchedule
	ReservationTime  time.Time `json:"reservation_time"`
	DurationHours    float64   `json:"duration_hours"`
	RestaurantType   string    `json:"restaurant_type"`
	ReservationName  string    `json:"reservation_name"`
	Cost             float64   `json:"cost"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

func (r *RestaurantWant) SetSchedule(schedule any) {
	s, ok := schedule.(RestaurantSchedule)
	if !ok {
		if sp, ok := schedule.(*RestaurantSchedule); ok {
			s = *sp
		} else {
			r.StoreLog("ERROR: Failed to cast schedule to RestaurantSchedule")
			return
		}
	}

	r.SetCurrent("completed", true)
	r.SetCurrent("reservation_time", s.ReservationTime.Format("15:04 Jan 2"))
	r.SetCurrent("restaurant_type", s.RestaurantType)
	r.SetCurrent("reservation_name", s.ReservationName)
	r.SetCurrent("cost", s.Cost)
	r.SetCurrent("actual_cost", s.Cost)
	r.SetCurrent("total_processed", 1)

	if s.PremiumLevel != "" {
		r.SetCurrent("premium_processed", true)
		r.SetCurrent("premium_level", s.PremiumLevel)
	}
	if s.ServiceTier != "" {
		r.SetCurrent("service_tier", s.ServiceTier)
	}
	if len(s.PremiumAmenities) > 0 {
		r.SetCurrent("premium_amenities", s.PremiumAmenities)
	}

	r.ProvideDone()
}

func (r *RestaurantWant) tryAgentExecution() any {
	if len(r.Spec.Requires) > 0 {
		if err := r.ExecuteAgents(); err != nil {
			r.SetCurrent("agent_execution_status", "failed")
			r.SetCurrent("agent_execution_error", err.Error())
			return nil
		}
		r.SetCurrent("agent_execution_status", "completed")
		r.SetCurrent("execution_source", "agent")
		if schedule, ok := GetStateAs[RestaurantSchedule](&r.Want, "agent_result"); ok {
			return &schedule
		}
	}
	return nil
}

func (r *RestaurantWant) generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule {
	rl, ok := locals.(*RestaurantWantLocals)
	if !ok {
		return nil
	}
	baseDate := time.Now().AddDate(0, 0, 1)
	dinnerStart := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		18+rand.Intn(3), rand.Intn(60), 0, 0, time.Local)
	restaurantName := generateRealisticRestaurantName(rl.RestaurantType)
	partySize := r.GetIntParam("party_size", 2)
	eventName := fmt.Sprintf("%s - Party of %d at %s restaurant", restaurantName, partySize, rl.RestaurantType)
	event := TimeSlot{
		Start: dinnerStart,
		End:   dinnerStart.Add(rl.Duration),
		Type:  "restaurant",
		Name:  eventName,
	}
	r.SetCurrent("total_processed", 1)
	r.SetCurrent("reservation_type", rl.RestaurantType)
	r.SetCurrent("reservation_start_time", event.Start.Format("15:04"))
	r.SetCurrent("reservation_end_time", event.End.Format("15:04"))
	r.SetCurrent("reservation_duration_hours", rl.Duration.Hours())
	r.SetCurrent("reservation_name", eventName)
	r.SetCurrent("schedule_date", baseDate.Format("2006-01-02"))
	r.SetCurrent("achieving_percentage", 100)
	return &TravelSchedule{Date: baseDate, Events: []TimeSlot{event}, Completed: true}
}

func (r *RestaurantWant) formatResult(result any) string {
	if s, ok := result.(*RestaurantSchedule); ok {
		return fmt.Sprintf("Confirmed: %s at %s. Est cost: $%.2f", s.ReservationName, s.ReservationTime.Format("15:04"), s.Cost)
	}
	return "Restaurant confirmed"
}

// HotelWant handles hotel reservations
type HotelWant struct {
	BaseTravelWant
}

func (h *HotelWant) GetLocals() *HotelWantLocals {
	return CheckLocalsInitialized[HotelWantLocals](&h.Want)
}

func (h *HotelWant) Initialize() {
	h.BaseTravelWant.Initialize()
	h.BaseTravelWant.executor = h
}

type HotelSchedule struct {
	TravelSchedule
	HotelName         string    `json:"hotel_name"`
	Location          string    `json:"location"`
	CheckInTime       time.Time `json:"check_in_time"`
	CheckOutTime      time.Time `json:"check_out_time"`
	HotelType         string    `json:"hotel_type"`
	StayDurationHours float64   `json:"stay_duration_hours"`
	ReservationName   string    `json:"reservation_name"`
	Cost              float64   `json:"cost"`
	PremiumLevel      string    `json:"premium_level,omitempty"`
	ServiceTier       string    `json:"service_tier,omitempty"`
	PremiumAmenities  []string  `json:"premium_amenities,omitempty"`
}

func (h *HotelWant) tryAgentExecution() any {
	if len(h.Spec.Requires) > 0 {
		if err := h.ExecuteAgents(); err != nil {
			h.SetCurrent("agent_execution_status", "failed")
			return nil
		}
		if res, ok := GetStateAs[HotelSchedule](&h.Want, "agent_result"); ok {
			return &res
		}
	}
	return nil
}

func (h *HotelWant) generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule {
	hl, ok := locals.(*HotelWantLocals)
	if !ok { return nil }
	baseDate := time.Now().AddDate(0, 0, 1)
	hotelName := generateRealisticHotelName(hl.HotelType)
	eventName := fmt.Sprintf("%s (%s hotel)", hotelName, hl.HotelType)
	event := TimeSlot{Start: baseDate.Add(hl.CheckIn), End: baseDate.AddDate(0, 0, 1).Add(hl.CheckOut), Type: "hotel", Name: eventName}
	h.SetCurrent("total_processed", 1)
	h.SetCurrent("hotel_type", hl.HotelType)
	h.SetCurrent("check_in_time", event.Start.Format("15:04 Jan 2"))
	h.SetCurrent("check_out_time", event.End.Format("15:04 Jan 2"))
	h.SetCurrent("stay_duration_hours", event.End.Sub(event.Start).Hours())
	h.SetCurrent("reservation_name", eventName)
	h.SetCurrent("achieving_percentage", 100)
	return &TravelSchedule{Date: baseDate, Events: []TimeSlot{event}, Completed: true}
}

func (h *HotelWant) SetSchedule(schedule any) {
	s, ok := schedule.(HotelSchedule)
	if !ok { if sp, ok := schedule.(*HotelSchedule); ok { s = *sp } else { return } }
	h.SetCurrent("hotel_name", s.HotelName)
	h.SetCurrent("cost", s.Cost)
	h.SetCurrent("reservation_name", s.ReservationName)
	h.ProvideDone()
}

func (h *HotelWant) formatResult(result any) string {
	if s, ok := result.(*HotelSchedule); ok {
		return fmt.Sprintf("Hotel: %s. Cost: $%.2f", s.HotelName, s.Cost)
	}
	return "Hotel reserved"
}

// BuffetWant handles buffet reservations
type BuffetWant struct {
	BaseTravelWant
}

func (b *BuffetWant) GetLocals() *BuffetWantLocals {
	return CheckLocalsInitialized[BuffetWantLocals](&b.Want)
}

func (b *BuffetWant) Initialize() {
	b.BaseTravelWant.Initialize()
	b.BaseTravelWant.executor = b
}

type BuffetSchedule struct {
	TravelSchedule
	ReservationTime  time.Time `json:"reservation_time"`
	DurationHours    float64   `json:"duration_hours"`
	BuffetType       string    `json:"buffet_type"`
	ReservationName  string    `json:"reservation_name"`
	Cost             float64   `json:"cost"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

func (b *BuffetWant) tryAgentExecution() any {
	if len(b.Spec.Requires) > 0 {
		if err := b.ExecuteAgents(); err != nil {
			return nil
		}
		if res, ok := GetStateAs[BuffetSchedule](&b.Want, "agent_result"); ok {
			return &res
		}
	}
	return nil
}

func (b *BuffetWant) generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule {
	bl, ok := locals.(*BuffetWantLocals)
	if !ok { return nil }
	baseDate := time.Now().AddDate(0, 0, 2)
	buffetName := generateRealisticBuffetName(bl.BuffetType)
	eventName := fmt.Sprintf("%s (%s buffet)", buffetName, bl.BuffetType)
	buffetStart := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		8+rand.Intn(2), rand.Intn(30), 0, 0, time.Local)
	event := TimeSlot{Start: buffetStart, End: buffetStart.Add(bl.Duration), Type: "buffet", Name: eventName}
	b.SetCurrent("total_processed", 1)
	b.SetCurrent("buffet_type", bl.BuffetType)
	b.SetCurrent("buffet_start_time", event.Start.Format("15:04 Jan 2"))
	b.SetCurrent("buffet_end_time", event.End.Format("15:04 Jan 2"))
	b.SetCurrent("buffet_duration_hours", bl.Duration.Hours())
	b.SetCurrent("reservation_name", eventName)
	b.SetCurrent("achieving_percentage", 100)
	return &TravelSchedule{Date: baseDate, Events: []TimeSlot{event}, Completed: true}
}

func (b *BuffetWant) SetSchedule(schedule any) {
	s, ok := schedule.(BuffetSchedule)
	if !ok { if sp, ok := schedule.(*BuffetSchedule); ok { s = *sp } else { return } }
	b.SetCurrent("reservation_name", s.ReservationName)
	b.SetCurrent("cost", s.Cost)
	b.ProvideDone()
}

func (b *BuffetWant) formatResult(result any) string {
	if s, ok := result.(*BuffetSchedule); ok {
		return fmt.Sprintf("Buffet: %s at %s. Cost: $%.2f", s.ReservationName, s.ReservationTime.Format("15:04"), s.Cost)
	}
	return "Buffet reserved"
}

// FlightWant Implementation
type FlightWantLocals struct {
	BaseTravelWantLocals
	monitoringStartTime time.Time
	lastLogTime         time.Time
	monitoringDone      chan struct{}

	// State fields (auto-synced)
	// Only FlightPhase is internal (Progress() owns it exclusively).
	// Agent-written fields (_previous_flight_id, _previous_flight_status, monitor_state_hash,
	// attempted) are registered as label:current in flight.yaml and accessed via SetCurrent/GetCurrent.
	FlightPhase string `mywant:"internal,_flight_phase"`
}

type StatusChange struct {
	Timestamp time.Time
	OldStatus string
	NewStatus string
	Details   string
}

const (
	PhaseInitial    = "initial"
	PhaseBooking    = "booking"
	PhaseMonitoring = "monitoring"
	PhaseCanceling  = "canceling"
	PhaseCompleted  = "completed"
)

type FlightWant struct {
	BaseTravelWant
}

func (f *FlightWant) GetLocals() *FlightWantLocals {
	return CheckLocalsInitialized[FlightWantLocals](&f.Want)
}

func (f *FlightWant) Initialize() {
	f.BaseTravelWant.Initialize()
	f.BaseTravelWant.executor = f
	locals := f.GetLocals()
	locals.monitoringDone = make(chan struct{})
	locals.FlightPhase = PhaseInitial
}

func (f *FlightWant) IsAchieved() bool {
	completed, _ := f.GetStateBool("completed", false)
	if completed {
		return true
	}
	phase := f.GetLocals().FlightPhase
	return phase == PhaseMonitoring || phase == PhaseCompleted
}

type FlightSchedule struct {
	TravelSchedule
	DepartureTime    time.Time `json:"departure_time"`
	ArrivalTime      time.Time `json:"arrival_time"`
	FlightType       string    `json:"flight_type"`
	FlightNumber     string    `json:"flight_number"`
	ReservationName  string    `json:"reservation_name"`
	Cost             float64   `json:"cost"`
	PremiumLevel     string    `json:"premium_level,omitempty"`
	ServiceTier      string    `json:"service_tier,omitempty"`
	PremiumAmenities []string  `json:"premium_amenities,omitempty"`
}

func (f *FlightWant) tryAgentExecution() any {
	if len(f.Spec.Requires) > 0 {
		if err := f.ExecuteAgents(); err != nil {
			return nil
		}
		if result := GetCurrent(f, "agent_result", any(nil)); result != nil {
			return f.extractFlightSchedule(result)
		}
	}
	return nil
}

func (f *FlightWant) generateSchedule(locals TravelWantLocalsInterface) *TravelSchedule { return nil }

func (f *FlightWant) SetSchedule(schedule any) {
	s, ok := schedule.(FlightSchedule)
	if !ok {
		if sp, ok := schedule.(*FlightSchedule); ok {
			s = *sp
		} else {
			return
		}
	}
	f.SetCurrent("completed", true)
	f.SetCurrent("reservation_name", s.ReservationName)
	f.SetCurrent("cost", s.Cost)
	f.ProvideDone()
}

func (f *FlightWant) formatResult(result any) string {
	if s, ok := result.(*FlightSchedule); ok {
		return fmt.Sprintf("Flight %s: %s -> %s. Cost: $%.2f", s.FlightNumber, s.DepartureTime.Format("15:04"), s.ArrivalTime.Format("15:04"), s.Cost)
	}
	return "Flight booked"
}

func (f *FlightWant) extractFlightSchedule(result any) *FlightSchedule {
	var s FlightSchedule
	data, _ := json.Marshal(result)
	if err := json.Unmarshal(data, &s); err == nil {
		return &s
	}
	return nil
}

func (f *FlightWant) Progress() {
	locals := f.GetLocals()
	lastLoggedPhase := GetCurrent(f, "last_logged_phase", "")
	if lastLoggedPhase != locals.FlightPhase {
		f.StoreLog("[FLIGHT] Phase: %s", locals.FlightPhase)
		f.SetCurrent("last_logged_phase", locals.FlightPhase)
	}

	switch locals.FlightPhase {
	case PhaseInitial:
		locals.FlightPhase = PhaseBooking
	case PhaseBooking:
		res := f.tryAgentExecution()
		if res != nil {
			if s, ok := res.(*FlightSchedule); ok {
				f.SetSchedule(*s)
				locals.FlightPhase = PhaseMonitoring
				locals.monitoringStartTime = time.Now()

				// Ensure status is marked as achieved
				f.SetCurrent("completed", true)
				f.SetStatus(WantStatusAchieved)
				f.ProvideDone()

				// Force parent Target to re-evaluate children status by sending a state update
				f.MergeParentState(map[string]any{"_child_updated": f.Metadata.Name})
			}
		}
	case PhaseMonitoring:
		if time.Since(locals.monitoringStartTime) > 10*time.Minute {
			locals.FlightPhase = PhaseCompleted
			f.ProvideDone()
		} else {
			f.ExecuteAgents()
		}
	case PhaseCompleted:
		f.SetCurrent("agent_result", nil)
	}
}

