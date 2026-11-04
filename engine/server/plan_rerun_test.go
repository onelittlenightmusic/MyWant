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
	text := rerunParamsText(map[string]any{"from": "中野坂上", "to": "戸塚", "time": "21:17"})
	for _, want := range []string{"Its parameters now:", "time=21:17", "Fill only the parameters the person changes", "Leave every other parameter empty"} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q:\n%s", want, text)
		}
	}

	change := rerunChange(map[string]string{"time": " 22:05 ", "from": "", "to": "-", " ": "x"})
	if len(change) != 1 || change["time"] != "22:05" {
		t.Errorf("only what says something: %v", change)
	}

	now := time.Date(2026, 11, 4, 13, 5, 0, 0, time.UTC)
	if got := planNowText("Asia/Tokyo", now); !strings.Contains(got, "2026-11-04 22:05") || !strings.Contains(got, "Wednesday") {
		t.Errorf("the asker's time: %q", got)
	}
	if got := planNowText("Nowhere/Else", now); !strings.Contains(got, "2026-11-04 13:05") {
		t.Errorf("an unknown zone, the server's: %q", got)
	}
}
