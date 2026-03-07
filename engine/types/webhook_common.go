package types

import (
	"encoding/json"
	"fmt"
	. "mywant/engine/core"
	"time"
)

// WebhookLocals holds common local state shared by all webhook want types.
type WebhookLocals struct {
	LastProcessedCount int

	// State fields (auto-synced)
	WebhookSecret string `mywant:"internal,webhook_secret"`
	ChannelFilter string `mywant:"internal,channel_filter"`
	WebhookUrl    string `mywant:"current,webhook_url"`
}

// WebhookWantConfig defines per-webhook-type prefixes and names used by the common helpers.
type WebhookWantConfig struct {
	// StatePrefix is prepended to state keys, e.g. "teams" → "teams_webhook_status".
	StatePrefix string
	// MonitorAgentName is the registered poll-agent name, e.g. "monitor_teams_webhook".
	MonitorAgentName string
	// LogPrefix appears in log messages, e.g. "[TEAMS-WEBHOOK]".
	LogPrefix string
	// SecretParamName is the parameter name for the webhook secret.
	SecretParamName string
}

// state key helpers
func (c WebhookWantConfig) StatusKey() string        { return c.StatePrefix + "_webhook_status" }
func (c WebhookWantConfig) MessagesKey() string      { return c.StatePrefix + "_messages" }
func (c WebhookWantConfig) MessageCountKey() string  { return c.StatePrefix + "_message_count" }
func (c WebhookWantConfig) LatestMessageKey() string { return c.StatePrefix + "_latest_message" }

// ParseMessageCount extracts an int from values that may be int, float64 or json.Number.
func ParseMessageCount(val any) int {
	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return 0
}

// InitializeWebhook performs common initialization for any webhook want type.
func InitializeWebhook(want *Want, cfg WebhookWantConfig, locals *WebhookLocals) {
	want.StoreLog("%s Initializing webhook: %s", cfg.LogPrefix, want.Metadata.Name)

	if err := want.StopAllBackgroundAgents(); err != nil {
		want.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	locals.WebhookSecret = want.GetStringParam(cfg.SecretParamName, "")
	locals.ChannelFilter = want.GetStringParam("channel_filter", "")
	locals.LastProcessedCount = 0
	locals.WebhookUrl = fmt.Sprintf("/api/v1/webhooks/%s", want.Metadata.Name)
	
	want.SetCurrent(cfg.StatusKey(), "active")
	want.SetCurrent(cfg.MessagesKey(), []any{})
	want.SetCurrent(cfg.MessageCountKey(), 0)

	want.StoreLog("%s Webhook URL: POST /api/v1/webhooks/%s", cfg.LogPrefix, want.Metadata.Name)

	StartWebhookMonitor(want, cfg)
}

// StartWebhookMonitor starts the background monitor agent if not already running.
func StartWebhookMonitor(want *Want, cfg WebhookWantConfig) {
	agentName := cfg.StatePrefix + "-webhook-monitor-" + want.Metadata.ID
	if _, exists := want.GetBackgroundAgent(agentName); exists {
		return
	}
	if agent, ok := want.GetAgentRegistry().GetAgent(cfg.MonitorAgentName); ok {
		if err := want.AddMonitoringAgent(agentName, 5*time.Second, agent.Exec); err != nil {
			want.StoreLog("ERROR: Failed to start webhook monitoring: %v", err)
		} else {
			want.StoreLog("%s Background monitor agent started", cfg.LogPrefix)
		}
	} else {
		want.StoreLog("ERROR: Monitor agent %s not found in registry", cfg.MonitorAgentName)
	}
}

// ProgressWebhook performs common progress-cycle logic
func ProgressWebhook(want *Want, cfg WebhookWantConfig, locals *WebhookLocals) {
	status := GetCurrent(want, cfg.StatusKey(), "active")
	if status == "stopped" {
		return
	}

	StartWebhookMonitor(want, cfg)

	currentCount := GetCurrent(want, cfg.MessageCountKey(), 0)

	if currentCount > locals.LastProcessedCount {
		newCount := currentCount - locals.LastProcessedCount
		want.StoreLog("%s %d new message(s) received (total: %d)", cfg.LogPrefix, newCount, currentCount)

		if latestMsg, ok := want.GetCurrent(cfg.LatestMessageKey()); ok && latestMsg != nil {
			want.Provide(latestMsg)
		}

		locals.LastProcessedCount = currentCount
	}

	want.SetPredefined("achieving_percentage", CalcWebhookPercentage(want, cfg))
}

// IsWebhookAchieved returns true when the webhook status is "stopped".
func IsWebhookAchieved(want *Want, cfg WebhookWantConfig) bool {
	return GetCurrent(want, cfg.StatusKey(), "") == "stopped"
}

// CalcWebhookPercentage returns 100 when stopped, 50 when active, 0 otherwise.
func CalcWebhookPercentage(want *Want, cfg WebhookWantConfig) int {
	if IsWebhookAchieved(want, cfg) {
		return 100
	}
	status := GetCurrent(want, cfg.StatusKey(), "")
	if status == "active" {
		return 50
	}
	return 0
}

// RestoreWebhookLocals restores locals from persisted state.
func RestoreWebhookLocals(want *Want, cfg WebhookWantConfig) *WebhookLocals {
	locals := &WebhookLocals{}
	locals.LastProcessedCount = GetCurrent(want, cfg.MessageCountKey(), 0)
	locals.WebhookSecret = GetInternal(want, "webhook_secret", "")
	locals.ChannelFilter = GetInternal(want, "channel_filter", "")
	want.Locals = locals
	return locals
}
