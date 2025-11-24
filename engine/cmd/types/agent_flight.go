package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/src"
	"time"
)

// AgentFlight extends DoAgent with flight booking capabilities
type AgentFlight struct {
	DoAgent
	PremiumLevel string
	ServiceTier  string
}

// NewAgentFlight creates a new flight agent
func NewAgentFlight(name string, capabilities []string, uses []string, premiumLevel string) *AgentFlight {
	return &AgentFlight{
		DoAgent: DoAgent{
			BaseAgent: BaseAgent{
				Name:         name,
				Capabilities: capabilities,
				Uses:         uses,
				Type:         DoAgentType,
			},
		},
		PremiumLevel: premiumLevel,
		ServiceTier:  "premium",
	}
}

// Exec executes flight agent actions and returns FlightSchedule
// NOTE: Exec cycle wrapping is handled by the agent execution framework in want_agent.go
// Individual agents should NOT call BeginExecCycle/EndExecCycle
func (a *AgentFlight) Exec(ctx context.Context, want *Want) error {
	// Generate flight booking schedule
	schedule := a.generateFlightSchedule(want)

	// Store the result using StoreState method
	// NOTE: Wrapping is handled by the framework, not here
	want.StoreState("agent_result", schedule)

	// Record activity description for agent history
	activity := fmt.Sprintf("Flight reservation has been confirmed for %s flight %s departing at %s",
		schedule.FlightType, schedule.FlightNumber, schedule.DepartureTime.Format("15:04 Jan 2"))
	want.SetAgentActivity(a.Name, activity)

	want.StoreLog(fmt.Sprintf("Flight booking completed: %s departing at %s for %.1f hours",
		schedule.FlightType, schedule.DepartureTime.Format("15:04 Jan 2"),
		schedule.ArrivalTime.Sub(schedule.DepartureTime).Hours()))

	return nil
}

// generateFlightSchedule creates a flight booking schedule
func (a *AgentFlight) generateFlightSchedule(want *Want) FlightSchedule {
	want.StoreLog(fmt.Sprintf("Processing flight booking for %s with premium service", want.Metadata.Name))

	// Generate flight booking with appropriate timing
	baseDate := time.Now()
	// Flight departures typically in morning hours (6 AM - 12 PM)
	departureTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day()+1,
		6+rand.Intn(6), rand.Intn(60), 0, 0, time.Local) // 6 AM - 12 PM next day

	// Flight durations typically 2-14 hours depending on distance
	durationHours := 2.0 + rand.Float64()*12.0 // 2-14 hours
	arrivalTime := departureTime.Add(time.Duration(durationHours * float64(time.Hour)))

	// Extract flight type from want parameters
	flightType := want.GetStringParam("flight_type", "economy")

	// Generate flight number
	flightNumber := fmt.Sprintf("FL%d", 100+rand.Intn(900))

	// Create and return structured flight schedule
	return FlightSchedule{
		DepartureTime:    departureTime,
		ArrivalTime:      arrivalTime,
		FlightType:       flightType,
		FlightNumber:     flightNumber,
		ReservationName:  fmt.Sprintf("%s %s flight %s", want.Metadata.Name, flightType, flightNumber),
		PremiumLevel:     a.PremiumLevel,
		ServiceTier:      a.ServiceTier,
		PremiumAmenities: []string{"priority_boarding", "lounge_access", "extra_baggage"},
	}
}
