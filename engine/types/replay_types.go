package types

import (
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[ReplayWant, ReplayLocals]("replay")
}

// ReplayLocals holds type-specific local state for ReplayWant
type ReplayLocals struct {
	MonitorStarted bool

	// State fields (auto-synced)
	StartWebhookId               string `mywant:"internal,startWebhookId"`
	StopWebhookId                string `mywant:"internal,stopWebhookId"`
	StartRecordingRequested      bool   `mywant:"internal,start_recording_requested"`
	StopRecordingRequested       bool   `mywant:"internal,stop_recording_requested"`
	StartDebugRecordingRequested bool   `mywant:"internal,start_debug_recording_requested"`
	StopDebugRecordingRequested  bool   `mywant:"internal,stop_debug_recording_requested"`
	RecordingSessionId           string `mywant:"internal,recording_session_id"`
	DebugStartWebhookId          string `mywant:"internal,debugStartWebhookId"`
	DebugStopWebhookId           string `mywant:"internal,debugStopWebhookId"`
	DebugRecordingSessionId      string `mywant:"internal,debug_recording_session_id"`
	ReplayWebhookId              string `mywant:"internal,replayWebhookId"`
	StartReplayRequested         bool   `mywant:"internal,start_replay_requested"`
	ReplaySessionId              string `mywant:"internal,replay_session_id"`

	// Fields from YAML (Current)
}

func (r *ReplayWant) IsRecordingActive() bool {
	return GetCurrent(r, "recording_active", false)
}

func (r *ReplayWant) GetReplayScript() string {
	return GetCurrent(r, "replay_script", "")
}

// ReplayWant represents a browser recording want driven by the playwright_record_monitor agent
type ReplayWant struct {
	Want
}

func (r *ReplayWant) GetLocals() *ReplayLocals {
	return CheckLocalsInitialized[ReplayLocals](&r.Want)
}

// Initialize starts the background playwright recording monitor agent.
func (r *ReplayWant) Initialize() {
	r.StoreLog("[REPLAY] Initializing browser recording want: %s", r.Metadata.Name)

	locals := r.GetLocals()
	locals.MonitorStarted = false

	// Clear/Initialize state
	locals.StartWebhookId = ""
	locals.StopWebhookId = ""
	locals.StartRecordingRequested = false
	locals.StopRecordingRequested = false
	locals.StartDebugRecordingRequested = false
	locals.StopDebugRecordingRequested = false
	locals.RecordingSessionId = ""
	locals.DebugStartWebhookId = ""
	locals.DebugStopWebhookId = ""
	locals.DebugRecordingSessionId = ""
	locals.ReplayWebhookId = ""
	locals.StartReplayRequested = false
	locals.ReplaySessionId = ""

	// Ensure current state is cleared
	r.SetCurrent("recording_active", false)
	r.SetCurrent("replay_script", "")

	// Copy config params → state so the agent reads from GetCurrent instead of GetStringParam
	r.SetCurrent("target_url", r.GetStringParam("target_url", "https://example.com"))
	r.SetCurrent("debug_chrome_host", r.GetStringParam("debug_chrome_host", "localhost"))
	r.SetCurrent("debug_chrome_port", r.GetStringParam("debug_chrome_port", "9222"))

	typeDef := r.WantTypeDefinition
	if typeDef == nil || len(typeDef.MonitorCapabilities) == 0 {
		r.StoreLog("[REPLAY] WARNING: no MonitorCapabilities found in type definition")
		return
	}

	registry := r.GetAgentRegistry()
	if registry == nil {
		r.StoreLog("[REPLAY] WARNING: no agent registry available")
		return
	}

	for _, monCap := range typeDef.MonitorCapabilities {
		agents := registry.FindMonitorAgentsByCapabilityName(monCap.Capability)
		if len(agents) == 0 {
			r.StoreLog("[REPLAY] WARNING: no MonitorAgent found for capability '%s'", monCap.Capability)
			continue
		}
		agentName := monCap.Capability + "-" + r.Metadata.ID
		interval := 3 * time.Second
		if monCap.IntervalSeconds > 0 {
			interval = time.Duration(monCap.IntervalSeconds) * time.Second
		}
		if err := r.AddMonitoringAgent(agentName, interval, agents[0].Exec); err != nil {
			r.StoreLog("[REPLAY] ERROR: failed to start monitoring agent: %v", err)
			continue
		}
		locals.MonitorStarted = true
		r.StoreLog("[REPLAY] Monitoring agent started for capability '%s', waiting for webhook trigger", monCap.Capability)
	}
}

// IsAchieved returns true when a replay has been successfully completed.
// Recording alone does not achieve the want — the monitoring agent must stay alive
// to handle replay requests after recording finishes.
func (r *ReplayWant) IsAchieved() bool {
	return r.GetReplayScript() != "" && GetCurrent(r, "replay_result", "") != ""
}

// CalculateAchievingPercentage returns progress percentage
func (r *ReplayWant) CalculateAchievingPercentage() int {
	locals := r.GetLocals()
	if r.IsAchieved() || r.Status == WantStatusAchieved {
		return 100
	}
	if r.IsRecordingActive() {
		return 50
	}
	if locals.StartWebhookId != "" {
		return 10
	}
	return 0
}

// Progress monitors recording state and marks the want as achieved when done
func (r *ReplayWant) Progress() {
	r.SetCurrent("achieving_percentage", r.CalculateAchievingPercentage())

	if r.IsAchieved() {
		r.SetStatus(WantStatusAchieved)
	}
}
