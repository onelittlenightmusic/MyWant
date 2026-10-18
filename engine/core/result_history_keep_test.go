package mywant

import "testing"

// With history-if, a check that found no reservation is not an answer: its
// result field still carries the last reservation, and it is not kept. Those
// kept before the type said so are let go; one read from history-from, which
// does not carry the field, stays.
func TestResultHistoryKeepsOnlyWhatTheTypeCallsAnAnswer(t *testing.T) {
	want := NewWantWithLocals(Metadata{Name: "golf"}, WantSpec{FinalResultField: "current_reservation"}, nil, "base")
	def := &WantTypeDefinition{State: []StateDef{
		{Name: "current_reservation"},
		{Name: "is_reserved", Persistent: true},
		{Name: "past_reservations"},
	}}
	want.WantTypeDefinition = def
	cycle := func(updates map[string]any) {
		want.BeginProgressCycle()
		for k, v := range updates {
			want.storeState(k, v)
		}
		want.EndProgressCycle()
	}
	res := func(at, store string) map[string]any { return map[string]any{"datetime_rfc3339": at, "store": store} }
	stores := func() string {
		var out []string
		for _, e := range want.BuildHistory().ResultHistory {
			out = append(out, e.Result.(map[string]any)["store"].(string))
		}
		return join(out)
	}

	// Before the type says: the empty check is kept, as it always was.
	cycle(map[string]any{"current_reservation": res("2026-10-10T09:00:00+09:00", "A"), "is_reserved": true})
	cycle(map[string]any{"current_reservation": res("2026-10-10T09:00:00+09:00", "A-stale"), "is_reserved": false})
	if got := stores(); got != "A A-stale" {
		t.Fatalf("before history-if: %q", got)
	}

	// The type says: the empty one goes, the next empty check is not kept, a
	// reservation is — and one read from the page stays.
	def.Metadata.Labels = map[string]string{historyIfLabel: "is_reserved", historyFromLabel: "past_reservations", historyAtLabel: "datetime_rfc3339"}
	cycle(map[string]any{"current_reservation": res("2026-10-10T09:00:00+09:00", "B-stale"), "is_reserved": false})
	cycle(map[string]any{"past_reservations": []any{res("2026-10-05T10:00:00+09:00", "D")}})
	cycle(map[string]any{"current_reservation": res("2026-10-17T09:00:00+09:00", "C"), "is_reserved": true})
	if got := stores(); got != "D A C" {
		t.Fatalf("with history-if: %q", got)
	}
}
