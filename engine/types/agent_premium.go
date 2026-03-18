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

const agentPremiumName = "agent_premium"

func init() {
	RegisterDoAgent(agentPremiumName, executeHotelReservation)
}

// executeHotelReservation performs a hotel reservation.
func executeHotelReservation(ctx context.Context, want *Want) error {
	// ── GCP Pattern: Only execute if a plan exists ────────────────────────
	if !GetPlan(want, "execute_booking", false) {
		if !GetCurrent(want, "good_to_reserve", false) {
			return nil // No plan to execute
		}
	}

	schedule := generateHotelSchedule(want)
	return executeReservation(want, agentPremiumName, schedule, func(s interface{}, isRebooking bool) (string, string) {
		sch := s.(HotelSchedule)
		verb := "confirmed"
		if isRebooking {
			verb = "rebooked at lower cost"
		}
		activity := fmt.Sprintf("Hotel reservation has been %s for %s hotel at %s",
			verb, sch.HotelType, sch.CheckInTime.Format("15:04 Jan 2"))
		logMsg := fmt.Sprintf("Hotel reservation %s: %s at %s",
			verb, sch.HotelType, sch.CheckInTime.Format("15:04 Jan 2"))
		return activity, logMsg
	})
}

// generateHotelCost returns a realistic cost based on hotel type
func generateHotelCost(hotelType string) float64 {
	var minCost, maxCost float64
	switch hotelType {
	case "budget": minCost, maxCost = 50.0, 120.0
	case "standard": minCost, maxCost = 250.0, 400.0  // always over 200 budget
	case "discounted": minCost, maxCost = 80.0, 150.0 // always under 200 budget (hotel_discount capability)
	case "boutique": minCost, maxCost = 200.0, 450.0
	default: minCost, maxCost = 400.0, 1200.0
	}
	cost := minCost + rand.Float64()*(maxCost-minCost)
	return math.Round(cost*100) / 100
}

// generateHotelSchedule creates a hotel reservation schedule
func generateHotelSchedule(want *Want) HotelSchedule {
	want.StoreLog("Processing hotel reservation for %s with premium service", want.Metadata.Name)

	baseDate := time.Now()
	checkInTime := GenerateRandomTimeWithOptions(baseDate, DinnerTimeRange)
	checkOutTime := checkInTime.Add(time.Duration(GenerateRandomDuration(12.0, 24.0) * float64(time.Hour)))

	hotelType := GetCurrent(want, "hotel_type", "luxury")
	hotelName := generateRealisticHotelName(hotelType)
	hotelCost := generateHotelCost(hotelType)
	premiumLevel := GetCurrent(want, "premium_level", "premium")
	serviceTier := GetCurrent(want, "service_tier", "premium")

	schedule := HotelSchedule{
		CheckInTime:       checkInTime,
		CheckOutTime:      checkOutTime,
		HotelName:         hotelName,
		HotelType:         hotelType,
		ReservationName:   fmt.Sprintf("%s (%s hotel)", hotelName, hotelType),
		Cost:              hotelCost,
		PremiumLevel:      premiumLevel,
		ServiceTier:       serviceTier,
		PremiumAmenities:  []string{"spa_access", "concierge_service", "room_upgrade"},
	}

	want.SetCurrent("hotel_name", hotelName)
	want.SetCurrent("reservation_name", schedule.ReservationName)
	want.SetCurrent("hotel_type", hotelType)
	want.SetCurrent("check_in_time", checkInTime.Format("15:04 Jan 2"))
	want.SetCurrent("check_out_time", checkOutTime.Format("15:04 Jan 2"))
	want.SetCurrent("stay_duration_hours", checkOutTime.Sub(checkInTime).Hours())

	return schedule
}

func generateRealisticHotelName(hotelType string) string {
	names := map[string][]string{
		"luxury": {
			"The Grand Plaza", "Royal Palace Hotel", "Platinum Suites", "Signature Towers",
			"The Ritz Collection", "Prestige Residences", "Crown Jewel Hotel", "Luxury Haven",
		},
		"standard": {
			"City Central Hotel", "Comfort Inn Express", "Downtown Plaza", "Urban Oasis",
			"Gateway Hotel", "The Meridian", "Riverside Inn", "Sunrise Hotel",
		},
		"discounted": {
			"City Central Hotel (割引)", "Comfort Inn Express (会員価格)", "Downtown Plaza (特別割引)",
			"Urban Oasis (割引)", "Gateway Hotel (会員割引)", "The Meridian (特別料金)",
		},
		"budget": {
			"Budget Stay Inn", "Economy Hotel", "Value Lodge", "Basic Inn",
			"The Affordable", "Quick Stop Hotel", "Smart Stay", "City Budget",
		},
		"boutique": {
			"Artistic House", "Heritage Inn", "Boutique Noir", "Cultural Quarters",
			"The Artisan Hotel", "Signature Boutique", "Modern Spaces", "Unique Stay",
		},
	}
	if list, ok := names[strings.ToLower(hotelType)]; ok && len(list) > 0 {
		return list[rand.Intn(len(list))]
	}
	return names["standard"][rand.Intn(len(names["standard"]))]
}
