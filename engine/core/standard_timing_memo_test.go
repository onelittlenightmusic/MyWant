package mywant

import "testing"

func TestStandardAnswerTiming(t *testing.T) {
	def := &WantTypeDefinition{Timings: []TimingDef{{Name: "soon", Murmur: true, Notify: true}}}
	withStandardTimings(def)
	if len(def.Timings) != 2 || def.Timings[0].Name != AnswerTiming || !def.Timings[0].Notify {
		t.Fatalf("timings = %+v, want answer first, telling", def.Timings)
	}
	withStandardTimings(def) // again: not twice
	if len(def.Timings) != 2 {
		t.Fatalf("added twice: %+v", def.Timings)
	}

	own := &WantTypeDefinition{Timings: []TimingDef{{Name: AnswerTiming}, {Name: "album", Murmur: true}}}
	withStandardTimings(own)
	if len(own.Timings) != 2 || own.Timings[0].Murmur {
		t.Fatalf("a type's own answer replaced: %+v", own.Timings)
	}

	none := &WantTypeDefinition{}
	withStandardTimings(none)
	if len(none.Timings) != 0 {
		t.Fatalf("a type with no answers given a timing: %+v", none.Timings)
	}
	answers := &WantTypeDefinition{}
	answers.FinalResultField = "summary"
	withStandardTimings(answers)
	if len(answers.Timings) != 1 || answers.Timings[0].Name != AnswerTiming || !answers.Timings[0].Notify {
		t.Fatalf("a type with answers and no timings: %+v, want answer, telling", answers.Timings)
	}
	if (&Want{WantTypeDefinition: answers}).HasTimings() {
		t.Fatal("the standard timing alone counted as timings of its own")
	}

	// A new answer at the standard timing tells, the type's own declared or not.
	w := &Want{WantTypeDefinition: def}
	if tell := w.TellingOf(ResultHistoryEntry{Timings: []string{AnswerTiming}}); !tell.Murmur || !tell.Notify {
		t.Fatalf("a new answer not told: %+v", tell)
	}
	w2 := &Want{WantTypeDefinition: own}
	if tell := w2.TellingOf(ResultHistoryEntry{Timings: []string{AnswerTiming}}); tell.Murmur || tell.Notify {
		t.Fatalf("a type that turned answer off told: %+v", tell)
	}
}

func TestMemoMergesAndLetsGo(t *testing.T) {
	w := &Want{Metadata: Metadata{ID: "want-memo"}}
	MRSApplyDirective(w, map[string]any{"_memo": map[string]any{"a": "1", "b": 2.0}})
	MRSApplyDirective(w, map[string]any{"_memo": map[string]any{"b": nil, "c": true}})
	got := wantMemo(w)
	if len(got) != 2 || got["a"] != "1" || got["c"] != true {
		t.Fatalf("memo = %v", got)
	}
	w.prepareForRestart()
	if len(wantMemo(w)) != 2 {
		t.Fatal("a restart lost the memo")
	}
	if c := MRSWantContext(w); c["memo"].(map[string]any)["a"] != "1" {
		t.Fatalf("not handed back: %v", c["memo"])
	}
}

func TestTimingMoments(t *testing.T) {
	w := &Want{WantTypeDefinition: &WantTypeDefinition{Timings: []TimingDef{
		{Name: AnswerTiming, Description: "新しい結果が出たとき"},
		{Name: "cancelled", Description: "キャンセルされたとき"},
		{Name: "soon"},
	}}}
	if got := w.TimingMoments([]string{AnswerTiming}); got != "" {
		t.Errorf("only a new answer: %q, want nothing", got)
	}
	if got := w.TimingMoments([]string{AnswerTiming, "cancelled", "soon"}); got != "キャンセルされたとき、soon" {
		t.Errorf("moments: %q", got)
	}
}
