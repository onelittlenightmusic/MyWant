package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/src"
	"time"
)

// AgentRestaurant extends DoAgent with restaurant reservation capabilities
type AgentRestaurant struct {
	DoAgent
	PremiumLevel string
	ServiceTier  string
}

// NewAgentRestaurant creates a new restaurant agent
func NewAgentRestaurant(name string, capabilities []string, uses []string, premiumLevel string) *AgentRestaurant {
	return &AgentRestaurant{
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

// Exec executes restaurant agent actions and returns RestaurantSchedule
func (a *AgentRestaurant) Exec(ctx context.Context, want *Want) error {
	// Generate restaurant reservation schedule
	schedule := a.generateRestaurantSchedule(want)

	// Store the result using StoreState method
	want.StoreState("agent_result", schedule)

	// Record activity description for agent history
	activity := fmt.Sprintf("Restaurant reservation has been booked at %s %s for %.1f hours",
		schedule.RestaurantType, schedule.ReservationTime.Format("15:04 Jan 2"), schedule.DurationHours)
	want.SetAgentActivity(a.Name, activity)

	fmt.Printf("[AGENT_RESTAURANT] Restaurant reservation completed: %s at %s for %.1f hours\n",
		schedule.RestaurantType, schedule.ReservationTime.Format("15:04 Jan 2"), schedule.DurationHours)

	return nil
}

// generateRestaurantSchedule creates a restaurant reservation schedule
func (a *AgentRestaurant) generateRestaurantSchedule(want *Want) RestaurantSchedule {
	fmt.Printf("[AGENT_RESTAURANT] Processing restaurant reservation for %s with premium service\n", want.Metadata.Name)

	// Generate restaurant reservation with appropriate timing
	baseDate := time.Now()
	// Restaurant reservations typically in evening hours (6-9 PM)
	reservationTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		18+rand.Intn(3), rand.Intn(60), 0, 0, time.Local) // 6-9 PM

	// Restaurant meals typically 1.5-3 hours
	durationHours := 1.5 + rand.Float64()*1.5 // 1.5-3 hours

	// Extract restaurant type from want parameters
	restaurantType := "fine dining" // default
	if rt, ok := want.Spec.Params["restaurant_type"]; ok {
		if rts, ok := rt.(string); ok {
			restaurantType = rts
		}
	}

	// Create and return structured restaurant schedule
	return RestaurantSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		RestaurantType:   restaurantType,
		ReservationName:  fmt.Sprintf("%s reservation at %s restaurant", want.Metadata.Name, restaurantType),
		PremiumLevel:     a.PremiumLevel,
		ServiceTier:      a.ServiceTier,
		PremiumAmenities: []string{"wine_pairing", "chef_special", "priority_seating"},
	}
}
