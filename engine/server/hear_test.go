package server

import (
	"testing"

	mywant "mywant/engine/core"
)

func TestHearsAt(t *testing.T) {
	if !hearsAt(nil, []string{"answer"}) || !hearsAt([]string{"booked"}, []string{"answer", "booked"}) || hearsAt([]string{"booked"}, []string{"answer"}) {
		t.Fatal("hearsAt")
	}
}

func TestAmongNearest(t *testing.T) {
	at := func(id, typ, x, y string) *mywant.Want {
		w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: typ, Labels: map[string]string{
			"mywant.io/canvas-x": x, "mywant.io/canvas-y": y}}}
		return w
	}
	hearer := at("h", "weather", "0", "0")
	near := at("g1", "golf", "1", "0")
	far := at("g2", "golf", "6", "6")
	all := []*mywant.Want{hearer, near, far}
	if !amongNearest(near, hearer, all, "golf", 1) || amongNearest(far, hearer, all, "golf", 1) {
		t.Fatal("nearest 1")
	}
	if !amongNearest(far, hearer, all, "golf", 2) {
		t.Fatal("nearest 2")
	}
	// Beyond the radius it is not heard, even as the only one of its type.
	gone := at("g3", "golf", "11", "0")
	alone := []*mywant.Want{hearer, gone}
	if amongNearest(gone, hearer, alone, "golf", 1) {
		t.Fatal("heard a want 11 cells away")
	}
	edge := at("g4", "golf", "6", "8") // exactly 10
	if !amongNearest(edge, hearer, []*mywant.Want{hearer, edge}, "golf", 1) {
		t.Fatal("did not hear a want 10 cells away")
	}
}

func TestHearAsksLabelsFirst(t *testing.T) {
	s := &Server{}
	mywant.HearInterpret = s.interpretHeard
	defer func() { mywant.HearInterpret = nil }()
	m := mywant.HeardMurmur{
		Result: map[string]any{"datetime": "2026-11-22 10:00", "store": "北新宿店"},
		Labels: map[string]string{"place": "新宿"},
	}
	got := mywant.Interpret(nil, m, []string{mywant.HearWhen, mywant.HearWhere})
	if got.Meaning["where"].Choice != "新宿" {
		t.Fatalf("label not preferred: %+v", got.Meaning)
	}
	if c := got.Meaning["when"].Choice; c == "" || c[:16] != "2026-11-22T10:00" {
		t.Fatalf("when %+v", got.Meaning)
	}
	m.Labels = nil
	if got := mywant.Interpret(nil, m, []string{mywant.HearWhere}); got.Meaning["where"].Choice != "北新宿店" {
		t.Fatalf("where from result %+v", got.Meaning)
	}
}
