package server

import (
	"testing"
	"time"

	ws "github.com/onelittlenightmusic/want-spec"
)

// outcomes is the turn.outcome events held for a character: turn → outcome.
func outcomes(r *storyRecorder, character string) map[string]ws.StoryEvent {
	out := map[string]ws.StoryEvent{}
	for _, ev := range r.pending[character] {
		if ev.Kind == ws.StoryKindTurnOutcome {
			out[ev.Ref] = ev
		}
	}
	return out
}

func storyWantAt(id, typ string) *ws.StoryPlace {
	return &ws.StoryPlace{Kind: "want", ID: id, Name: id, Type: typ}
}

// One exchange as the board has it: asked, the act shows want-a, the client
// brings the character there at once (before the answer is said), answered.
func storyExchange(r *storyRecorder, now *time.Time, turn, question string) {
	r.ask("chr-1", turn, "", question)
	*now = now.Add(5 * time.Second)
	r.actDone(ws.Act{Metadata: ws.ResourceMeta{Name: "act-" + turn}, Spec: ws.ActSpec{Turn: turn},
		Status: ws.ActStatus{Phase: "Succeeded", Cards: []*robotCard{{Kind: "want", ID: "want-a", Type: "transit_search"}}}},
		ws.PlanSpec{Action: "existing", Target: "want-a"})
	r.arrive("chr-1", storyWantAt("want-a", "transit_search")) // the client's doing
	*now = now.Add(2 * time.Second)
	r.answer(turn, []robotCard{{Kind: "want", ID: "want-a", Type: "transit_search"}}, false)
}

// Brought to the answer, and then opening it: the bringing judges nothing
// (marked via=answer), the opening is a success.
func TestStoryOutcomeSuccessNotFromTheBringing(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	told := make(chan string, 4)
	r.onOutcome = func(turn, outcome string) { told <- turn + "=" + outcome }
	storyExchange(r, &now, "t1", "中野坂上から銀座の乗り換え")

	for _, ev := range r.pending["chr-1"] {
		if ev.Kind == ws.StoryKindWantArrive && ev.Detail["via"] != "answer" {
			t.Errorf("the arrival the answer brought about is not marked: %+v", ev)
		}
	}
	if len(outcomes(r, "chr-1")) != 0 {
		t.Fatalf("judged from the bringing alone: %v", outcomes(r, "chr-1"))
	}
	now = now.Add(1 * time.Second)
	r.arrive("chr-1", storyWantAt("want-a", "transit_search")) // still within the bringing
	if len(outcomes(r, "chr-1")) != 0 {
		t.Fatalf("judged within the bringing")
	}
	now = now.Add(10 * time.Second)
	r.guiState(map[string]any{"expanded_want_id_chr-1": ""}, func(id string) *ws.StoryPlace { return storyWantAt(id, "transit_search") })
	r.guiState(map[string]any{"expanded_want_id_chr-1": "want-a"}, func(id string) *ws.StoryPlace { return storyWantAt(id, "transit_search") })
	got := outcomes(r, "chr-1")["t1"]
	if got.Detail["outcome"] != ws.TurnOutcomeSuccess || got.Detail["seconds"] != "11" || got.Detail["plan.action"] != "existing" {
		t.Fatalf("outcome %+v, want success after 11s with the plan", got)
	}
	// told to the turn, off the lock
	select {
	case got := <-told:
		if got != "t1=success" {
			t.Errorf("the turn was told %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the turn was not told")
	}
}

// Brought to the answer and nothing more: ignored, once the time is up.
func TestStoryOutcomeIgnored(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	storyExchange(r, &now, "t1", "中野坂上から銀座の乗り換え")
	now = now.Add(storyJudgeWithin - time.Second)
	r.noticeStops()
	if len(outcomes(r, "chr-1")) != 0 {
		t.Fatalf("judged before the time was up")
	}
	now = now.Add(2 * time.Second)
	r.noticeStops()
	if got := outcomes(r, "chr-1")["t1"]; got.Detail["outcome"] != ws.TurnOutcomeIgnored {
		t.Fatalf("outcome %+v, want ignored", got)
	}
}

// Going to another want of the type the answer showed: corrected, with where.
func TestStoryOutcomeCorrected(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	storyExchange(r, &now, "t1", "中野坂上から銀座の乗り換え")
	now = now.Add(20 * time.Second)
	r.arrive("chr-1", storyWantAt("want-other", "weather")) // another type: says nothing
	r.arrive("chr-1", storyWantAt("want-b", "transit_search"))
	got := outcomes(r, "chr-1")["t1"]
	if got.Detail["outcome"] != ws.TurnOutcomeCorrected || got.To == nil || got.To.ID != "want-b" {
		t.Fatalf("outcome %+v, want corrected to want-b", got)
	}
}

// Asking much the same again: reask; a different question does not.
func TestStoryOutcomeReask(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	storyExchange(r, &now, "t1", "中野坂上から銀座の乗り換え")
	now = now.Add(10 * time.Second)
	r.ask("chr-1", "t2", "", "中野の天気は？")
	if len(outcomes(r, "chr-1")) != 0 {
		t.Fatalf("another question judged the first: %v", outcomes(r, "chr-1"))
	}
	r.ask("chr-1", "t3", "", "中野坂上から銀座に乗り換え")
	got := outcomes(r, "chr-1")["t1"]
	if got.Detail["outcome"] != ws.TurnOutcomeReask || got.Detail["again"] != "t3" {
		t.Fatalf("outcome %+v, want reask by t3", got)
	}
}

// An answer that showed no want has nowhere to be gone to: no outcome when
// the time is up.
func TestStoryOutcomeNothingShown(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	r.ask("chr-1", "t1", "", "中野の天気は？")
	now = now.Add(3 * time.Second)
	r.answer("t1", nil, false)
	now = now.Add(storyJudgeWithin + time.Second)
	r.noticeStops()
	if len(outcomes(r, "chr-1")) != 0 || len(r.judging) != 0 {
		t.Fatalf("an answer showing nothing was judged or kept: %v", outcomes(r, "chr-1"))
	}
}

func TestStorySimilar(t *testing.T) {
	if s := storySimilar("中野坂上から銀座の乗り換え", "中野坂上から銀座に乗り換え"); s < storyReaskAt {
		t.Errorf("alike questions %.2f", s)
	}
	if s := storySimilar("中野の天気は？", "中野坂上から銀座の乗り換え"); s >= storyReaskAt {
		t.Errorf("different questions %.2f", s)
	}
}
