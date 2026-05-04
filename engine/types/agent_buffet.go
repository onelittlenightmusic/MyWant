package types

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	. "mywant/engine/core"
	"strings"
	"time"
)

const agentBuffetName = "agent_buffet_premium"

func init() {
	RegisterWithInit(func() {
		RegisterDoAgent(agentBuffetName, executeBuffetReservation)
	})
}

// executeBuffetReservation performs a buffet reservation.
func executeBuffetReservation(ctx context.Context, want *Want) error {
	// ── GCP Pattern: Only execute if a plan exists ────────────────────────
	if !GetPlan(want, "execute_booking", false) {
		if !GetCurrent(want, "good_to_reserve", false) {
			return nil // No plan to execute
		}
	}

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
	default:
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

	buffetType := GetCurrent(want, "buffet_type", "international")
	buffetName := generateRealisticBuffetName(buffetType)
	buffetCost := generateBuffetCost(buffetType)
	premiumLevel := GetCurrent(want, "premium_level", "premium")
	serviceTier := GetCurrent(want, "service_tier", "premium")

	reservationName := fmt.Sprintf("%s (%s buffet)", buffetName, buffetType)
	schedule := BuffetSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		BuffetType:       buffetType,
		ReservationName:  reservationName,
		Cost:             buffetCost,
		PremiumLevel:     premiumLevel,
		ServiceTier:      serviceTier,
		PremiumAmenities: []string{"premium_stations", "chef_interaction", "unlimited_beverages"},
	}

	want.SetCurrent("reservation_name", reservationName)
	want.SetCurrent("buffet_type", buffetType)
	want.SetCurrent("buffet_start_time", reservationTime.Format("15:04 Jan 2"))
	want.SetCurrent("buffet_end_time", reservationTime.Add(time.Duration(durationHours*float64(time.Hour))).Format("15:04 Jan 2"))
	want.SetCurrent("buffet_duration_hours", durationHours)

	return schedule
}

func generateRealisticBuffetName(buffetType string) string {
	names := map[string][]string{
		"continental": {
			"The Continental Breakfast", "Morning Spread Buffet", "Continental Sunrise",
			"Classic Breakfast House", "The Morning Table",
		},
		"international": {
			"World Flavors Buffet", "Global Taste", "International Feast",
			"The Passport Cafe", "Around the World Dining",
		},
		"asian": {
			"Asian Cuisine Buffet", "Dragon's Feast", "The Orient Express",
			"Asian Spice Buffet", "East Meets Plate",
		},
		"mediterranean": {
			"Mediterranean Buffet", "The Greek Table", "Olive & Vine",
			"Mediterranean Feast", "The Coastline",
		},
	}
	if list, ok := names[strings.ToLower(buffetType)]; ok && len(list) > 0 {
		return list[rand.Intn(len(list))]
	}
	return names["continental"][rand.Intn(len(names["continental"]))]
}
