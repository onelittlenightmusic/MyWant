package server

// plan_previous.go — a plan about something from earlier in the conversation.
//
// 「乗り換えを再検索して」 names nothing on the board: it means the route the
// person was shown a moment ago. The model that plans is the one to see that
// — it reads the question — and says so by choosing the action previous, with
// as type what it was about (transit_search), and as capability what to ask
// of it (restart). Nothing here reads the question for words like 再検索.
//
// What it was is found here, in the earlier turns (turns.go), newest first:
// the latest want of that type they showed — or the latest want, the type
// left empty — that is still on the board or put away. The act is then
// carried out on it as an existing plan, and what was found is written into
// its status (resolved). Found nothing, the act says nothing was found.

import (
	ws "github.com/onelittlenightmusic/want-spec"
)

// previousTarget is the latest want of type typ (any type, typ empty) the
// turns before current showed, among wants. turns are oldest first, as they
// are kept.
func previousTarget(turns []robotTurn, current, typ string, wants []robotWant) (ws.ActResolved, bool) {
	byRef := map[string]robotWant{}
	for _, w := range wants {
		if w.Metadata.Name == robotWantName {
			continue // the robot itself is never what was asked about
		}
		byRef[w.Metadata.ID] = w
		byRef[w.Metadata.Name] = w
	}
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		if t.ID == current {
			continue
		}
		// The cards settled with the answer first, then those the steps
		// brought the robot to, the last step first.
		var cards []*robotCard
		for j := range t.Cards {
			cards = append(cards, &t.Cards[j])
		}
		for j := len(t.Steps) - 1; j >= 0; j-- {
			if c := t.Steps[j].Card; c != nil {
				cards = append(cards, c)
			}
		}
		for _, c := range cards {
			if c.Kind != "" && c.Kind != "want" {
				continue
			}
			w, ok := byRef[c.ID]
			if !ok {
				w, ok = byRef[c.Name]
			}
			if !ok || (typ != "" && w.Metadata.Type != typ) {
				continue
			}
			return ws.ActResolved{Turn: t.ID, Target: w.Metadata.Name, Type: w.Metadata.Type}, true
		}
	}
	return ws.ActResolved{}, false
}

// resolvePrevious finds what a previous plan is about (previousTarget), in
// the turns kept on the robot and the wants on the board.
func (s *Server) resolvePrevious(current, typ string) (ws.ActResolved, bool) {
	if typ == planNone {
		typ = ""
	}
	turnsMu.Lock()
	var turns []robotTurn
	if robot, err := s.robotWantNow(); err == nil {
		turns = robotLoadTurns(robot)
	}
	turnsMu.Unlock()
	if len(turns) == 0 {
		return ws.ActResolved{}, false
	}
	wants, err := s.robotWantList()
	if err != nil {
		return ws.ActResolved{}, false
	}
	return previousTarget(turns, current, typ, wants)
}
