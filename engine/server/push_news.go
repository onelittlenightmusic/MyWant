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
	"log"
	"os"
	"strconv"
	"strings"

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

// outputNews asks Laya whether the want's answer (msg, in its own voice) is
// news; ok false when it is not asked or nobody answered.
func (s *Server) outputNews(w *mywant.Want, msg string) (decisionAnswer, string, bool) {
	ri := s.initiative
	if ri == nil || w == nil || characterWantTypes[w.Metadata.Type] || !pushFiltered(w) {
		return decisionAnswer{}, "", false
	}
	a, by, err := ri.decider.decide(outputNewsText(w.Metadata.Name, msg), map[string]decisionQuestion{newsQuestionID: newsQuestion})
	if err != nil {
		if err != errNoModel {
			log.Printf("[push] laya did not answer for %s (%v); pushing", w.Metadata.Name, err)
		}
		return decisionAnswer{}, by, false
	}
	ans, ok := a[newsQuestionID]
	return ans, by, ok && ans.Choice != ""
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
