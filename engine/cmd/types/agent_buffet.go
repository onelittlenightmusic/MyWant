package types

import (
	"context"
	"fmt"
	. "mywant/engine/src"
	"time"
)

// AgentBuffet extends PremiumServiceAgent with buffet reservation capabilities
type AgentBuffet struct {
	PremiumServiceAgent
}

// NewAgentBuffet creates a new buffet agent
func NewAgentBuffet(name string, capabilities []string, uses []string, premiumLevel string) *AgentBuffet {
	return &AgentBuffet{
		PremiumServiceAgent: NewPremiumServiceAgent(name, capabilities, premiumLevel),
	}
}

// Exec executes buffet agent actions and returns BuffetSchedule
func (a *AgentBuffet) Exec(ctx context.Context, want *Want) (bool, error) {
	schedule := a.generateBuffetSchedule(want)
	return a.ExecuteReservation(ctx, want, schedule, func(s interface{}) (string, string) {
		sch := s.(BuffetSchedule)
		activity := fmt.Sprintf("Buffet reservation has been confirmed for %s buffet at %s for %.1f hours",
			sch.BuffetType, sch.ReservationTime.Format("15:04 Jan 2"), sch.DurationHours)
		logMsg := fmt.Sprintf("Buffet reservation completed: %s at %s for %.1f hours",
			sch.BuffetType, sch.ReservationTime.Format("15:04 Jan 2"), sch.DurationHours)
		return activity, logMsg
	})
}

// generateBuffetSchedule creates a buffet reservation schedule
func (a *AgentBuffet) generateBuffetSchedule(want *Want) BuffetSchedule {
	want.StoreLog("Processing buffet reservation for %s with premium service", want.Metadata.Name)

	// Generate buffet reservation with appropriate timing (lunch or dinner)
	baseDate := time.Now()
	reservationTime := GenerateRandomTimeWithOptions(baseDate, LunchTimeRange, DinnerTimeRange)

	// Buffet meals typically 2-4 hours (more relaxed dining)
	durationHours := GenerateRandomDuration(2.0, 4.0)

	// Extract buffet type from want parameters
	buffetType := want.GetStringParam("buffet_type", "international")
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
