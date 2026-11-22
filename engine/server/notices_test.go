package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
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

// Rated again, the last rating stands; "none" takes it back — in the list and
// in what Laya learns from (the last label of the decision, with nothing).
func TestNoticeRatedAgainAndTakenBack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	noteNotice(noticeRecord{ID: "notif-1", WantID: "w", Want: "golf", Text: "予約できた"})
	noteNotice(noticeRecord{ID: "notif-2", WantID: "w", Want: "golf", Text: "空いた"})
	s := &Server{}
	rate := func(id, rating string) {
		r := httptest.NewRequest("POST", "/api/v1/notices/"+id+"/rate", strings.NewReader(`{"rating":"`+rating+`","character":"chr-1"}`))
		r = mux.SetURLVars(r, map[string]string{"id": id})
		w := httptest.NewRecorder()
		s.rateNotice(w, r)
		if w.Code != 200 {
			t.Fatalf("rate %s %s: %d %s", id, rating, w.Code, w.Body.String())
		}
	}
	list := func() map[string]noticeListItem {
		w := httptest.NewRecorder()
		s.listNotices(w, httptest.NewRequest("GET", "/api/v1/notices", nil))
		var got struct{ Notices []noticeListItem }
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		out := map[string]noticeListItem{}
		for _, n := range got.Notices {
			out[n.ID] = n
		}
		return out
	}
	rate("notif-1", "up")
	rate("notif-1", "down")
	rate("notif-2", "up")
	rate("notif-2", "none")
	got := list()
	if got["notif-1"].Rating != "down" || got["notif-1"].RatedBy != "chr-1" {
		t.Errorf("rated again: %+v", got["notif-1"])
	}
	if got["notif-2"].Rating != "" {
		t.Errorf("taken back: %+v", got["notif-2"])
	}
	n2, _ := findNotice("notif-2")
	if l := initiativeLabels()[s.noticeDecisionID(n2)]; l.gold() != nil {
		t.Errorf("taken back, still learnt from: %+v", l)
	}
}
