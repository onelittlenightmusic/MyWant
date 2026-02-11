package types

import . "mywant/engine/core"

// ActivityFormatter formats activity and log messages from a schedule
type ActivityFormatter func(schedule interface{}) (activity, logMessage string)

// executeReservation executes common reservation flow for premium service agents.
// Stores the schedule as agent_result, sets agent activity, and logs completion.
func executeReservation(want *Want, agentName string, schedule interface{}, formatter ActivityFormatter) error {
	want.StoreStateForAgent("agent_result", schedule)

	activity, logMsg := formatter(schedule)
	want.SetAgentActivity(agentName, activity)

	want.StoreLog("%s", logMsg)
	return nil
}
