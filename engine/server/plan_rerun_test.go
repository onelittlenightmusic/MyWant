package server

import (
	"strings"
	"testing"
	"time"
)

// 「出発時刻を今にして再検索」 on the route asked at 21:17: the model is told
// the want's parameters and the time now, and fills only what changes; what
// it leaves empty is not set.
func TestRerunWithWhatChanges(t *testing.T) {
	text := rerunParamsText()
	for _, want := range []string{"Fill only the parameters the person changes", "Leave every other parameter empty", "Never copy the value it has now"} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q:\n%s", want, text)
		}
	}
	// What it is now is said beside each parameter, with the time now beside
	// a time: 「今の時刻で再検索」 is 22:05, not the 21:17 it has.
	now := time.Date(2026, 11, 4, 22, 5, 0, 0, time.UTC)
	if got := rerunParamHint("21:17") + paramNowHint("time", now); got != " — now 21:17; a new value only if the person changes it, else empty. (the time now: 22:05)" {
		t.Errorf("time: %q", got)
	}
	if got := rerunParamHint(nil); !strings.Contains(got, "not set now") {
		t.Errorf("unset: %q", got)
	}
	if paramNowHint("date", now) != " (today: 2026-11-04)" || paramNowHint("station", now) != "" {
		t.Error("today beside a date, nothing beside a station")
	}

	change := rerunChange(map[string]string{"time": " 22:05 ", "from": "", "to": "-", " ": "x"})
	if len(change) != 1 || change["time"] != "22:05" {
		t.Errorf("only what says something: %v", change)
	}

	now = time.Date(2026, 11, 4, 13, 5, 0, 0, time.UTC)
	if got := planNowText("Asia/Tokyo", now); !strings.Contains(got, "2026-11-04 22:05") || !strings.Contains(got, "Wednesday") {
		t.Errorf("the asker's time: %q", got)
	}
	if got := planNowText("Nowhere/Else", now); !strings.Contains(got, "2026-11-04 13:05") {
		t.Errorf("an unknown zone, the server's: %q", got)
	}
}
