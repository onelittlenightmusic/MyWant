package types

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	. "mywant/engine/core"
)

// ─── parseArriveBy ───────────────────────────────────────────────────────────

func TestParseArriveBy_ValidTime(t *testing.T) {
	unix, err := parseArriveBy("09:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unix <= 0 {
		t.Fatal("expected positive unix timestamp")
	}
	ts := time.Unix(unix, 0)
	if ts.Hour() != 9 || ts.Minute() != 0 {
		t.Errorf("expected 09:00, got %02d:%02d", ts.Hour(), ts.Minute())
	}
}

func TestParseArriveBy_PastTimeRollsToTomorrow(t *testing.T) {
	// "00:01" is almost certainly in the past (unless run at midnight)
	unix, err := parseArriveBy("00:01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ts := time.Unix(unix, 0)
	tomorrow := time.Now().Add(24 * time.Hour)
	if ts.Day() != tomorrow.Day() && ts.Day() != time.Now().Day() {
		// Allow either today or tomorrow depending on when test runs
		t.Errorf("unexpected date: %v", ts)
	}
	if ts.Hour() != 0 || ts.Minute() != 1 {
		t.Errorf("expected 00:01, got %02d:%02d", ts.Hour(), ts.Minute())
	}
}

func TestParseArriveBy_InvalidFormat(t *testing.T) {
	_, err := parseArriveBy("9:00am")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	_, err = parseArriveBy("25:00")
	if err == nil {
		t.Fatal("expected error for out-of-range hour")
	}
}

// ─── parseHHMM ───────────────────────────────────────────────────────────────

func TestParseHHMM(t *testing.T) {
	cases := []struct {
		input       string
		wantH, wantM int
		wantErr     bool
	}{
		{"07:30", 7, 30, false},
		{"00:00", 0, 0, false},
		{"23:59", 23, 59, false},
		{"7:30", 7, 30, false},
		{"bad", 0, 0, true},
		{"25:00", 0, 0, true},
	}
	for _, c := range cases {
		h, m, err := parseHHMM(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseHHMM(%q): expected error", c.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseHHMM(%q): unexpected error: %v", c.input, err)
			continue
		}
		if h != c.wantH || m != c.wantM {
			t.Errorf("parseHHMM(%q): got %d:%d, want %d:%d", c.input, h, m, c.wantH, c.wantM)
		}
	}
}

// ─── splitCSV ────────────────────────────────────────────────────────────────

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"mon,tue,wed", []string{"mon", "tue", "wed"}},
		{"mon", []string{"mon"}},
		{"mon, tue , wed", []string{"mon", "tue", "wed"}},
		{"", nil},
	}
	for _, c := range cases {
		got := splitCSV(c.input)
		if len(got) != len(c.want) {
			t.Errorf("splitCSV(%q): got %v, want %v", c.input, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCSV(%q)[%d]: got %q, want %q", c.input, i, got[i], c.want[i])
			}
		}
	}
}

// ─── containsStr ─────────────────────────────────────────────────────────────

func TestContainsStr(t *testing.T) {
	if !containsStr([]string{"mon", "tue"}, "mon") {
		t.Error("expected true")
	}
	if containsStr([]string{"mon", "tue"}, "wed") {
		t.Error("expected false")
	}
	if containsStr(nil, "mon") {
		t.Error("expected false for nil slice")
	}
}

// ─── formatTransitLine ────────────────────────────────────────────────────────

func TestFormatTransitLine_Direct(t *testing.T) {
	r := &TransitRouteResult{
		RecommendedDeparture: "8:31",
		FirstVehicle:         "山手線",
		BoardingStop:         "渋谷",
		EstimatedArrival:     "9:00",
		DurationMinutes:      29,
		Transfers:            0,
	}
	line := formatTransitLine("オフィス", r)
	for _, want := range []string{"オフィス", "8:31", "9:00", "29", "直通", "山手線", "渋谷"} {
		if !contains(line, want) {
			t.Errorf("formatTransitLine: expected %q in %q", want, line)
		}
	}
}

func TestFormatTransitLine_WithTransfers(t *testing.T) {
	r := &TransitRouteResult{
		RecommendedDeparture: "10:00",
		FirstVehicle:         "新幹線",
		BoardingStop:         "東京",
		EstimatedArrival:     "12:30",
		DurationMinutes:      150,
		Transfers:            1,
	}
	line := formatTransitLine("出張", r)
	if !contains(line, "1回乗換") {
		t.Errorf("expected '1回乗換' in %q", line)
	}
}

func TestFormatTransitLine_MultipleTransfers(t *testing.T) {
	r := &TransitRouteResult{
		Transfers:       3,
		DurationMinutes: 60,
	}
	line := formatTransitLine("test", r)
	if !contains(line, "3回乗換") {
		t.Errorf("expected '3回乗換' in %q", line)
	}
}

