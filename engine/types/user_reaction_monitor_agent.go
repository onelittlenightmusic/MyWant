package types

import (
	"context"
	"encoding/json"
	"fmt"
	. "mywant/engine/core"
)

const userReactionMonitorAgentName = "user_reaction_monitor"

func init() {
	RegisterPollAgentType(userReactionMonitorAgentName,
		[]Capability{
			Cap("reminder_monitoring"),
			Cap("reaction_auto_approval"),
		},
		pollUserReactions)
}

// pollUserReactions is a PollFunc — includes stop logic
func pollUserReactions(ctx context.Context, want *Want) (bool, error) {
	// Check if want is still active (waiting or reaching)
	phase, _ := want.GetStateString("reminder_phase", "")
	if phase != ReminderPhaseWaiting && phase != ReminderPhaseReaching {
		return true, nil // Stop monitoring - want completed, failed or doesn't have phase
	}

	// Monitor reaction status
	err := monitorUserReactions(ctx, want)
	if err != nil {
		return false, err
	}

	// Check if reaction was received (non-empty)
	userReaction, exists := want.GetState("user_reaction")
	if exists && userReaction != nil {
		if reactionMap, ok := userReaction.(map[string]any); ok && len(reactionMap) > 0 {
			if _, ok := reactionMap["approved"].(bool); ok {
				want.StoreLog("[MONITOR] Valid reaction received, stopping monitor")
				return true, nil // Stop monitoring - reaction received
			}
		}
	}

	return false, nil // Continue monitoring
}

// monitorUserReactions monitors a single want for user reactions via HTTP API
func monitorUserReactions(ctx context.Context, want *Want) error {
	// Only process reminder wants
	if want.Metadata.Type != "reminder" {
		return nil
	}

	var phase string
	var requireReaction bool
	var queueID string

	want.GetStateMulti(Dict{
		"reminder_phase":    &phase,
		"require_reaction":  &requireReaction,
		"reaction_queue_id": &queueID,
	})

	if phase != ReminderPhaseWaiting && phase != ReminderPhaseReaching {
		return nil
	}

	want.StoreLog("[MONITOR] Monitoring reminder want %s in phase %s", want.Metadata.Name, phase)

	// If reaction not required, nothing to monitor
	if !requireReaction {
		want.StoreLog("[MONITOR] Reaction not required for %s, skipping", want.Metadata.Name)
		return nil
	}

	// Get the reaction queue ID (set by DoAgent when queue was created)
	if queueID == "" {
		want.StoreLog("[MONITOR] No reaction_queue_id found for %s", want.Metadata.Name)
		// Queue not created yet, nothing to monitor
		return nil
	}

	want.StoreLog("[MONITOR] Checking queue %s for reactions...", queueID)

	// Get HTTP client for API calls
	httpClient := want.GetHTTPClient()
	if httpClient == nil {
		want.StoreLog("[MONITOR] ERROR: HTTP client not available for want %s - cannot monitor reactions", want.Metadata.ID)
		return nil
	}

	// Call GET /api/v1/reactions/{id} to retrieve reactions
	path := fmt.Sprintf("/api/v1/reactions/%s", queueID)
	want.StoreLog("[MONITOR] Sending GET request to: %s", path)
	resp, err := httpClient.GET(path)
	if err != nil {
		// Queue might not exist yet or other error - just log and continue
		want.StoreLog("[MONITOR] Failed to get reaction queue %s: %v", queueID, err)
		return nil
	}
	defer resp.Body.Close()

	want.StoreLog("[MONITOR] GET response status: %d", resp.StatusCode)

	// Parse response
	var result struct {
		QueueID   string         `json:"queue_id"`
		Reactions []ReactionData `json:"reactions"`
		CreatedAt string         `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		want.StoreLog("[MONITOR] Failed to decode reaction queue response: %v", err)
		return nil
	}

	want.StoreLog("[MONITOR] Retrieved %d reactions from queue %s", len(result.Reactions), queueID)

	// Check if there are any reactions
	if len(result.Reactions) == 0 {
		// No reactions yet, continue waiting
		return nil
	}

	// Process the first reaction (FIFO order)
	reaction := result.Reactions[0]
	want.StoreLog("[MONITOR] Processing reaction: approved=%v, comment=%s", reaction.Approved, reaction.Comment)

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
		want.StoreLog("User approved reminder reaction (comment: %s)", reaction.Comment)
	} else {
		want.StoreLog("User rejected reminder reaction (comment: %s)", reaction.Comment)
	}

	want.StoreLog("[INFO] MonitorAgent processed reaction %s from queue %s for want %s",
		reaction.ReactionID, queueID, want.Metadata.ID)

	return nil
}
