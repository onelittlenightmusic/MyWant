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

// 「乗り換えを再検索して」 after 「中野坂上から大森の乗り換え」, which showed the
// route to 銀座 by mistake: the brief carries that question, the want only
// faintly (named, without its parameters, not first among the candidates), so
// the model decides again from what was asked.
func TestEarlierInTheBrief(t *testing.T) {
	now := time.Now()
	at := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	wrong := earlierTestWant("want-2118", "transit_search-2118", "transit_search", map[string]any{"from": "中野坂上", "to": "銀座"})
	wants := []robotWant{
		wrong,
		earlierTestWant("want-9", "Nakanoの天気", "weather", map[string]any{"location": "中野"}),
		earlierTestWant("want-robot", robotWantName, "robot", nil),
	}
	card := func(id string) *robotCard { return &robotCard{Kind: "want", ID: id} }
	turns := []robotTurn{
		{ID: "turn-0", Question: "昨日の乗り換え", StartedAt: at(3 * time.Hour), Steps: []robotStep{{Card: card("want-2118")}}},
		{ID: "turn-1", Question: "中野の天気は？", StartedAt: at(10 * time.Minute), Cards: []robotCard{{Kind: "want", ID: "want-9"}}},
		{ID: "turn-2", Question: "こんにちは", StartedAt: at(8 * time.Minute)},
		{ID: "turn-3", Question: "中野坂上から大森の乗り換え", StartedAt: at(2 * time.Minute), Steps: []robotStep{{Card: card("want-2118")}, {Card: card("want-robot")}}},
		{ID: "turn-4", Question: "乗り換えを再検索して", StartedAt: at(0)},
	}

	// The last two questions, newest first, whether or not they showed a want.
	earlier := earlierTurns(turns, "turn-4", "乗り換えを再検索して", wants, now)
	if len(earlier) != 2 || earlier[0].question != "中野坂上から大森の乗り換え" || earlier[1].question != "こんにちは" {
		t.Fatalf("the last two questions: %+v", earlier)
	}
	if len(earlier[0].wants) != 1 || earlier[0].wants[0].Metadata.Name != "transit_search-2118" || len(earlier[1].wants) != 0 {
		t.Errorf("the wants shown, not the robot: %+v", earlier)
	}
	// The client naming no turn: the newest, asking the same, is the one answered.
	if got := earlierTurns(turns, "", "乗り換えを再検索して", wants, now); len(got) != 2 || got[0].question != "中野坂上から大森の乗り換え" {
		t.Errorf("no turn named: %+v", got)
	}
	// Nothing older than the window; a want since deleted leaves the question.
	if got := earlierTurns(turns[:1], "", "x", wants, now); len(got) != 0 {
		t.Errorf("three hours ago is another conversation: %+v", got)
	}
	if got := earlierTurns(turns, "turn-4", "q", nil, now); len(got) != 2 || got[0].question != "中野坂上から大森の乗り換え" || len(got[0].wants) != 0 {
		t.Errorf("the route deleted, its question kept: %+v", got)
	}

	text := earlierText(earlier)
	for _, want := range []string{
		"Asked earlier in this conversation",
		"- 「中野坂上から大森の乗り換え」 (shown then, maybe not right: transit_search-2118)",
		"- 「こんにちは」\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("brief lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "銀座") {
		t.Errorf("the want's parameters are not carried:\n%s", text)
	}
	if earlierText(nil) != "" {
		t.Error("nothing earlier, nothing said")
	}

	// Among the candidates, after them, only when not there already.
	cands := []planCandidate{{name: "Nakanoの天気", line: "- Nakanoの天気: kept line", want: true}}
	got := withEarlierWants(cands, earlierTurns(turns, "turn-4", "", wants, now.Add(0)))
	if len(got) != 2 || got[0].name != "Nakanoの天気" || got[1].name != "transit_search-2118" {
		t.Errorf("candidates: %+v", got)
	}
	if strings.Contains(got[1].line, "銀座") {
		t.Errorf("an earlier want's line, without its parameters: %q", got[1].line)
	}
	if got := withEarlierWants([]planCandidate{{name: "transit_search-2118", line: "kept"}}, earlier); len(got) != 1 || got[0].line != "kept" {
		t.Errorf("one already there is left as it is: %+v", got)
	}
}

// 「中野坂上から戸塚への乗り換え」 calls transit_search by its alias: the thing
// 戸塚 is where the route goes, not a candidate answer. 「戸塚はどこ？」 calls no
// type, and the thing stays one.
func TestThingIsAValueWhenATypeIsCalled(t *testing.T) {
	aliases := map[string][]string{"transit_search": {"乗り換え", "経路"}, "checklist": {"やること"}}
	if !robotTypeCalled("中野坂上から戸塚への乗り換え", aliases) {
		t.Error("乗り換え calls transit_search")
	}
	if robotTypeCalled("戸塚はどこ？", aliases) {
		t.Error("no type is called")
	}
	if !thingIsAValue("thing", true) {
		t.Error("a thing, a type called: a value")
	}
	if thingIsAValue("want", true) || thingIsAValue("thing", false) {
		t.Error("a want stays a candidate; a thing stays one when no type is called")
	}
}
