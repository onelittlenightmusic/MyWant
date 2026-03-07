package types

import (
	"context"

	. "mywant/engine/core"
)

// PollWebhook contains common polling logic shared by all webhook monitor agents.
func PollWebhook(ctx context.Context, want *Want, cfg WebhookWantConfig) (shouldStop bool, err error) {
	status := GetCurrent(want, "status", "active")
	
	if status == "stopped" {
		return true, nil
	}

	// Get current messages from GCP current state
	messages := GetCurrent(want, "messages", []any{})

	// Trim to last 20 messages if needed
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
		want.SetCurrent("messages", messages)
	}

	// Health check logging for error state
	if status == "error" {
		messageCount := GetCurrent(want, "message_count", 0)
		want.StoreLog("%s-MONITOR Webhook in error state, total messages: %d", cfg.LogPrefix, messageCount)
	}

	return false, nil
}
