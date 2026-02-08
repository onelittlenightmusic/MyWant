package types

import (
	"context"
	"fmt"
	. "mywant/engine/src"
	"time"
)

const agentPremiumName = "agent_premium"

func init() {
	RegisterDoAgentType(agentPremiumName,
		[]Capability{Cap("hotel_reservation")},
		executeHotelReservation)
}

// executeHotelReservation performs a premium hotel reservation
func executeHotelReservation(ctx context.Context, want *Want) error {
	schedule := generateHotelSchedule(want)
	return executeReservation(want, agentPremiumName, schedule, func(s interface{}) (string, string) {
		sch := s.(HotelSchedule)
		activity := fmt.Sprintf("Hotel reservation has been confirmed for %s from %s to %s (%s premium)",
			sch.HotelType,
			sch.CheckInTime.Format("15:04 Jan 2"),
			sch.CheckOutTime.Format("15:04 Jan 2"),
			sch.PremiumLevel)
		logMsg := fmt.Sprintf("Premium hotel booking completed: %s from %s to %s",
			sch.HotelType, sch.CheckInTime.Format("15:04 Jan 2"), sch.CheckOutTime.Format("15:04 Jan 2"))
		return activity, logMsg
	})
}

// generateHotelSchedule creates a premium hotel schedule
func generateHotelSchedule(want *Want) HotelSchedule {
	want.StoreLog("Processing hotel reservation for %s with premium service", want.Metadata.Name)

	baseDate := time.Now().AddDate(0, 0, 1)
	checkInTime := GenerateRandomTimeInRange(baseDate, CheckInRange)

	nextDay := baseDate.AddDate(0, 0, 1)
	checkOutTime := GenerateRandomTimeInRange(nextDay, CheckOutRange)

	hotelType := want.GetStringParam("hotel_type", "luxury")
	premiumLevel := want.GetStringParam("premium_level", "platinum")
	serviceTier := want.GetStringParam("service_tier", "premium")

	return HotelSchedule{
		CheckInTime:       checkInTime,
		CheckOutTime:      checkOutTime,
		HotelType:         hotelType,
		StayDurationHours: checkOutTime.Sub(checkInTime).Hours(),
		ReservationName:   fmt.Sprintf("%s stay at %s hotel", want.Metadata.Name, hotelType),
		PremiumLevel:      premiumLevel,
		ServiceTier:       serviceTier,
		PremiumAmenities:  []string{"spa_access", "concierge_service", "room_upgrade"},
	}
}
