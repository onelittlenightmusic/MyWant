package types

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	. "mywant/engine/core"
	"time"
)

const agentBuffetName = "agent_buffet_premium"

func init() {
	RegisterDoAgent(agentBuffetName, executeBuffetReservation)
}

// executeBuffetReservation performs a buffet reservation.
// Cancel + rebook handling (prev_want_id) is delegated to executeReservation.
func executeBuffetReservation(ctx context.Context, want *Want) error {
	schedule := generateBuffetSchedule(want)
	return executeReservation(want, agentBuffetName, schedule, func(s interface{}, isRebooking bool) (string, string) {
		sch := s.(BuffetSchedule)
		verb := "confirmed"
		if isRebooking {
			verb = "rebooked at lower cost"
		}
		activity := fmt.Sprintf("Buffet reservation has been %s for %s buffet at %s for %.1f hours",
			verb, sch.BuffetType, sch.ReservationTime.Format("15:04 Jan 2"), sch.DurationHours)
		logMsg := fmt.Sprintf("Buffet reservation %s: %s at %s for %.1f hours",
			verb, sch.BuffetType, sch.ReservationTime.Format("15:04 Jan 2"), sch.DurationHours)
		return activity, logMsg
	})
}

// generateBuffetCost returns a realistic cost based on buffet type
func generateBuffetCost(buffetType string) float64 {
	var minCost, maxCost float64
	switch buffetType {
	case "continental":
		minCost, maxCost = 20.0, 45.0
	case "full":
		minCost, maxCost = 35.0, 70.0
	case "asian":
		minCost, maxCost = 30.0, 65.0
	case "vegetarian":
		minCost, maxCost = 25.0, 55.0
	default: // international
		minCost, maxCost = 40.0, 90.0
	}
	cost := minCost + rand.Float64()*(maxCost-minCost)
	return math.Round(cost*100) / 100
}

// generateBuffetSchedule creates a buffet reservation schedule
func generateBuffetSchedule(want *Want) BuffetSchedule {
	want.StoreLog("Processing buffet reservation for %s with premium service", want.Metadata.Name)

	baseDate := time.Now()
	reservationTime := GenerateRandomTimeWithOptions(baseDate, LunchTimeRange, DinnerTimeRange)
	durationHours := GenerateRandomDuration(2.0, 4.0)

	buffetType := want.GetStringParam("buffet_type", "international")
	buffetCost := generateBuffetCost(buffetType)
	premiumLevel := want.GetStringParam("premium_level", "premium")
	serviceTier := want.GetStringParam("service_tier", "premium")

	return BuffetSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		BuffetType:       buffetType,
		ReservationName:  fmt.Sprintf("%s reservation at %s buffet", want.Metadata.Name, buffetType),
		Cost:             buffetCost,
		PremiumLevel:     premiumLevel,
		ServiceTier:      serviceTier,
		PremiumAmenities: []string{"premium_stations", "chef_interaction", "unlimited_beverages"},
	}
}
