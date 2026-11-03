package server

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	mwlabels "mywant/engine/labels"
)

// One constellation for a want's answers (constellation_answers.go): the
// constellations made per answer are folded into it, each member marked for
// its answer; a wire to an answer ball joins the want's; the photos of an
// answer are found by the mark.
func TestAnswerConstellationsAreTheWants(t *testing.T) {
	s := patchTestServer(t)
	dir := t.TempDir()
	s.thingStore = &ThingStore{path: filepath.Join(dir, "memo.yaml")}
	s.thingLabels = &ThingLabelStore{mwlabels.NewFileStore(filepath.Join(dir, "memo-labels.yaml"))}
	wantID := robotArchiveTestWant(t, s, "golf", nil, map[string]any{"service_time": 0.1})
	a1, a2 := wantID+answerSep+"k1", wantID+answerSep+"k2"

	// The old way: a constellation per answer, the answer one of its members.
	for _, m := range []string{a1, "photo-1"} {
		s.setConstellationMembership("thing", "golf 10.11 09:00", m, true)
	}
	s.setConstellationCaptureEverywhere("golf 10.11 09:00", a1)
	for _, m := range []string{a2, "photo-2", "photo-3"} {
		s.setConstellationMembership("thing", "golf 10.17 13:00", m, true)
	}
	s.migrateAnswerConstellations()

	if got := s.membersOfConstellation("thing", "golf"); !sameSet(got, []string{"photo-1", "photo-2", "photo-3"}) {
		t.Fatalf("the want's constellation = %v", got)
	}
	for _, old := range []string{"golf 10.11 09:00", "golf 10.17 13:00"} {
		if got := s.allMembersOfConstellation(old); len(got) != 0 {
			t.Errorf("%s still has %v", old, got)
		}
	}
	marks := map[string]string{"photo-1": "k1", "photo-2": "k2", "photo-3": "k2"}
	for id, key := range marks {
		if got := s.thingLabels.Get(id)[constellationAnswerKey("golf")]; got != key {
			t.Errorf("%s marked %q, want %q", id, got, key)
		}
	}
	// The hub is a member too.
	waitFor(t, func() bool { return sameSet(s.membersOfConstellation("want", "golf"), []string{wantID}) },
		"the want a member of its constellation")
	if got := s.anyConstellationCapture("golf"); got != wantID {
		t.Errorf("hub = %q, want the want", got)
	}
	joined := s.answerMembers()
	if !sameSet(joined[a2], []string{"photo-2", "photo-3"}) || !sameSet(joined[a1], []string{"photo-1"}) {
		t.Errorf("answer members = %v", joined)
	}

	// A wire from an answer ball to a thing: into the want's, marked.
	if hub, ok := s.joinAnswersIn([]string{a1, "photo-4"}); !ok || hub != "golf" {
		t.Fatalf("joinAnswersIn = %q %v", hub, ok)
	}
	if got := s.thingLabels.Get("photo-4")[constellationAnswerKey("golf")]; got != "k1" {
		t.Errorf("photo-4 marked %q", got)
	}

	// Leaving takes the mark; a rename carries it.
	s.setConstellationMembership("thing", "golf", "photo-4", false)
	if got := s.thingLabels.Get("photo-4")[constellationAnswerKey("golf")]; got != "" {
		t.Errorf("left, still marked %q", got)
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func waitFor(t *testing.T, ok func() bool, what string) {
	t.Helper()
	for i := 0; i < 150; i++ {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("never: %s", what)
}
