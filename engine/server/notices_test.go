package server

import (
	"testing"
	"time"
)

func TestRecentDownsResetByUp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now()
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	for _, r := range []noticeRating{
		{ID: "a", At: at(40 * 24 * time.Hour), WantID: "w", Timings: []string{"booked"}, Rating: "down"}, // too old
		{ID: "b", At: at(3 * time.Hour), WantID: "w", Timings: []string{"booked"}, Rating: "down"},
		{ID: "c", At: at(2 * time.Hour), WantID: "w", Timings: []string{"booked"}, Rating: "up"},
		{ID: "d", At: at(time.Hour), WantID: "w", Timings: []string{"booked"}, Rating: "down"},
		{ID: "e", At: at(time.Hour), WantID: "w", Timings: []string{"soon"}, Rating: "down"},
		{ID: "f", At: at(time.Hour), WantID: "x", Timings: []string{"booked"}, Rating: "down"},
	} {
		if err := appendJSONLine(noticeRatingsFile(), r); err != nil {
			t.Fatal(err)
		}
	}
	if got := recentDowns("w", []string{"booked"}, now); got != 1 {
		t.Fatalf("downs = %d, want 1 (reset by the 👍)", got)
	}
}

func TestNoticeKeptToRate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	noteNotice(noticeRecord{ID: "notif-1", WantID: "w", Want: "golf", Text: "予約できた", Timings: []string{"booked"}})
	n, ok := findNotice("notif-1")
	if !ok || n.Want != "golf" || n.At == "" {
		t.Fatalf("notice %+v %v", n, ok)
	}
	s := &Server{}
	id := s.noticeDecisionID(n)
	if id == "" || s.noticeDecisionID(n) != id {
		t.Fatalf("decision not entered once: %q", id)
	}
}
