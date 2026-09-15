package types

import "testing"

// Open-Meteo answers in WMO codes; the words they become must still land on the
// condition the card is drawn with.
func TestWMOWeatherTextClassifies(t *testing.T) {
	cases := map[int]string{
		0:  "sunny",
		1:  "sunny",
		3:  "cloudy",
		45: "fog",
		53: "rain",
		63: "rain",
		81: "rain",
		73: "snow",
		95: "storm",
	}
	for code, want := range cases {
		if got := classifyWeatherCondition(wmoWeatherText(code)); got != want {
			t.Errorf("code %d (%s): condition = %q, want %q", code, wmoWeatherText(code), got, want)
		}
	}
}
