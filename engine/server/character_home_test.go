package server

import (
	"testing"
	"time"

	mywant "mywant/engine/core"
)

// Hero's evenings, as the board kept them: home from the wants, said in each
// type's own words.
func TestCharacterHomeIsThePlaceAskedMost(t *testing.T) {
	ask := func(typ, param, value, kind string) homeAsk { return homeAsk{typ, param, value, kind} }
	evening := [][]homeAsk{
		{ask("transit_search", "from", "国分寺", "station"), ask("transit_search", "to", "中野坂上", "station")},
		{ask("transit_search", "from", "荻窪", "station"), ask("transit_search", "to", "中野坂上", "station")},
		{ask("transit_search", "from", "中野坂上", "station"), ask("transit_search", "to", "荻窪", "station")},
	}
	var h characterHome
	pickHome(&h, evening)
	if h.Home != "中野坂上" || h.Kind != "station" || h.Asks != 3 || h.Of != 3 {
		t.Fatalf("home = %+v", h)
	}

	say := func(typ string, params [][2]string, groups []homeGroup) homeWord {
		w, _ := sayHomeAs(h, typ, params, evening, groups)
		return w
	}
	// Transit: the parameter it is asked with in the evening, `to` (twice) over `from` (once).
	if w := say("transit_search", [][2]string{{"from", "station"}, {"to", "station"}}, nil); w.Param != "to" || w.Value != "中野坂上" {
		t.Errorf("transit = %+v", w)
	}
	// A route never asked: the first parameter that takes a station.
	if w := say("route", [][2]string{{"from", "station"}, {"to", "station"}}, nil); w.Param != "from" || w.Value != "中野坂上" {
		t.Errorf("route = %+v", w)
	}
	// Weather takes a city: through the constellation holding both.
	groups := []homeGroup{
		{name: "スマートゴルフ", things: [][2]string{{"places", "北新宿店"}}},
		{name: "中野", things: [][2]string{{"stations", "中野坂上"}, {"cities", "nakano"}}},
	}
	if w := say("weather", [][2]string{{"at", "city"}}, groups); w.Param != "at" || w.Value != "nakano" || w.Through != "中野" {
		t.Errorf("weather = %+v", w)
	}
	// No constellation to say it through: no home for weather — never a guess.
	if w, ok := sayHomeAs(h, "weather", [][2]string{{"at", "city"}}, evening, groups[:1]); ok {
		t.Errorf("weather without a constellation = %+v", w)
	}
}

// A tie, or a place asked once, is no habit.
func TestCharacterHomeNeedsAHabit(t *testing.T) {
	for name, perWant := range map[string][][]homeAsk{
		"tie":  {{{"t", "from", "中野坂上", "station"}, {"t", "to", "国分寺", "station"}}, {{"t", "from", "中野坂上", "station"}, {"t", "to", "国分寺", "station"}}},
		"once": {{{"t", "to", "中野坂上", "station"}}},
	} {
		var h characterHome
		pickHome(&h, perWant)
		if h.Home != "" {
			t.Errorf("%s: home = %+v", name, h)
		}
	}
}

// The robot's deploy is the ask of whoever is at the controls: theirs, so
// their habits are learned from it — never the robot's, and never over an
// owner already stamped.
func TestRobotDeployIsAskedByWhoeverIsAtTheControls(t *testing.T) {
	clearCursors(t)
	t.Cleanup(func() { clearCursors(t) })
	putCursor("chr-me", cursorEntry{X: 3, Y: 4, LastSeen: 1000})
	putCursor(robotCharacterID, cursorEntry{X: 9, Y: 9, LastSeen: 2000})

	run := func(owner string) string {
		w := &mywant.Want{Metadata: mywant.Metadata{Name: "r", Labels: map[string]string{canvasLabelNear: canvasNearCursor}}}
		if owner != "" {
			w.SetLabel(wantLabelOwnerCharacter, owner)
		}
		if err := (&CanvasNearHook{}).Run(w, nil, nil); err != nil {
			t.Fatal(err)
		}
		return w.GetLabel(wantLabelOwnerCharacter)
	}
	if got := run(""); got != "chr-me" {
		t.Errorf("owner = %q, want chr-me", got)
	}
	if got := run("chr-other"); got != "chr-other" {
		t.Errorf("stamped owner overwritten: %q", got)
	}
}

// Laya reads the time coarsely, and a part's home is asked inside that part.
func TestOfferNowAndDayPartAt(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	at := time.Date(2026, 11, 18, 21, 7, 0, 0, jst)
	if got := offerNow(at); got != "今: 夜 21時。" {
		t.Errorf("offerNow = %q", got)
	}
	for _, p := range dayParts {
		if got := dayPart(dayPartAt(p.Name, at)); got != p.Name {
			t.Errorf("dayPartAt(%s) is in %s", p.Name, got)
		}
	}
}

// Where the person is, as Laya reads it: the nearest places by name.
func TestOfferWhere(t *testing.T) {
	tiles := []robotTile{
		{name: "国分寺", cardKind: "thing", x: 5, y: 5},
		{name: "中野坂上", cardKind: "thing", x: 6, y: 5},
		{name: "荻窪", cardKind: "thing", x: 7, y: 7},
		{name: "transit-search-instance", cardKind: "want", x: 5, y: 5},
		{name: "銀座", cardKind: "thing", x: 20, y: 20},
	}
	for _, c := range []struct {
		x, y int
		ok   bool
		want string
	}{
		{5, 5, true, "本人の近く: 国分寺、中野坂上。"},
		{8, 8, true, "本人の近く: 荻窪、中野坂上。"},
		{40, 40, true, "本人の近く: なし。"},
		{5, 5, false, "本人の近く: なし。"},
	} {
		if got := offerWhere(c.x, c.y, c.ok, tiles); got != c.want {
			t.Errorf("(%d,%d,%v) = %q, want %q", c.x, c.y, c.ok, got, c.want)
		}
	}
}
