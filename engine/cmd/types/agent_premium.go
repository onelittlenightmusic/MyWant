package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/src"
	"time"
)

// AgentPremium extends DoAgent with premium service capabilities
type AgentPremium struct {
	DoAgent
	PremiumLevel string
	ServiceTier  string
}

// NewAgentPremium creates a new premium agent
func NewAgentPremium(name string, capabilities []string, uses []string, premiumLevel string) *AgentPremium {
	return &AgentPremium{
		DoAgent: DoAgent{
			BaseAgent: *NewBaseAgent(name, capabilities, DoAgentType),
		},
		PremiumLevel: premiumLevel,
		ServiceTier:  "premium",
	}
}

// Exec executes premium agent actions with enhanced capabilities
func (a *AgentPremium) Exec(ctx context.Context, want *Want) (bool, error) {
	// Generate premium hotel booking schedule
	schedule := a.generateHotelSchedule(want)
	want.StoreStateForAgent("agent_result", schedule)

	// Record activity description for agent history
	activity := fmt.Sprintf("Hotel reservation has been confirmed for %s from %s to %s (%s premium)",
		schedule.HotelType,
		schedule.CheckInTime.Format("15:04 Jan 2"),
		schedule.CheckOutTime.Format("15:04 Jan 2"),
		a.PremiumLevel)
	want.SetAgentActivity(a.Name, activity)

	want.StoreLog(fmt.Sprintf("Premium hotel booking completed: %s from %s to %s",
		schedule.HotelType, schedule.CheckInTime.Format("15:04 Jan 2"), schedule.CheckOutTime.Format("15:04 Jan 2")))

	return false, nil // Action completed
}

// generateHotelSchedule creates a premium hotel schedule
func (a *AgentPremium) generateHotelSchedule(want *Want) HotelSchedule {
	want.StoreLog(fmt.Sprintf("Processing hotel reservation for %s with premium service", want.Metadata.Name))

	// Generate premium hotel booking with better times and luxury amenities
	baseDate := time.Now().AddDate(0, 0, 1) // Tomorrow
	// Premium service: earlier check-in, later check-out
	checkInTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		14+rand.Intn(2), rand.Intn(60), 0, 0, time.Local) // 2-4 PM early check-in

	nextDay := baseDate.AddDate(0, 0, 1)
	checkOutTime := time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(),
		11+rand.Intn(2), rand.Intn(60), 0, 0, time.Local) // 11 AM - 1 PM late check-out

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
