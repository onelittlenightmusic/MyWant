package server

import (
	"log"

	mywant "mywant/engine/core"
)

// webhookMessage represents a parsed incoming webhook message from any platform.
type webhookMessage struct {
	Sender    string `json:"sender"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	ChannelID string `json:"channel_id"`
}

// webhookStateConfig holds state key names for a specific webhook platform.
type webhookStateConfig struct {
	LatestMessageKey string
	MessagesKey      string
	MessageCountKey  string
	LogPrefix        string
}

// storeWebhookMessage appends a message to the want state using FIFO (max 20) and
// increments the message count. This logic is shared between Teams and Slack handlers.
func storeWebhookMessage(want *mywant.Want, msg webhookMessage, cfg webhookStateConfig) {
	// Get existing messages
	messages := mywant.GetState(want, cfg.MessagesKey, []any{})

	// Append new message (FIFO, keep last 20)
	msgMap := map[string]any{
		"sender":     msg.Sender,
		"text":       msg.Text,
		"timestamp":  msg.Timestamp,
		"channel_id": msg.ChannelID,
	}
	messages = append(messages, msgMap)
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
	}

	// Get current message count
	messageCount := mywant.GetState(want, cfg.MessageCountKey, 0)
	messageCount++

	// Update want state
	mywant.StoreStateMulti(want, map[string]any{
		cfg.LatestMessageKey: msgMap,
		cfg.MessagesKey:      messages,
		cfg.MessageCountKey:  messageCount,
		"action_by_agent":    "webhook_handler",
	})

	log.Printf("%s Received message: %s from %s", cfg.LogPrefix, msg.Text, msg.Sender)
}

// webhookTypes is the list of want type names whose instances should appear in the
// webhook endpoint listing. "teams_notify" and "slack_notify" are no longer built-in
// but are loaded from custom-types; they are still listed here so that the
// GET /api/v1/webhooks endpoint remains useful even before a want of each type exists.
// Additional types are detected dynamically: any want that has a non-empty "webhook_url"
// state is also included.
var webhookTypes = []string{"webhook_receiver", "claude_code_thread", "goal"}

// storeRawWebhookPayload stores a raw JSON payload into the FIFO buffer keyed by cfg.
// Unlike storeWebhookMessage it does not try to extract sender/text fields —
// platform-specific parsing is left to downstream agents or skills.
func storeRawWebhookPayload(want *mywant.Want, payload map[string]any, cfg webhookStateConfig) {
	messages := mywant.GetState(want, cfg.MessagesKey, []any{})
	messages = append(messages, payload)
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
	}
	messageCount := mywant.GetState(want, cfg.MessageCountKey, 0)
	messageCount++
	mywant.StoreStateMulti(want, map[string]any{
		cfg.LatestMessageKey: payload,
		cfg.MessagesKey:      messages,
		cfg.MessageCountKey:  messageCount,
		"action_by_agent":    "webhook_handler",
	})
	log.Printf("%s Stored payload (count: %d)", cfg.LogPrefix, messageCount)
}

// ccStateCfg holds state key names for Claude Code webhook messages.
var ccStateCfg = webhookStateConfig{
	LatestMessageKey: "cc_latest_message",
	MessagesKey:      "cc_messages",
	MessageCountKey:  "cc_message_count",
	LogPrefix:        "[CC-WEBHOOK]",
}
