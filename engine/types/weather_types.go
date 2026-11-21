package types

import (
	"context"
	"strings"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[WeatherWant, WeatherLocals]("weather")
	})
}

type WeatherLocals struct{}

type WeatherWant struct{ Want }

func (w *WeatherWant) GetLocals() *WeatherLocals {
	return CheckLocalsInitialized[WeatherLocals](&w.Want)
}

func (w *WeatherWant) Initialize() {
	// The parameter is `at` — the want is a probe, and `at` is where it reads.
	city := w.GetStringParam("at", "Tokyo")
	apiKey := w.GetStringParam("openweathermap_api_key", "")
	w.SetCurrent("weather_city", city)
	w.SetCurrent("openweathermap_api_key", apiKey)
	w.SetCurrent("weather_text", "")
	w.SetCurrent("weather_date", "")
	w.SetCurrent("weather_condition", "")
	w.SetCurrent("error", "")
	w.SetCurrent("achieving_percentage", 0)
	w.ExecuteAgents() //nolint:errcheck
}

func (w *WeatherWant) IsAchieved() bool {
	text := GetCurrent(w, "weather_text", "")
	return text != ""
}

// IsFailed is true once the agent has tried every source and recorded why none
// answered.
func (w *WeatherWant) IsFailed() bool {
	return GetCurrent(w, "error", "") != ""
}

func (w *WeatherWant) Progress() {
	text := GetCurrent(w, "weather_text", "")
	if text != "" {
		w.SetCurrent("weather_condition", classifyWeatherCondition(text))
		w.SetCurrent("achieving_percentage", 100)
	}
	w.forecastHeard()
}

// forecastHeard looks up what it heard another want say (spec.hear) — when and
// where it is about, its labels first (Hear) — the forecast there, then: the
// place heard ("西新宿店", "西新宿"), else its own. Kept for its voice to say
// in answer (heard_forecast); each murmur once.
func (w *WeatherWant) forecastHeard() {
	heard := w.Hear(HearWhen, HearWhere)
	if len(heard) == 0 {
		return
	}
	m := heard[len(heard)-1]
	if prev, ok := GetCurrent(w, "heard_forecast", map[string]any(nil))["murmur_id"].(string); ok && prev == m.MurmurID {
		return
	}
	at, err := time.Parse(time.RFC3339, m.Meaning[HearWhen].Choice)
	if err != nil {
		return
	}
	place := m.Meaning[HearWhere].Choice
	own := GetCurrent(w, "weather_city", "Tokyo")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	text, city, err := fetchForecastAt(ctx, []string{place, strings.TrimSuffix(place, "店"), own}, at)
	kept := map[string]any{"murmur_id": m.MurmurID, "time": at.Format(time.RFC3339), "place": place}
	if err != nil {
		kept["error"] = err.Error()
	} else {
		kept["city"], kept["text"] = city, text
	}
	w.StoreState("heard_forecast", kept)
}

// classifyWeatherCondition maps weather description text to a condition key.
func classifyWeatherCondition(text string) string {
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "thunder", "storm", "lightning", "雷", "嵐"):
		return "storm"
	case containsAny(lower, "snow", "blizzard", "sleet", "雪"):
		return "snow"
	case containsAny(lower, "rain", "shower", "drizzle", "雨"):
		return "rain"
	case containsAny(lower, "fog", "mist", "haze", "霧"):
		return "fog"
	case containsAny(lower, "cloud", "overcast", "曇"):
		return "cloudy"
	case containsAny(lower, "sun", "clear", "晴", "fine"):
		return "sunny"
	default:
		return "default"
	}
}

func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
