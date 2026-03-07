package types

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	. "mywant/engine/core"
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
	case "standard": minCost, maxCost = 120.0, 250.0
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

	hotelType := want.GetStringParam("hotel_type", "luxury")
	hotelCost := generateHotelCost(hotelType)
	premiumLevel := want.GetStringParam("premium_level", "premium")
	serviceTier := want.GetStringParam("service_tier", "premium")

	return HotelSchedule{
		CheckInTime:       checkInTime,
		CheckOutTime:      checkOutTime,
		HotelType:         hotelType,
		ReservationName:   fmt.Sprintf("%s stay at %s hotel", want.Metadata.Name, hotelType),
		Cost:              hotelCost,
		PremiumLevel:      premiumLevel,
		ServiceTier:       serviceTier,
		PremiumAmenities:  []string{"spa_access", "concierge_service", "room_upgrade"},
	}
}
