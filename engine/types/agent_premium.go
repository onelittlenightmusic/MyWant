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
// Cancel + rebook handling (prev_want_id) is delegated to executeReservation.
func executeHotelReservation(ctx context.Context, want *Want) error {
	schedule := generateHotelSchedule(want)
	return executeReservation(want, agentPremiumName, schedule, func(s interface{}, isRebooking bool) (string, string) {
		sch := s.(HotelSchedule)
		verb := "confirmed"
		if isRebooking {
			verb = "rebooked at lower cost"
		}
		activity := fmt.Sprintf("Hotel reservation has been %s for %s from %s to %s",
			verb, sch.HotelType,
			sch.CheckInTime.Format("15:04 Jan 2"),
			sch.CheckOutTime.Format("15:04 Jan 2"))
		logMsg := fmt.Sprintf("Hotel booking %s: %s from %s to %s",
			verb, sch.HotelType,
			sch.CheckInTime.Format("15:04 Jan 2"),
			sch.CheckOutTime.Format("15:04 Jan 2"))
		return activity, logMsg
	})
}

// generateHotelCost returns a realistic per-night cost based on hotel type
func generateHotelCost(hotelType string) float64 {
	var minCost, maxCost float64
	switch hotelType {
	case "luxury", "5-star":
		minCost, maxCost = 500.0, 1500.0
	case "boutique":
		minCost, maxCost = 200.0, 600.0
	case "business":
		minCost, maxCost = 100.0, 300.0
	case "budget":
		minCost, maxCost = 50.0, 120.0
	default:
		minCost, maxCost = 150.0, 500.0
	}
	cost := minCost + rand.Float64()*(maxCost-minCost)
	return math.Round(cost*100) / 100
}

// generateHotelSchedule creates a premium hotel schedule
func generateHotelSchedule(want *Want) HotelSchedule {
	want.StoreLog("Processing hotel reservation for %s with premium service", want.Metadata.Name)

	baseDate := time.Now().AddDate(0, 0, 1)
	checkInTime := GenerateRandomTimeInRange(baseDate, CheckInRange)

	nextDay := baseDate.AddDate(0, 0, 1)
	checkOutTime := GenerateRandomTimeInRange(nextDay, CheckOutRange)

	hotelType := want.GetStringParam("hotel_type", "luxury")
	hotelCost := generateHotelCost(hotelType)
	premiumLevel := want.GetStringParam("premium_level", "platinum")
	serviceTier := want.GetStringParam("service_tier", "premium")

	return HotelSchedule{
		CheckInTime:       checkInTime,
		CheckOutTime:      checkOutTime,
		HotelType:         hotelType,
		StayDurationHours: checkOutTime.Sub(checkInTime).Hours(),
		ReservationName:   fmt.Sprintf("%s stay at %s hotel", want.Metadata.Name, hotelType),
		Cost:              hotelCost,
		PremiumLevel:      premiumLevel,
		ServiceTier:       serviceTier,
		PremiumAmenities:  []string{"spa_access", "concierge_service", "room_upgrade"},
	}
}
