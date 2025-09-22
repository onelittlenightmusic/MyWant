package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/src"
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
			BaseAgent: BaseAgent{
				Name:         name,
				Capabilities: capabilities,
				Uses:         uses,
				Type:         DoAgentType,
			},
		},
		PremiumLevel: premiumLevel,
		ServiceTier:  "premium",
	}
}

// Exec executes premium agent actions with enhanced capabilities
func (a *AgentPremium) Exec(ctx context.Context, want *Want) error {
	// Don't call parent DoAgent.Exec to avoid infinite recursion
	// The Action function already delegates to this method

	// Generate ALL hotel booking state in the agent
	fmt.Printf("[AGENT_PREMIUM] Processing hotel reservation for %s with premium service\n", want.Metadata.Name)

	// Generate premium hotel booking with better times and luxury amenities

	baseDate := time.Now().AddDate(0, 0, 1) // Tomorrow
	// Premium service: earlier check-in, later check-out
	checkInTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		14+rand.Intn(2), rand.Intn(60), 0, 0, time.Local) // 2-4 PM early check-in

	nextDay := baseDate.AddDate(0, 0, 1)
	checkOutTime := time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(),
		11+rand.Intn(2), rand.Intn(60), 0, 0, time.Local) // 11 AM - 1 PM late check-out

	// Extract hotel type from want parameters
	hotelType := "luxury" // default
	if ht, ok := want.Spec.Params["hotel_type"]; ok {
		if hts, ok := ht.(string); ok {
			hotelType = hts
		}
	}

	// Store ALL hotel booking state using proper state storage
	want.StoreState("attempted", true)
	want.StoreState("check_in_time", checkInTime.Format("15:04 Jan 2"))
	want.StoreState("check_out_time", checkOutTime.Format("15:04 Jan 2"))
	want.StoreState("hotel_type", hotelType)
	want.StoreState("stay_duration_hours", checkOutTime.Sub(checkInTime).Hours())
	want.StoreState("reservation_name", fmt.Sprintf("%s stay at %s hotel", want.Metadata.Name, hotelType))
	want.StoreState("total_processed", 1)

	// Add premium-specific processing using proper state storage
	want.StoreState("premium_processed", true)
	want.StoreState("premium_level", a.PremiumLevel)
	want.StoreState("service_tier", a.ServiceTier)
	want.StoreState("premium_amenities", []string{"spa_access", "concierge_service", "room_upgrade"})

	fmt.Printf("[AGENT_PREMIUM] Premium hotel booking completed: %s from %s to %s\n",
		hotelType, checkInTime.Format("15:04 Jan 2"), checkOutTime.Format("15:04 Jan 2"))

	return nil
}