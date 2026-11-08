package server

import (
	"strings"
	"testing"
	"time"
)

func initiativeOffered(sit initiativeSituation) []initiativeCapability {
	var offered []initiativeCapability
	for _, c := range robotInitiatives {
		if c.Offers(sit) {
			offered = append(offered, c)
		}
	}
	return offered
}

func initiativeNames(cs []initiativeCapability) string {
	var names []string
	for _, c := range cs {
		names = append(names, c.Name)
	}
	return strings.Join(names, ",")
}

func TestInitiativeOffers(t *testing.T) {
	done := initiativeSituation{
		Event:   initiativeEvent{Kind: initiativeOutput, WantName: "中野坂上→銀座", Message: "12:24着"},
		HasTile: true, HasPerson: true, Distance: 5,
	}
	if got := initiativeNames(initiativeOffered(done)); got != "report,return,stay" {
		t.Errorf("a new answer far from the person offers %q", got)
	}
	idleNear := initiativeSituation{Event: initiativeEvent{Kind: initiativeIdle}, HasPerson: true, Distance: 1}
	if got := initiativeNames(initiativeOffered(idleNear)); got != "stay" {
		t.Errorf("a quiet beside the person offers %q", got)
	}
	// A want with no tile (archived, not drawn) has nowhere to report at.
	noTile := done
	noTile.HasTile = false
	if got := initiativeNames(initiativeOffered(noTile)); got != "return,stay" {
		t.Errorf("a new answer from a want off the board offers %q", got)
	}
}

func TestInitiativeChooseKeepsQuiet(t *testing.T) {
	offered := initiativeOffered(initiativeSituation{
		Event:   initiativeEvent{Kind: initiativeOutput, Message: "done"},
		HasTile: true, HasPerson: true, Distance: 5,
	})
	talk := map[string]decisionAnswer{
		"interrupt": {Choice: "talk"},
		"action":    {Choice: "report", Probabilities: map[string]float64{"report": 0.8, "return": 0.05, "stay": 0.15}},
	}
	if got := initiativeChoose(offered, talk).Name; got != "report" {
		t.Errorf("talk + report did %q", got)
	}
	// Wait: report speaks, so the likeliest quiet one instead — stay over return here.
	wait := talk
	wait["interrupt"] = decisionAnswer{Choice: "wait"}
	if got := initiativeChoose(offered, wait).Name; got != "stay" {
		t.Errorf("wait + report did %q, want the likeliest quiet one (stay)", got)
	}
	// An action not on offer (the model's own word), told to wait, falls to a quiet one.
	odd := map[string]decisionAnswer{"interrupt": {Choice: "wait"}, "action": {Choice: "dance"}}
	if got := initiativeChoose(offered, odd); got.Speaks {
		t.Errorf("an unknown action did %q, which speaks", got.Name)
	}
}

func TestInitiativeChoosePassesOverWhatIsNotOffered(t *testing.T) {
	// Beside the person, a quiet: only stay is on offer, though the head
	// likes return.
	offered := initiativeOffered(initiativeSituation{Event: initiativeEvent{Kind: initiativeIdle}, HasPerson: true, Distance: 1})
	answers := map[string]decisionAnswer{
		"interrupt": {Choice: "wait"},
		"action":    {Choice: "return", Probabilities: map[string]float64{"report": 0.1, "return": 0.7, "stay": 0.2}},
	}
	if got := initiativeChoose(offered, answers).Name; got != "stay" {
		t.Errorf("return not on offer, did %q", got)
	}
}

func TestInitiativeRules(t *testing.T) {
	sit := initiativeSituation{
		Event:   initiativeEvent{Kind: initiativeOutput, Message: "done"},
		HasTile: true, HasPerson: true, Distance: 5, SinceSpoke: -1,
	}
	offered := initiativeOffered(sit)
	if got := initiativeChoose(offered, initiativeRules(sit, offered)).Name; got != "report" {
		t.Errorf("rules on a new answer did %q", got)
	}
	sit.SinceSpoke = time.Minute
	if got := initiativeChoose(offered, initiativeRules(sit, offered)).Name; got != "return" {
		t.Errorf("rules on a new answer a minute after a remark did %q", got)
	}
}

func TestInitiativeStateIsShort(t *testing.T) {
	sit := initiativeSituation{
		Event: initiativeEvent{Kind: initiativeOutput, WantName: "中野坂上→銀座",
			Message: strings.Repeat("とても長い結果の説明", 20)},
		SinceSpokenTo: 40 * time.Second, SinceSpoke: -1, Distance: 3,
	}
	state := initiativeState(sit)
	if n := len([]rune(state)); n > 140 {
		t.Errorf("state is %d runes: %s", n, state)
	}
	for _, want := range []string{"中野坂上→銀座", "40秒", "なし", "3マス"} {
		if !strings.Contains(state, want) {
			t.Errorf("state %q lacks %q", state, want)
		}
	}
}

func TestInitiativeInterruptWeighsTheTimeInCode(t *testing.T) {
	for _, c := range []struct {
		news  bool
		spoke time.Duration
		want  string
	}{
		{true, -1, "talk"},               // never spoken
		{true, 10 * time.Minute, "talk"}, // long ago
		{true, time.Minute, "wait"},      // just now
		{false, -1, "wait"},              // routine
	} {
		if got := initiativeInterrupt(c.news, initiativeSituation{SinceSpoke: c.spoke}); got != c.want {
			t.Errorf("news=%v spoke=%s: %s, want %s", c.news, c.spoke, got, c.want)
		}
	}
}

func TestInitiativeEventTextHasNoNumbersOfTheSituation(t *testing.T) {
	sit := initiativeSituation{Event: initiativeEvent{Kind: initiativeOutput, WantName: "g", Message: "空き"},
		SinceSpokenTo: 40 * time.Second, SinceSpoke: time.Minute, Distance: 7}
	text := initiativeEventText(sit)
	for _, n := range []string{"40秒", "1分", "7マス"} {
		if strings.Contains(text, n) {
			t.Errorf("%q carries %q", text, n)
		}
	}
	if initiativeEventText(initiativeSituation{Event: initiativeEvent{Kind: initiativeIdle}}) != "" {
		t.Error("a quiet has words to read")
	}
}
