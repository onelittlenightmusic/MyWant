package types

import . "mywant/engine/core"

// ActivityFormatter formats activity and log messages from a schedule.
// isRebooking is true when this execution is replacing a previous booking.
type ActivityFormatter func(schedule interface{}, isRebooking bool) (activity, logMessage string)

// executeReservation executes the common reservation flow for travel agents
// (hotel, restaurant, buffet).
//
// If the want carries a "prev_want_id" param (injected by the itinerary when
// the action has cancels_action set), the previous booking's want is marked
// as cancelled before the new booking is created.  This centralises the
// cancel + rebook pattern so individual agents do not need to repeat it.
func executeReservation(want *Want, agentName string, schedule interface{}, formatter ActivityFormatter) error {
	isRebooking := false
	if prevWantID := want.GetStringParam("prev_want_id", ""); prevWantID != "" {
		cancelPreviousWant(want, prevWantID, agentName)
		isRebooking = true
	}

	want.StoreStateForAgent("agent_result", schedule)

	activity, logMsg := formatter(schedule, isRebooking)
	want.SetAgentActivity(agentName, activity)
	want.StoreLog("%s", logMsg)
	return nil
}
