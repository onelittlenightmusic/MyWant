package types

import (
	. "mywant/engine/core"
)

// ActivityFormatter formats activity and log messages from a schedule.
type ActivityFormatter func(schedule interface{}, isRebooking bool) (activity, logMessage string)

// executeReservation executes the common reservation flow for travel agents
func executeReservation(want *Want, agentName string, schedule interface{}, formatter ActivityFormatter) error {
	// 1. Store the full schedule in GCP current state and system metadata
	want.SetCurrent("reservation_detail", schedule)
	want.SetCurrent("agent_result", schedule)

	// 2. Extract cost and reservation name
	cost := extractCostFromSchedule(schedule)
	want.SetCurrent("actual_cost", cost)

	reservationName := extractReservationNameFromSchedule(schedule)
	if reservationName != "" {
		want.SetCurrent("reservation_name", reservationName)
	}

	// 3. Update status to confirmed in GCP current state
	want.SetCurrent("res_status", "confirmed")

	// 4. Mark the plan as completed by clearing it
	want.ClearPlan("execute_booking")

	// 5. Set activity and log
	activity, logMsg := formatter(schedule, false)
	want.SetAgentActivity(agentName, activity)
	want.StoreLog("%s", logMsg)
	
	return nil
}

// extractReservationNameFromSchedule tries to extract a ReservationName field from various schedule types.
func extractReservationNameFromSchedule(schedule interface{}) string {
	switch s := schedule.(type) {
	case RestaurantSchedule:
		return s.ReservationName
	case BuffetSchedule:
		return s.ReservationName
	case HotelSchedule:
		return s.ReservationName
	case *RestaurantSchedule:
		return s.ReservationName
	case *BuffetSchedule:
		return s.ReservationName
	case *HotelSchedule:
		return s.ReservationName
	case FlightSchedule:
		return s.ReservationName
	case *FlightSchedule:
		return s.ReservationName
	default:
		return ""
	}
}

// extractCostFromSchedule tries to extract a Cost field from various schedule types.
func extractCostFromSchedule(schedule interface{}) float64 {
	switch s := schedule.(type) {
	case RestaurantSchedule:
		return s.Cost
	case BuffetSchedule:
		return s.Cost
	case HotelSchedule:
		return s.Cost
	case *RestaurantSchedule:
		return s.Cost
	case *BuffetSchedule:
		return s.Cost
	case *HotelSchedule:
		return s.Cost
	case FlightSchedule:
		return s.Cost
	case *FlightSchedule:
		return s.Cost
	default:
		return 0
	}
}
