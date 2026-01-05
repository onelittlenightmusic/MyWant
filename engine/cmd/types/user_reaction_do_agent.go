package types

import (
	"context"
	"fmt"
	. "mywant/engine/src"
)

// UserReactionDoAgent handles performing reaction actions
type UserReactionDoAgent struct {
	DoAgent
}

// NewUserReactionDoAgent creates a DoAgent that performs reaction actions
func NewUserReactionDoAgent() *UserReactionDoAgent {
	agent := &UserReactionDoAgent{}
	agent.BaseAgent = *NewBaseAgent(
		"user_reaction_do",
		[]string{"reaction_auto_approval"},
		DoAgentType,
	)
	agent.Action = func(ctx context.Context, want *Want) error {
		return performAutoApproval(ctx, want)
	}
	return agent
}

// performAutoApproval performs automatic approval if configured
func performAutoApproval(ctx context.Context, want *Want) error {
	want.StoreLog("[SILENCER:DO] Starting auto-approval check")
	// Get target reaction ID from state (set by SilencerWant)
	reactionIDVal, exists := want.GetState("_target_reaction_id")
	if !exists || reactionIDVal == nil || reactionIDVal == "" {
		want.StoreLog("[SILENCER:DO] No target reaction ID found in state")
		return nil
	}

	reactionID := fmt.Sprintf("%v", reactionIDVal)
	want.StoreLog("[SILENCER:DO] Target reaction ID: %s", reactionID)
	
	httpClient := want.GetHTTPClient()
	if httpClient == nil {
		want.StoreLog("[SILENCER:DO] ERROR: HTTP client not available")
		return fmt.Errorf("HTTP client not available")
	}

	want.StoreLog("[SILENCER:DO] Automatically approving reaction %s", reactionID)
	want.StoreLog("[SILENCER:DO] Sending PUT request to approve %s", reactionID)

	requestBody := map[string]any{
		"approved": true,
		"comment":  fmt.Sprintf("Automatically approved by Silencer Agent for want '%s'", want.Metadata.Name),
	}

	path := fmt.Sprintf("/api/v1/reactions/%s", reactionID)
	resp, err := httpClient.PUT(path, requestBody)
	if err != nil {
		want.StoreLog("[SILENCER:DO] ERROR: PUT request failed: %v", err)
		return fmt.Errorf("failed to send approve request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		want.StoreLog("[SILENCER:DO] ERROR: Approve request returned status %d", resp.StatusCode)
		return fmt.Errorf("approve request returned status %d", resp.StatusCode)
	}

	want.StoreLog("[SILENCER:DO] Successfully approved reaction %s", reactionID)
	
	// Clear the target reaction ID so we don't process it again
	want.StoreState("_target_reaction_id", "")
	
	return nil
}

// RegisterUserReactionDoAgent registers the user reaction do agent with the registry
func RegisterUserReactionDoAgent(registry *AgentRegistry) error {
	agent := NewUserReactionDoAgent()
	registry.RegisterAgent(agent)
	return nil
}
