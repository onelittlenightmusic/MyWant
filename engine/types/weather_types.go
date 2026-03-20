package types

import (
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[WeatherWant, WeatherLocals]("weather")
}

type WeatherLocals struct{}

// WeatherWant fetches weather for today and stores the result in its own state.
// Sibling coordinator Wants (e.g. MorningBriefingWant) read weather_text / weather_date directly.
type WeatherWant struct {
	Want
}

func (w *WeatherWant) GetLocals() *WeatherLocals {
	return CheckLocalsInitialized[WeatherLocals](&w.Want)
}

func (w *WeatherWant) Initialize() {
	// Promote params to state so the agent reads exclusively from state.
	w.SetCurrent("weather_city", w.GetStringParam("weather_city", "Tokyo"))
	w.SetCurrent("openweathermap_api_key", w.GetStringParam("openweathermap_api_key", ""))
	// Clear weather_date on each restart so Progress() re-fetches.
	w.SetCurrent("weather_date", "")
}

func (w *WeatherWant) IsAchieved() bool {
	today := time.Now().Format("2006-01-02")
	return GetCurrent(w, "weather_date", "") == today
}

func (w *WeatherWant) CalculateAchievingPercentage() float64 {
	if w.IsAchieved() {
		return 100
	}
	return 10
}

// Progress runs the weather agent and propagates results to the parent coordinator.
func (w *WeatherWant) Progress() {
	w.SetCurrent("achieving_percentage", w.CalculateAchievingPercentage())

	if w.IsAchieved() {
		if text := GetCurrent(w, "weather_text", ""); text != "" {
			w.MergeParentState(map[string]any{
				"weather_done": true,
				"weather_text": text,
			})
		}
		return
	}

	if err := w.ExecuteAgents(); err != nil {
		w.StoreLog("[WEATHER] Agent error: %v", err)
	}
}
