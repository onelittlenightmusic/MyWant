package types

import "testing"

// What a yes is, and what only looks like one.
//
// The case that matters is the last group: a message that starts with a yes and
// then asks for something else. The agent's own gate claimed to exclude it and
// did not — it took any short message beginning with a yes, and
// "はい、でも先に天気を見せて" is thirteen characters — so a deletion could be
// confirmed by a sentence that was about the weather.
func TestIsOnlyConsent(t *testing.T) {
	yes := []string{
		"はい", "はい。", "はい！", "うん", "ええ", "そう", "そうして", "どうぞ", "いいよ",
		"お願い", "おねがい", "お願いします", "はいお願いします", "やって", "やってください",
		"実行", "実行して", "消して", "削除して",
		"ok", "okay", "yes", "y", "sure", "go ahead", "do it", "please",
		"はい、お願いします", "yes, please",
	}
	for _, said := range yes {
		if !isOnlyConsent(said) {
			t.Errorf("isOnlyConsent(%q) = false, want true", said)
		}
	}

	no := []string{
		"",
		"はい、でも先に天気を見せて",
		"はいでも天気を見せて",
		"はい、そのあとで新宿に行って",
		"yes but show me the weather first",
		"新宿はどこ",
		"天気を見せて",
		"いいえ",
		"だめ",
	}
	for _, said := range no {
		if isOnlyConsent(said) {
			t.Errorf("isOnlyConsent(%q) = true, want false", said)
		}
	}
}

// The model writes the command it chose into its arguments as well, and the
// CLI does not mind — which is how `mywant wants list wants list` ran for a
// while without anyone noticing.
func TestFMDropEcho(t *testing.T) {
	cases := []struct {
		command string
		args    []string
		want    []string
	}{
		{"wants list", []string{"wants list"}, nil},
		{"wants list", []string{"wants", "list"}, nil},
		{"wants get", []string{"wants", "get", "Nakano"}, []string{"Nakano"}},
		{"wants get", []string{"Nakano"}, []string{"Nakano"}},
		{"thing pin", []string{"新宿", "5", "0"}, []string{"新宿", "5", "0"}},
		{"board", nil, nil},
		{"wants get", []string{"wants"}, []string{"wants"}},
	}
	for _, c := range cases {
		got := fmDropEcho(c.command, c.args)
		if len(got) != len(c.want) {
			t.Errorf("fmDropEcho(%q, %q) = %q, want %q", c.command, c.args, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("fmDropEcho(%q, %q) = %q, want %q", c.command, c.args, got, c.want)
				break
			}
		}
	}
}

// A sentence waiting for a yes has to come apart again into something
// runnable — including a deletion, which is what is usually waiting.
func TestSplitPendingCommand(t *testing.T) {
	if catalogue, err := fmEveryCommand(); err != nil || len(catalogue) == 0 {
		t.Skip("no mywant CLI here to read a command list from")
	}
	cases := []struct{ sentence, command, args string }{
		{"mywant wants delete delete-me-test", "wants delete", "delete-me-test"},
		{"mywant wants create --type weather --param at=Nakano", "wants create", "--type weather --param at=Nakano"},
		{"mywant thing pin 新宿 5 0", "thing pin", "新宿 5 0"},
		{"mywant undo", "undo", ""},
	}
	for _, c := range cases {
		command, args := splitPendingCommand(c.sentence)
		if command != c.command || args != c.args {
			t.Errorf("splitPendingCommand(%q) = %q, %q; want %q, %q",
				c.sentence, command, args, c.command, c.args)
		}
	}
}
