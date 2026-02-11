package server

import (
	"encoding/json"

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
	var messages []any
	if existing, ok := want.GetState(cfg.MessagesKey); ok {
		if arr, ok := existing.([]any); ok {
			messages = arr
		}
	}

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
	var messageCount int
	if countVal, ok := want.GetState(cfg.MessageCountKey); ok {
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
	messageCount++

	// Update want state
	want.StoreStateMultiForAgent(map[string]any{
		cfg.LatestMessageKey: msgMap,
		cfg.MessagesKey:      messages,
		cfg.MessageCountKey:  messageCount,
		"action_by_agent":    "webhook_handler",
	})

	want.StoreLog("%s Received message from %s: %s", cfg.LogPrefix, msg.Sender, msg.Text)
}

// webhookTypes is the list of want types that represent webhook endpoints.
var webhookTypes = []string{"teams webhook", "slack webhook"}
