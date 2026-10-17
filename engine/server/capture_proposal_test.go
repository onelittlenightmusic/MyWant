package server

import (
	"testing"
	"time"

	mywant "mywant/engine/core"
)

func TestNearestAnswer(t *testing.T) {
	res := func(at string) mywant.ResultHistoryEntry {
		return mywant.ResultHistoryEntry{ID: at, Timestamp: time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local),
			Result: map[string]any{"datetime_rfc3339": at}}
	}
	h := []mywant.ResultHistoryEntry{
		res("2026-10-10T09:00:00+09:00"), res("2026-10-10T12:00:00+09:00"),
		res("2026-10-17T09:00:00+09:00"), res("2026-10-17T13:00:00+09:00"),
	}
	at := func(s string) time.Time { v, _ := parseLocalTime(s); return v }
	cases := map[string]string{
		"2026-10-17 09:39:48": "2026-10-17T09:00:00+09:00", // played this morning: the 09:00, not the 13:00
		"2026-10-10 12:56:53": "2026-10-10T12:00:00+09:00",
		"2026-10-17 12:45:00": "2026-10-17T13:00:00+09:00", // a little early for the 13:00
		"2026-09-01 10:00:00": "2026-10-10T09:00:00+09:00", // nothing near: the nearest
	}
	for photo, want := range cases {
		e, ok := nearestAnswer(h, at(photo))
		if !ok || e.ID != want {
			t.Errorf("%s: got %s, want %s", photo, e.ID, want)
		}
	}
}
