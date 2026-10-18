package mywant

import "testing"

// Answers edited by hand: added in order, changed, let go — and one let go is
// not brought back, neither by a list the run reads nor by the run finding it
// again.
func TestResultHistoryCRUD(t *testing.T) {
	want := NewWantWithLocals(Metadata{Name: "golf"}, WantSpec{FinalResultField: "current_reservation"}, nil, "base")
	def := &WantTypeDefinition{State: []StateDef{{Name: "current_reservation"}, {Name: "past_reservations"}}}
	def.Metadata.Labels = map[string]string{historyFromLabel: "past_reservations", historyAtLabel: "datetime_rfc3339"}
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
		for _, e := range want.ResultHistoryEntries() {
			out = append(out, e.Result.(map[string]any)["store"].(string))
		}
		return join(out)
	}

	cycle(map[string]any{"current_reservation": res("2026-10-17T09:00:00+09:00", "B")})
	added := want.AddResultHistory(ResultHistoryEntry{Result: res("2026-10-10T09:00:00+09:00", "A")})
	if got := stores(); got != "A B" {
		t.Fatalf("after add: %q", got)
	}
	if _, ok := want.UpdateResultHistory(ResultHistoryKey(added), func(e *ResultHistoryEntry) { e.Result = res("2026-10-10T09:00:00+09:00", "A2") }); !ok {
		t.Fatal("update: not found")
	}
	if got := stores(); got != "A2 B" {
		t.Fatalf("after update: %q", got)
	}

	// Let go: the read list has it, and the run finds the current one again.
	var current string
	for _, e := range want.ResultHistoryEntries() {
		if e.Result.(map[string]any)["store"] == "B" {
			current = ResultHistoryKey(e)
		}
	}
	if !want.DeleteResultHistory(ResultHistoryKey(added)) || !want.DeleteResultHistory(current) {
		t.Fatal("delete: not found")
	}
	cycle(map[string]any{"past_reservations": []any{res("2026-10-10T09:00:00+09:00", "A2")}})
	cycle(map[string]any{"current_reservation": res("2026-10-17T09:00:00+09:00", "B"), "past_reservations": []any{res("2026-10-10T09:00:00+09:00", "A2"), res("2026-10-05T10:00:00+09:00", "D")}})
	if got := stores(); got != "D" {
		t.Fatalf("after delete: %q, want only D", got)
	}
	if want.DeleteResultHistory("nope") {
		t.Error("deleted what is not there")
	}
}

// A change to the answers is said, and moves the want's hash — the GUI reads
// a want again only when that moves.
func TestResultHistoryChangeIsSaidAndHashed(t *testing.T) {
	want := NewWantWithLocals(Metadata{Name: "golf"}, WantSpec{FinalResultField: "current_reservation"}, nil, "base")
	said := make(chan string, 4)
	saved := OnResultHistoryChanged
	OnResultHistoryChanged = func(w *Want) { said <- w.GetMetadata().Name }
	defer func() { OnResultHistoryChanged = saved }()

	before := CalculateWantHash(want)
	e := want.AddResultHistory(ResultHistoryEntry{Result: map[string]any{"store": "A"}})
	if <-said != "golf" {
		t.Fatal("add not said")
	}
	added := CalculateWantHash(want)
	if added == before {
		t.Error("hash did not move on add")
	}
	want.DeleteResultHistory(ResultHistoryKey(e))
	<-said
	if CalculateWantHash(want) == added {
		t.Error("hash did not move on delete")
	}
}
