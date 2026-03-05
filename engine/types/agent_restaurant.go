package types

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	. "mywant/engine/core"
	"time"
)

const agentRestaurantName = "agent_restaurant_premium"

func init() {
	RegisterDoAgent(agentRestaurantName, executeRestaurantReservation)
}

// executeRestaurantReservation performs a restaurant reservation.
func executeRestaurantReservation(ctx context.Context, want *Want) error {
	// ── GCP Pattern: Only execute if a plan exists ────────────────────────
	if plan, _ := want.GetPlan("execute_booking"); plan == nil {
		if legacy, _ := want.GetStateBool("good_to_reserve", false); !legacy {
			return nil // No plan to execute
		}
	}

	schedule := generateRestaurantSchedule(want)
	return executeReservation(want, agentRestaurantName, schedule, func(s interface{}, isRebooking bool) (string, string) {
		sch := s.(RestaurantSchedule)
		verb := "booked"
		if isRebooking {
			verb = "updated"
		}
		activity := fmt.Sprintf("Restaurant table %s for %s cuisine at %s",
			verb, sch.RestaurantType, sch.ReservationTime.Format("15:04 Jan 2"))
		logMsg := fmt.Sprintf("Restaurant %s: %s at %s",
			verb, sch.RestaurantType, sch.ReservationTime.Format("15:04 Jan 2"))
		return activity, logMsg
	})
}

// generateRestaurantCost returns a realistic cost based on cuisine type
func generateRestaurantCost(cuisineType string) float64 {
	var minCost, maxCost float64
	switch cuisineType {
	case "casual": minCost, maxCost = 25.0, 60.0
	case "fine dining": minCost, maxCost = 150.0, 400.0
	case "bistro": minCost, maxCost = 45.0, 95.0
	default: minCost, maxCost = 40.0, 120.0
	}
	cost := minCost + rand.Float64()*(maxCost-minCost)
	return math.Round(cost*100) / 100
}

// generateRestaurantSchedule creates a restaurant reservation schedule
func generateRestaurantSchedule(want *Want) RestaurantSchedule {
	want.StoreLog("Processing restaurant reservation for %s with premium service", want.Metadata.Name)

	baseDate := time.Now()
	reservationTime := GenerateRandomTimeWithOptions(baseDate, DinnerTimeRange)
	durationHours := 2.5

	cuisineType := want.GetStringParam("restaurant_type", "fine dining")
	restaurantCost := generateRestaurantCost(cuisineType)
	premiumLevel := want.GetStringParam("premium_level", "premium")
	serviceTier := want.GetStringParam("service_tier", "premium")

	return RestaurantSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		RestaurantType:   cuisineType,
		ReservationName:  fmt.Sprintf("%s dinner at %s restaurant", want.Metadata.Name, cuisineType),
		Cost:             restaurantCost,
		PremiumLevel:     premiumLevel,
		ServiceTier:      serviceTier,
		PremiumAmenities: []string{"window_seat", "sommelier_service", "complimentary_aperitif"},
	}
}
