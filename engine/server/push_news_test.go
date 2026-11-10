package server

import (
	"testing"

	mywant "mywant/engine/core"
)

func TestPushHeldOnlyWhenLayaIsSureItIsRoutine(t *testing.T) {
	cases := []struct {
		name string
		a    decisionAnswer
		held bool
	}{
		{"news", decisionAnswer{Choice: "news", Probabilities: map[string]float64{"news": 0.8}}, false},
		{"sure routine", decisionAnswer{Choice: "routine", Probabilities: map[string]float64{"news": 0.1}}, true},
		{"doubtful routine", decisionAnswer{Choice: "routine", Probabilities: map[string]float64{"news": 0.45}}, false},
		{"routine without probabilities", decisionAnswer{Choice: "routine"}, true},
	}
	for _, c := range cases {
		if got := pushHeld(c.a, 0.3); got != c.held {
			t.Errorf("%s: held %v, want %v", c.name, got, c.held)
		}
	}
}

func TestOutputNewsIsTheTextTheInitiativeReads(t *testing.T) {
	sit := initiativeSituation{Event: initiativeEvent{Kind: initiativeOutput, WantName: "Spotify", Message: "曲が変わりました"}}
	if got, want := initiativeEventText(sit), outputNewsText("Spotify", "曲が変わりました"); got != want {
		t.Errorf("initiative reads %q, the push asks %q", got, want)
	}
}

// Laya's verdict on a want's answer is a processing line in the robot's chat,
// where it fell between what was said — kept apart from its words, so the
// conversation is not crowded out nor the line taken for an answer.
func TestPushDecisionIsAProcessingLineInTheRobotsChat(t *testing.T) {
	s := patchTestServer(t)
	robot, err := s.robotWantNow()
	if err != nil || !isChatWant(robot) {
		t.Skip("no robot type on the test board")
	}
	s.notePushDecision(pushDecision{Want: "Spotify", WantID: "w1", Choice: "routine",
		Probs: map[string]float64{"news": 0.12}, Push: "held", Reason: "routine", By: "laya@test", Ms: 40})
	s.notePushDecision(pushDecision{Want: "Spotify", Push: "sent", Reason: "no-worker"})
	chat, _ := s.robotChat(0)
	var traces []robotChatLine
	for _, l := range chat.Lines {
		if l.Subtype == traceSubtype {
			traces = append(traces, l)
		}
	}
	if len(traces) != 1 { // no-worker: nothing was weighed, no line
		t.Fatalf("traces = %+v", traces)
	}
	want := "laya.push want=Spotify verdict=routine p(news)=0.12 push=held reason=routine by=laya@test ms=40"
	if got := traces[0]; got.Text != want || got.Who != "robot" || got.Seq == 0 || got.Trace["push"] != "held" {
		t.Errorf("trace = %+v, want %q", got, want)
	}
	if _, ok := traces[0].Trace["error"]; ok {
		t.Errorf("an empty value kept: %+v", traces[0].Trace)
	}
	if responses := mywant.GetCurrent(robot, "cc_responses", []any{}); len(responses) > 0 {
		if m, _ := responses[len(responses)-1].(map[string]any); m["subtype"] == traceSubtype {
			t.Errorf("a trace in cc_responses: %+v", m)
		}
	}
}
