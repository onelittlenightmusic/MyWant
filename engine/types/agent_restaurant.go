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

const agentRestaurantName = "agent_restaurant_premium"

func init() {
	RegisterWithInit(func() {
		RegisterDoAgent(agentRestaurantName, executeRestaurantReservation)
	})
}

// executeRestaurantReservation performs a restaurant reservation.
func executeRestaurantReservation(ctx context.Context, want *Want) error {
	// ── GCP Pattern: Only execute if a plan exists ────────────────────────
	if !GetPlan(want, "execute_booking", false) {
		if !GetCurrent(want, "good_to_reserve", false) {
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
	case "casual":
		minCost, maxCost = 25.0, 60.0
	case "fine dining":
		minCost, maxCost = 150.0, 400.0
	case "bistro":
		minCost, maxCost = 45.0, 95.0
	default:
		minCost, maxCost = 40.0, 120.0
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

	cuisineType := GetCurrent(want, "restaurant_type", "fine dining")
	restaurantName := generateRealisticRestaurantName(cuisineType)
	restaurantCost := generateRestaurantCost(cuisineType)
	premiumLevel := GetCurrent(want, "premium_level", "premium")
	serviceTier := GetCurrent(want, "service_tier", "premium")
	partySize := GetCurrent(want, "party_size", 2)

	reservationName := fmt.Sprintf("%s - Party of %d at %s restaurant", restaurantName, partySize, cuisineType)
	schedule := RestaurantSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		RestaurantType:   cuisineType,
		ReservationName:  reservationName,
		Cost:             restaurantCost,
		PremiumLevel:     premiumLevel,
		ServiceTier:      serviceTier,
		PremiumAmenities: []string{"window_seat", "sommelier_service", "complimentary_aperitif"},
	}

	want.SetCurrent("reservation_name", reservationName)
	want.SetCurrent("reservation_type", cuisineType)
	want.SetCurrent("reservation_start_time", reservationTime.Format("15:04"))
	want.SetCurrent("reservation_end_time", reservationTime.Add(time.Duration(durationHours*float64(time.Hour))).Format("15:04"))
	want.SetCurrent("reservation_duration_hours", durationHours)

	return schedule
}

func generateRealisticRestaurantName(cuisineType string) string {
	names := map[string][]string{
		"fine dining": {
			"L'Élégance", "The Michelin House", "Le Bernardin", "Per Se", "The French Laundry",
			"Alinea", "The Ledbury", "Noma", "Chef's Table", "Sous Vide",
		},
		"casual": {
			"The Garden Bistro", "Rustic Table", "Harvest Kitchen", "Homestead",
			"The Local Taste", "Farm to Fork", "Urban Eats", "Downtown Cafe",
		},
		"buffet": {
			"The Buffet House", "All You Can Eat Palace", "Golden Buffet", "Celebration Buffet",
		},
		"steakhouse": {
			"The Prime Cut", "Bone & Barrel", "Wagyu House", "The Smokehouse",
			"Texas Grill", "The Cattleman's", "Ribeye Room",
		},
		"seafood": {
			"The Lobster Cove", "Catch of the Day", "The Oyster House", "Sea Pearl",
			"Harbor View", "Dockside Grille", "The Fish House", "Captain's Table",
		},
	}
	if list, ok := names[strings.ToLower(cuisineType)]; ok && len(list) > 0 {
		return list[rand.Intn(len(list))]
	}
	return names["fine dining"][rand.Intn(len(names["fine dining"]))]
}
