package mywant

import "testing"

func TestOnWantChanged(t *testing.T) {
	var got []string
	saved := OnWantChanged
	OnWantChanged = func(id string) { got = append(got, id) }
	defer func() { OnWantChanged = saved }()

	w := &Want{Metadata: Metadata{ID: "w1", Name: "w1"}}
	w.WantTypeDefinition = &WantTypeDefinition{State: []StateDef{{Name: "ticks", Volatile: true}}}

	w.StoreState("hp", 3.0)
	w.StoreState("hp", 3.0) // unchanged: not news
	w.StoreState("ticks", 1) // volatile: not news
	w.SetLabel("mywant.io/canvas-x", "4")
	w.SetStatus(WantStatusSuspended)
	w.DeleteState("hp")
	w.DeleteState("hp") // already gone

	if len(got) != 4 {
		t.Fatalf("told %d times (%v), want 4: state, label, status, state deleted", len(got), got)
	}
}
