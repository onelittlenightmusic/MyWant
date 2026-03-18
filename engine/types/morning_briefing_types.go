package types

import (
	"fmt"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[MorningBriefingWant, MorningBriefingLocals]("morning_briefing")
}

// MorningBriefingLocals holds no runtime locals; all state is managed via labeled state fields.
type MorningBriefingLocals struct{}

// MorningBriefingWant is an OPA-driven coordinator.
//
// On each tick it:
//  1. Reads sibling Want states (weather, transit-N) and updates own "current" object
//     so the background opa_llm_thinker ThinkAgent can evaluate the policy.
//  2. Reads the "directions" plan field written by the thinker.
//  3. When "post_slack" direction appears, composes the briefing message from sibling
//     states and publishes it as outgoing_message for the sibling SlackPostWant.
//
// Sibling naming convention (based on own name):
//
//	weather:   {name}-weather
//	transit N: {name}-transit-{N}
//	slack:     {name}-slack
type MorningBriefingWant struct {
	Want
}

func (m *MorningBriefingWant) GetLocals() *MorningBriefingLocals {
	return CheckLocalsInitialized[MorningBriefingLocals](&m.Want)
}

// Initialize sets the OPA goal so the thinker can evaluate the policy from the first tick.
// Trigger timing is handled by the Want's "when" spec — no time param needed here.
func (m *MorningBriefingWant) Initialize() {
	// Copy config params → state so Progress() reads from GetCurrent instead of Spec.Params
	weatherCity := m.GetStringParam("weather_city", "Tokyo")
	m.SetCurrent("weather_city", weatherCity)
	if rawRoutes, ok := m.Spec.Params["routes"]; ok && rawRoutes != nil {
		m.SetCurrent("routes", rawRoutes)
	}

	routes, _ := parseBriefingRoutes(&m.Want)
	routeGoals := make([]map[string]any, 0, len(routes))
	for _, r := range routes {
		routeGoals = append(routeGoals, map[string]any{
			"name":        r.Name,
			"origin":      r.Origin,
			"destination": r.Destination,
			"arrive_by":   r.ArriveBy,
			"days":        r.Days,
		})
	}
	m.SetGoal("goal", map[string]any{
		"weather_city": weatherCity,
		"routes":       routeGoals,
	})
	m.StoreLog("[BRIEFING] Initialized with %d route(s)", len(routes))
}

// IsAchieved always returns false — repeats every day.
func (m *MorningBriefingWant) IsAchieved() bool { return false }

// CalculateAchievingPercentage reflects whether today's message has been composed.
func (m *MorningBriefingWant) CalculateAchievingPercentage() float64 {
	today := time.Now().Format("2006-01-02")
	if GetCurrent(m, "outgoing_date", "") == today {
		return 100
	}
	if GetCurrent(m, "briefing_status", "") == "composing" {
		return 50
	}
	return 10
}

// Progress syncs sibling state into own "current" for the OPA thinker, then
// acts on any directions the thinker has written.
func (m *MorningBriefingWant) Progress() {
	m.SetCurrent("achieving_percentage", m.CalculateAchievingPercentage())

	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	states := cb.GetAllWantStates()
	myName := m.Want.Metadata.Name
	today := time.Now().Format("2006-01-02")

	// Build current snapshot from sibling Want states.
	// The OPA thinker reads this to evaluate whether all prerequisites are met.
	currentSnapshot := map[string]any{
		"today":        today,
		"outgoing_date": GetCurrent(m, "outgoing_date", ""),
	}

	if w, found := states[myName+"-weather"]; found {
		currentSnapshot["weather_date"] = GetCurrent(w, "weather_date", "")
	}

	routes, _ := parseBriefingRoutes(&m.Want)
	for i := range routes {
		key := fmt.Sprintf("transit_date_%d", i)
		transitName := fmt.Sprintf("%s-transit-%d", myName, i)
		if t, found := states[transitName]; found {
			currentSnapshot[key] = GetCurrent(t, "transit_date", "")
		} else {
			currentSnapshot[key] = ""
		}
	}

	m.SetCurrent("current", currentSnapshot)

	// Act on directions from the OPA thinker.
	for _, dir := range toStringSlice(GetPlan(&m.Want, "directions", []any{})) {
		if dir == "post_slack" {
			m.composeAndPublish(states, myName, today, routes)
		}
	}
}

// composeAndPublish builds the briefing message from sibling states and stores it
// as outgoing_message for the SlackPostWant sibling to pick up.
func (m *MorningBriefingWant) composeAndPublish(states map[string]*Want, myName, today string, routes []briefingRouteConfig) {
	if GetCurrent(m, "outgoing_date", "") == today {
		return // already published today
	}

	m.SetCurrent("briefing_status", "composing")

	weatherText := "天気情報なし"
	if w, found := states[myName+"-weather"]; found {
		if t := GetCurrent(w, "weather_text", ""); t != "" {
			weatherText = t
		}
	}

	var transitLines []string
	for i := range routes {
		transitName := fmt.Sprintf("%s-transit-%d", myName, i)
		if t, found := states[transitName]; found {
			if line := GetCurrent(t, "transit_text", ""); line != "" {
				transitLines = append(transitLines, line)
			}
		}
	}

	message := composeBriefingMessage(weatherText, transitLines)
	m.SetCurrent("outgoing_message", message)
	m.SetCurrent("outgoing_date", today)
	m.SetCurrent("briefing_status", "ready")
	m.StoreLog("[BRIEFING] Message composed for %s", today)
}

// parseHHMM splits "HH:MM" into hour and minute integers.
func parseHHMM(s string) (int, int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, err
	}
	return t.Hour(), t.Minute(), nil
}

// toStringSlice converts []any to []string, skipping non-strings.
func toStringSlice(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
