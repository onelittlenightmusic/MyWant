package types

import (
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[SlackPostWant, SlackPostLocals]("slack_post")
}

type SlackPostLocals struct{}

// SlackPostWant posts a message to Slack. It can operate in two modes:
//
//  1. source_want mode: reads outgoing_message / outgoing_date from a named sibling Want
//     (e.g. MorningBriefingWant). Posts when a message for today is available.
//
//  2. message param mode: posts the message passed directly as a param.
type SlackPostWant struct {
	Want
}

func (s *SlackPostWant) GetLocals() *SlackPostLocals {
	return CheckLocalsInitialized[SlackPostLocals](&s.Want)
}

func (s *SlackPostWant) Initialize() {
	// Promote params to state so the agent reads exclusively from state.
	s.SetCurrent("slack_webhook_url", s.GetStringParam("slack_webhook_url", ""))
}

// IsAchieved returns true when a message has been posted today.
func (s *SlackPostWant) IsAchieved() bool {
	today := time.Now().Format("2006-01-02")
	return GetCurrent(s, "last_posted_date", "") == today
}

func (s *SlackPostWant) CalculateAchievingPercentage() float64 {
	if s.IsAchieved() {
		return 100
	}
	return 10
}

// Progress triggers the Slack post agent once a message is available.
func (s *SlackPostWant) Progress() {
	s.SetCurrent("achieving_percentage", s.CalculateAchievingPercentage())

	if s.IsAchieved() {
		return
	}

	msg := s.resolveMessage()
	if msg == "" {
		return
	}

	s.SetCurrent("last_message", msg)
	if err := s.ExecuteAgents(); err != nil {
		s.StoreLog("[SLACK_POST] Agent error: %v", err)
	}
}

// resolveMessage returns the message to post, or "" if not yet available.
// It prefers reading from a sibling source_want; falls back to the message param.
func (s *SlackPostWant) resolveMessage() string {
	sourceWantName := s.GetStringParam("source_want", "")
	if sourceWantName != "" {
		cb := GetGlobalChainBuilder()
		if cb == nil {
			return ""
		}
		states := cb.GetAllWantStates()
		sourceWant, found := states[sourceWantName]
		if !found {
			s.StoreLog("[SLACK_POST] source Want %q not found", sourceWantName)
			return ""
		}
		today := time.Now().Format("2006-01-02")
		if GetCurrent(sourceWant, "outgoing_date", "") != today {
			return "" // source not ready for today
		}
		return GetCurrent(sourceWant, "outgoing_message", "")
	}

	// Fallback: static message param
	return s.GetStringParam("message", "")
}
