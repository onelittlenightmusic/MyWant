package types

import (
	"fmt"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[WebhookReceiverWant, WebhookReceiverLocals]("webhook_receiver")
	})
}

// WebhookReceiverLocals holds local state for the generic webhook receiver.
type WebhookReceiverLocals struct {
	WebhookLocals
}

// WebhookReceiverWant is a generic HTTP POST receiver whose verification strategy,
// challenge handling, and state-key prefix are all driven by Spec params.
// Platform-specific YAML types (teams_notify, slack_notify, …) declare
// goType: webhook_receiver and supply their own param defaults.
type WebhookReceiverWant struct {
	Want
}

func (w *WebhookReceiverWant) getConfig() WebhookWantConfig {
	statePrefix := w.GetStringParam("state_prefix", "webhook")
	secretParam := w.GetStringParam("secret_param", "webhook_secret")
	return WebhookWantConfig{
		StatePrefix:      statePrefix,
		MonitorAgentName: "monitor_webhook_receiver",
		LogPrefix:        fmt.Sprintf("[WEBHOOK-RECEIVER:%s]", statePrefix),
		SecretParamName:  secretParam,
	}
}

func (w *WebhookReceiverWant) GetLocals() *WebhookReceiverLocals {
	return CheckLocalsInitialized[WebhookReceiverLocals](&w.Want)
}

func (w *WebhookReceiverWant) Initialize() {
	locals := w.GetLocals()
	InitializeWebhook(&w.Want, w.getConfig(), &locals.WebhookLocals)
}

func (w *WebhookReceiverWant) IsAchieved() bool {
	return IsWebhookAchieved(&w.Want, w.getConfig())
}

func (w *WebhookReceiverWant) CalculateAchievingPercentage() int {
	return CalcWebhookPercentage(&w.Want, w.getConfig())
}

func (w *WebhookReceiverWant) Progress() {
	locals := w.GetLocals()
	ProgressWebhook(&w.Want, w.getConfig(), &locals.WebhookLocals)
}