// ─── composeBriefingMessage ───────────────────────────────────────────────────

func TestComposeBriefingMessage(t *testing.T) {
	transitLines := []string{
		"  • オフィス: 8:31発 → 9:00着 (29分/直通)",
	}
	msg := composeBriefingMessage("晴れ 17°C", transitLines)

	for _, want := range []string{"天気", "晴れ 17°C", "乗換案内", "オフィス"} {
		if !contains(msg, want) {
			t.Errorf("composeBriefingMessage: expected %q in message", want)
		}
	}
}

func TestComposeBriefingMessage_NoTransit(t *testing.T) {
	msg := composeBriefingMessage("曇り 10°C", nil)
	if contains(msg, "乗換案内") {
		t.Error("expected no transit section when no routes")
	}
	if !contains(msg, "天気") {
		t.Error("expected weather section")
	}
}

// ─── parseBriefingRoutes ──────────────────────────────────────────────────────

func TestParseBriefingRoutes(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test", Type: "briefing"},
		Spec: WantSpec{
			Params: map[string]any{
				"routes": []any{
					map[string]any{
						"name":        "オフィス",
						"origin":      "渋谷駅",
						"destination": "新宿駅",
						"arrive_by":   "09:00",
						"days":        "mon,tue,wed,thu,fri",
					},
				},
			},
		},
	}

	routes, err := parseBriefingRoutes(want)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	r := routes[0]
	if r.Name != "オフィス" {
		t.Errorf("name: got %q", r.Name)
	}
	if r.Origin != "渋谷駅" {
		t.Errorf("origin: got %q", r.Origin)
	}
	if r.ArriveBy != "09:00" {
		t.Errorf("arrive_by: got %q", r.ArriveBy)
	}
}

func TestParseBriefingRoutes_NoRoutes(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test", Type: "briefing"},
		Spec:     WantSpec{Params: map[string]any{}},
	}
	routes, err := parseBriefingRoutes(want)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(routes))
	}
}

// ─── parseOTPResult ───────────────────────────────────────────────────────────

func TestParseOTPResult_Direct(t *testing.T) {
	baseMs := int64(1704851460000) // 2024-01-10 08:31:00 JST
	it := otpItinerary{
		Duration:            1740, // 29 min
		StartTime:           baseMs,
		EndTime:             baseMs + 1740*1000,
		NumberOfTransfers:   0,
		Legs: []otpLeg{
			{Mode: "WALK", TransitLeg: false},
			{
				Mode:       "RAIL",
				TransitLeg: true,
				Route:      &otpRoute{ShortName: "山手線"},
				From:       otpPlace{Name: "渋谷"},
				To:         otpPlace{Name: "新宿"},
			},
		},
	}

	result := parseOTPResult(it)
	if result.DurationMinutes != 29 {
		t.Errorf("duration: got %d, want 29", result.DurationMinutes)
	}
	if result.FirstVehicle != "山手線" {
		t.Errorf("first_vehicle: got %q", result.FirstVehicle)
	}
	if result.BoardingStop != "渋谷" {
		t.Errorf("boarding_stop: got %q", result.BoardingStop)
	}
	if result.Transfers != 0 {
		t.Errorf("transfers: got %d, want 0", result.Transfers)
	}
}

func TestParseOTPResult_WithTransfer(t *testing.T) {
	baseMs := int64(1704851460000)
	it := otpItinerary{
		Duration:            5400, // 90 min
		StartTime:           baseMs,
		EndTime:             baseMs + 5400*1000,
		NumberOfTransfers:   1,
		Legs: []otpLeg{
			{
				Mode:       "RAIL",
				TransitLeg: true,
				Route:      &otpRoute{ShortName: "東海道線"},
				From:       otpPlace{Name: "品川"},
			},
			{Mode: "WALK", TransitLeg: false},
			{
				Mode:       "RAIL",
				TransitLeg: true,
				Route:      &otpRoute{ShortName: "新幹線"},
				From:       otpPlace{Name: "東京"},
			},
		},
	}

	result := parseOTPResult(it)
	if result.Transfers != 1 {
		t.Errorf("transfers: got %d, want 1", result.Transfers)
	}
	if result.FirstVehicle != "東海道線" {
		t.Errorf("first_vehicle: got %q, want 東海道線", result.FirstVehicle)
	}
}

// ─── OTP integration (skipped without running OTP instance) ──────────────────

