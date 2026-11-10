package server

import "testing"

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
