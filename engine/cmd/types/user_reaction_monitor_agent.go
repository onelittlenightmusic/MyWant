package types

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	. "mywant/engine/src"
)

// CreateReactionMonitorPollFunc returns a PollFunc for reaction monitoring
// This is used with Want.AddMonitoringAgent()
func CreateReactionMonitorPollFunc() PollFunc {
	return func(ctx context.Context, w *Want) (bool, error) {
		// Check if want is still in reaching phase
		phase, exists := w.GetState("reminder_phase")
		if !exists || phase != ReminderPhaseReaching {
			return true, nil // Stop monitoring
		}

		// Monitor reaction status
		err := monitorUserReactions(ctx, w)
		if err != nil {
			return false, err
		}

		// Check if reaction was received (non-empty)
		if userReaction, exists := w.GetState("user_reaction"); exists && userReaction != nil {
			if reactionMap, ok := userReaction.(map[string]any); ok && len(reactionMap) > 0 {
				return true, nil // Stop monitoring
			}
		}

		return false, nil
	}
}

// NewUserReactionMonitorAgent creates a MonitorAgent that monitors user reactions via HTTP API
// This is used for registration in the agent registry
// The actual continuous monitoring is handled by AddMonitoringAgent in ReminderWant
func NewUserReactionMonitorAgent() *MonitorAgent {
	agent := &MonitorAgent{
		BaseAgent: *NewBaseAgent(
			"user_reaction_monitor",
			[]string{"reminder_monitoring"},
			MonitorAgentType,
		),
	}

	agent.Monitor = func(ctx context.Context, want *Want) error {
		return monitorUserReactions(ctx, want)
	}

	return agent
}

// monitorUserReactions monitors a single want for user reactions via HTTP API
func monitorUserReactions(ctx context.Context, want *Want) error {
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

	log.Printf("[MONITOR] Monitoring reminder want %s in reaching phase", want.Metadata.Name)

	// Check if reaction is required
	requireReaction, exists := want.GetState("require_reaction")
	if !exists {
		log.Printf("[MONITOR] No require_reaction state found for %s", want.Metadata.Name)
		return nil
	}

	requireReactionBool := false
	if boolVal, ok := requireReaction.(bool); ok {
		requireReactionBool = boolVal
	}

	// If reaction not required, nothing to monitor
	if !requireReactionBool {
		log.Printf("[MONITOR] Reaction not required for %s, skipping", want.Metadata.Name)
		return nil
	}

	// Get the reaction queue ID (set by DoAgent when queue was created)
	queueIDValue, exists := want.GetState("reaction_queue_id")
	if !exists || queueIDValue == nil || queueIDValue == "" {
		log.Printf("[MONITOR] No reaction_queue_id found for %s", want.Metadata.Name)
		// Queue not created yet, nothing to monitor
		return nil
	}

	queueID := fmt.Sprintf("%v", queueIDValue)
	log.Printf("[MONITOR] Checking queue %s for reactions...", queueID)

	// Get HTTP client for API calls
	httpClient := want.GetHTTPClient()
	if httpClient == nil {
		log.Printf("[MONITOR] ERROR: HTTP client not available for want %s - cannot monitor reactions", want.Metadata.ID)
		return nil
	}

	// Call GET /api/v1/reactions/{id} to retrieve reactions
	path := fmt.Sprintf("/api/v1/reactions/%s", queueID)
	log.Printf("[MONITOR] Sending GET request to: %s", path)
	resp, err := httpClient.GET(path)
	if err != nil {
		// Queue might not exist yet or other error - just log and continue
		log.Printf("[MONITOR] Failed to get reaction queue %s: %v", queueID, err)
		return nil
	}
	defer resp.Body.Close()

	log.Printf("[MONITOR] GET response status: %d", resp.StatusCode)

	// Parse response
	var result struct {
		QueueID   string         `json:"queue_id"`
		Reactions []ReactionData `json:"reactions"`
		CreatedAt string         `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[MONITOR] Failed to decode reaction queue response: %v", err)
		return nil
	}

	log.Printf("[MONITOR] Retrieved %d reactions from queue %s", len(result.Reactions), queueID)

	// Check if there are any reactions
	if len(result.Reactions) == 0 {
		// No reactions yet, continue waiting
		log.Printf("[MONITOR] No reactions yet in queue %s", queueID)
		return nil
	}

	// Process the first reaction (FIFO order)
	reaction := result.Reactions[0]
	log.Printf("[MONITOR] Processing reaction: approved=%v, comment=%s", reaction.Approved, reaction.Comment)

	// Convert reaction to state-compatible format
	reactionData := map[string]any{
		"approved":  reaction.Approved,
		"comment":   reaction.Comment,
		"timestamp": reaction.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Write reaction to state (use StoreStateForAgent for BackgroundAgent)
	want.StoreStateForAgent("user_reaction", reactionData)

	// Write action by agent
	want.StoreStateForAgent("action_by_agent", "MonitorAgent")

	// Log the reaction
	if reaction.Approved {
		want.StoreLog(fmt.Sprintf("User approved reminder reaction (comment: %s)", reaction.Comment))
	} else {
		want.StoreLog(fmt.Sprintf("User rejected reminder reaction (comment: %s)", reaction.Comment))
	}

	log.Printf("[INFO] MonitorAgent processed reaction %s from queue %s for want %s",
		reaction.ReactionID, queueID, want.Metadata.ID)

	return nil
}

// RegisterUserReactionMonitorAgent registers the user reaction monitor agent with the registry
func RegisterUserReactionMonitorAgent(registry *AgentRegistry) error {
	agent := NewUserReactionMonitorAgent()
	registry.RegisterAgent(agent)
	return nil
}
