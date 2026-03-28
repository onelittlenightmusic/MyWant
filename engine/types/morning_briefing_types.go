package types

import (
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[MorningBriefingWant, MorningBriefingLocals]("briefing")
}

// MorningBriefingLocals holds no runtime locals; all state is managed via labeled state fields.
type MorningBriefingLocals struct{}

// MorningBriefingWant is an OPA-driven coordinator that follows the direction_map pattern.
//
// The `when` spec triggers a daily restart which calls Initialize() and resets all flags.
// On each tick, Progress() syncs completion flags from child Wants (written via MergeParentState)
// into the OPA current snapshot. The opa_llm_thinker evaluates the policy and writes directions;
// DispatchThinker dispatches child Wants accordingly.
//
// Flow: fetch_weather → fetch_transit → post_slack (controlled by morning_briefing.rego)
type MorningBriefingWant struct {
	Want
}

func (m *MorningBriefingWant) GetLocals() *MorningBriefingLocals {
	return CheckLocalsInitialized[MorningBriefingLocals](&m.Want)
}

// Initialize resets daily flags each time `when` triggers a restart.
// Also copies OPA config params to current state so opaLLMThinkerThink reads them correctly.
func (m *MorningBriefingWant) Initialize() {
	m.StoreState("weather_done", false)
	m.StoreState("transit_done", false)
	m.StoreState("briefing_done", false)
	m.StoreState("weather_text", "")
	m.StoreState("transit_text", "")
	m.SetCurrent("outgoing_message", "")
	// Copy OPA thinker config params → current state (opaLLMThinkerThink reads via GetCurrent)
	m.SetCurrent("use_llm", m.GetBoolParam("use_llm", false))
	m.SetCurrent("opa_llm_planner_command", m.GetStringParam("opa_llm_planner_command", "opa-llm-planner"))
	m.SetCurrent("policy_dir", m.GetStringParam("policy_dir", ""))
	m.SetCurrent("llm_provider", m.GetStringParam("llm_provider", "anthropic"))
	m.StoreLog("[BRIEFING] Initialized — daily flags reset")
}

// IsAchieved always returns false — repeats every day via `when`.
func (m *MorningBriefingWant) IsAchieved() bool { return false }

// CalculateAchievingPercentage reflects how far along today's briefing is.
func (m *MorningBriefingWant) CalculateAchievingPercentage() float64 {
	if GetState[bool](&m.Want, "briefing_done", false) {
		return 100
	}
	if GetState[bool](&m.Want, "weather_done", false) || GetState[bool](&m.Want, "transit_done", false) {
		return 50
	}
	return 10
}

// Progress syncs completion flags into the OPA current snapshot and composes the briefing
// message once weather and transit are both done.
func (m *MorningBriefingWant) Progress() {
	m.SetCurrent("achieving_percentage", m.CalculateAchievingPercentage())

	currentSnapshot := map[string]any{
		"weather_done":  GetState[bool](&m.Want, "weather_done", false),
		"transit_done":  GetState[bool](&m.Want, "transit_done", false),
		"briefing_done": GetState[bool](&m.Want, "briefing_done", false),
	}
	m.SetCurrent("current", currentSnapshot)

	// Interpret OPA directions and propose dispatch to parent Target
	InterpretDirections(&m.Want)

	weatherDone, _ := currentSnapshot["weather_done"].(bool)
	transitDone, _ := currentSnapshot["transit_done"].(bool)
	if weatherDone && transitDone && GetCurrent(m, "outgoing_message", "") == "" {
		weatherText := GetState[string](&m.Want, "weather_text", "天気情報なし")
		transitText := GetState[string](&m.Want, "transit_text", "")
		var transitLines []string
		if transitText != "" {
			transitLines = []string{transitText}
		}
		m.SetCurrent("outgoing_message", composeBriefingMessage(weatherText, transitLines))
		m.StoreLog("[BRIEFING] Message composed")
	}
}

// parseHHMM splits "HH:MM" into hour and minute integers.
func parseHHMM(s string) (int, int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, err
	}
	return t.Hour(), t.Minute(), nil
}
