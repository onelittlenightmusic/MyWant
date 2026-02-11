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
	return GetLocals[SlackWebhookLocals](&s.Want)
}

func (s *SlackWebhookWant) Initialize() {
	locals := &SlackWebhookLocals{}
	s.Locals = locals
	InitializeWebhook(&s.Want, slackWebhookConfig, &locals.WebhookLocals)
}

func (s *SlackWebhookWant) IsAchieved() bool {
	return IsWebhookAchieved(&s.Want, slackWebhookConfig)
}

func (s *SlackWebhookWant) CalculateAchievingPercentage() int {
	return CalcWebhookPercentage(&s.Want, slackWebhookConfig)
}

func (s *SlackWebhookWant) Progress() {
	locals := s.getOrInitializeLocals()
	ProgressWebhook(&s.Want, slackWebhookConfig, &locals.WebhookLocals)
}

func (s *SlackWebhookWant) getOrInitializeLocals() *SlackWebhookLocals {
	if locals := s.GetLocals(); locals != nil {
		return locals
	}
	common := RestoreWebhookLocals(&s.Want, slackWebhookConfig)
	locals := &SlackWebhookLocals{WebhookLocals: *common}
	s.Locals = locals
	return locals
}
