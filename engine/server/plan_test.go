package server

import (
	"strings"
	"testing"
)

func TestFMNamePartSaid(t *testing.T) {
	if !robotNamePartSaid("NakanoのWeather", robotLoose("Nakanoの天気は？")) {
		t.Error("Nakano of NakanoのWeather was said")
	}
	if robotNamePartSaid("新宿", robotLoose("新宿から国分寺")) {
		t.Error("a name of one part is matched whole, not by its parts")
	}
	if robotNamePartSaid("transit-search-instance", robotLoose("新宿から国分寺の乗り換え")) {
		t.Error("nothing of it was said")
	}
}

func TestFMAnswerPrompt(t *testing.T) {
	none := robotAnswerPrompt(actRequest{Question: "こんにちは", Kind: "none"}, nil)
	if !strings.Contains(none, "Reply to what they said") || strings.Contains(none, "could not") {
		t.Errorf("a greeting: %q", none)
	}
	made := robotAnswerPrompt(actRequest{Question: "新宿から国分寺", Kind: "new"}, []string{"[deploy_want] Deployed \"新宿→国分寺\""})
	if !strings.Contains(made, "新宿→国分寺") || !strings.Contains(made, "from these lines only") {
		t.Errorf("made: %q", made)
	}
}

func TestFMPlanHint(t *testing.T) {
	if got := planHint("deploy_want transit_search at once"); got != "make transit_search at once" {
		t.Errorf("got %q", got)
	}
}

func TestFMSaidIn(t *testing.T) {
	q := "新宿を10時に出て横浜へ"
	for _, v := range []string{"新宿", "横浜", "10:00"} {
		if !robotSaidIn(v, q) {
			t.Errorf("%s was said in %s", v, q)
		}
	}
	for _, v := range []string{"中野坂上", "09:40", "出発"} {
		if robotSaidIn(v, "こんにちは") {
			t.Errorf("%s was not said", v)
		}
	}
}
