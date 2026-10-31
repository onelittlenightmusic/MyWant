package server

import (
	"reflect"
	"sync"
	"testing"
	"time"

	mywant "mywant/engine/core"
)

// testMoments puts a hub of the test's own in place of the server's: sent
// moments are collected, and its clock is the test's.
func testMoments(t *testing.T, window time.Duration) (*momentHub, func() [][]moment, *time.Time) {
	t.Helper()
	h := newMomentHub()
	h.window = window
	clock := time.Unix(1_000_000, 0)
	h.now = func() time.Time { return clock }
	var mu sync.Mutex
	var sent [][]moment
	h.send = func(ms []moment) {
		mu.Lock()
		sent = append(sent, append([]moment(nil), ms...))
		mu.Unlock()
	}
	was := moments
	moments = h
	t.Cleanup(func() { moments = was })
	return h, func() [][]moment {
		mu.Lock()
		defer mu.Unlock()
		return append([][]moment(nil), sent...)
	}, &clock
}

func TestMomentsGatherWithinTheWindowAndSayEachOnce(t *testing.T) {
	h, sent, _ := testMoments(t, 30*time.Millisecond)
	h.add(emote("want", "w1", markBang))
	h.add(emote("want", "w1", markBang), emote("want", "w2", markHeart))
	time.Sleep(80 * time.Millisecond)
	got := sent()
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("sent = %v, want one event of two moments", got)
	}
}

func TestMomentsHoldBackTheSameMarkForAWhile(t *testing.T) {
	h, sent, clock := testMoments(t, 10*time.Millisecond)
	h.add(emote("want", "w1", markQuestion))
	time.Sleep(40 * time.Millisecond)
	// Again within momentRepeatAfter: not sent. Another mark on it is.
	*clock = clock.Add(time.Second)
	h.add(emote("want", "w1", markQuestion), emote("want", "w1", markHeart))
	time.Sleep(40 * time.Millisecond)
	// After it: sent again.
	*clock = clock.Add(momentRepeatAfter)
	h.add(emote("want", "w1", markQuestion))
	time.Sleep(40 * time.Millisecond)
	got := sent()
	if len(got) != 3 || len(got[1]) != 1 || got[1][0].Mark != markHeart || got[2][0].Mark != markQuestion {
		t.Fatalf("sent = %v", got)
	}
}

func TestWantMomentsMarkErrorsFailuresAndValuesArriving(t *testing.T) {
	s := patchTestServer(t)
	h, sent, _ := testMoments(t, 10*time.Millisecond)
	id := patchTestWant(t, s)
	w, _, _ := s.globalBuilder.FindWantByID(id)

	s.wantMoment(w, "error", "boom", "")
	s.wantMoment(w, "error", "boom again", "boom") // not a new error
	s.wantMoment(w, mywant.MomentStatusKey, mywant.WantStatusFailed, mywant.WantStatusReaching)
	time.Sleep(40 * time.Millisecond)
	if got := sent(); len(got) != 1 || len(got[0]) != 1 || !reflect.DeepEqual(got[0][0], emote("want", id, markQuestion)) {
		t.Fatalf("sent = %v, want one ？ over the want", got)
	}

	s.wantMoment(w, mywant.ImpactField, map[string]any{"kind": "feed", "to": []any{w.Metadata.Name}, "n": 1}, nil)
	s.wantMoment(w, mywant.ImpactField, map[string]any{"kind": "attack", "to": []any{w.Metadata.Name}, "n": 2}, nil)
	time.Sleep(40 * time.Millisecond)
	name := w.Metadata.Name
	want := []moment{
		{Kind: "impact", Type: "feed", From: name, On: name},
		emote("want", id, markBang),
		{Kind: "impact", Type: "attack", From: name, On: name},
	}
	got := sent()
	if len(got) != 2 || len(got[1]) != len(want) {
		t.Fatalf("sent = %v, want both effects and one ！ for the value fed", got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[1][i], want[i]) {
			t.Fatalf("moment %d = %+v, want %+v", i, got[1][i], want[i])
		}
	}
	_ = h
}

func TestPostMomentsTakesOnlyMarks(t *testing.T) {
	s := patchTestServer(t)
	_, sent, _ := testMoments(t, 10*time.Millisecond)
	w := patchTestDo(t, s, "POST", "/api/v1/moments", map[string]any{"moments": []map[string]any{
		{"kind": "emote", "on": "character", "id": "chr-1", "mark": "question"},
		{"kind": "emote", "on": "character", "id": "chr-1", "mark": "dance"},
		{"kind": "speech", "on": "want", "id": "w"},
	}})
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	time.Sleep(40 * time.Millisecond)
	if got := sent(); len(got) != 1 || len(got[0]) != 1 || !reflect.DeepEqual(got[0][0], emote("character", "chr-1", markQuestion)) {
		t.Fatalf("sent = %v", got)
	}
}

func TestWantSayIsASpeechMoment(t *testing.T) {
	s := patchTestServer(t)
	_, sent, _ := testMoments(t, 10*time.Millisecond)
	id := patchTestWant(t, s)
	w, _, _ := s.globalBuilder.FindWantByID(id)

	s.wantMoment(w, "say", "あと2個", "")
	s.wantMoment(w, "say", "", "あと2個") // falling silent: no line
	time.Sleep(40 * time.Millisecond)
	got := sent()
	if len(got) != 1 || len(got[0]) != 1 {
		t.Fatalf("sent = %v, want one line", got)
	}
	m := got[0][0]
	if m.Kind != "speech" || m.By != "want" || m.ID != id || m.Text != "あと2個" || m.At == 0 {
		t.Fatalf("moment = %+v", m)
	}
}
