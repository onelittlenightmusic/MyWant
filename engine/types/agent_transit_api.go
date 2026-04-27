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

const transitDoAgentName = "agent_transit_api"

func init() {
	RegisterWithInit(func() {
		RegisterDoAgent(transitDoAgentName, executeTransitQuery)
	})
}

// --- OpenTripPlanner 2.x GraphQL response types ---

type otpGraphQLResponse struct {
	Data   *otpGraphQLData `json:"data"`
	Errors []otpGraphQLErr `json:"errors,omitempty"`
}

type otpGraphQLErr struct {
	Message string `json:"message"`
}

type otpGraphQLData struct {
	Plan *otpPlan `json:"plan"`
}

type otpPlan struct {
	Itineraries []otpItinerary `json:"itineraries"`
}

type otpItinerary struct {
	Duration            int      `json:"duration"`            // seconds
	StartTime           int64    `json:"startTime"`           // Unix ms
	EndTime             int64    `json:"endTime"`             // Unix ms
	NumberOfTransfers   int      `json:"numberOfTransfers"`
	Legs                []otpLeg `json:"legs"`
}

type otpLeg struct {
	Mode       string    `json:"mode"`
	StartTime  int64     `json:"startTime"`
	EndTime    int64     `json:"endTime"`
	TransitLeg bool      `json:"transitLeg"`
	Route      *otpRoute `json:"route,omitempty"`
	From       otpPlace  `json:"from"`
	To         otpPlace  `json:"to"`
}

type otpRoute struct {
	ShortName string `json:"shortName"`
	LongName  string `json:"longName"`
}

type otpPlace struct {
	Name string `json:"name"`
}

// --- Nominatim geocoding ---

type nominatimResult struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// TransitRouteResult holds the parsed result of a transit query
type TransitRouteResult struct {
	RouteName            string `json:"route_name"`
	Origin               string `json:"origin"`
	Destination          string `json:"destination"`
	RecommendedDeparture string `json:"recommended_departure"`
	EstimatedArrival     string `json:"estimated_arrival"`
	DurationMinutes      int    `json:"duration_minutes"`
	FirstVehicle         string `json:"first_vehicle"`
	BoardingStop         string `json:"boarding_stop"`
	Transfers            int    `json:"transfers"`
	ArriveBy             string `json:"arrive_by"`
}

