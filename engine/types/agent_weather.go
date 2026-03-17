package types

import (
	"context"
	"encoding/json"
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
	RegisterDoAgent(weatherAgentName, executeWeatherFetch)
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

// executeWeatherFetch is the DoAgent entry point for weather queries
func executeWeatherFetch(ctx context.Context, want *Want) error {
	city := want.GetStringParam("weather_city", "Tokyo")
	apiKey := want.GetStringParam("openweathermap_api_key", "")
	if apiKey == "" {
		apiKey = os.Getenv("OPENWEATHERMAP_API_KEY")
	}

	want.StoreLog("[WEATHER] Fetching weather for %s", city)
	text, err := fetchWeatherText(ctx, city, apiKey)
	if err != nil {
		want.StoreLog("[WEATHER] Fetch failed: %v", err)
		text = "天気情報を取得できませんでした"
	}
	want.SetCurrent("weather_text", text)
	want.SetCurrent("weather_date", time.Now().Format("2006-01-02"))
	want.StoreLog("[WEATHER] Done: %s", text)
	return nil
}

// fetchWeatherText queries OpenWeatherMap (with key) or wttr.in (without key)
func fetchWeatherText(ctx context.Context, city, apiKey string) (string, error) {
	if apiKey == "" {
		return fetchWeatherWttr(ctx, city)
	}

	params := url.Values{}
	params.Set("q", city)
	params.Set("units", "metric")
	params.Set("lang", "ja")
	params.Set("appid", apiKey)
	apiURL := "https://api.openweathermap.org/data/2.5/weather?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var owm owmResponse
	if err := json.Unmarshal(body, &owm); err != nil {
		return "", fmt.Errorf("parse weather response: %w", err)
	}

	desc := ""
	if len(owm.Weather) > 0 {
		desc = owm.Weather[0].Description
	}
	return fmt.Sprintf("%s %.0f°C (最高 %.0f° / 最低 %.0f°)",
		desc, owm.Main.Temp, owm.Main.TempMax, owm.Main.TempMin), nil
}

// fetchWeatherWttr uses wttr.in which needs no API key
func fetchWeatherWttr(ctx context.Context, city string) (string, error) {
	apiURL := fmt.Sprintf("https://wttr.in/%s?format=%%C+%%t(%%h)&lang=ja", url.PathEscape(city))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
