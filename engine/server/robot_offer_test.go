package server

import (
	"fmt"
	"testing"
	"time"
)

func TestOfferReplyKind(t *testing.T) {
	for said, want := range map[string]string{
		"はい": "accept", "はい、お願い": "accept", "うん": "accept", "お願いします": "accept", "OK": "accept",
		"いいえ": "skip", "いらない": "skip", "大丈夫です": "skip", "No": "skip",
		"いいよ":     "accept",
		"中野の天気は？": "",
		"はいはい、ところで新宿から渋谷の乗り換えを調べて": "", // long: a question of its own
		"": "",
	} {
		if got := offerReplyKind(said); got != want {
			t.Errorf("offerReplyKind(%q) = %q, want %q", said, got, want)
		}
	}
}

func TestOfferReplyNeedsAnOffer(t *testing.T) {
	robotOffers.Lock()
	robotOffers.pending = nil
	robotOffers.Unlock()
	if _, ok := (&Server{}).robotOfferReply("はい"); ok {
		t.Error("はい with no offer waiting was taken as a reply")
	}
}

func TestInitiativeCompress(t *testing.T) {
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	var old []initiativeExample
	// The weather offered after the golf slot: accepted 7 times, declined once.
	for i := range 8 {
		gold := "accept"
		if i == 3 {
			gold = "skip"
		}
		old = append(old, initiativeExample{State: fmt.Sprintf("golf %d / weather", i), At: at.Add(time.Duration(i) * time.Hour),
			Gold: map[string]string{"offer": gold}, Group: "offer|golf|restart:weather"})
	}
	// An even split teaches nothing.
	old = append(old,
		initiativeExample{State: "golf / backup", At: at, Gold: map[string]string{"offer": "accept"}, Group: "offer|golf|restart:backup"},
		initiativeExample{State: "golf / backup", At: at, Gold: map[string]string{"offer": "skip"}, Group: "offer|golf|restart:backup"})
	rows := initiativeCompress(old)
	if len(rows) != 3 { // round(log2(1+8)) = 3
		t.Fatalf("8 old replies became %d rows, want 3: %v", len(rows), rows)
	}
	for _, r := range rows {
		if r["state"] != "golf 7 / weather" || r["gold"].(map[string]string)["offer"] != "accept" {
			t.Errorf("row %v: want the latest words, labelled as most were", r)
		}
	}
}

func TestInitiativeTrainingSetWeighsTheRecent(t *testing.T) {
	recent := []map[string]any{{"state": "r", "gold": map[string]string{"offer": "skip"}}}
	set := initiativeTrainingSet(recent, nil, nil)
	n := 0
	for _, r := range set {
		if r["state"] == "r" {
			n++
		}
	}
	if n != initiativeRecentWeight {
		t.Errorf("a recent reply came %d times, want %d", n, initiativeRecentWeight)
	}
	// A made row repeated is folded to log2(1+n) copies.
	if got := len(initiativeFold([]map[string]any{{"s": 1}, {"s": 1}, {"s": 1}, {"s": 2}})); got != 2+1 {
		t.Errorf("3+1 rows folded to %d, want 2+1", got)
	}
}
