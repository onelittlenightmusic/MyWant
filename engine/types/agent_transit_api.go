package types

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	. "mywant/engine/core"
)

const transitDoAgentName = "agent_transit_api"

func init() {
	RegisterDoAgent(transitDoAgentName, executeTransitQuery)
}

// --- Google Maps Directions API response types ---

type directionsResponse struct {
	Status            string            `json:"status"`
	ErrorMessage      string            `json:"error_message"`
	Routes            []directionsRoute `json:"routes"`
}

type directionsRoute struct {
	Legs []directionsLeg `json:"legs"`
}

type directionsLeg struct {
	DepartureTime directionsTime `json:"departure_time"`
	ArrivalTime   directionsTime `json:"arrival_time"`
	Duration      directionsDur  `json:"duration"`
	Steps         []directionsStep `json:"steps"`
}

type directionsTime struct {
	Text     string `json:"text"`
	TimeZone string `json:"time_zone"`
	Value    int64  `json:"value"` // Unix timestamp
}

type directionsDur struct {
	Text  string `json:"text"`
	Value int    `json:"value"` // seconds
}

type directionsStep struct {
	TravelMode     string                `json:"travel_mode"`
	TransitDetails *transitDetails       `json:"transit_details,omitempty"`
}

type transitDetails struct {
	Line           transitLine `json:"line"`
	DepartureStop  transitStop `json:"departure_stop"`
	ArrivalStop    transitStop `json:"arrival_stop"`
	NumStops       int         `json:"num_stops"`
}

type transitLine struct {
	ShortName string         `json:"short_name"`
	Name      string         `json:"name"`
	Vehicle   transitVehicle `json:"vehicle"`
}

type transitVehicle struct {
	Name string `json:"name"`
}

type transitStop struct {
	Name string `json:"name"`
}

// TransitRouteResult holds the parsed result of a transit query
type TransitRouteResult struct {
	RouteName          string `json:"route_name"`
	Origin             string `json:"origin"`
	Destination        string `json:"destination"`
	RecommendedDeparture string `json:"recommended_departure"` // e.g. "8:31"
	EstimatedArrival   string `json:"estimated_arrival"`
	DurationMinutes    int    `json:"duration_minutes"`
	FirstVehicle       string `json:"first_vehicle"` // e.g. "山手線"
	BoardingStop       string `json:"boarding_stop"`
	Transfers          int    `json:"transfers"`
	ArriveBy           string `json:"arrive_by"`
}

// executeTransitQuery calls Google Maps Directions API and stores the result
func executeTransitQuery(ctx context.Context, want *Want) error {
	apiKey := GetCurrent(want, "api_key", "")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_MAPS_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("google maps api_key not provided and GOOGLE_MAPS_API_KEY env var not set")
	}

	origin := GetCurrent(want, "origin", "")
	destination := GetCurrent(want, "destination", "")
	arriveBy := GetCurrent(want, "arrive_by", "09:00")
	routeName := GetCurrent(want, "route_name", fmt.Sprintf("%s → %s", origin, destination))

	if origin == "" || destination == "" {
		return fmt.Errorf("origin and destination are required")
	}

	arrivalUnix, err := parseArriveBy(arriveBy)
	if err != nil {
		return fmt.Errorf("invalid arrive_by format %q (use HH:MM): %w", arriveBy, err)
	}

	want.StoreLog("[TRANSIT] Querying route: %s → %s (arrive by %s)", origin, destination, arriveBy)
	want.SetCurrent("transit_status", "querying")

	result, err := callDirectionsAPI(ctx, origin, destination, arrivalUnix, apiKey)
	if err != nil {
		want.SetCurrent("transit_status", "failed")
		want.SetCurrent("error", err.Error())
		return err
	}

	result.RouteName = routeName
	result.Origin = origin
	result.Destination = destination
	result.ArriveBy = arriveBy

	resultJSON, _ := json.Marshal(result)
	want.SetCurrent("transit_result", string(resultJSON))
	want.SetCurrent("recommended_departure", result.RecommendedDeparture)
	want.SetCurrent("estimated_arrival", result.EstimatedArrival)
	want.SetCurrent("duration_minutes", result.DurationMinutes)
	want.SetCurrent("first_vehicle", result.FirstVehicle)
	want.SetCurrent("boarding_stop", result.BoardingStop)
	want.SetCurrent("transfers", result.Transfers)
	want.SetCurrent("route_name", result.RouteName)
	want.SetCurrent("transit_status", "done")
	want.SetCurrent("queried_at", time.Now().Format(time.RFC3339))

	want.SetAgentActivity(transitDoAgentName, fmt.Sprintf(
		"Transit query complete: %s → %s, depart %s (arrive by %s, %d min, via %s)",
		origin, destination, result.RecommendedDeparture, arriveBy, result.DurationMinutes, result.FirstVehicle,
	))
	want.StoreLog("[TRANSIT] Done: depart %s, arrive %s (%d min, via %s, %d transfer(s))",
		result.RecommendedDeparture, result.EstimatedArrival, result.DurationMinutes,
		result.FirstVehicle, result.Transfers)

	return nil
}

// callDirectionsAPI performs the HTTP request to the Directions API
func callDirectionsAPI(ctx context.Context, origin, destination string, arrivalTime int64, apiKey string) (*TransitRouteResult, error) {
	params := url.Values{}
	params.Set("origin", origin)
	params.Set("destination", destination)
	params.Set("mode", "transit")
	params.Set("arrival_time", strconv.FormatInt(arrivalTime, 10))
	params.Set("language", "ja")
	params.Set("key", apiKey)

	apiURL := "https://maps.googleapis.com/maps/api/directions/json?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("directions API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var dr directionsResponse
	if err := json.Unmarshal(body, &dr); err != nil {
		return nil, fmt.Errorf("failed to parse directions response: %w", err)
	}

	if dr.Status != "OK" {
		return nil, fmt.Errorf("directions API returned status %q: %s", dr.Status, dr.ErrorMessage)
	}
	if len(dr.Routes) == 0 || len(dr.Routes[0].Legs) == 0 {
		return nil, fmt.Errorf("no routes found from %q to %q", origin, destination)
	}

	return parseDirectionsResult(dr.Routes[0].Legs[0]), nil
}

// parseDirectionsResult extracts the key fields from a directions leg
func parseDirectionsResult(leg directionsLeg) *TransitRouteResult {
	result := &TransitRouteResult{
		DurationMinutes:      leg.Duration.Value / 60,
		RecommendedDeparture: leg.DepartureTime.Text,
		EstimatedArrival:     leg.ArrivalTime.Text,
	}

	// Find first transit step for vehicle and boarding info, count transfers
	transitSteps := 0
	for _, step := range leg.Steps {
		if step.TravelMode == "TRANSIT" && step.TransitDetails != nil {
			transitSteps++
			if transitSteps == 1 {
				td := step.TransitDetails
				result.FirstVehicle = td.Line.ShortName
				if result.FirstVehicle == "" {
					result.FirstVehicle = td.Line.Name
				}
				result.BoardingStop = td.DepartureStop.Name
			}
		}
	}
	if transitSteps > 0 {
		result.Transfers = transitSteps - 1
	}

	return result
}

// parseArriveBy parses "HH:MM" and returns today's Unix timestamp at that time
func parseArriveBy(arriveBy string) (int64, error) {
	now := time.Now()
	t, err := time.ParseInLocation("15:04", arriveBy, now.Location())
	if err != nil {
		return 0, err
	}
	target := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	// If target is already past, use tomorrow
	if target.Before(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Unix(), nil
}
