package types

import (
	"encoding/json"
	"fmt"
	"time"

	. "mywant/engine/core"
)

const transitMaxRetries = 3

func init() {
	RegisterWantImplementation[TransitWant, TransitLocals]("transit")
}

// TransitLocals holds type-specific local state for TransitWant
type TransitLocals struct {
	Origin      string
	Destination string
	ArriveBy    string
	Days        []string      // ["mon","tue","wed","thu","fri"] etc.
	RefreshAt   time.Duration // how early before arrive_by to refresh (default 2h)
}

// TransitWant queries Google Maps Directions API for a transit route
type TransitWant struct {
	Want
}

func (t *TransitWant) GetLocals() *TransitLocals {
	return CheckLocalsInitialized[TransitLocals](&t.Want)
}

func (t *TransitWant) getStatus() string {
	return GetCurrent(t, "transit_status", "pending")
}

// Initialize parses and validates parameters
func (t *TransitWant) Initialize() {
	locals := t.GetLocals()

	locals.Origin = t.GetStringParam("origin", "")
	if locals.Origin == "" {
		t.SetConfigError("origin", "Missing required parameter 'origin'")
		return
	}

	locals.Destination = t.GetStringParam("destination", "")
	if locals.Destination == "" {
		t.SetConfigError("destination", "Missing required parameter 'destination'")
		return
	}

	locals.ArriveBy = t.GetStringParam("arrive_by", "09:00")

	// Parse days filter (default: all weekdays)
	daysParam := t.GetStringParam("days", "")
	if daysParam == "" {
		locals.Days = []string{"mon", "tue", "wed", "thu", "fri"}
	} else {
		locals.Days = splitCSV(daysParam)
	}

	if t.getStatus() == "" {
		t.SetCurrent("transit_status", "pending")
	}

	t.StoreLog("[TRANSIT] Initialized: %s → %s (arrive by %s)", locals.Origin, locals.Destination, locals.ArriveBy)
}

// IsAchieved returns true when the transit query is done for today
func (t *TransitWant) IsAchieved() bool {
	if t.getStatus() != "done" {
		return false
	}
	// Re-query daily: check if last query was today
	queriedAt := GetCurrent(t, "queried_at", "")
	if queriedAt == "" {
		return false
	}
	ts, err := time.Parse(time.RFC3339, queriedAt)
	if err != nil {
		return false
	}
	now := time.Now()
	return ts.Year() == now.Year() && ts.Month() == now.Month() && ts.Day() == now.Day()
}

// CalculateAchievingPercentage reports progress
func (t *TransitWant) CalculateAchievingPercentage() float64 {
	switch t.getStatus() {
	case "done":
		return 100
	case "querying":
		return 50
	case "failed":
		return 100
	default:
		return 10
	}
}

// Progress orchestrates the transit query lifecycle
func (t *TransitWant) Progress() {
	t.SetCurrent("achieving_percentage", t.CalculateAchievingPercentage())

	// Reset state at the start of a new day
	if t.getStatus() == "done" && !t.IsAchieved() {
		t.SetCurrent("transit_status", "pending")
		t.SetCurrent("retry_count", 0)
	}

	if t.IsAchieved() {
		// Store formatted result in own state for coordinator Wants to read
		t.storeTransitText()
		return
	}

	locals := t.GetLocals()

	// Skip if today is not in the configured days
	if !isTodayInDays(locals.Days) {
		t.StoreLog("[TRANSIT] Skipping: today (%s) not in configured days", weekdayShort())
		return
	}

	status := t.getStatus()

	// On failure, check retry count before retrying
	if status == "failed" {
		retries := GetCurrent(t, "retry_count", 0)
		if retries >= transitMaxRetries {
			t.storeExhaustedTransitText()
			return
		}
		t.SetCurrent("retry_count", retries+1)
		t.StoreLog("[TRANSIT] Retrying (%d/%d): %s → %s", retries+1, transitMaxRetries, locals.Origin, locals.Destination)
	}

	if status == "pending" || status == "failed" {
		t.StoreLog("[TRANSIT] Querying route: %s → %s", locals.Origin, locals.Destination)
		if err := t.ExecuteAgents(); err != nil {
			t.StoreLog("ERROR: transit agent failed: %v", err)
		}
	}
}

// storeExhaustedTransitText marks the route as done with an error message when
// all retries are exhausted, so the briefing coordinator can proceed without it.
func (t *TransitWant) storeExhaustedTransitText() {
	routeName := GetCurrent(t, "route_name", t.GetStringParam("name", ""))
	if routeName == "" {
		routeName = fmt.Sprintf("%s → %s", t.GetStringParam("origin", "?"), t.GetStringParam("destination", "?"))
	}
	today := time.Now().Format("2006-01-02")
	t.SetCurrent("transit_text", fmt.Sprintf("  • %s: 取得失敗（リトライ上限）", routeName))
	t.SetCurrent("transit_date", today)
	t.SetCurrent("transit_status", "done")
	t.SetCurrent("queried_at", time.Now().Format(time.RFC3339))
	t.StoreLog("[TRANSIT] Max retries (%d) exhausted for %q, proceeding without transit info", transitMaxRetries, routeName)
}

// storeTransitText formats the transit result and stores transit_text / transit_date
// in this Want's own state so sibling coordinator Wants can read it directly.
func (t *TransitWant) storeTransitText() {
	routeName := GetCurrent(t, "route_name", t.GetStringParam("name", "route"))
	resultJSON := GetCurrent(t, "transit_result", "")
	if resultJSON == "" {
		return
	}

	var result TransitRouteResult
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return
	}
	result.RouteName = routeName

	line := formatTransitLine(routeName, &result)
	today := time.Now().Format("2006-01-02")
	t.SetCurrent("transit_text", line)
	t.SetCurrent("transit_date", today)
}

// --- helpers ---

func weekdayShort() string {
	days := map[time.Weekday]string{
		time.Monday:    "mon",
		time.Tuesday:   "tue",
		time.Wednesday: "wed",
		time.Thursday:  "thu",
		time.Friday:    "fri",
		time.Saturday:  "sat",
		time.Sunday:    "sun",
	}
	return days[time.Now().Weekday()]
}

func isTodayInDays(days []string) bool {
	today := weekdayShort()
	for _, d := range days {
		if d == today {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			tok := trimSpace(s[start:i])
			if tok != "" {
				out = append(out, tok)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
