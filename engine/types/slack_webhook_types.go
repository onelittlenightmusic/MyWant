package types

import (
	. "mywant/engine/core"
)

var slackWebhookConfig = WebhookWantConfig{
	StatePrefix:      "slack",
	MonitorAgentName: "monitor_slack_webhook",
	LogPrefix:        "[SLACK-WEBHOOK]",
	SecretParamName:  "signing_secret",
}

func init() {
	RegisterWantImplementation[SlackWebhookWant, SlackWebhookLocals]("slack webhook")
}

// SlackWebhookLocals holds type-specific local state for SlackWebhookWant
type SlackWebhookLocals struct {
	WebhookLocals
}

// SlackWebhookWant represents a want that receives messages from Slack Events API
type SlackWebhookWant struct {
	Want
}

func (s *SlackWebhookWant) GetLocals() *SlackWebhookLocals {
	return CheckLocalsInitialized[SlackWebhookLocals](&s.Want)
}

func (s *SlackWebhookWant) Initialize() {
	// Initialize locals (guaranteed to be initialized by framework)
	locals := s.GetLocals()
	InitializeWebhook(&s.Want, slackWebhookConfig, &locals.WebhookLocals)
}

func (s *SlackWebhookWant) IsAchieved() bool {
	return IsWebhookAchieved(&s.Want, slackWebhookConfig)
}

func (s *SlackWebhookWant) CalculateAchievingPercentage() int {
	return CalcWebhookPercentage(&s.Want, slackWebhookConfig)
}

func (s *SlackWebhookWant) Progress() {
	locals := s.GetLocals()
	ProgressWebhook(&s.Want, slackWebhookConfig, &locals.WebhookLocals)
}
