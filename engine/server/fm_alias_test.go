package server

import "testing"

// A want found by what its type is also called, the longest alias winning.
func TestFMByAliasPicksTheLongestAlias(t *testing.T) {
	aliases := map[string][]string{
		"smartgolf_check_reserved": {"スマートゴルフ", "スマートゴルフの予約", "ゴルフ"},
		"smartgolf_list_available": {"スマートゴルフの空き", "ゴルフの空き"},
		"weather":                  {"天気"},
	}
	typeOf := []string{"weather", "smartgolf_check_reserved", "smartgolf_list_available"}

	for said, want := range map[string]int{
		"スマートゴルフ":    1,
		"スマートゴルフの予約": 1,
		"スマートゴルフの空き": 2,
		"ゴルフの空き時間":   2,
		"天気":         0,
	} {
		got := fmByAlias(said, typeOf, aliases)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: got %v, want [%d]", said, got, want)
		}
	}
	if got := fmByAlias("荻窪", typeOf, aliases); len(got) != 0 {
		t.Errorf("no alias: got %v", got)
	}
}
