package server

import (
	"strings"
	"testing"
	"time"

	mywant "mywant/engine/core"
)

var ctxKinds = []string{"place.destination", "place.origin", "place.area", "time.date", "time.at", "time.arrive_by",
	"time.duration", "people.party_size", "interest.topic", "interest.subject", "preference.mode", "preference.budget", "preference.cuisine"}

func TestUserContextRead(t *testing.T) {
	now := time.Date(2026, 11, 25, 9, 0, 0, 0, time.Local)
	list := []userRequest{
		{Said: "中野坂上から大森"},
		{Said: "国分寺駅まで10時に着きたい", Type: "transit_search", Params: map[string]string{"to": "国分寺", "from": "中野坂上"}},
		{Said: "明日19時に4人でラーメン、予算3000円"},
		{Said: "阪神の試合は？歩いて行く"},
	}
	got := userContextRead(strings.Join([]string{
		"place.destination=国分寺駅", "place.origin: 中野坂上", "time.arrive_by=10:00", "interest.topic=移動",
		"time.date=2026-11-26", "time.at=19:00", "people.party_size=4", "preference.cuisine=ラーメン",
		"preference.budget=3000", "interest.subject=阪神", "preference.mode=walking", "note=x",
	}, "\n"), list, ctxKinds, nil, now)
	want := map[string]string{
		"place.destination": "国分寺", "place.origin": "中野坂上", "time.arrive_by": "10:00", "interest.topic": "移動",
		"time.date": "2026-11-26", "time.at": "19:00", "people.party_size": "4", "preference.cuisine": "ラーメン",
		"preference.budget": "3000", "interest.subject": "阪神", "preference.mode": "walking",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["note"]; ok {
		t.Error("a kind not asked was kept")
	}
	// Made up: a place nobody said, a time or day or count not said, a mode
	// not said, a sentence as the place.
	board := func(v string) bool { return v == "大森" }
	got = userContextRead(strings.Join([]string{
		"place.destination=横浜", "time.at=22:00", "time.date=2026-11-27", "people.party_size=6",
		"preference.mode=driving", "place.area=10時までに着くには？", "place.origin=大森",
	}, "\n"), list, ctxKinds, board, now)
	if len(got) != 1 || got["place.origin"] != "大森" {
		t.Errorf("got %v, want only place.origin=大森", got)
	}
}

func TestUserContextReadJSON(t *testing.T) {
	list := []userRequest{{Said: "中野坂上から国分寺", Type: "transit_search", Params: map[string]string{"from": "中野坂上", "to": "国分寺"}}, {Said: "4人で"}}
	got := userContextRead("```json\n{\"place.destination\": \"国分寺\", \"people.party_size\": 4, \"time.at\": \"\", \"place.origin\": \"渋谷\"}\n```", list, ctxKinds, nil, time.Now())
	if len(got) != 2 || got["place.destination"] != "国分寺" || got["people.party_size"] != "4" {
		t.Errorf("got %v", got)
	}
}

func TestMadeContext(t *testing.T) {
	kindOf := func(typ, p string) string {
		return map[string]string{"transit_search.to": "place.destination", "transit_search.from": "place.origin"}[typ+"."+p]
	}
	got := madeContext([]userRequest{
		{Said: "国分寺まで", Type: "transit_search", Params: map[string]string{"from": "中野坂上", "to": "国分寺"}},
		{Said: "荻窪から吉祥寺", Type: "transit_search", Params: map[string]string{"from": "荻窪", "to": "吉祥寺"}},
		{Said: "スマートゴルフの予約は？"},
	}, kindOf)
	if got["place.destination"] != "吉祥寺" || got["place.origin"] != "荻窪" {
		t.Errorf("got %v", got)
	}
}

func TestUserContextPrompt(t *testing.T) {
	kindOf := func(typ, p string) string {
		if typ == "transit_search" && p == "to" {
			return "place.destination"
		}
		return ""
	}
	p := userContextPrompt([]userRequest{{Said: "国分寺まで", Type: "transit_search", Params: map[string]string{"to": "国分寺"}}},
		[]string{"place.destination", "interest.topic"}, kindOf)
	for _, w := range []string{"「国分寺まで」", "to=国分寺 [place.destination]", "place.destination: the place they are going to", `"place.destination": ""`, `"interest.topic": ""`} {
		if !strings.Contains(p, w) {
			t.Errorf("prompt lacks %q:\n%s", w, p)
		}
	}
	if strings.Contains(p, "time.date") {
		t.Error("a kind not asked is in the prompt")
	}
}

// withGlobalState runs f with a global state of its own (none on disk).
func withGlobalState(t *testing.T, f func()) {
	t.Helper()
	before := mywant.GetGlobalChainBuilder()
	mywant.SetGlobalChainBuilder(mywant.NewChainBuilder([]*mywant.Want{}))
	defer mywant.SetGlobalChainBuilder(before)
	f()
}

func TestUserContextGlobalState(t *testing.T) {
	withGlobalState(t, func() {
		storeUserContext("ctx_probe", userContext{Fields: map[string]string{"place.destination": "国分寺", "time.at": "19:00"}, UpdatedAt: time.Now()})
		if v, _ := mywant.GetGlobalState("ctx_probe.place.destination"); v != "国分寺" {
			t.Errorf("not kept as <name>.<kind>: %v", mywant.GetAllGlobalState())
		}
		// Read again, a kind no longer said goes.
		storeUserContext("ctx_probe", userContext{Fields: map[string]string{"place.destination": "大森"}, UpdatedAt: time.Now()})
		if _, ok := mywant.GetGlobalState("ctx_probe.time.at"); ok {
			t.Error("a kind not read again was kept")
		}
		// Set by anyone: read as it is.
		mywant.StoreGlobalState("ctx_probe.interest.subject", "阪神")
		c, ok := userContextNow("ctx_probe")
		if !ok || c.Fields["place.destination"] != "大森" || c.Fields["interest.subject"] != "阪神" {
			t.Errorf("read %v %v", c.Fields, ok)
		}
		// Old: not now's.
		mywant.StoreGlobalState("ctx_probe.context_at", time.Now().Add(-2*time.Hour).Format(time.RFC3339))
		if _, ok := userContextNow("ctx_probe"); ok {
			t.Error("an old context was taken as now's")
		}
	})
}

func TestRobotContextParams(t *testing.T) {
	withGlobalState(t, func() {
		storeUserContext("ctx_probe", userContext{Fields: map[string]string{"place.destination": "国分寺", "place.origin": "中野坂上"}, UpdatedAt: time.Now()})
		s := &Server{}
		of := map[string]string{"to": "place.destination", "from": "place.origin"}
		// Said wins; the here-param is never filled; the rest from the context.
		params := map[string]any{"to": "大森"}
		s.robotContextParams(of, "from", params, "ctx_probe", nil)
		if params["to"] != "大森" || params["from"] != nil {
			t.Errorf("said or here changed: %v", params)
		}
		params = map[string]any{}
		if filled := s.robotContextParams(of, "", params, "ctx_probe", nil); params["to"] != "国分寺" || params["from"] != "中野坂上" || filled == "" {
			t.Errorf("not filled: %v %q", params, filled)
		}
		// Not one of the parameter's values: left.
		params = map[string]any{}
		s.robotContextParams(of, "", params, "ctx_probe", map[string][]any{"to": {"品川", "大崎"}})
		if params["to"] != nil {
			t.Errorf("filled outside its enum: %v", params)
		}
		params = map[string]any{}
		s.robotContextParams(of, "", params, "nobody_here", nil)
		if len(params) != 0 {
			t.Errorf("filled for nobody: %v", params)
		}
	})
}
