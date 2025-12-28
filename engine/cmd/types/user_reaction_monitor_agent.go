package types

import (
	"context"
	"fmt"
	. "mywant/engine/src"
)

// UserReactionMonitorAgentFactory creates a MonitorAgent that monitors user reactions
// It's a system-wide agent that monitors all reminders for user reactions
func NewUserReactionMonitorAgent(reactionQueue *ReactionQueue) *MonitorAgent {
	agent := &MonitorAgent{
		BaseAgent: *NewBaseAgent(
			"user_reaction_monitor",
			[]string{"reminder_monitoring"},
			[]string{},
			MonitorAgentType,
		),
	}

	agent.Monitor = func(ctx context.Context, want *Want) error {
		return monitorUserReactions(ctx, want, reactionQueue)
	}

	return agent
}

// monitorUserReactions monitors a single want for user reactions
func monitorUserReactions(ctx context.Context, want *Want, reactionQueue *ReactionQueue) error {
	// Only process reminder wants in reaching phase
	if want.Metadata.Type != "reminder" {
		return nil
	}

	phase, exists := want.GetState("reminder_phase")
	if !exists {
		return nil
	}

	phaseStr := fmt.Sprintf("%v", phase)
	if phaseStr != ReminderPhaseReaching {
		return nil
	}

	// Check if reaction is required
	requireReaction, exists := want.GetState("require_reaction")
	if !exists {
		return nil
	}

	requireReactionBool := false
	if boolVal, ok := requireReaction.(bool); ok {
		requireReactionBool = boolVal
	}

	// If reaction not required, nothing to monitor
	if !requireReactionBool {
		return nil
	}

	// Get the reaction ID
	reactionID, exists := want.GetState("reaction_id")
	if !exists {
		return fmt.Errorf("reaction_id not found in state")
	}

	reactionIDStr := fmt.Sprintf("%v", reactionID)

	// Check if user reaction is available in the queue
	reaction, found := reactionQueue.GetReaction(reactionIDStr)
	if !found {
		// No reaction yet, continue waiting
		return nil
	}

	// Convert reaction to state-compatible format
	reactionData := map[string]any{
		"approved": reaction.Approved,
		"comment":  reaction.Comment,
		"timestamp": reaction.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Write reaction to state
	want.StoreState("user_reaction", reactionData)

	// Write action by agent
	want.StoreState("action_by_agent", "MonitorAgent")

	// Log the reaction
	if reaction.Approved {
		want.StoreLog(fmt.Sprintf("User approved reminder reaction (comment: %s)", reaction.Comment))
	} else {
		want.StoreLog(fmt.Sprintf("User rejected reminder reaction (comment: %s)", reaction.Comment))
	}

	// Remove reaction from queue after processing
	reactionQueue.RemoveReaction(reactionIDStr)

	return nil
}

// RegisterUserReactionMonitorAgent registers the user reaction monitor agent with the registry
func RegisterUserReactionMonitorAgent(registry *AgentRegistry, reactionQueue *ReactionQueue) error {
	agent := NewUserReactionMonitorAgent(reactionQueue)
	registry.RegisterAgent(agent)
	return nil
}
