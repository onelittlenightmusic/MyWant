package server

// How much the person has looked at each want lately — what the widget's
// "wants worth a look" are chosen by (robot_suggest.go): read off the
// characters' stories (story.go), where every want opened (want.open) and
// every tile stood on (want.arrive) is kept. Every character but the robot's:
// one person may come as any of them.
//
//	opened 3, stood on 1; a week older, half as much; at this part of the
//	day (dayPart), half again more. The last 30 days, worked out at most
//	every 5 minutes.

import (
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	lookedAtDays     = 30
	lookedAtHalfLife = 7 * 24 * time.Hour
	lookedAtFresh    = 5 * time.Minute
)

var lookedAtWeight = map[string]float64{"want.open": 3, "want.arrive": 1}

var lookedAt struct {
	sync.Mutex
	at     time.Time
	scores map[string]float64
}

// wantLook is how a want has been looked at.
type wantLook struct {
	Opens   int     `json:"opens"`
	Arrives int     `json:"arrives"`
	Score   float64 `json:"score"`
}

// wantLookedAt is each want's score (by id), every character's; a want never
// looked at has none.
func wantLookedAt(now time.Time) map[string]float64 {
	lookedAt.Lock()
	defer lookedAt.Unlock()
	if lookedAt.scores != nil && now.Sub(lookedAt.at) < lookedAtFresh {
		return lookedAt.scores
	}
	scores := map[string]float64{}
	entries, _ := os.ReadDir(story.root())
	for _, e := range entries {
		ch := e.Name()
		if !e.IsDir() || ch == robotCharacterID || strings.HasPrefix(ch, "_") {
			continue
		}
		for id, l := range characterLooks(ch, now, lookedAtDays) {
			scores[id] += l.Score
		}
	}
	lookedAt.at, lookedAt.scores = now, scores
	return scores
}

// characterLooks is how one character has looked at each want in the last
// days.
func characterLooks(ch string, now time.Time, days int) map[string]wantLook {
	out := map[string]wantLook{}
	part := dayPart(now)
	q := storyQuery{since: now.AddDate(0, 0, -days), until: now.Add(time.Minute),
		kinds: map[string]bool{"want.open": true, "want.arrive": true}}
	events, err := story.read(ch, q)
	if err != nil {
		return out
	}
	for _, ev := range events {
		if ev.To == nil || ev.To.ID == "" || storyCharacterPlace(ev.To) {
			continue // a character's own want (kept before it was left out)
		}
		t, err := time.Parse(time.RFC3339, ev.At)
		if err != nil {
			continue
		}
		w := lookedAtWeight[ev.Kind] * math.Pow(0.5, now.Sub(t).Hours()/lookedAtHalfLife.Hours())
		if dayPart(t) == part {
			w *= 1.5
		}
		l := out[ev.To.ID]
		if ev.Kind == "want.open" {
			l.Opens++
		} else {
			l.Arrives++
		}
		l.Score += w
		out[ev.To.ID] = l
	}
	return out
}

// lookBoost is how much a want looked at adds to the likelihood Laya gives a
// suggestion about it: up to 0.15, nearer it the more it is looked at.
func lookBoost(score float64) float64 {
	if score <= 0 {
		return 0
	}
	return 0.15 * score / (score + 10)
}
