package mywant

import "testing"

// The kept answer that is the want's answer now is marked current: the last
// one while the want has an answer, none once it has not — even though the
// result field, never written empty, still holds the last reservation.
func TestResultHistoryMarksTheCurrentAnswer(t *testing.T) {
	want := NewWantWithLocals(Metadata{Name: "golf"}, WantSpec{FinalResultField: "current_reservation"}, nil, "base")
	want.WantTypeDefinition = &WantTypeDefinition{
		Metadata: WantTypeMetadata{Labels: map[string]string{historyIfLabel: "is_reserved"}},
		State: []StateDef{
			{Name: "current_reservation"},
			{Name: "is_reserved", Persistent: true},
		},
	}
	cycle := func(updates map[string]any) {
		want.BeginProgressCycle()
		for k, v := range updates {
			want.storeState(k, v)
		}
		want.EndProgressCycle()
	}
	res := func(store string) map[string]any { return map[string]any{"store": store} }
	current := func() string {
		var out []string
		for _, e := range want.BuildHistory().ResultHistory {
			if e.Current {
				out = append(out, e.Result.(map[string]any)["store"].(string))
			}
		}
		return join(out)
	}

	cycle(map[string]any{"current_reservation": res("A"), "is_reserved": true})
	if got := current(); got != "A" {
		t.Fatalf("one answer: current %q", got)
	}
	cycle(map[string]any{"current_reservation": res("B")})
	if got := current(); got != "B" {
		t.Fatalf("a new answer: current %q", got)
	}
	// Played: no reservation now, the result field still B.
	cycle(map[string]any{"is_reserved": false})
	if got := current(); got != "" {
		t.Fatalf("no answer now: current %q", got)
	}
	// Found again.
	cycle(map[string]any{"is_reserved": true})
	if got := current(); got != "B" {
		t.Fatalf("found again: current %q", got)
	}
}
