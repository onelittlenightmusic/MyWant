package types

import (
	"context"
	"fmt"
	. "mywant/engine/src"
	"time"
)

// AgentPremium extends PremiumServiceAgent with premium hotel service capabilities
type AgentPremium struct {
	PremiumServiceAgent
}

// NewAgentPremium creates a new premium agent
func NewAgentPremium(name string, capabilities []string, uses []string, premiumLevel string) *AgentPremium {
	return &AgentPremium{
		PremiumServiceAgent: NewPremiumServiceAgent(name, capabilities, premiumLevel),
	}
}

// Exec executes premium agent actions with enhanced capabilities
func (a *AgentPremium) Exec(ctx context.Context, want *Want) (bool, error) {
	schedule := a.generateHotelSchedule(want)
	premiumLevel := a.PremiumLevel // Capture for closure
	return a.ExecuteReservation(ctx, want, schedule, func(s interface{}) (string, string) {
		sch := s.(HotelSchedule)
		activity := fmt.Sprintf("Hotel reservation has been confirmed for %s from %s to %s (%s premium)",
			sch.HotelType,
			sch.CheckInTime.Format("15:04 Jan 2"),
			sch.CheckOutTime.Format("15:04 Jan 2"),
			premiumLevel)
		logMsg := fmt.Sprintf("Premium hotel booking completed: %s from %s to %s",
			sch.HotelType, sch.CheckInTime.Format("15:04 Jan 2"), sch.CheckOutTime.Format("15:04 Jan 2"))
		return activity, logMsg
	})
}

// generateHotelSchedule creates a premium hotel schedule
func (a *AgentPremium) generateHotelSchedule(want *Want) HotelSchedule {
	want.StoreLog(fmt.Sprintf("Processing hotel reservation for %s with premium service", want.Metadata.Name))

	// Generate premium hotel booking with better times and luxury amenities
	baseDate := time.Now().AddDate(0, 0, 1) // Tomorrow
	// Premium service: earlier check-in, later check-out
	checkInTime := GenerateRandomTimeInRange(baseDate, CheckInRange)

	nextDay := baseDate.AddDate(0, 0, 1)
	checkOutTime := GenerateRandomTimeInRange(nextDay, CheckOutRange)

	// Extract hotel type from want parameters
	hotelType := want.GetStringParam("hotel_type", "luxury")
	return HotelSchedule{
		CheckInTime:       checkInTime,
		CheckOutTime:      checkOutTime,
		HotelType:         hotelType,
		StayDurationHours: checkOutTime.Sub(checkInTime).Hours(),
		ReservationName:   fmt.Sprintf("%s stay at %s hotel", want.Metadata.Name, hotelType),
		PremiumLevel:      a.PremiumLevel,
		ServiceTier:       a.ServiceTier,
		PremiumAmenities:  []string{"spa_access", "concierge_service", "room_upgrade"},
	}
}
