package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[ClaudeCodeThreadWant, ClaudeCodeThreadLocals]("claude_code_thread")
}

// Phase constants for ClaudeCodeThreadWant
const (
	CCPhaseMonitoring       = "monitoring"
	CCPhaseTriggerReady     = "trigger_ready"
	CCPhaseRequesting       = "requesting"
	CCPhaseAwaitingResponse = "awaiting_response"
	CCPhaseResponseReceived = "response_received"
	CCPhaseAchieved         = "achieved"
	CCPhaseError            = "error"
)

// ccWebhookConfig defines webhook state key prefixes for claude_code_thread.
var ccWebhookConfig = WebhookWantConfig{
	StatePrefix:      "cc",
	MonitorAgentName: "monitor_cc_webhook",
	LogPrefix:        "[CC-WEBHOOK]",
	SecretParamName:  "webhook_secret",
}

// ClaudeCodeThreadLocals holds type-specific local state.
type ClaudeCodeThreadLocals struct {
	WebhookLocals
	SessionID  string `mywant:"internal,session_id"`
	TriggerOn  string `mywant:"internal,trigger_on"`
	MaxReqs    int    `mywant:"internal,max_requests"`
	ReqCount   int    `mywant:"internal,request_count"`
	TimeoutSec int    `mywant:"internal,timeout_seconds"`
}

// ClaudeCodeThreadWant monitors Claude Code sessions and sends requests on trigger.
type ClaudeCodeThreadWant struct {
	Want
}

func (w *ClaudeCodeThreadWant) GetLocals() *ClaudeCodeThreadLocals {
	return CheckLocalsInitialized[ClaudeCodeThreadLocals](&w.Want)
}

func (w *ClaudeCodeThreadWant) Initialize() {
	w.StoreLog("[CLAUDE_CODE] Initializing: %s", w.Metadata.Name)

	if err := w.StopAllBackgroundAgents(); err != nil {
		w.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	locals := w.GetLocals()
	// session_id is optional: if empty, DoAgent creates a new session on first trigger
	locals.SessionID = w.GetStringParam("session_id", "")

	locals.TriggerOn = w.GetStringParam("trigger_on", "pattern")
	locals.MaxReqs = w.GetIntParam("max_requests", 3)
	locals.TimeoutSec = w.GetIntParam("timeout_seconds", 300)
	locals.ReqCount = 0

	// Goal: what we're watching for
	w.SetGoal("session_id", locals.SessionID)
	w.SetGoal("trigger_on", locals.TriggerOn)
	w.SetGoal("watch_pattern", w.GetStringParam("watch_pattern", ""))
	w.SetGoal("auto_request", w.GetStringParam("auto_request", ""))
	w.SetGoal("max_requests", locals.MaxReqs)

	// Current: phase and operational state
	w.SetCurrent("phase", CCPhaseMonitoring)
	w.SetCurrent("request_count", 0)
	w.SetCurrent("interactive", true)

	// Webhook: register endpoint and start webhook monitor
	InitializeWebhook(&w.Want, ccWebhookConfig, &locals.WebhookLocals)
}

// Progress reads ThinkAgent's Plan decisions and executes state transitions.
func (w *ClaudeCodeThreadWant) Progress() {
	locals := w.GetLocals()

	// Process incoming webhook messages (updates cc_message_count, etc.)
	ProgressWebhook(&w.Want, ccWebhookConfig, &locals.WebhookLocals)

	phase := GetCurrent(&w.Want, "phase", CCPhaseMonitoring)
	nextAction := GetPlan(&w.Want, "next_action", "")

	switch phase {
	case CCPhaseMonitoring:
		if nextAction == "send_request" {
			w.SetCurrent("phase", CCPhaseTriggerReady)
		}

	case CCPhaseTriggerReady:
		// Execute DoAgent to send the request
		w.SetCurrent("phase", CCPhaseRequesting)
		if err := w.ExecuteAgents(); err != nil {
			w.StoreLog("ERROR: DoAgent execution failed: %v", err)
			w.SetCurrent("phase", CCPhaseError)
			w.SetCurrent("last_error", err.Error())
			return
		}
		w.SetCurrent("phase", CCPhaseAwaitingResponse)
		w.SetPlan("next_action", "") // consumed

	case CCPhaseAwaitingResponse:
		if nextAction == "process_response" {
			w.SetCurrent("phase", CCPhaseResponseReceived)
		} else if nextAction == "handle_timeout" {
			w.StoreLog("[CLAUDE_CODE] Response timeout, resuming monitoring")
			w.SetCurrent("phase", CCPhaseMonitoring)
			w.SetPlan("next_action", "")
		}

	case CCPhaseResponseReceived:
		locals.ReqCount++
		w.SetCurrent("request_count", locals.ReqCount)
		w.StoreLog("[CLAUDE_CODE] Request %d/%d completed", locals.ReqCount, locals.MaxReqs)

		if locals.ReqCount >= locals.MaxReqs {
			w.SetCurrent("phase", CCPhaseAchieved)
		} else {
			w.SetCurrent("phase", CCPhaseMonitoring)
		}
		w.SetPlan("next_action", "")

	case CCPhaseError:
		// Allow ThinkAgent to recover
		if nextAction == "retry" {
			w.SetCurrent("phase", CCPhaseMonitoring)
			w.SetPlan("next_action", "")
		}
	}
}

func (w *ClaudeCodeThreadWant) IsAchieved() bool {
	return GetCurrent(&w.Want, "phase", "") == CCPhaseAchieved
}
