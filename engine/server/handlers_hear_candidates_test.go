package server

import (
	"testing"

	mywant "mywant/engine/core"
)

func TestHearCandidatesOf(t *testing.T) {
	list := &mywant.Want{Metadata: mywant.Metadata{ID: "l", Name: "持ち物", Type: "checklist"}}
	list.WantTypeDefinition = &mywant.WantTypeDefinition{Timings: []mywant.TimingDef{
		{Name: "answer", Murmur: false},
		{Name: "added", Description: "アイテムが追加されたとき", Murmur: true},
		{Name: "completed", Description: "全部終わったとき", Murmur: true},
	}}
	fire := &mywant.Want{Metadata: mywant.Metadata{ID: "f", Name: "花火", Type: "fireworks_effect"}}
	fire.WantTypeDefinition = &mywant.WantTypeDefinition{Hearing: &mywant.HearingDef{Kind: "act", Description: "盤面で花火を上げる"}}

	got := hearCandidatesOf(fire, list)
	if len(got) != 2 || got[0].Timing != "added" || got[1].Timing != "completed" {
		t.Fatalf("candidates = %+v, want added and completed", got)
	}
	if got[1].Kind != "act" || got[1].Action != "盤面で花火を上げる" || got[1].Hearer.ID != "f" || got[1].Speaker.ID != "l" {
		t.Fatalf("candidate = %+v", got[1])
	}
	// The fireworks murmur at nothing: nothing to hear of it.
	if back := hearCandidatesOf(list, fire); len(back) != 0 {
		t.Fatalf("heard of a want that murmurs at nothing: %+v", back)
	}
	// What it hears already is not offered again.
	fire.Spec.Hear = []mywant.HearSpec{{Type: "checklist", Timings: []string{"completed"}}}
	if got := hearCandidatesOf(fire, list); len(got) != 1 || got[0].Timing != "added" {
		t.Fatalf("after hearing completed = %+v, want added only", got)
	}
	// A type that says nothing of its hearing answers in a word.
	if k, a := hearingOf(list); k != "say" || a == "" {
		t.Fatalf("hearing of a type without one = %q %q", k, a)
	}
}
