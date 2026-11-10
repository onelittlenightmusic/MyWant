package server

// Whether a want's new answer is pushed to the phone: asked of Laya, with the
// question the robot's initiative asks (newsQuestion) — is it news the person
// wants now? Every answer is still a notice, in the list as before; only the
// push, the one that lights the phone, waits on the answer. A song changing
// is a notice; a seat coming free is a push.
//
// It is the same answer the robot acts on: asked once here, handed to the
// initiative (initiativeEvent.News), kept in robot-initiative.jsonl with what
// became of the push, and taught by what the person did after it — opened the
// notice, or spoke to the robot (robot_initiative_learn.go). A notice held
// back can still be opened from the list, so a held one that mattered still
// says so.
//
// Only while the initiative is on (MYWANT_ROBOT_INITIATIVE=1): it is what
// learns from the answers. Pushed as before when no worker answers, when Laya
// is unsure (p(news) at or above pushNewsMin), and for a type that says
// `push-filter: none` (what must always reach the person).

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	mywant "mywant/engine/core"
)

// pushFilterLabel on a want type: "none" pushes every answer of it.
const pushFilterLabel = "push-filter"

// pushNewsMin is how likely news an answer must be not to be held back, unless
// MYWANT_PUSH_NEWS_MIN says otherwise: Laya's routine is followed only when it
// is fairly sure, so a doubtful one still reaches the person — and teaches.
const pushNewsMin = 0.3

func pushNewsThreshold() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("MYWANT_PUSH_NEWS_MIN"), 64); err == nil && v >= 0 && v <= 1 {
		return v
	}
	return pushNewsMin
}

// pushDecision is one want's new answer as Laya weighed it for the push —
// kept a line each in ~/.mywant/push-decisions.jsonl, and shown in the robot's
// chat as a processing line (robotChatTrace).
type pushDecision struct {
	At       string             `json:"at"`
	WantID   string             `json:"wantId"`
	Want     string             `json:"want"`
	WantType string             `json:"wantType"`
	Text     string             `json:"text"` // what Laya read
	Choice   string             `json:"choice,omitempty"`
	Probs    map[string]float64 `json:"probabilities,omitempty"`
	Min      float64            `json:"min"` // p(news) under which it is held
	By       string             `json:"by,omitempty"`
	Push     string             `json:"push"` // sent, held
	Reason   string             `json:"reason"`
	Ms       int64              `json:"ms"`
	Error    string             `json:"error,omitempty"`
}

// outputNews asks Laya whether the want's answer (msg, in its own voice) is
// news; ok false when it is not asked or nobody answered. Every asking is
// recorded (notePushDecision), its answer or why there was none.
func (s *Server) outputNews(w *mywant.Want, msg string) (decisionAnswer, string, bool) {
	ri := s.initiative
	if ri == nil || w == nil || characterWantTypes[w.Metadata.Type] || !pushFiltered(w) {
		return decisionAnswer{}, "", false
	}
	text := outputNewsText(w.Metadata.Name, msg)
	d := pushDecision{
		At: time.Now().Format(time.RFC3339), WantID: w.Metadata.ID, Want: w.Metadata.Name,
		WantType: w.Metadata.Type, Text: text, Min: pushNewsThreshold(), Push: "sent",
	}
	started := time.Now()
	a, by, err := ri.decider.decide(text, map[string]decisionQuestion{newsQuestionID: newsQuestion})
	d.Ms, d.By = time.Since(started).Milliseconds(), by
	ans, ok := a[newsQuestionID]
	switch {
	case err == errNoModel:
		d.Reason = "no-worker"
	case err != nil:
		d.Reason, d.Error = "no-answer", err.Error()
		log.Printf("[push] laya did not answer for %s (%v); pushing", w.Metadata.Name, err)
	case !ok || ans.Choice == "":
		d.Reason = "no-answer"
	default:
		d.Choice, d.Probs = ans.Choice, ans.Probabilities
		if pushHeld(ans, d.Min) {
			d.Push, d.Reason = "held", "routine"
		} else if ans.Choice == "routine" {
			d.Reason = "unsure" // routine, but not sure enough to hold it
		} else {
			d.Reason = "news"
		}
	}
	s.notePushDecision(d)
	if d.Choice == "" {
		return decisionAnswer{}, by, false
	}
	return ans, by, true
}

// notePushDecision keeps the decision (the log) and shows it in the robot's
// chat — no notice, just what was done: e.g.
//
//	laya.push want=Spotify verdict=routine p(news)=0.12 push=held reason=routine by=laya@iPhone ms=1830
func (s *Server) notePushDecision(d pushDecision) {
	_ = appendJSONLine(initiativeHomeFile("push-decisions.jsonl"), d)
	if d.Reason == "no-worker" {
		return // nothing was weighed: the log has it, the chat need not
	}
	fields := map[string]any{
		"want": d.Want, "wantId": d.WantID, "verdict": d.Choice, "push": d.Push, "reason": d.Reason,
		"by": d.By, "ms": d.Ms, "error": d.Error,
	}
	if p, ok := d.Probs["news"]; ok {
		fields["p(news)"] = fmt.Sprintf("%.2f", p)
	}
	s.robotChatTrace("laya.push", fields, []string{"want", "verdict", "p(news)", "push", "reason", "by", "ms", "error"})
}

// pushFiltered: the type lets Laya hold its pushes back.
func pushFiltered(w *mywant.Want) bool {
	if w.WantTypeDefinition == nil {
		return true
	}
	return strings.TrimSpace(w.WantTypeDefinition.Metadata.Labels[pushFilterLabel]) != "none"
}

// pushHeld: Laya is sure enough it is routine.
func pushHeld(a decisionAnswer, min float64) bool {
	if a.Choice != "routine" {
		return false
	}
	if p, ok := a.Probabilities["news"]; ok {
		return p < min
	}
	return true
}
