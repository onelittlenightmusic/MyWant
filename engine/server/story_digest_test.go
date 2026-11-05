package server

import (
	"os"
	"testing"
	"time"

	ws "github.com/onelittlenightmusic/want-spec"
)

// A day as the recorder holds it: a question answered and gone to, another
// asked again, one answered just before midnight and judged after it; walks
// between wants.
func TestStoryDigestOfADay(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)

	a := storyWantAt("want-a", "transit_search")
	b := storyWantAt("want-b", "weather")
	r.arrive("chr-1", b)
	now = now.Add(30 * time.Second)
	r.leave("chr-1", b)

	r.ask("chr-1", "t1", "", "中野坂上から銀座の乗り換え")
	now = now.Add(5 * time.Second)
	r.actDone(ws.Act{Metadata: ws.ResourceMeta{Name: "act-1"}, Spec: ws.ActSpec{Turn: "t1"},
		Status: ws.ActStatus{Phase: "Succeeded", Cards: []*robotCard{{Kind: "want", ID: "want-a", Type: "transit_search"}}}},
		ws.PlanSpec{Action: "new", Type: "transit_search", Params: map[string]string{"from": "中野坂上", "to": "銀座"}})
	r.arrive("chr-1", a) // brought there
	now = now.Add(2 * time.Second)
	r.answer("t1", []robotCard{{Kind: "want", ID: "want-a", Type: "transit_search"}}, false)
	now = now.Add(8 * time.Second)
	r.guiState(map[string]any{"sidebar_want_id_chr-1": ""}, func(id string) *ws.StoryPlace { return storyWantAt(id, "transit_search") })
	r.guiState(map[string]any{"sidebar_want_id_chr-1": "want-a"}, func(id string) *ws.StoryPlace { return storyWantAt(id, "transit_search") })
	now = now.Add(20 * time.Second)
	r.leave("chr-1", a)
	r.arrive("chr-1", b) // want-a → want-b

	r.ask("chr-1", "t2", "", "中野の天気は？")
	now = now.Add(3 * time.Second)
	r.answer("t2", []robotCard{{Kind: "want", ID: "want-b", Type: "weather"}}, false)
	now = now.Add(5 * time.Second)
	r.ask("chr-1", "t3", "", "中野の天気は")

	// t4: answered at 23:59:30, judged (ignored) after midnight.
	now = time.Date(2026, 11, 5, 23, 59, 0, 0, time.Local)
	r.ask("chr-1", "t4", "", "新宿から横浜")
	now = now.Add(30 * time.Second)
	r.answer("t4", []robotCard{{Kind: "want", ID: "want-c", Type: "transit_search"}}, false)
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(storyJudgeWithin + time.Second)
	r.noticeStops()
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}

	d, err := r.digest("chr-1", "2026-11-05")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Turns) != 0 || d.Counts[ws.StoryKindChatAsk] != 4 {
		t.Fatalf("turns %d (the ledger's, not the digest's), asks %d, want 0 and 4", len(d.Turns), d.Counts[ws.StoryKindChatAsk])
	}
	// t4, answered just before midnight, is judged (ignored) after it.
	if d.Outcomes[ws.TurnOutcomeSuccess] != 1 || d.Outcomes[ws.TurnOutcomeReask] != 1 || d.Outcomes[ws.TurnOutcomeIgnored] != 1 {
		t.Errorf("outcomes %v", d.Outcomes)
	}
	if len(d.Transitions) != 1 || d.Transitions[0].From.ID != "want-a" || d.Transitions[0].To.ID != "want-b" {
		t.Errorf("transitions %+v", d.Transitions)
	}
	var va, vb *ws.StoryVisit
	for i := range d.Visits {
		switch d.Visits[i].Place.ID {
		case "want-a":
			va = &d.Visits[i]
		case "want-b":
			vb = &d.Visits[i]
		}
	}
	if va == nil || va.Arrived != 1 || va.Opened != 1 || va.Seconds != 30 || vb == nil || vb.Arrived != 2 || vb.Seconds != 30 {
		t.Errorf("visits a %+v b %+v", va, vb)
	}
}

// A day is digested once past storyDigestAfter after its midnight, once; its
// digest outlives its events.
func TestStoryDigestWrittenOnceDueAndKept(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	r.ask("chr-1", "t1", "", "中野の天気は？")
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	path := r.digestPath("chr-1", "2026-11-05")

	now = time.Date(2026, 11, 6, 0, 10, 0, 0, time.Local)
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("digested before its last turns could be judged")
	}
	now = time.Date(2026, 11, 6, 0, 20, 0, 0, time.Local)
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("not digested when due: %v", err)
	}
	if _, err := os.Stat(r.digestPath("chr-1", "2026-11-06")); err == nil {
		t.Fatal("today digested")
	}

	// Long after: the events let go, the digest still read, today's worked out.
	now = time.Date(2027, 3, 1, 9, 0, 0, 0, time.Local)
	r.ask("chr-1", "t9", "", "新宿から横浜")
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.root() + "/chr-1/2026-11-05.jsonl.gz"); err == nil {
		t.Fatal("the day's events were kept past storyGzipDays")
	}
	got, err := r.digests("chr-1", "2026-11-01", "2027-03-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Date != "2026-11-05" || got[0].Counts[ws.StoryKindChatAsk] != 1 ||
		got[1].Date != "2027-03-01" || got[1].Counts[ws.StoryKindChatAsk] != 1 {
		t.Fatalf("digests %+v", got)
	}
}
