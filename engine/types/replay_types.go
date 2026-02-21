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
}

// ReplayWant represents a browser recording want driven by the playwright_record_monitor agent
type ReplayWant struct {
	Want
}

func (r *ReplayWant) GetLocals() *ReplayLocals {
	return CheckLocalsInitialized[ReplayLocals](&r.Want)
}

// Initialize starts the background playwright recording monitor agent
func (r *ReplayWant) Initialize() {
	r.StoreLog("[REPLAY] Initializing browser recording want: %s", r.Metadata.Name)

	locals := r.GetLocals()
	locals.MonitorStarted = false

	agentName := "playwright-record-" + r.Metadata.ID
	registry := r.GetAgentRegistry()
	if registry == nil {
		r.StoreLog("[REPLAY] WARNING: no agent registry available")
		return
	}

	agent, ok := registry.GetAgent(playwrightRecordAgentName)
	if !ok {
		r.StoreLog("[REPLAY] WARNING: agent '%s' not found in registry", playwrightRecordAgentName)
		return
	}

	if err := r.AddMonitoringAgent(agentName, 3*time.Second, agent.Exec); err != nil {
		r.StoreLog("[REPLAY] ERROR: failed to start monitoring agent: %v", err)
		return
	}

	locals.MonitorStarted = true
	r.StoreLog("[REPLAY] Monitoring agent started, waiting for webhook trigger")
}

// IsAchieved returns true when a replay script has been recorded
func (r *ReplayWant) IsAchieved() bool {
	script, _ := r.GetStateString("replay_script", "")
	return script != ""
}

// CalculateAchievingPercentage returns progress percentage
func (r *ReplayWant) CalculateAchievingPercentage() int {
	if r.IsAchieved() || r.Status == WantStatusAchieved {
		return 100
	}
	active, _ := r.GetStateBool("recording_active", false)
	if active {
		return 50
	}
	if _, exists := r.GetState("startWebhookId"); exists {
		return 10
	}
	return 0
}

// Progress monitors recording state and marks the want as achieved when done
func (r *ReplayWant) Progress() {
	r.StoreState("achieving_percentage", r.CalculateAchievingPercentage())

	if r.IsAchieved() {
		r.SetStatus(WantStatusAchieved)
	}
}
