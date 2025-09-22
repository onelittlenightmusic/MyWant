package types

import (
	"context"
	"fmt"
	. "mywant/src"
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
	// Call parent DoAgent.Exec first
	if err := a.DoAgent.Exec(ctx, want); err != nil {
		return fmt.Errorf("premium agent %s failed: %w", a.Name, err)
	}

	// Add premium-specific processing
	if want.State == nil {
		want.State = make(map[string]interface{})
	}
	want.State["premium_processed"] = true
	want.State["premium_level"] = a.PremiumLevel
	want.State["service_tier"] = a.ServiceTier

	return nil
}