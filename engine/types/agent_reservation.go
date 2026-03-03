package types

import . "mywant/engine/core"

// ActivityFormatter formats activity and log messages from a schedule.
// isRebooking is true when this execution is replacing a previous booking.
type ActivityFormatter func(schedule interface{}, isRebooking bool) (activity, logMessage string)

// executeReservation executes the common reservation flow for travel agents
// (hotel, restaurant, buffet).
func executeReservation(want *Want, agentName string, schedule interface{}, formatter ActivityFormatter) error {
	want.StoreStateForAgent("agent_result", schedule)
	activity, logMsg := formatter(schedule, false)
	want.SetAgentActivity(agentName, activity)
	want.StoreLog("%s", logMsg)
	return nil
}
