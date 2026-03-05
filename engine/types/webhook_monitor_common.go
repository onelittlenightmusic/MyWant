package types

import (
	"context"

	. "mywant/engine/core"
)

// PollWebhook contains common polling logic shared by all webhook monitor agents.
func PollWebhook(ctx context.Context, want *Want, cfg WebhookWantConfig) (shouldStop bool, err error) {
	statusRaw, ok := want.GetCurrent("status")
	status := "active"
	if ok && statusRaw != nil { status = statusRaw.(string) }
	
	if status == "stopped" {
		return true, nil
	}

	// Get current messages from GCP current state
	var messages []any
	if existing, ok := want.GetCurrent("messages"); ok {
		if arr, ok := existing.([]any); ok {
			messages = arr
		}
	}

	// Trim to last 20 messages if needed
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
		want.SetCurrent("messages", messages)
	}

	// Health check logging for error state
	if status == "error" {
		var messageCount int
		if countVal, ok := want.GetCurrent("message_count"); ok {
			messageCount = int(toFloat64(countVal))
		}
		want.StoreLog("%s-MONITOR Webhook in error state, total messages: %d", cfg.LogPrefix, messageCount)
	}

	return false, nil
}
