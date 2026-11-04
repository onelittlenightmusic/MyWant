package server

import (
	"strings"
	"testing"
	"time"
)

func earlierTestWant(id, name, typ string, params map[string]any) robotWant {
	var w robotWant
	w.Metadata.ID, w.Metadata.Name, w.Metadata.Type = id, name, typ
	w.Spec.Params = params
	return w
}

// 「乗り換えを再検索して」 after 「中野坂上から大森の乗り換え」: the brief carries
// that turn and its want, first among the candidates, so the model can choose
// it (existing, restart) — no action of its own, no words read here.
func TestEarlierInTheBrief(t *testing.T) {
	now := time.Now()
	at := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	route := earlierTestWant("want-2131", "transit_search-2131", "transit_search", map[string]any{"from": "中野坂上", "to": "大森"})
	wants := []robotWant{
		earlierTestWant("want-2118", "transit_search-2118", "transit_search", map[string]any{"from": "中野坂上", "to": "銀座"}),
		route,
		earlierTestWant("want-9", "Nakanoの天気", "weather", nil),
		earlierTestWant("want-robot", robotWantName, "robot", nil),
	}
	card := func(id string) *robotCard { return &robotCard{Kind: "want", ID: id} }
	turns := []robotTurn{
		{ID: "turn-0", Question: "昨日の乗り換え", StartedAt: at(3 * time.Hour), Steps: []robotStep{{Card: card("want-2118")}}},
		{ID: "turn-1", Question: "中野の天気は？", StartedAt: at(10 * time.Minute), Cards: []robotCard{{Kind: "want", ID: "want-9"}}},
		{ID: "turn-2", Question: "こんにちは", StartedAt: at(8 * time.Minute)},
		{ID: "turn-3", Question: "中野坂上から大森の乗り換え", StartedAt: at(2 * time.Minute), Steps: []robotStep{{Card: card("want-2131")}, {Card: card("want-robot")}}},
		{ID: "turn-4", Question: "乗り換えを再検索して", StartedAt: at(0)},
	}

	earlier := earlierTurns(turns, "turn-4", "乗り換えを再検索して", wants, now)
	if len(earlier) != 2 || earlier[0].question != "中野坂上から大森の乗り換え" || earlier[1].question != "中野の天気は？" {
		t.Fatalf("the last two turns that showed a want, newest first: %+v", earlier)
	}
	if len(earlier[0].wants) != 1 || earlier[0].wants[0].Metadata.Name != "transit_search-2131" {
		t.Errorf("its want, not the robot: %+v", earlier[0].wants)
	}
	// The client naming no turn: the newest, asking the same, is the one answered.
	if got := earlierTurns(turns, "", "乗り換えを再検索して", wants, now); len(got) != 2 || got[0].question != "中野坂上から大森の乗り換え" {
		t.Errorf("no turn named: %+v", got)
	}
	// Nothing older than the window; a want since deleted is not carried.
	if got := earlierTurns(turns[:1], "", "x", wants, now); len(got) != 0 {
		t.Errorf("three hours ago is another conversation: %+v", got)
	}
	if got := earlierTurns(turns, "turn-4", "q", wants[2:], now); len(got) != 1 || got[0].question != "中野の天気は？" {
		t.Errorf("the route deleted: %+v", got)
	}

	text := earlierText(earlier)
	for _, want := range []string{"Said earlier in this conversation", "「中野坂上から大森の乗り換え」 → transit_search-2131 (", "大森", "「中野の天気は？」 → Nakanoの天気"} {
		if !strings.Contains(text, want) {
			t.Errorf("brief lacks %q:\n%s", want, text)
		}
	}
	if earlierText(nil) != "" {
		t.Error("nothing earlier, nothing said")
	}

	// First among the candidates: the one already there moved up, the other added.
	cands := []planCandidate{{name: "transit_search-2118", line: "- transit_search-2118: ...", want: true}, {name: "Nakanoの天気", line: "- Nakanoの天気: kept line", want: true}}
	got := withEarlierFirst(cands, earlier)
	if len(got) != 3 || got[0].name != "transit_search-2131" || got[1].name != "Nakanoの天気" || got[1].line != "- Nakanoの天気: kept line" || got[2].name != "transit_search-2118" {
		t.Errorf("candidates: %+v", got)
	}
	if !strings.Contains(got[0].line, "want transit_search (") {
		t.Errorf("an added one's line: %q", got[0].line)
	}
}
