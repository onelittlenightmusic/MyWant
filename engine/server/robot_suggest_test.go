package server

import (
	"fmt"
	"testing"
	"time"
)

// Without Laya the pills are the home of this part of the day, then the wants
// that find things in their order; homes of other parts are left out, and
// there are three at most.
func TestSuggestByRules(t *testing.T) {
	night := time.Date(2026, 11, 19, 21, 0, 0, 0, time.Local)
	cands := []offerCandidate{
		{Key: "restart:中野の天気", Pill: "「中野の天気」を最新に"},
		{Key: "home:朝:transit_search", Pill: "いつもの国分寺への乗り換え"},
		{Key: "restart:北新宿の空き", Pill: "「北新宿の空き」を最新に"},
		{Key: "home:夜:transit_search", Pill: "いつもの中野坂上への乗り換え"},
		{Key: "restart:スマートゴルフ", Pill: "「スマートゴルフ」を最新に"},
	}
	got := suggestByRules(cands, night)
	want := []int{3, 0, 2, 4}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	shown, by := (&Server{}).pickSuggestions("base", cands, night)
	if by != "rules" || len(shown) != 4 { // all four the rules keep, up to suggestMaxAll
		t.Fatalf("worked out %d by %q, want 4 by rules", len(shown), by)
	}
	if v := suggestView(shown, by, suggestMax); len(v.Suggestions) != suggestMax || !shown[0].served || shown[3].served {
		t.Errorf("an ask's view = %d pills; served %v, %v", len(v.Suggestions), shown[0].served, shown[3].served)
	}
	if shown[0].Pill != "いつもの中野坂上への乗り換え" {
		t.Errorf("first pill = %q", shown[0].Pill)
	}
	for _, s := range shown {
		if s.ID == "" {
			t.Errorf("pill %q has no id", s.Pill)
		}
	}
}

func TestTakeSuggestionNeedsAPillShown(t *testing.T) {
	robotSuggests.Lock()
	robotSuggests.shown, robotSuggests.kept = nil, nil
	robotSuggests.Unlock()
	if _, ok := (&Server{}).takeSuggestion("suggest-1-0"); ok {
		t.Error("a pill never shown was taken")
	}
}

func TestPickLikelyMakesUpThePills(t *testing.T) {
	// Two likely: both, then the next likeliest to make three.
	if got := pickLikely([]float64{0.2, 0.7, 0.05, 0.6, 0.3}, 0.5, 0.1, 3); fmt.Sprint(got) != "[1 3 4]" {
		t.Errorf("two likely: %v, want [1 3 4]", got)
	}
	// None likely: the three likeliest above the floor.
	if got := pickLikely([]float64{0.36, 0.21, 0.05, 0.2, 0.09}, 0.5, 0.1, 3); fmt.Sprint(got) != "[0 1 3]" {
		t.Errorf("none likely: %v, want [0 1 3]", got)
	}
	// More likely than the pills: every likely one (the widget shows six).
	if got := pickLikely([]float64{0.9, 0.8, 0.7, 0.6, 0.2}, 0.5, 0.1, 3); fmt.Sprint(got) != "[0 1 2 3]" {
		t.Errorf("four likely: %v, want [0 1 2 3]", got)
	}
	// Below the floor: not even to make up.
	if got := pickLikely([]float64{0.05, 0.02}, 0.5, 0.1, 3); len(got) != 0 {
		t.Errorf("all below the floor: %v", got)
	}
}
