package server

import (
	"testing"

	ws "github.com/onelittlenightmusic/want-spec"
)

func previousTestWant(id, name, typ string) robotWant {
	var w robotWant
	w.Metadata.ID, w.Metadata.Name, w.Metadata.Type = id, name, typ
	return w
}

// 「中野坂上から大森の乗り換え」 made transit_search-2131; 「乗り換えを再検索して」,
// planned as previous transit_search, is about it — the latest of the type,
// not the older route to 銀座, nor the weather shown since.
func TestPreviousTarget(t *testing.T) {
	wants := []robotWant{
		previousTestWant("want-2118", "transit_search-2118", "transit_search"),
		previousTestWant("want-2131", "transit_search-2131", "transit_search"),
		previousTestWant("want-9", "Nakanoの天気", "weather"),
		previousTestWant("want-robot", robotWantName, "robot"),
	}
	card := func(id, name string) *robotCard { return &robotCard{Kind: "want", ID: id, Name: name} }
	turns := []robotTurn{
		{ID: "turn-1", Question: "中野坂上から銀座の乗り換え", Steps: []robotStep{{Tool: "point", Card: card("want-2118", "transit_search-2118")}}},
		{ID: "turn-2", Question: "中野坂上から大森の乗り換え", Steps: []robotStep{{Tool: "deploy_want", Card: card("want-2131", "transit_search-2131")}}},
		{ID: "turn-3", Question: "中野の天気は？", Cards: []robotCard{{Kind: "want", ID: "want-9", Name: "Nakanoの天気"}}},
		{ID: "turn-4", Question: "乗り換えを再検索して"},
	}

	got, ok := previousTarget(turns, "turn-4", "transit_search", wants)
	if !ok || got != (ws.ActResolved{Turn: "turn-2", Target: "transit_search-2131", Type: "transit_search"}) {
		t.Errorf("the latest route: %+v %v", got, ok)
	}
	// No type said: the latest want shown, whatever it is.
	if got, _ := previousTarget(turns, "turn-4", "", wants); got.Target != "Nakanoの天気" {
		t.Errorf("the latest want: %+v", got)
	}
	// A want since deleted is passed over for the one before it.
	if got, _ := previousTarget(turns, "turn-4", "transit_search", wants[:1]); got.Target != "transit_search-2118" {
		t.Errorf("the one still there: %+v", got)
	}
	// Nothing of the type, or no earlier turns: nothing found.
	if _, ok := previousTarget(turns, "turn-4", "golf", wants); ok {
		t.Error("no golf was shown")
	}
	if _, ok := previousTarget(turns[3:], "turn-4", "", wants); ok {
		t.Error("only the turn itself")
	}
	// The turn asking is never its own earlier turn, and the robot never the want.
	self := []robotTurn{{ID: "turn-5", Cards: []robotCard{{Kind: "want", ID: "want-2131"}}},
		{ID: "turn-6", Cards: []robotCard{{Kind: "want", ID: "want-robot", Name: robotWantName}}}}
	if got, _ := previousTarget(self, "turn-5", "", wants); got.Target != "" {
		t.Errorf("neither itself nor the robot: %+v", got)
	}
}

// A previous plan is a plan: kept as posted, its type what it is about.
func TestPreviousPlanAccepted(t *testing.T) {
	p := ws.Plan{Spec: ws.PlanSpec{Question: "乗り換えを再検索して", Action: ws.PlanActionPrevious, Type: "transit_search", Capability: "restart"}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}
