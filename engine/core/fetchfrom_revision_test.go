package mywant

import (
	"testing"

	want_spec "github.com/onelittlenightmusic/want-spec"
)

// The fetchFrom expansion is skipped while the want's state revision stands
// still, because nothing derived from that state can have moved. These pin the
// two halves of that bargain: it does not run when there is nothing to do, and
// it does run the moment there is.
func fetchFromWant() *Want {
	w := &Want{Metadata: Metadata{Name: "monitor", Type: "probe"}}
	w.WantTypeDefinition = &WantTypeDefinition{
		State: []want_spec.StateDef{{
			Name:        "summary",
			Label:       "current",
			FetchFrom:   "mrs_raw_output",
			OnFetchData: "summary",
		}},
	}
	w.StateLabels = map[string]StateLabel{
		"mrs_raw_output": LabelCurrent,
		"summary":        LabelCurrent,
	}
	return w
}

func cycle(w *Want) {
	w.BeginProgressCycle()
	w.EndProgressCycle()
}

// A new source value has to reach the derived field, every time it changes.
func TestFetchFromExpansionFollowsAChangedSource(t *testing.T) {
	w := fetchFromWant()

	w.StoreState("mrs_raw_output", map[string]any{"summary": "waiting"})
	cycle(w)
	if got := GetCurrent(w, "summary", ""); got != "waiting" {
		t.Fatalf("first expansion did not land: %q", got)
	}

	// The case the skip could plausibly break: a second, different reading.
	w.StoreState("mrs_raw_output", map[string]any{"summary": "powered"})
	cycle(w)
	if got := GetCurrent(w, "summary", ""); got != "powered" {
		t.Fatalf("a changed source did not reach the derived field: %q", got)
	}
}

// And a settled want has to stay settled across idle cycles. This pins the
// property the skip rests on rather than the skip itself: if an otherwise idle
// cycle could move the revision, the expansion would run every time anyway and
// the skip would buy nothing. That it does buy something is visible in a CPU
// profile, not here.
func TestFetchFromExpansionSkipsAnUnchangedSource(t *testing.T) {
	w := fetchFromWant()
	w.StoreState("mrs_raw_output", map[string]any{"summary": "powered"})
	cycle(w)

	settled := w.stateRevision.Load()
	for range 5 {
		cycle(w)
	}
	if now := w.stateRevision.Load(); now != settled {
		t.Errorf("idle cycles moved the state revision: %d -> %d", settled, now)
	}
	if got := GetCurrent(w, "summary", ""); got != "powered" {
		t.Errorf("the derived field did not survive the idle cycles: %q", got)
	}
}