// executeTransitQuery queries OpenTripPlanner and stores the result in Want state.
func executeTransitQuery(ctx context.Context, want *Want) error {
	otpURL := GetCurrent(want, "otp_url", "")
	if otpURL == "" {
		otpURL = os.Getenv("OTP_URL")
	}
	if otpURL == "" {
		otpURL = "http://localhost:8082"
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

	want.StoreLog("[TRANSIT] Querying route via OTP: %s → %s (arrive by %s)", origin, destination, arriveBy)
	want.SetCurrent("transit_status", "querying")

	fromLatLon, err := geocodeLocation(ctx, origin)
	if err != nil {
		want.SetCurrent("transit_status", "failed")
		return fmt.Errorf("geocode origin %q: %w", origin, err)
	}
	toLatLon, err := geocodeLocation(ctx, destination)
	if err != nil {
		want.SetCurrent("transit_status", "failed")
		return fmt.Errorf("geocode destination %q: %w", destination, err)
	}

	result, err := callOTPPlanAPI(ctx, otpURL, fromLatLon, toLatLon, arrivalUnix)
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

// geocodeLocation resolves a place name to "lat,lon" via Nominatim (OpenStreetMap).
func geocodeLocation(ctx context.Context, place string) (string, error) {
	params := url.Values{}
	params.Set("q", place)
	params.Set("format", "json")
	params.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://nominatim.openstreetmap.org/search?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "mywant-transit/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("nominatim request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var results []nominatimResult
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 {
		return "", fmt.Errorf("no location found for %q", place)
	}
	return results[0].Lat + "," + results[0].Lon, nil
}

// callOTPPlanAPI calls the OTP 2.x GraphQL /otp/gtfs/v1 endpoint with arriveBy=true.
func callOTPPlanAPI(ctx context.Context, otpURL, fromLatLon, toLatLon string, arrivalTime int64) (*TransitRouteResult, error) {
	t := time.Unix(arrivalTime, 0)

	fromLat, fromLon, err := splitLatLon(fromLatLon)
	if err != nil {
		return nil, fmt.Errorf("invalid from coordinate %q: %w", fromLatLon, err)
	}
	toLat, toLon, err := splitLatLon(toLatLon)
	if err != nil {
		return nil, fmt.Errorf("invalid to coordinate %q: %w", toLatLon, err)
	}

	query := fmt.Sprintf(`{
  plan(
    from: {lat: %s, lon: %s}
    to: {lat: %s, lon: %s}
    numItineraries: 5
    arriveBy: true
    date: "%s"
    time: "%s"
    transportModes: [{mode: TRANSIT}, {mode: WALK}]
  ) {
    itineraries {
      startTime endTime duration numberOfTransfers
      legs {
        mode startTime endTime transitLeg
        route { shortName longName }
        from { name }
        to { name }
      }
    }
  }
}`, fromLat, fromLon, toLat, toLon,
		t.Format("2006-01-02"),
		t.Format("15:04:05"))

	body, _ := json.Marshal(map[string]string{"query": query})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		otpURL+"/otp/gtfs/v1", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to build OTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OTP response: %w", err)
	}

	var gr otpGraphQLResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return nil, fmt.Errorf("failed to parse OTP response: %w", err)
	}
	if len(gr.Errors) > 0 {
		return nil, fmt.Errorf("OTP GraphQL error: %s", gr.Errors[0].Message)
	}
	if gr.Data == nil || gr.Data.Plan == nil || len(gr.Data.Plan.Itineraries) == 0 {
		return nil, fmt.Errorf("no itineraries found from %q to %q", fromLatLon, toLatLon)
	}

	// Prefer the shortest itinerary that includes at least one transit leg.
	// OTP sometimes returns a walk-only option first with arriveBy=true.
	best := gr.Data.Plan.Itineraries[0]
	for _, itin := range gr.Data.Plan.Itineraries {
		hasTransit := false
		for _, leg := range itin.Legs {
			if leg.TransitLeg {
				hasTransit = true
				break
			}
		}
		if hasTransit && itin.Duration < best.Duration {
			best = itin
		}
	}
	return parseOTPResult(best), nil
}

// splitLatLon splits a "lat,lon" string into two separate strings.
func splitLatLon(latLon string) (string, string, error) {
	idx := strings.Index(latLon, ",")
	if idx < 0 {
		return "", "", fmt.Errorf("expected lat,lon format")
	}
	return strings.TrimSpace(latLon[:idx]), strings.TrimSpace(latLon[idx+1:]), nil
}

// parseOTPResult extracts key fields from an OTP 2.x itinerary.
func parseOTPResult(it otpItinerary) *TransitRouteResult {
	result := &TransitRouteResult{
		DurationMinutes:      it.Duration / 60,
		Transfers:            it.NumberOfTransfers,
		RecommendedDeparture: time.Unix(it.StartTime/1000, 0).Format("15:04"),
		EstimatedArrival:     time.Unix(it.EndTime/1000, 0).Format("15:04"),
	}
	for _, leg := range it.Legs {
		if leg.TransitLeg && leg.Route != nil {
			result.FirstVehicle = leg.Route.ShortName
			if result.FirstVehicle == "" {
				result.FirstVehicle = leg.Route.LongName
			}
			result.BoardingStop = leg.From.Name
			break
		}
	}
	return result
}

// parseArriveBy parses "HH:MM" and returns today's Unix timestamp at that time.
func parseArriveBy(arriveBy string) (int64, error) {
	now := time.Now()
	t, err := time.ParseInLocation("15:04", arriveBy, now.Location())
	if err != nil {
		return 0, err
	}
	target := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	if target.Before(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Unix(), nil
}
