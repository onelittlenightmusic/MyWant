package server

import (
	"strings"
	"testing"
)

func TestUserContextRead(t *testing.T) {
	list := []userRequest{
		{Said: "中野坂上から大森"},
		{Said: "国分寺駅まで10時に着きたい", Type: "transit_search", Params: map[string]string{"to": "国分寺", "from": "中野坂上"}},
	}
	got := userContextRead("destination=国分寺駅\norigin: 中野坂上\narrive_by=10:00\ntopic=移動\nnote=x", list, nil)
	want := map[string]string{"destination": "国分寺", "origin": "中野坂上", "arrive_by": "10:00", "topic": "移動"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all %v)", k, got[k], v, got)
		}
	}
	// A place nobody said, a time not a time: dropped.
	got = userContextRead("destination=横浜\narrive_by=朝\n", list, nil)
	if len(got) != 0 {
		t.Errorf("made-up values kept: %v", got)
	}
	// A request written as the place (the model did, 2026-11-25): dropped;
	// a thing on the board the person said: kept.
	board := func(v string) bool { return v == "大森" }
	got = userContextRead("destination=10時までに着くには？\norigin=大森", list, board)
	if got["destination"] != "" || got["origin"] != "大森" {
		t.Errorf("got %v", got)
	}
	got = userContextRead("destination=横浜\narrive_by=朝\n", list, nil)
	if len(got) != 0 {
		t.Errorf("made-up values kept: %v", got)
	}
}

func TestUserContextPrompt(t *testing.T) {
	p := userContextPrompt([]userRequest{{Said: "国分寺まで", Type: "transit_search", Params: map[string]string{"to": "国分寺"}}})
	for _, w := range []string{"「国分寺まで」", "transit_search to=国分寺", "destination=\n", "topic=\n"} {
		if !strings.Contains(p, w) {
			t.Errorf("prompt lacks %q:\n%s", w, p)
		}
	}
}
