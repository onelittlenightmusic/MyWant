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

func TestFMFirstSentence(t *testing.T) {
	for in, want := range map[string]string{
		"Store name of the next reservation (e.g. 北新宿店). More text.": "Store name of the next reservation (e.g. 北新宿店).",
		"Checks a thing by\nscraping a page. Completes once.":        "Checks a thing by scraping a page.",
		"一行の説明。続き":                                                   "一行の説明。",
		"":                                                           "",
	} {
		if got := fmFirstSentence(in, 200); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// The whole want goes to the model, but never a value whose name is a secret.
func TestFMDropSecrets(t *testing.T) {
	v := map[string]any{
		"state": map[string]any{"current": map[string]any{"next_store": "北新宿店", "api_token": "x"}},
		"defs":  []any{map[string]any{"name": "webhook_secret", "description": "d"}, map[string]any{"name": "next_room"}},
	}
	fmDropSecrets(v)
	cur := v["state"].(map[string]any)["current"].(map[string]any)
	if _, ok := cur["api_token"]; ok || cur["next_store"] != "北新宿店" {
		t.Errorf("current: %v", cur)
	}
	defs := v["defs"].([]any)
	if d := defs[0].(map[string]any); d["description"] != nil {
		t.Errorf("secret definition kept: %v", d)
	}
	if d := defs[1].(map[string]any); d["name"] != "next_room" {
		t.Errorf("plain definition lost: %v", d)
	}
}
