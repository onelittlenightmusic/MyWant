package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	. "mywant/engine/core"
)

const weatherAgentName = "agent_weather"

func init() {
	RegisterWithInit(func() {
		RegisterDoAgent(weatherAgentName, executeWeatherFetch)
	})
}

// owmResponse is the minimal OpenWeatherMap current weather response
type owmResponse struct {
	Weather []struct {
		Description string `json:"description"`
	} `json:"weather"`
	Main struct {
		Temp    float64 `json:"temp"`
		TempMax float64 `json:"temp_max"`
		TempMin float64 `json:"temp_min"`
	} `json:"main"`
}

// executeWeatherFetch is the DoAgent entry point for weather queries.
//
// A fetch that fails leaves weather_text empty and records why in `error`, so
// the want ends failed (WeatherWant.IsFailed) instead of achieving with a
// placeholder sentence that looks like weather.
func executeWeatherFetch(ctx context.Context, want *Want) error {
	city := GetCurrent(want, "weather_city", "Tokyo")
	apiKey := GetCurrent(want, "openweathermap_api_key", "")
	if apiKey == "" {
		apiKey = os.Getenv("OPENWEATHERMAP_API_KEY")
	}

	want.StoreLog("[WEATHER] Fetching weather for %s", city)
	text, source, err := fetchWeatherText(ctx, city, apiKey)
	if err != nil {
		want.StoreLog("[WEATHER] Fetch failed: %v", err)
		want.SetCurrent("weather_text", "")
		want.SetCurrent("weather_condition", "")
		want.SetCurrent("error", fmt.Sprintf("天気情報を取得できませんでした: %v", err))
		return nil
	}
	cond := classifyWeatherCondition(text)
	want.SetCurrent("weather_text", text)
	want.SetCurrent("weather_condition", cond)
	want.SetCurrent("weather_date", time.Now().Format("2006-01-02"))
	want.SetCurrent("error", "")
	want.StoreLog("[WEATHER] Done via %s: %s → condition=%s", source, text, cond)
	return nil
}

// fetchWeatherText asks each source in turn until one answers: OpenWeatherMap
// when a key is given, then wttr.in, then Open-Meteo. wttr.in is a single
// free host that goes down (expired certificates, HTTP/2 stream resets), so
// Open-Meteo — also keyless — stands behind it.
func fetchWeatherText(ctx context.Context, city, apiKey string) (text, source string, err error) {
	type weatherSource struct {
		name  string
		fetch func(context.Context, string) (string, error)
	}
	var sources []weatherSource
	if apiKey != "" {
		sources = append(sources, weatherSource{"openweathermap", func(ctx context.Context, city string) (string, error) {
			return fetchWeatherOWM(ctx, city, apiKey)
		}})
	}
	sources = append(sources,
		weatherSource{"wttr.in", fetchWeatherWttr},
		weatherSource{"open-meteo", fetchWeatherOpenMeteo},
	)

	var errs []error
	for _, s := range sources {
		text, err := s.fetch(ctx, city)
		if err == nil {
			return text, s.name, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", s.name, err))
	}
	return "", "", errors.Join(errs...)
}

func fetchWeatherOWM(ctx context.Context, city, apiKey string) (string, error) {
	params := url.Values{}
	params.Set("q", city)
	params.Set("units", "metric")
	params.Set("lang", "ja")
	params.Set("appid", apiKey)
	body, err := weatherGet(ctx, "https://api.openweathermap.org/data/2.5/weather?"+params.Encode())
	if err != nil {
		return "", err
	}

	var owm owmResponse
	if err := json.Unmarshal(body, &owm); err != nil {
		return "", fmt.Errorf("parse weather response: %w", err)
	}
	if len(owm.Weather) == 0 {
		return "", fmt.Errorf("no weather in response")
	}
	return fmt.Sprintf("%s %.0f°C (最高 %.0f° / 最低 %.0f°)",
		owm.Weather[0].Description, owm.Main.Temp, owm.Main.TempMax, owm.Main.TempMin), nil
}

// fetchWeatherWttr uses wttr.in which needs no API key
func fetchWeatherWttr(ctx context.Context, city string) (string, error) {
	apiURL := fmt.Sprintf("https://wttr.in/%s?format=%%C+%%t(%%h)&lang=ja", url.PathEscape(city))
	body, err := weatherGet(ctx, apiURL)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(body))
	if text == "" || strings.Contains(text, "Unknown location") {
		return "", fmt.Errorf("no weather for %q", city)
	}
	return text, nil
}

// fetchWeatherOpenMeteo resolves the city with Open-Meteo's geocoder, then
// reads the current conditions there. Neither call needs a key.
func fetchWeatherOpenMeteo(ctx context.Context, city string) (string, error) {
	geoParams := url.Values{}
	geoParams.Set("name", city)
	geoParams.Set("count", "1")
	geoParams.Set("language", "ja")
	body, err := weatherGet(ctx, "https://geocoding-api.open-meteo.com/v1/search?"+geoParams.Encode())
	if err != nil {
		return "", err
	}
	var geo struct {
		Results []struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &geo); err != nil {
		return "", fmt.Errorf("parse geocoding response: %w", err)
	}
	if len(geo.Results) == 0 {
		return "", fmt.Errorf("unknown location %q", city)
	}

	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%f", geo.Results[0].Latitude))
	params.Set("longitude", fmt.Sprintf("%f", geo.Results[0].Longitude))
	params.Set("current", "temperature_2m,relative_humidity_2m,weather_code")
	params.Set("daily", "temperature_2m_max,temperature_2m_min")
	params.Set("timezone", "auto")
	params.Set("forecast_days", "1")
	body, err = weatherGet(ctx, "https://api.open-meteo.com/v1/forecast?"+params.Encode())
	if err != nil {
		return "", err
	}
	var fc struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
			Humidity    float64 `json:"relative_humidity_2m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
		Daily struct {
			TempMax []float64 `json:"temperature_2m_max"`
			TempMin []float64 `json:"temperature_2m_min"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(body, &fc); err != nil {
		return "", fmt.Errorf("parse forecast response: %w", err)
	}

	text := fmt.Sprintf("%s %.0f°C (湿度 %.0f%%)",
		wmoWeatherText(fc.Current.WeatherCode), fc.Current.Temperature, fc.Current.Humidity)
	if len(fc.Daily.TempMax) > 0 && len(fc.Daily.TempMin) > 0 {
		text += fmt.Sprintf(" (最高 %.0f° / 最低 %.0f°)", fc.Daily.TempMax[0], fc.Daily.TempMin[0])
	}
	return text, nil
}

// wmoWeatherText names a WMO weather code in words classifyWeatherCondition
// recognises.
func wmoWeatherText(code int) string {
	switch {
	case code == 0:
		return "快晴"
	case code == 1:
		return "晴れ"
	case code == 2:
		return "晴れ時々曇り"
	case code == 3:
		return "曇り"
	case code == 45 || code == 48:
		return "霧"
	case code >= 51 && code <= 57:
		return "霧雨"
	case code >= 61 && code <= 67, code >= 80 && code <= 82:
		return "雨"
	case code >= 71 && code <= 77, code == 85 || code == 86:
		return "雪"
	case code >= 95:
		return "雷雨"
	default:
		return "不明"
	}
}

func weatherGet(ctx context.Context, apiURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}
