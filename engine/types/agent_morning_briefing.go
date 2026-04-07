package types

import (
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

// parseBriefingRoutes extracts the routes from state (copied from params by Initialize) into []briefingRouteConfig.
// Falls back to Spec.Params["routes"] if state is not yet populated (e.g. during Initialize itself).
func parseBriefingRoutes(want *Want) ([]briefingRouteConfig, error) {
	raw := GetCurrent[any](want, "routes", nil)
	if raw == nil {
		// Fallback for when called during Initialize before state is populated
		var ok bool
		raw, ok = want.Spec.GetParam("routes")
		if !ok || raw == nil {
			return nil, nil
		}
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
