package types

import (
	"strings"

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
	city := w.GetStringParam("weather_city", "Tokyo")
	apiKey := w.GetStringParam("openweathermap_api_key", "")
	w.SetCurrent("weather_city", city)
	w.SetCurrent("openweathermap_api_key", apiKey)
	w.SetCurrent("weather_text", "")
	w.SetCurrent("weather_date", "")
	w.SetCurrent("weather_condition", "")
	w.SetCurrent("achieving_percentage", 0)
	w.ExecuteAgents() //nolint:errcheck
}

func (w *WeatherWant) IsAchieved() bool {
	text := GetCurrent(w, "weather_text", "")
	return text != ""
}

func (w *WeatherWant) Progress() {
	text := GetCurrent(w, "weather_text", "")
	if text != "" {
		w.SetCurrent("weather_condition", classifyWeatherCondition(text))
		w.SetCurrent("achieving_percentage", 100)
	}
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
