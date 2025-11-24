package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/src"
	"time"
)

// AgentBuffet extends DoAgent with buffet reservation capabilities
type AgentBuffet struct {
	DoAgent
	PremiumLevel string
	ServiceTier  string
}

// NewAgentBuffet creates a new buffet agent
func NewAgentBuffet(name string, capabilities []string, uses []string, premiumLevel string) *AgentBuffet {
	return &AgentBuffet{
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

// Exec executes buffet agent actions and returns BuffetSchedule
func (a *AgentBuffet) Exec(ctx context.Context, want *Want) error {
	// Generate buffet reservation schedule
	schedule := a.generateBuffetSchedule(want)

	// Store the result using StoreState method
	want.StoreState("agent_result", schedule)

	// Record activity description for agent history
	activity := fmt.Sprintf("Buffet reservation has been confirmed for %s buffet at %s for %.1f hours",
		schedule.BuffetType, schedule.ReservationTime.Format("15:04 Jan 2"), schedule.DurationHours)
	want.SetAgentActivity(a.Name, activity)

	want.StoreLog(fmt.Sprintf("Buffet reservation completed: %s at %s for %.1f hours",
		schedule.BuffetType, schedule.ReservationTime.Format("15:04 Jan 2"), schedule.DurationHours))

	return nil
}

// generateBuffetSchedule creates a buffet reservation schedule
func (a *AgentBuffet) generateBuffetSchedule(want *Want) BuffetSchedule {
	want.StoreLog(fmt.Sprintf("Processing buffet reservation for %s with premium service", want.Metadata.Name))

	// Generate buffet reservation with appropriate timing
	baseDate := time.Now()
	// Buffet reservations typically during lunch (11 AM-2 PM) or dinner (6-9 PM)
	var reservationTime time.Time
	if rand.Intn(2) == 0 {
		// Lunch buffet (11 AM - 2 PM)
		reservationTime = time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
			11+rand.Intn(3), rand.Intn(60), 0, 0, time.Local)
	} else {
		// Dinner buffet (6-9 PM)
		reservationTime = time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
			18+rand.Intn(3), rand.Intn(60), 0, 0, time.Local)
	}

	// Buffet meals typically 2-4 hours (more relaxed dining)
	durationHours := 2.0 + rand.Float64()*2.0 // 2-4 hours

	// Extract buffet type from want parameters
	buffetType := want.GetStringParam("buffet_type", "international")

	// Create and return structured buffet schedule
	return BuffetSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		BuffetType:       buffetType,
		ReservationName:  fmt.Sprintf("%s reservation at %s buffet", want.Metadata.Name, buffetType),
		PremiumLevel:     a.PremiumLevel,
		ServiceTier:      a.ServiceTier,
		PremiumAmenities: []string{"premium_stations", "chef_interaction", "unlimited_beverages"},
	}
}
