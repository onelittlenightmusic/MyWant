package server

// story_outcome.go — an answer judged by what the person did after it
// (turn.outcome, want-spec's TurnOutcome*):
//
//	success    went to (stepped onto, opened, expanded) a want the answer showed
//	corrected  went to another want of the same type instead
//	reask      asked much the same again soon after
//	ignored    the answer showed wants and none of this followed
//
// Only what the person did is evidence. A client brings its character to the
// want an answer shows — before the answer is even said, as the act finishes
// — so an arrival at a shown want up to storyPromptedFor after the act or the
// answer is the answer's doing, not the person's: it is marked via=answer and
// judges nothing. An answer that showed no want (a weather read out) has
// nowhere to be gone to: only a reask judges it, and otherwise it is let go
// without an outcome, so `ignored` always means "shown somewhere, not gone".
//
// Judged within storyJudgeWithin of the answer; the first evidence decides.
// Held in memory: a turn still being judged when the server stops is not
// judged.

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	ws "github.com/onelittlenightmusic/want-spec"
)

const (
	storyJudgeWithin = 2 * time.Minute
	storyPromptedFor = 3 * time.Second
	// storyUnanswered: a turn never answered is let go after this.
	storyUnanswered = 10 * time.Minute
	// storyReaskAt: how alike two questions are (their characters' bigrams,
	// Jaccard) to be one asked again.
	storyReaskAt = 0.5
)

// storyJudging is a turn waiting to be judged.
type storyJudging struct {
	turn, character, question string
	askedAt, answeredAt       time.Time // answeredAt zero until answered
	prompted                  time.Time // arrivals at a shown want until then are the answer's
	shown                     map[string]bool
	shownOrder                []string
	types                     map[string]bool
	plan                      map[string]string // what the act did: action, type, target, capability
}

func (j *storyJudging) show(id, typ string) {
	if id != "" && !j.shown[id] {
		j.shown[id] = true
		j.shownOrder = append(j.shownOrder, id)
	}
	if typ != "" {
		j.types[typ] = true
	}
}

// askedLocked opens a turn's judging, and judges as asked again any of the
// character's answered turns this question repeats (r.mu held).
func (r *storyRecorder) askedLocked(character, turn, question string) {
	now := r.now()
	for _, j := range r.judgingOf(character) {
		if !j.answeredAt.IsZero() && storySimilar(j.question, question) >= storyReaskAt {
			r.decideLocked(j, ws.TurnOutcomeReask, nil, map[string]string{"again": turn})
		}
	}
	r.judging[turn] = &storyJudging{turn: turn, character: character, question: question, askedAt: now,
		shown: map[string]bool{}, types: map[string]bool{}}
}

// actedLocked is an act done for a turn: what it showed is the answer's, and
// the client may be bringing the character to it now.
func (r *storyRecorder) actedLocked(turn string, plan ws.PlanSpec, cards []*robotCard) {
	j := r.judging[turn]
	if j == nil {
		return
	}
	j.prompted = r.now().Add(storyPromptedFor)
	j.plan = map[string]string{}
	for k, v := range map[string]string{"action": plan.Action, "type": plan.Type, "target": plan.Target, "capability": plan.Capability} {
		if v != "" {
			j.plan[k] = v
		}
	}
	if plan.Type != "" {
		j.types[plan.Type] = true
	}
	for _, c := range cards {
		if c != nil && c.Kind == "want" {
			j.show(c.ID, c.Type)
		}
	}
}

// answeredLocked is a turn answered with these cards: judging starts.
func (r *storyRecorder) answeredLocked(turn string, cards []robotCard) {
	j := r.judging[turn]
	if j == nil {
		return
	}
	now := r.now()
	j.answeredAt = now
	j.prompted = now.Add(storyPromptedFor)
	for _, c := range cards {
		if c.Kind == "want" {
			j.show(c.ID, c.Type)
		}
	}
}

