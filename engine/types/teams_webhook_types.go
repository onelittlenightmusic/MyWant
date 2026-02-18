package types

import (
	. "mywant/engine/core"
)

var teamsWebhookConfig = WebhookWantConfig{
	StatePrefix:      "teams",
	MonitorAgentName: "monitor_teams_webhook",
	LogPrefix:        "[TEAMS-WEBHOOK]",
	SecretParamName:  "webhook_secret",
}

func init() {
	RegisterWantImplementation[TeamsWebhookWant, TeamsWebhookLocals]("teams webhook")
}

// TeamsWebhookLocals holds type-specific local state for TeamsWebhookWant
type TeamsWebhookLocals struct {
	WebhookLocals
}

// TeamsWebhookWant represents a want that receives messages from Teams Outgoing Webhooks
type TeamsWebhookWant struct {
	Want
}

func (t *TeamsWebhookWant) GetLocals() *TeamsWebhookLocals {
	return CheckLocalsInitialized[TeamsWebhookLocals](&t.Want)
}

func (t *TeamsWebhookWant) Initialize() {
	// Initialize locals (guaranteed to be initialized by framework)
	locals := t.GetLocals()
	InitializeWebhook(&t.Want, teamsWebhookConfig, &locals.WebhookLocals)
}

func (t *TeamsWebhookWant) IsAchieved() bool {
	return IsWebhookAchieved(&t.Want, teamsWebhookConfig)
}

func (t *TeamsWebhookWant) CalculateAchievingPercentage() int {
	return CalcWebhookPercentage(&t.Want, teamsWebhookConfig)
}

func (t *TeamsWebhookWant) Progress() {
	locals := t.GetLocals()
	ProgressWebhook(&t.Want, teamsWebhookConfig, &locals.WebhookLocals)
}
