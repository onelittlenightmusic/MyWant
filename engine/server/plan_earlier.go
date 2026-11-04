package server

// plan_earlier.go — what was said just before, in the brief a plan is made from.
//
// 「乗り換えを再検索して」 names nothing on the board: it means the route shown
// a moment ago. So the brief (plan.go) carries the conversation's last turns,
// a line each — what was asked and the wants it showed, with what they were
// asked:
//
//	Said earlier in this conversation (only when the question is about it):
//	- 「中野坂上から大森の乗り換え」 → transit_search-2131 (from=中野坂上, to=大森)
//
// and those wants come first among the candidates. The model that plans reads
// the question and decides whether it is about them — an existing plan on
// transit_search-2131 asking restart — as it decides everything else; nothing
// here reads the question for words like 再検索. A line or two, not the
// turns' answers: the brief is read by a small model with a small window.
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
	planEarlierTurns  = 2                // turns carried into the brief
	planEarlierWants  = 2                // wants named per turn
	planEarlierWindow = 30 * time.Minute // older than this, not this conversation
)

// planEarlier is one earlier turn, as the brief carries it.
type planEarlier struct {
	question string
	wants    []robotWant
}

// earlierTurns picks, from turns (oldest first, as kept), the last
// planEarlierTurns before the turn being answered that showed a want still
// among wants — newest first. current is that turn's id, or "" when the client
// did not say: then the newest turn, asking question, is the one being
// answered.
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
		var cards []robotCard
		cards = append(cards, t.Cards...)
		for _, st := range t.Steps {
			if st.Card != nil {
				cards = append(cards, *st.Card)
			}
		}
		e := planEarlier{question: t.Question}
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
		if len(e.wants) > 0 {
			out = append(out, e)
		}
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
	wants, err := s.robotWantList()
	if err != nil {
		return nil
	}
	return earlierTurns(turns, current, question, wants, time.Now())
}

// earlierText is the brief's lines for them, or "" for none.
func earlierText(earlier []planEarlier) string {
	if len(earlier) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Said earlier in this conversation (only when the question is about it):\n")
	for _, e := range earlier {
		names := make([]string, len(e.wants))
		for i, w := range e.wants {
			names[i] = earlierWantText(w)
		}
		fmt.Fprintf(&b, "- 「%s」 → %s\n", e.question, strings.Join(names, ", "))
	}
	b.WriteString("\n")
	return b.String()
}

func earlierWantText(w robotWant) string {
	if len(w.Spec.Params) == 0 {
		return w.Metadata.Name
	}
	return w.Metadata.Name + " (" + robotParamsText(w.Spec.Params) + ")"
}

// withEarlierFirst puts the wants the earlier turns showed first among the
// candidates — those already there moved up, the rest added — so they are
// among the names the plan may choose. At most planCandidates.
func withEarlierFirst(cands []planCandidate, earlier []planEarlier) []planCandidate {
	if len(earlier) == 0 {
		return cands
	}
	var out []planCandidate
	placed := map[string]bool{}
	for _, e := range earlier {
		for _, w := range e.wants {
			if placed[w.Metadata.Name] {
				continue
			}
			placed[w.Metadata.Name] = true
			c := planCandidate{name: w.Metadata.Name, want: true,
				line: fmt.Sprintf("- %s: want %s", w.Metadata.Name, w.Metadata.Type)}
			if len(w.Spec.Params) > 0 {
				c.line += " (" + robotParamsText(w.Spec.Params) + ")"
			}
			for _, have := range cands {
				if have.name == c.name {
					c = have
					break
				}
			}
			out = append(out, c)
		}
	}
	for _, c := range cands {
		if !placed[c.name] {
			out = append(out, c)
		}
	}
	if len(out) > planCandidates {
		out = out[:planCandidates]
	}
	return out
}
