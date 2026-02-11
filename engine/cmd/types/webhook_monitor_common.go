package types

import (
	"context"

	. "mywant/engine/core"
)

// PollWebhook contains common polling logic shared by all webhook monitor agents.
// It trims the message buffer to 20 entries and performs health checks.
func PollWebhook(ctx context.Context, want *Want, cfg WebhookWantConfig) (shouldStop bool, err error) {
	status, _ := want.GetStateString(cfg.StatusKey(), "active")
	if status == "stopped" {
		return true, nil
	}

	// Get current messages
	var messages []any
	if existing, ok := want.GetState(cfg.MessagesKey()); ok {
		if arr, ok := existing.([]any); ok {
			messages = arr
		}
	}

	// Trim to last 20 messages if needed
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
		want.StoreStateMultiForAgent(map[string]any{
			cfg.MessagesKey(): messages,
			"action_by_agent": "MonitorAgent",
		})
	}

	// Health check logging for error state
	if status == "error" {
		var messageCount int
		if countVal, ok := want.GetState(cfg.MessageCountKey()); ok {
			messageCount = ParseMessageCount(countVal)
		}
		want.StoreLog("%s-MONITOR Webhook in error state, total messages: %d", cfg.LogPrefix, messageCount)
	}

	return false, nil
}
