package types

import (
	"encoding/json"
	"fmt"
	. "mywant/engine/src"
	"time"
)

const teamsWebhookMonitorAgentName = "monitor_teams_webhook"

func init() {
	RegisterWantImplementation[TeamsWebhookWant, TeamsWebhookLocals]("teams webhook")
}

// TeamsWebhookLocals holds type-specific local state for TeamsWebhookWant
type TeamsWebhookLocals struct {
	WebhookSecret      string
	ChannelFilter      string
	LastProcessedCount int
}

// TeamsWebhookWant represents a want that receives messages from Teams Outgoing Webhooks
type TeamsWebhookWant struct {
	Want
}

func (t *TeamsWebhookWant) GetLocals() *TeamsWebhookLocals {
	return GetLocals[TeamsWebhookLocals](&t.Want)
}

// Initialize prepares the teams webhook want for receiving messages
func (t *TeamsWebhookWant) Initialize() {
	t.StoreLog("[TEAMS-WEBHOOK] Initializing teams webhook: %s", t.Metadata.Name)

	// Stop any existing background agents before fresh start
	if err := t.StopAllBackgroundAgents(); err != nil {
		t.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	locals := t.GetLocals()
	if locals == nil {
		locals = &TeamsWebhookLocals{}
		t.Locals = locals
	}

	// Read params
	locals.WebhookSecret = t.GetStringParam("webhook_secret", "")
	locals.ChannelFilter = t.GetStringParam("channel_filter", "")
	locals.LastProcessedCount = 0

	// Store initial state
	webhookURL := fmt.Sprintf("/api/v1/webhooks/%s", t.Metadata.ID)
	stateMap := map[string]any{
		"teams_webhook_status": "active",
		"teams_messages":       []any{},
		"teams_message_count":  0,
		"webhook_url":          webhookURL,
	}

	// Store webhook_secret in state so the webhook handler can read it for HMAC verification
	if locals.WebhookSecret != "" {
		stateMap["webhook_secret"] = locals.WebhookSecret
	}

	if locals.ChannelFilter != "" {
		stateMap["channel_filter"] = locals.ChannelFilter
	}

	t.StoreStateMulti(stateMap)
	t.Locals = locals

	t.StoreLog("[TEAMS-WEBHOOK] Webhook URL: POST /api/v1/webhooks/%s", t.Metadata.ID)

	// Start background monitoring agent
	t.startMonitoringAgent()
}

// startMonitoringAgent starts the background monitor agent if not already running
func (t *TeamsWebhookWant) startMonitoringAgent() {
	agentName := "teams-webhook-monitor-" + t.Metadata.ID
	if _, exists := t.GetBackgroundAgent(agentName); exists {
		return
	}

	if agent, ok := t.GetAgentRegistry().GetAgent(teamsWebhookMonitorAgentName); ok {
		if err := t.AddMonitoringAgent(agentName, 5*time.Second, agent.Exec); err != nil {
			t.StoreLog("ERROR: Failed to start teams webhook monitoring: %v", err)
		} else {
			t.StoreLog("[TEAMS-WEBHOOK] Background monitor agent started")
		}
	} else {
		t.StoreLog("ERROR: Monitor agent %s not found in registry", teamsWebhookMonitorAgentName)
	}
}

// IsAchieved checks if the webhook has been stopped
func (t *TeamsWebhookWant) IsAchieved() bool {
	status, _ := t.GetStateString("teams_webhook_status", "")
	return status == "stopped"
}

// CalculateAchievingPercentage returns the progress percentage
func (t *TeamsWebhookWant) CalculateAchievingPercentage() int {
	if t.IsAchieved() {
		return 100
	}
	status, _ := t.GetStateString("teams_webhook_status", "")
	if status == "active" {
		return 50
	}
	return 0
}

// Progress checks for new messages and processes them
func (t *TeamsWebhookWant) Progress() {
	locals := t.getOrInitializeLocals()

	status, _ := t.GetStateString("teams_webhook_status", "active")
	if status == "stopped" {
		return
	}

	// Ensure monitor agent is running (handles restart case)
	t.startMonitoringAgent()

	// Get current message count from state
	currentCount := 0
	if countVal, ok := t.GetState("teams_message_count"); ok {
		switch v := countVal.(type) {
		case int:
			currentCount = v
		case float64:
			currentCount = int(v)
		case json.Number:
			if n, err := v.Int64(); err == nil {
				currentCount = int(n)
			}
		}
	}

	// Check for new messages
	if currentCount > locals.LastProcessedCount {
		newCount := currentCount - locals.LastProcessedCount
		t.StoreLog("[TEAMS-WEBHOOK] %d new message(s) received (total: %d)", newCount, currentCount)

		// Get the latest message and provide to downstream wants
		if latestMsg, ok := t.GetState("teams_latest_message"); ok {
			t.Provide(latestMsg)
		}

		locals.LastProcessedCount = currentCount
		t.Locals = locals
	}

	t.StoreState("achieving_percentage", t.CalculateAchievingPercentage())
}

// getOrInitializeLocals retrieves or initializes the locals
func (t *TeamsWebhookWant) getOrInitializeLocals() *TeamsWebhookLocals {
	if locals := t.GetLocals(); locals != nil {
		return locals
	}

	locals := &TeamsWebhookLocals{}

	// Restore from state
	if count, ok := t.GetState("teams_message_count"); ok {
		switch v := count.(type) {
		case int:
			locals.LastProcessedCount = v
		case float64:
			locals.LastProcessedCount = int(v)
		}
	}

	secret, _ := t.GetStateString("webhook_secret", "")
	locals.WebhookSecret = secret
	filter, _ := t.GetStateString("channel_filter", "")
	locals.ChannelFilter = filter

	t.Locals = locals
	return locals
}