// judgeLocked looks at an event about to be held: an arrival brought about by
// an answer is marked so, and one the person made may judge a turn. What it
// decides is held after the event.
func (r *storyRecorder) judgeLocked(ev *ws.StoryEvent) {
	switch ev.Kind {
	case ws.StoryKindWantArrive, ws.StoryKindWantOpen, ws.StoryKindWantExpand:
	default:
		return
	}
	to := ev.To
	if to == nil || to.Kind != "want" || to.ID == "" {
		return
	}
	now := r.now()
	var decided []func()
	for _, j := range r.judgingOf(ev.Character) {
		if !j.shown[to.ID] && !j.answeredAt.IsZero() && !now.After(j.prompted) {
			continue // being brought to the answer: nothing else counts yet
		}
		switch {
		case j.shown[to.ID] && (j.answeredAt.IsZero() || !now.After(j.prompted)):
			if ev.Detail == nil {
				ev.Detail = map[string]string{}
			}
			ev.Detail["via"] = "answer"
			if ev.Ref == "" {
				ev.Ref = j.turn
			}
		case j.answeredAt.IsZero():
			// Before the answer: the person's own business.
		case j.shown[to.ID]:
			j := j
			decided = append(decided, func() { r.decideLocked(j, ws.TurnOutcomeSuccess, to, nil) })
		case to.Type != "" && j.types[to.Type]:
			j := j
			decided = append(decided, func() { r.decideLocked(j, ws.TurnOutcomeCorrected, to, nil) })
		}
	}
	// After the event itself, so the story reads: went there, and so it was right.
	r.afterEvent = append(r.afterEvent, decided...)
}

// expireLocked lets go of the turns whose time is up: ignored when the answer
// showed somewhere, without an outcome when it did not.
func (r *storyRecorder) expireLocked() {
	now := r.now()
	for turn, j := range r.judging {
		switch {
		case j.answeredAt.IsZero():
			if now.Sub(j.askedAt) > storyUnanswered {
				delete(r.judging, turn)
			}
		case now.Sub(j.answeredAt) > storyJudgeWithin:
			if len(j.shown) > 0 {
				r.decideLocked(j, ws.TurnOutcomeIgnored, nil, nil)
			} else {
				delete(r.judging, turn)
			}
		}
	}
}

// decideLocked holds a turn's outcome in its character's story and lets go
// of the turn.
func (r *storyRecorder) decideLocked(j *storyJudging, outcome string, to *ws.StoryPlace, more map[string]string) {
	if _, open := r.judging[j.turn]; !open {
		return
	}
	delete(r.judging, j.turn)
	now := r.now()
	detail := map[string]string{"outcome": outcome, "question": j.question}
	if len(j.shownOrder) > 0 {
		detail["shown"] = strings.Join(j.shownOrder, ",")
	}
	for k, v := range j.plan {
		detail["plan."+k] = v
	}
	if !j.answeredAt.IsZero() && (outcome == ws.TurnOutcomeSuccess || outcome == ws.TurnOutcomeCorrected) {
		detail["seconds"] = fmt.Sprint(int(now.Sub(j.answeredAt).Round(time.Second) / time.Second))
	}
	for k, v := range more {
		detail[k] = v
	}
	ev := ws.StoryEvent{Character: j.character, Kind: ws.StoryKindTurnOutcome, Ref: j.turn,
		At: now.Format(time.RFC3339), To: to, Detail: detail}
	r.pending[ev.Character] = append(r.pending[ev.Character], ev)
	if r.onOutcome != nil {
		turn, hook := j.turn, r.onOutcome
		go hook(turn, outcome)
	}
}

// judgingOf is a character's turns being judged.
func (r *storyRecorder) judgingOf(character string) []*storyJudging {
	var out []*storyJudging
	for _, j := range r.judging {
		if j.character == character {
			out = append(out, j)
		}
	}
	return out
}

// storySimilar is how alike two questions are: their characters' bigrams
// (spaces and punctuation left out), Jaccard. 「中野坂上から銀座の乗り換え」 and
// 「中野坂上から銀座に乗り換え」 are alike; 「天気は？」 and 「乗り換え」 are not.
func storySimilar(a, b string) float64 {
	grams := func(s string) map[string]bool {
		var rs []rune
		for _, c := range strings.ToLower(s) {
			if unicode.IsLetter(c) || unicode.IsNumber(c) {
				rs = append(rs, c)
			}
		}
		out := map[string]bool{}
		if len(rs) == 1 {
			out[string(rs)] = true
		}
		for i := 0; i+1 < len(rs); i++ {
			out[string(rs[i:i+2])] = true
		}
		return out
	}
	ga, gb := grams(a), grams(b)
	if len(ga) == 0 || len(gb) == 0 {
		return 0
	}
	both := 0
	for g := range ga {
		if gb[g] {
			both++
		}
	}
	return float64(both) / float64(len(ga)+len(gb)-both)
}
