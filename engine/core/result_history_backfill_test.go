package mywant

import "testing"

// The past reservations a check reads go into the answers, each at its own
// time and once — and the next reservation, found by the check itself, stays
// the newest answer.
func TestBackfillResultHistoryMergesPastAnswersInOrder(t *testing.T) {
	want := NewWantWithLocals(
		Metadata{Name: "golf"},
		WantSpec{FinalResultField: "current_reservation"},
		nil,
		"base",
	)
	def := &WantTypeDefinition{State: []StateDef{{Name: "current_reservation"}, {Name: "past_reservations"}}}
	def.Metadata.Labels = map[string]string{historyFromLabel: "past_reservations", historyAtLabel: "datetime_rfc3339"}
	want.WantTypeDefinition = def

	res := func(at, store string) map[string]any {
		return map[string]any{"datetime_rfc3339": at, "store": store}
	}
	cycle := func(updates map[string]any) {
		want.BeginProgressCycle()
		for k, v := range updates {
			want.storeState(k, v)
		}
		want.EndProgressCycle()
	}

	// Found by a check: the 10th, then the 17th.
	cycle(map[string]any{"current_reservation": res("2026-10-10T09:00:00+09:00", "A")})
	cycle(map[string]any{"current_reservation": res("2026-10-17T09:00:00+09:00", "B")})
	// Read from the page: the 10th again, and two never seen.
	past := []any{
		res("2026-10-12T21:00:00+09:00", "C"),
		res("2026-10-10T09:00:00+09:00", "A"),
		res("2026-10-05T10:00:00+09:00", "D"),
	}
	cycle(map[string]any{"past_reservations": past})
	cycle(map[string]any{"past_reservations": past}) // again: nothing new

	got := want.BuildHistory().ResultHistory
	var stores []string
	for _, e := range got {
		stores = append(stores, e.Result.(map[string]any)["store"].(string))
	}
	if want, have := "D A C B", join(stores); have != want {
		t.Fatalf("answers = %q, want %q", have, want)
	}
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += " "
		}
		out += v
	}
	return out
}
