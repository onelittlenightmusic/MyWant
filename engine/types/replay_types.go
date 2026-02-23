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

// Initialize starts the background playwright recording monitor agent.
// The agent is discovered generically via MonitorCapabilities (derived from replay.yaml requires field)
// rather than by hardcoded agent name.
func (r *ReplayWant) Initialize() {
	r.StoreLog("[REPLAY] Initializing browser recording want: %s", r.Metadata.Name)

	locals := r.GetLocals()
	locals.MonitorStarted = false

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