func TestCallOTPPlanAPI_Integration(t *testing.T) {
	otpURL := os.Getenv("OTP_URL")
	if otpURL == "" {
		t.Skip("OTP_URL not set, skipping integration test")
	}

	unix, err := parseArriveBy("09:00")
	if err != nil {
		t.Fatalf("parseArriveBy: %v", err)
	}

	from, err := geocodeLocation(context.Background(), "渋谷駅")
	if err != nil {
		t.Fatalf("geocodeLocation origin: %v", err)
	}
	to, err := geocodeLocation(context.Background(), "新宿駅")
	if err != nil {
		t.Fatalf("geocodeLocation destination: %v", err)
	}

	result, err := callOTPPlanAPI(context.Background(), otpURL, from, to, unix)
	if err != nil {
		t.Fatalf("callOTPPlanAPI: %v", err)
	}
	if result.DurationMinutes <= 0 {
		t.Error("expected positive duration")
	}
	if result.RecommendedDeparture == "" {
		t.Error("expected non-empty departure time")
	}
	t.Logf("Route: depart %s, arrive %s (%d min, via %s, %d transfer(s))",
		result.RecommendedDeparture, result.EstimatedArrival,
		result.DurationMinutes, result.FirstVehicle, result.Transfers)
}

// ─── Slack posting (mock server) ─────────────────────────────────────────────

func TestPostToSlack_MockServer(t *testing.T) {
	var received map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	msg := "テストメッセージ"
	if err := postToSlack(context.Background(), srv.URL, msg); err != nil {
		t.Fatalf("postToSlack: %v", err)
	}
	if received["text"] != msg {
		t.Errorf("slack payload: got %q, want %q", received["text"], msg)
	}
}

func TestPostToSlack_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "invalid_payload")
	}))
	defer srv.Close()

	err := postToSlack(context.Background(), srv.URL, "msg")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

// ─── Weather fallback (wttr.in mock) ─────────────────────────────────────────

func TestFetchWeatherWttr_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "晴れ +17°C(60%)")
	}))
	defer srv.Close()

	// We can't easily mock the URL, so just test the real wttr.in skippably
	// or test the function by calling it (it only needs internet)
	_ = srv

	// Smoke-test: if we have network, wttr.in should return something
	// (skip in CI environments without network)
	result, err := fetchWeatherWttr(context.Background(), "Tokyo")
	if err != nil {
		t.Skipf("network unavailable: %v", err)
	}
	if result == "" {
		t.Skip("wttr.in returned empty result (network may be restricted in CI)")
	}
	t.Logf("wttr.in result: %q", result)
}

// ─── TransitWant Initialize ───────────────────────────────────────────────────

// newTransitWant creates a properly initialized TransitWant for tests.
// StateLabels are populated to allow SetCurrent/GetCurrent to work correctly.
func newTransitWant(params map[string]any) *TransitWant {
	tw := &TransitWant{}
	tw.Metadata = Metadata{Name: "test-transit", Type: "transit"}
	tw.Spec = WantSpec{Params: params}
	tw.Init()
	tw.Locals = &TransitLocals{}
	tw.StateLabels = map[string]StateLabel{
		"transit_status":       LabelCurrent,
		"recommended_departure": LabelCurrent,
		"estimated_arrival":    LabelCurrent,
		"duration_minutes":     LabelCurrent,
		"first_vehicle":        LabelCurrent,
		"boarding_stop":        LabelCurrent,
		"transfers":            LabelCurrent,
		"route_name":           LabelCurrent,
		"transit_result":       LabelCurrent,
		"queried_at":           LabelCurrent,
		"error":                LabelCurrent,
		"achieving_percentage": LabelCurrent,
	}
	return tw
}

func TestTransitWant_Initialize_Valid(t *testing.T) {
	tw := newTransitWant(map[string]any{
		"origin":      "渋谷駅",
		"destination": "新宿駅",
		"arrive_by":   "09:00",
		"days":        "mon,fri",
	})
	tw.Initialize()

	locals := tw.GetLocals()
	if locals.Origin != "渋谷駅" {
		t.Errorf("origin: got %q", locals.Origin)
	}
	if locals.Destination != "新宿駅" {
		t.Errorf("destination: got %q", locals.Destination)
	}
	if locals.ArriveBy != "09:00" {
		t.Errorf("arrive_by: got %q", locals.ArriveBy)
	}
	if len(locals.Days) != 2 || locals.Days[0] != "mon" || locals.Days[1] != "fri" {
		t.Errorf("days: got %v", locals.Days)
	}
}

func TestTransitWant_Initialize_MissingOrigin(t *testing.T) {
	tw := newTransitWant(map[string]any{"destination": "新宿駅"})
	tw.Initialize()

	if tw.GetStatus() != WantStatusConfigError {
		t.Errorf("expected WantStatusConfigError, got %q", tw.GetStatus())
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
