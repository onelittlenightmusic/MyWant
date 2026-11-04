package server

// plan_earlier.go — what was asked just before, in the brief a plan is made from.
//
// 「乗り換えを再検索して」 names nothing: it means the route asked about a
// moment ago. So the brief (plan.go) carries the conversation's last
// questions, a line each, and — faintly, after it — the wants that turn
// showed:
//
//	Asked earlier in this conversation (only when the question is about it):
//	- 「中野坂上から大森の乗り換え」 (shown then, maybe not right: transit_search-2118)
//
// The question is what is carried. What was done for it was a small model's
// choice and often wrong (that turn showed the route to 銀座), so the want is
// named only so it can be chosen, without its parameters, and it is not put
// first among the candidates. The model that plans reads the question and
// what was asked before it and decides again — here, a new route from 中野坂上
// to 大森, its parameters filled from the question before (/plans/params
// carries the same lines). Nothing here reads the question for words like
// 再検索. A line a turn, not the turns' answers: the brief is read by a small
// model with a small window.
//
// The turns are the robot's (turns.go), kept for every device that talks to
// it. Which are "before": those before the turn being answered, when the
// client names it; else the turn just opened for this question (the newest,
// asking the same) is left out. None older than planEarlierWindow: a
// conversation picked up the next morning is a new one.

import (
	"fmt"
	"strings"
	"time"
)

const (
	planEarlierTurns  = 2                // questions carried into the brief
	planEarlierWants  = 2                // wants named, faintly, per question
	planEarlierWindow = 30 * time.Minute // older than this, not this conversation
)

// planEarlier is one earlier turn, as the brief carries it: its question,
// and the wants it showed that are still on the board.
type planEarlier struct {
	question string
	wants    []robotWant
}

// earlierTurns picks, from turns (oldest first, as kept), the last
// planEarlierTurns questions before the turn being answered — newest first —
// with the wants still among wants that each showed. current is that turn's
// id, or "" when the client did not say: then the newest turn, asking
// question, is the one being answered.
func earlierTurns(turns []robotTurn, current, question string, wants []robotWant, now time.Time) []planEarlier {
	byRef := map[string]robotWant{}
	for _, w := range wants {
		if w.Metadata.Name == robotWantName {
			continue // the robot itself is never what was asked about
		}
		byRef[w.Metadata.ID] = w
		byRef[w.Metadata.Name] = w
	}
	end := len(turns)
	if current != "" {
		for i, t := range turns {
			if t.ID == current {
				end = i
				break
			}
		}
	} else if end > 0 && strings.TrimSpace(turns[end-1].Question) == strings.TrimSpace(question) {
		end--
	}
	var out []planEarlier
	for i := end - 1; i >= 0 && len(out) < planEarlierTurns; i-- {
		t := turns[i]
		if at, err := time.Parse(time.RFC3339, t.StartedAt); err == nil && now.Sub(at) > planEarlierWindow {
			break // older still, and older than this conversation
		}
		q := strings.TrimSpace(t.Question)
		if q == "" {
			continue
		}
		var cards []robotCard
		cards = append(cards, t.Cards...)
		for _, st := range t.Steps {
			if st.Card != nil {
				cards = append(cards, *st.Card)
			}
		}
		e := planEarlier{question: q}
		seen := map[string]bool{}
		for _, c := range cards {
			if c.Kind != "" && c.Kind != "want" {
				continue
			}
			w, ok := byRef[c.ID]
			if !ok {
				w, ok = byRef[c.Name]
			}
			if !ok || seen[w.Metadata.Name] || len(e.wants) == planEarlierWants {
				continue
			}
			seen[w.Metadata.Name] = true
			e.wants = append(e.wants, w)
		}
		out = append(out, e)
	}
	return out
}

// earlierInConversation is the turns before the one being answered
// (earlierTurns), from the robot's turns and the wants on the board.
func (s *Server) earlierInConversation(current, question string) []planEarlier {
	turnsMu.Lock()
	var turns []robotTurn
	if robot, err := s.robotWantNow(); err == nil {
		turns = robotLoadTurns(robot)
	}
	turnsMu.Unlock()
	if len(turns) == 0 {
		return nil
	}
	wants, _ := s.robotWantList() // without them, the questions alone
	return earlierTurns(turns, current, question, wants, time.Now())
}

// earlierText is the brief's lines for them, or "" for none: each question,
// and faintly after it the wants it showed, by name.
func earlierText(earlier []planEarlier) string {
	if len(earlier) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Asked earlier in this conversation (only when the question is about it):\n")
	for _, e := range earlier {
		fmt.Fprintf(&b, "- 「%s」", e.question)
		if len(e.wants) > 0 {
			names := make([]string, len(e.wants))
			for i, w := range e.wants {
				names[i] = w.Metadata.Name
			}
			fmt.Fprintf(&b, " (shown then, maybe not right: %s)", strings.Join(names, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

// withEarlierWants adds the wants the earlier turns showed to the candidates,
// after them, when they are not there already — so they may be chosen, not so
// they are chosen first. At most planCandidates: an earlier want is the first
// to give way.
func withEarlierWants(cands []planCandidate, earlier []planEarlier) []planCandidate {
	have := map[string]bool{}
	for _, c := range cands {
		have[c.name] = true
	}
	out := cands
	for _, e := range earlier {
		for _, w := range e.wants {
			if have[w.Metadata.Name] || len(out) >= planCandidates {
				continue
			}
			have[w.Metadata.Name] = true
			out = append(out, planCandidate{name: w.Metadata.Name, want: true,
				line: fmt.Sprintf("- %s: want %s, shown earlier in this conversation", w.Metadata.Name, w.Metadata.Type)})
		}
	}
	return out
}
