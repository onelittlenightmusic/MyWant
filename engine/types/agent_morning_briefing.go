package types

import (
	"context"
	"encoding/json"
	"fmt"

	. "mywant/engine/core"
)

// briefingRouteConfig is the parsed form of each route entry in params
type briefingRouteConfig struct {
	Name        string `json:"name"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	ArriveBy    string `json:"arrive_by"`
	Days        string `json:"days"` // comma-separated: "mon,tue,wed,thu,fri"
}

// formatTransitLine formats a single route result into a Slack-friendly line
func formatTransitLine(routeName string, r *TransitRouteResult) string {
	transferStr := "直通"
	if r.Transfers == 1 {
		transferStr = "1回乗換"
	} else if r.Transfers > 1 {
		transferStr = fmt.Sprintf("%d回乗換", r.Transfers)
	}

	vehicleStr := ""
	if r.FirstVehicle != "" {
		vehicleStr = " " + r.FirstVehicle
	}
	if r.BoardingStop != "" {
		vehicleStr += "(" + r.BoardingStop + "発)"
	}

	return fmt.Sprintf("  • %s: %s発%s → %s着 (%d分/%s)",
		routeName, r.RecommendedDeparture, vehicleStr,
		r.EstimatedArrival, r.DurationMinutes, transferStr)
}

// fetchTransitLine queries Google Maps for a single route and returns a formatted line.
// Returns empty string if the route should be skipped (no API key, error, etc.)
func fetchTransitLine(ctx context.Context, route briefingRouteConfig, googleKey string) string {
	if googleKey == "" {
		return fmt.Sprintf("  • %s: API キー未設定", route.Name)
	}
	arrivalUnix, err := parseArriveBy(route.ArriveBy)
	if err != nil {
		return fmt.Sprintf("  • %s: 時刻パースエラー(%v)", route.Name, err)
	}
	result, err := callDirectionsAPI(ctx, route.Origin, route.Destination, arrivalUnix, googleKey)
	if err != nil {
		InfoLog("[TRANSIT] callDirectionsAPI error for %s: %v", route.Name, err)
		return fmt.Sprintf("  • %s: 取得失敗(%v)", route.Name, err)
	}
	return formatTransitLine(route.Name, result)
}

// parseBriefingRoutes extracts the routes param ([]any from YAML) into []briefingRouteConfig
func parseBriefingRoutes(want *Want) ([]briefingRouteConfig, error) {
	raw, ok := want.Spec.Params["routes"]
	if !ok {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal routes: %w", err)
	}
	var routes []briefingRouteConfig
	if err := json.Unmarshal(data, &routes); err != nil {
		return nil, fmt.Errorf("parse routes: %w", err)
	}
	return routes, nil
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
