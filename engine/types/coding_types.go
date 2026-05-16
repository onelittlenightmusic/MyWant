package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[CodingWant, CodingLocals]("coding")
	})
}

// CodingLocals holds type-specific local state for the coding want.
type CodingLocals struct {
	WebhookLocals
	Provider   string `mywant:"internal,provider"`
	SessionID  string `mywant:"internal,session_id"`
	MaxReqs    int    `mywant:"internal,max_requests"`
	ReqCount   int    `mywant:"internal,request_count"`
	TimeoutSec int    `mywant:"internal,timeout_seconds"`
	WorkingDir string `mywant:"internal,working_dir"`
}

// CodingWant is an interactive AI coding assistant that supports Claude Code and Gemini.
type CodingWant struct {
	Want
}

func (w *CodingWant) GetLocals() *CodingLocals {
	return CheckLocalsInitialized[CodingLocals](&w.Want)
}

func (w *CodingWant) Initialize() {
	w.StoreLog("[CODING] Initializing: %s", w.Metadata.Name)

	if err := w.StopAllBackgroundAgents(); err != nil {
		w.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	locals := w.GetLocals()

	locals.Provider = w.GetStringParam("provider", "claude_code")
	locals.SessionID = w.GetStringParam("session_id", "")
	if locals.SessionID == "" {
		locals.SessionID = GetGoal(&w.Want, "session_id", "")
	}
	locals.MaxReqs = w.GetIntParam("max_requests", 0)
	locals.TimeoutSec = w.GetIntParam("timeout_seconds", 300)
	locals.WorkingDir = w.GetStringParam("working_dir", "")

	existingCount := GetCurrent(&w.Want, "request_count", -1)
	if existingCount < 0 {
		locals.ReqCount = 0
	} else {
		locals.ReqCount = existingCount
	}

	w.SetGoal("provider", locals.Provider)
	w.SetGoal("session_id", locals.SessionID)
	w.SetGoal("auto_request", w.GetStringParam("auto_request", ""))
	w.SetGoal("max_requests", locals.MaxReqs)
	w.SetGoal("working_dir", locals.WorkingDir)
	w.SetGoal("permission_mode", w.GetStringParam("permission_mode", "default"))
	w.SetGoal("allowed_tools", w.GetStringParam("allowed_tools", ""))

	// trigger_on is always webhook for the coding type (user-driven chat)
	w.SetGoal("trigger_on", "webhook")
	w.SetGoal("watch_pattern", "")

	w.SetCurrent("phase", CCPhaseMonitoring)
	w.SetCurrent("request_count", locals.ReqCount)
	w.SetCurrent("timeout_seconds", locals.TimeoutSec)
	w.SetCurrent("interactive", true)

	InitializeWebhook(&w.Want, ccWebhookConfig, &locals.WebhookLocals)
}

// Progress reads ThinkAgent's Plan decisions and executes state transitions.
func (w *CodingWant) Progress() {
	locals := w.GetLocals()

	// coding uses HTTP-handler-driven state (cc_message_count updated directly by webhook handler).
	// ProgressWebhook is intentionally not called here to avoid StartWebhookMonitor errors
	// (monitor_cc_webhook is not registered in the agent registry for HTTP-mode webhooks).
	w.SetCurrent("achieving_percentage", 50)

	phase := GetCurrent(&w.Want, "phase", CCPhaseMonitoring)
	nextAction := GetPlan(&w.Want, "next_action", "")

	switch phase {
	case CCPhaseMonitoring:
		if nextAction == "send_request" {
			w.SetCurrent("phase", CCPhaseTriggerReady)
		}

	case CCPhaseTriggerReady:
		// Allow DoAgent to re-run for each new message (chat mode).
		// DoAgent itself clears webhook_auto_request after reading it.
		w.FinishAgentRun(ccDoAgentName, false)
		w.SetCurrent("phase", CCPhaseRequesting)
		if err := w.ExecuteAgents(); err != nil {
			w.StoreLog("ERROR: DoAgent execution failed: %v", err)
			w.SetCurrent("phase", CCPhaseError)
			w.SetCurrent("last_error", err.Error())
			return
		}
		w.SetCurrent("phase", CCPhaseAwaitingResponse)
		w.SetPlan("next_action", "")

	case CCPhaseAwaitingResponse:
		if nextAction == "process_response" {
			w.SetCurrent("phase", CCPhaseResponseReceived)
		} else if nextAction == "handle_timeout" {
			w.StoreLog("[CODING] Response timeout, resuming monitoring")
			w.SetCurrent("phase", CCPhaseMonitoring)
			w.SetPlan("next_action", "")
		}

	case CCPhaseResponseReceived:
		locals.ReqCount++
		w.SetCurrent("request_count", locals.ReqCount)
		w.StoreLog("[CODING] Request %d completed", locals.ReqCount)

		// max_requests == 0 means unlimited
		if locals.MaxReqs > 0 && locals.ReqCount >= locals.MaxReqs {
			w.SetCurrent("phase", CCPhaseAchieved)
		} else {
			w.SetCurrent("phase", CCPhaseMonitoring)
		}
		w.SetPlan("next_action", "")

	case CCPhaseError:
		if nextAction == "retry" {
			w.SetCurrent("phase", CCPhaseMonitoring)
			w.SetPlan("next_action", "")
		}
	}
}

func (w *CodingWant) IsAchieved() bool {
	return GetCurrent(&w.Want, "phase", "") == CCPhaseAchieved
}
