package types

import (
	"context"
	"fmt"
	. "mywant/engine/core"
	"time"
)

const agentBuffetName = "agent_buffet_premium"

func init() {
	RegisterDoAgent(agentBuffetName, executeBuffetReservation)
}

// executeBuffetReservation performs a premium buffet reservation
func executeBuffetReservation(ctx context.Context, want *Want) error {
	schedule := generateBuffetSchedule(want)
	err := executeReservation(want, agentBuffetName, schedule, func(s interface{}) (string, string) {
		sch := s.(BuffetSchedule)
		activity := fmt.Sprintf("Buffet reservation has been confirmed for %s buffet at %s for %.1f hours",
			sch.BuffetType, sch.ReservationTime.Format("15:04 Jan 2"), sch.DurationHours)
		logMsg := fmt.Sprintf("Buffet reservation completed: %s at %s for %.1f hours",
			sch.BuffetType, sch.ReservationTime.Format("15:04 Jan 2"), sch.DurationHours)
		return activity, logMsg
	})
	if err != nil {
		return err
	}

	// Report cost to parent want for budget tracking
	buffetCost := want.GetFloatParam("cost", 150.0)
	want.MergeParentState(map[string]any{
		"costs": map[string]any{want.Metadata.Name: buffetCost},
	})

	return nil
}

// generateBuffetSchedule creates a buffet reservation schedule
func generateBuffetSchedule(want *Want) BuffetSchedule {
	want.StoreLog("Processing buffet reservation for %s with premium service", want.Metadata.Name)

	baseDate := time.Now()
	reservationTime := GenerateRandomTimeWithOptions(baseDate, LunchTimeRange, DinnerTimeRange)
	durationHours := GenerateRandomDuration(2.0, 4.0)

	buffetType := want.GetStringParam("buffet_type", "international")
	premiumLevel := want.GetStringParam("premium_level", "premium")
	serviceTier := want.GetStringParam("service_tier", "premium")

	return BuffetSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		BuffetType:       buffetType,
		ReservationName:  fmt.Sprintf("%s reservation at %s buffet", want.Metadata.Name, buffetType),
		PremiumLevel:     premiumLevel,
		ServiceTier:      serviceTier,
		PremiumAmenities: []string{"premium_stations", "chef_interaction", "unlimited_beverages"},
	}
}
