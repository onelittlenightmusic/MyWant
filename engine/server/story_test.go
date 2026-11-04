package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
)

func storyTestRecorder(t *testing.T, now *time.Time) *storyRecorder {
	r := newStoryRecorder(t.TempDir())
	r.now = func() time.Time { return *now }
	r.start.Do(func() {}) // no clock of its own: the test moves time
	return r
}

func kinds(evs []ws.StoryEvent) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

// A walk onto a want, off it into the open, stopping there, and on to
// another: arrived and left (told by the intersections, as the cursor PUT
// does), a move where it stopped (not each step), and the second arrival from
// the first want.
func TestStoryFromTheCursor(t *testing.T) {
	now := time.Date(2026, 11, 4, 22, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	cell := func(x, y int) (*int, *int) { return &x, &y }
	ax, ay := cell(3, 3)
	bx, by := cell(8, 8)
	wantA := &ws.StoryPlace{Kind: "want", ID: "want-a", Name: "transit_search-2118", Type: "transit_search", X: ax, Y: ay}
	wantB := &ws.StoryPlace{Kind: "want", ID: "want-b", Name: "戸塚", Type: "transit_search", X: bx, Y: by}
	on := map[[2]int]*ws.StoryPlace{{3, 3}: wantA, {8, 8}: wantB}
	var standing *ws.StoryPlace
	step := func(x, y int, msg string) {
		now = now.Add(300 * time.Millisecond)
		r.cursor("chr-1", x, y, "dev-1", msg)
		// what intersection.go tells, after the cursor
		if w := on[[2]int{x, y}]; w != standing {
			if standing != nil {
				r.leave("chr-1", standing)
			}
			if w != nil {
				r.arrive("chr-1", w)
			}
			standing = w
		}
	}

	step(0, 0, "")
	step(1, 1, "")
	step(3, 3, "") // onto want-a
	step(3, 3, "") // a keepalive: nothing
	now = now.Add(3 * time.Second)
	r.noticeStops() // stopped on a want: its arrival says it
	step(4, 4, "")  // off it
	step(5, 5, "")
	now = now.Add(3 * time.Second)
	r.noticeStops() // stopped at 5,5
	step(6, 6, "")  // through
	step(8, 8, "中野坂上から戸塚への乗り換え")

	evs := r.pending["chr-1"]
	want := []string{"want.arrive", "want.leave", "canvas.move", "say", "want.arrive"}
	if got := kinds(evs); len(got) != len(want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	for i, k := range want {
		if evs[i].Kind != k {
			t.Fatalf("events %v, want %v", kinds(evs), want)
		}
	}
	if evs[0].From == nil || evs[0].From.Kind != "cell" || *evs[0].From.X != 0 || evs[0].To.ID != "want-a" {
		t.Errorf("onto want-a from where they stood: %+v", evs[0])
	}
	if evs[1].From.ID != "want-a" || evs[1].Device != "dev-1" {
		t.Errorf("off want-a: %+v", evs[1])
	}
	if evs[2].From == nil || *evs[2].From.X != 3 || *evs[2].To.X != 5 {
		t.Errorf("the move, from the want's cell to where they stopped: %+v %+v", evs[2].From, evs[2].To)
	}
	if evs[4].From == nil || evs[4].From.ID != "want-a" || evs[4].To.ID != "want-b" {
		t.Errorf("want to want: %+v", evs[4])
	}
	if evs[3].Detail["text"] != "中野坂上から戸塚への乗り換え" || evs[3].Device != "dev-1" {
		t.Errorf("said: %+v", evs[3])
	}
}

// The rule is one of the intersections: characters only, every want, not
// announced.
func TestStoryIsAnIntersectionRule(t *testing.T) {
	found := false
	for _, rule := range intersectionRules {
		if rule.name == "story" {
			found = true
			note := &mywant.Want{Metadata: mywant.Metadata{ID: "want-1", Name: "note-1", Type: "note"}}
			robot := &mywant.Want{Metadata: mywant.Metadata{ID: "want-r", Name: "robot", Type: "robot"}}
			if !rule.quiet || rule.applies(intersectionContext{}, note, mover{moverThing, "thg-1"}) ||
				!rule.applies(intersectionContext{}, note, mover{moverCharacter, "chr-1"}) ||
				rule.applies(intersectionContext{}, robot, mover{moverCharacter, "robot"}) {
				t.Errorf("story rule: quiet, characters only, not a want that is a character")
			}
		}
	}
	if !found {
		t.Error("no story rule among the intersections")
	}
}

// The GUI's per-character keys: the first seen is where they are, a change is
// an event — a want opened and closed, a view switched; others' keys are not
// this character's.
func TestStoryFromTheGUIState(t *testing.T) {
	now := time.Date(2026, 11, 4, 22, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	want := func(id string) *ws.StoryPlace { return &ws.StoryPlace{Kind: "want", ID: id, Name: "name-" + id} }

	r.guiState(map[string]any{"sidebar_want_id_chr-1": "", "sidebar_active_tab_chr-1": "params", "unrelated": 1}, want)
	r.guiState(map[string]any{"sidebar_want_id_chr-1": "want-a"}, want)
	r.guiState(map[string]any{"sidebar_want_id_chr-1": "want-a"}, want) // rewritten, no change
	r.guiState(map[string]any{"sidebar_active_tab_chr-1": "results"}, want)
	r.guiState(map[string]any{"sidebar_want_id_chr-1": nil}, want)

	evs := r.pending["chr-1"]
	if got := kinds(evs); len(got) != 3 || got[0] != "want.open" || got[1] != "view.change" || got[2] != "want.close" {
		t.Fatalf("events %v", got)
	}
	if evs[0].To.ID != "want-a" || evs[0].To.Name != "name-want-a" || evs[2].From.ID != "want-a" {
		t.Errorf("open and close: %+v %+v", evs[0], evs[2])
	}
	if evs[1].From.Name != "params" || evs[1].To.Name != "results" {
		t.Errorf("view: %+v %+v", evs[1].From, evs[1].To)
	}
}

// A turn is the character's who asked: named, else the device's, else the
// one who just said its question; its answer and act follow it there.
func TestStoryTurnsAreTheAskers(t *testing.T) {
	now := time.Date(2026, 11, 4, 22, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	byDevice := func(d string) string {
		if d == "iphone" {
			return "chr-phone"
		}
		return ""
	}
	if c := r.characterOf("chr-x", "iphone", "q", byDevice); c != "chr-x" {
		t.Errorf("named: %q", c)
	}
	if c := r.characterOf("", "iphone", "q", byDevice); c != "chr-phone" {
		t.Errorf("the device's: %q", c)
	}
	r.cursor("chr-gui", 0, 0, "", "@robot 今の時刻で再検索")
	if c := r.characterOf("", "", "今の時刻で再検索", byDevice); c != "chr-gui" {
		t.Errorf("who just said it: %q", c)
	}
	now = now.Add(3 * time.Minute)
	if c := r.characterOf("", "", "今の時刻で再検索", byDevice); c != "" {
		t.Errorf("said too long ago: %q", c)
	}

	r.ask("chr-phone", "turn-1", "iphone", "中野坂上から戸塚への乗り換え")
	r.answer("turn-1", []robotCard{{Kind: "want", ID: "want-b", Name: "戸塚"}}, false)
	act := ws.Act{Metadata: ws.ResourceMeta{Name: "act-1"}, Spec: ws.ActSpec{Plan: "plan-1", Turn: "turn-1"},
		Status: ws.ActStatus{Phase: ws.ActSucceeded, Cards: []*ws.RobotCard{{Kind: "want", ID: "want-b", Name: "戸塚"}}}}
	r.actDone(act, ws.PlanSpec{Action: "existing", Target: "戸塚", Capability: "restart"})

	evs := r.pending["chr-phone"]
	if got := kinds(evs); len(got) != 3 || got[0] != "chat.ask" || got[1] != "chat.answer" || got[2] != "act.done" {
		t.Fatalf("events %v", got)
	}
	if evs[1].Detail["cards"] != "want:want-b" || evs[2].Detail["target"] != "戸塚" || evs[2].To.ID != "want-b" {
		t.Errorf("answer and act: %+v %+v", evs[1], evs[2])
	}
	r.answer("turn-unknown", nil, true)
	if len(r.pending[storyUnknown]) != 1 {
		t.Errorf("a turn of no one known: %v", r.pending)
	}
}

// Written a line an event, read back by kind and limit, held events too; a
// day older than ten gzipped and still read, one older than ninety let go.
func TestStoryWrittenReadTidied(t *testing.T) {
	now := time.Date(2026, 11, 4, 22, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	for i := 0; i < 3; i++ {
		r.add(ws.StoryEvent{Character: "chr-1", Kind: ws.StoryKindSay, At: now.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)})
	}
	r.add(ws.StoryEvent{Character: "chr-1", Kind: ws.StoryKindChatAsk, At: now.Add(5 * time.Minute).Format(time.RFC3339)})
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	day := filepath.Join(r.root(), "chr-1", "2026-11-04.jsonl")
	if _, err := os.Stat(day); err != nil {
		t.Fatalf("written: %v", err)
	}
	r.add(ws.StoryEvent{Character: "chr-1", Kind: ws.StoryKindSay, At: now.Add(6 * time.Minute).Format(time.RFC3339)})

	all, err := r.read("chr-1", storyQuery{since: now.Add(-time.Hour), until: now.Add(time.Hour)})
	if err != nil || len(all) != 5 {
		t.Fatalf("all, held too: %d %v", len(all), err)
	}
	says, _ := r.read("chr-1", storyQuery{since: now.Add(-time.Hour), until: now.Add(time.Hour), kinds: map[string]bool{"say": true}, limit: 2})
	if len(says) != 2 || says[1].At != now.Add(6*time.Minute).Format(time.RFC3339) {
		t.Errorf("the last two says: %+v", says)
	}
	if _, err := r.read("../etc", storyQuery{since: now, until: now}); err == nil {
		t.Error("a character that is a path")
	}

	// Days past.
	old := now.AddDate(0, 0, -12)
	ancient := now.AddDate(0, 0, -100)
	for _, d := range []time.Time{old, ancient} {
		if err := appendStoryLines(filepath.Join(r.root(), "chr-1", d.Format("2006-01-02")+".jsonl"),
			[]ws.StoryEvent{{Character: "chr-1", Kind: ws.StoryKindSay, At: d.Format(time.RFC3339)}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.tidy(); err != nil {
		t.Fatal(err)
	}
	if err := r.tidy(); err != nil { // the ancient day gzipped last time is let go now
		t.Fatal(err)
	}
	dir := filepath.Join(r.root(), "chr-1")
	if _, err := os.Stat(filepath.Join(dir, old.Format("2006-01-02")+".jsonl.gz")); err != nil {
		t.Errorf("twelve days ago, gzipped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, old.Format("2006-01-02")+".jsonl")); !os.IsNotExist(err) {
		t.Errorf("its plain file let go: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ancient.Format("2006-01-02")+".jsonl.gz")); !os.IsNotExist(err) {
		t.Errorf("a hundred days ago, let go: %v", err)
	}
	back, err := r.read("chr-1", storyQuery{since: old.Add(-time.Hour), until: old.Add(time.Hour)})
	if err != nil || len(back) != 1 {
		t.Errorf("read from the gzip: %d %v", len(back), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-11-04.jsonl")); err != nil {
		t.Errorf("today's left plain: %v", err)
	}
}

// A read across midnight finds the days on disk on both sides of it, whatever
// the time of day since and until are.
func TestStoryReadAcrossMidnight(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 30, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	r.add(ws.StoryEvent{Character: "c", Kind: ws.StoryKindSay, At: time.Date(2026, 11, 4, 23, 50, 0, 0, time.Local).Format(time.RFC3339)})
	r.add(ws.StoryEvent{Character: "c", Kind: ws.StoryKindSay, At: now.Add(-10 * time.Minute).Format(time.RFC3339)})
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name         string
		since, until time.Time
		want         int
	}{
		{"since 1h at 00:30", now.Add(-time.Hour), now.Add(time.Second), 2},
		{"until earlier in the day than since", time.Date(2026, 11, 4, 10, 0, 0, 0, time.Local), time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local), 2},
		{"today only", time.Date(2026, 11, 5, 0, 0, 0, 0, time.Local), now.Add(time.Second), 1},
	} {
		evs, err := r.read("c", storyQuery{since: c.since, until: c.until})
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != c.want {
			t.Errorf("%s: %d events, want %d", c.name, len(evs), c.want)
		}
	}
}
