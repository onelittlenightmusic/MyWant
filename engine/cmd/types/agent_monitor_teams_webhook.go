package types

import (
	"context"
	"encoding/json"

	. "mywant/engine/src"
)

func init() {
	RegisterPollAgentType("monitor_teams_webhook",
		[]Capability{Cap("teams_webhook_monitoring")},
		pollTeamsWebhook)
}

// pollTeamsWebhook monitors incoming Teams webhook messages.
// It trims the message buffer to 20 entries and performs health checks.
func pollTeamsWebhook(ctx context.Context, want *Want) (shouldStop bool, err error) {
	// Check webhook status
	status, _ := want.GetStateString("teams_webhook_status", "active")
	if status == "stopped" {
		return true, nil
	}

	// Get current messages
	var messages []any
	if existing, ok := want.GetState("teams_messages"); ok {
		if arr, ok := existing.([]any); ok {
			messages = arr
		}
	}

	// Trim to last 20 messages if needed
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
		want.StoreStateMultiForAgent(map[string]any{
			"teams_messages":  messages,
			"action_by_agent": "MonitorAgent",
		})
	}

	// Get message count for health check logging
	var messageCount int
	if countVal, ok := want.GetState("teams_message_count"); ok {
		switch v := countVal.(type) {
		case int:
			messageCount = v
		case float64:
			messageCount = int(v)
		case json.Number:
			if n, err := v.Int64(); err == nil {
				messageCount = int(n)
			}
		}
	}

	// Health check: if status is "error", log and continue
	if status == "error" {
		want.StoreLog("[TEAMS-WEBHOOK-MONITOR] Webhook in error state, total messages: %d", messageCount)
	}

	return false, nil
}
