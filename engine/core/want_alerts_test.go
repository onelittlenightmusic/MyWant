package mywant

import (
	"testing"
	"time"
)

func alertWant(alerts ...AlertRule) *Want {
	w := &Want{Metadata: Metadata{Name: "golf", Type: "picture"}}
	w.Spec.Alerts = alerts
	return w
}

func heldNames(held []HeldAlert) []string {
	var names []string
	for _, h := range held {
		names = append(names, h.Rule.Name)
	}
	return names
}

func TestHeldAlertsWhen(t *testing.T) {
	w := alertWant(AlertRule{Name: "雨で中止", When: &ConditionDef{Field: "weather", Operator: "==", Value: "rain"}})
	now := time.Now()

	if held := w.HeldAlerts(now, false); len(held) != 0 {
		t.Fatalf("no state yet, nothing should hold: %v", heldNames(held))
	}
	w.StoreState("weather", "sunny")
	if held := w.HeldAlerts(now, false); len(held) != 0 {
		t.Fatalf("sunny should not hold: %v", heldNames(held))
	}
	w.StoreState("weather", "rain")
	held := w.HeldAlerts(now, false)
	if len(held) != 1 || held[0].Rule.Name != "雨で中止" || held[0].Detail != "weather = rain" {
		t.Fatalf("rain should hold once: %+v", held)
	}
}

func TestHeldAlertsStatus(t *testing.T) {
	w := alertWant(AlertRule{Name: "失敗", Status: []string{"failed", "module_error"}})
	w.SetStatus(WantStatusReaching)
	if held := w.HeldAlerts(time.Now(), false); len(held) != 0 {
		t.Fatalf("reaching should not hold: %v", heldNames(held))
	}
	w.SetStatus(WantStatusFailed)
	if held := w.HeldAlerts(time.Now(), false); len(held) != 1 || held[0].Detail != "failed" {
		t.Fatalf("failed should hold: %+v", held)
	}
}

// Silence is measured from the last change — of one field, or of any state.
func TestHeldAlertsSilentFor(t *testing.T) {
	now := time.Now()
	w := alertWant(
		AlertRule{Name: "スコアが来ない", SilentFor: "2d", Field: "latest_url"},
		AlertRule{Name: "止まった", SilentFor: "6h"},
	)
	w.SetStatus(WantStatusReaching)

	if held := w.HeldAlerts(now, false); len(held) != 0 {
		t.Fatalf("a want that never changed has not gone quiet: %v", heldNames(held))
	}

	w.stateTimestamps.Store("latest_url", now.Add(-3*24*time.Hour))
	w.stateTimestamps.Store("heartbeat", now.Add(-1*time.Hour))
	if got := heldNames(w.HeldAlerts(now, false)); len(got) != 1 || got[0] != "スコアが来ない" {
		t.Fatalf("only the field's silence should hold: %v", got)
	}

	w.stateTimestamps.Store("heartbeat", now.Add(-7*time.Hour))
	if got := heldNames(w.HeldAlerts(now, false)); len(got) != 2 {
		t.Fatalf("both should hold once all state is 7h old: %v", got)
	}

	if held := w.HeldAlerts(now, true); len(held) != 0 {
		t.Fatalf("paused by its person, silence is expected: %v", heldNames(held))
	}
	w.SetStatus(WantStatusSuspended)
	if held := w.HeldAlerts(now, false); len(held) != 0 {
		t.Fatalf("suspended, silence is expected: %v", heldNames(held))
	}
}

func TestHeldAlertsIgnoresMalformed(t *testing.T) {
	w := alertWant(
		AlertRule{Name: "空"},
		AlertRule{Name: "壊れた期間", SilentFor: "soon"},
	)
	w.stateTimestamps.Store("x", time.Now().Add(-100*time.Hour))
	if held := w.HeldAlerts(time.Now(), false); len(held) != 0 {
		t.Fatalf("malformed rules should not hold: %v", heldNames(held))
	}
}

func TestParseAlertDuration(t *testing.T) {
	cases := map[string]time.Duration{"6h": 6 * time.Hour, "90m": 90 * time.Minute, "2d": 48 * time.Hour}
	for in, want := range cases {
		if got, err := parseAlertDuration(in); err != nil || got != want {
			t.Errorf("%q = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0h", "-1h", "0d", "xd", "soon"} {
		if _, err := parseAlertDuration(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}
