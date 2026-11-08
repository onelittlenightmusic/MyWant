package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// handleRobotInitiative hands the robot an event now, and answers with what it
// decided and did — to try a capability without waiting for a want's news or
// a quiet (robot_initiative.go).
//
//	POST /api/v1/robot/initiative {"event": "output", "want": "smartgolf-available"}
//	POST /api/v1/robot/initiative {"event": "idle"}
//
// An output event says the want's latest notice, unless "message" is given;
// "fresh": true forgets the robot's last remark first.
func (s *Server) handleRobotInitiative(w http.ResponseWriter, r *http.Request) {
	ri := s.initiative
	if ri == nil {
		http.Error(w, "the robot's initiative is off (MYWANT_ROBOT_INITIATIVE=1)", http.StatusConflict)
		return
	}
	var req struct {
		Event   string `json:"event"`
		Want    string `json:"want"`
		Message string `json:"message"`
		// Fresh: as if the robot had not spoken unasked yet — to show one
		// capability after another without the gap between remarks.
		Fresh bool `json:"fresh"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ev := initiativeEvent{Kind: req.Event, Message: req.Message, At: time.Now(), Injected: true}
	switch req.Event {
	case initiativeIdle:
	case initiativeOutput:
		want := s.findWantByIDOrName(req.Want)
		if want == nil {
			http.Error(w, "no want "+req.Want, http.StatusNotFound)
			return
		}
		ev.WantID, ev.WantName, ev.WantType = want.Metadata.ID, want.Metadata.Name, want.Metadata.Type
		if ev.Message == "" {
			if entries, _ := s.notifications.ForWant(want.Metadata.ID, 1); len(entries) > 0 {
				ev.Message = entries[0].Message
			}
		}
	default:
		http.Error(w, `event is "output" or "idle"`, http.StatusBadRequest)
		return
	}
	if req.Fresh {
		ri.mu.Lock()
		ri.lastSpoke = time.Time{}
		ri.mu.Unlock()
	}
	robotWriteJSON(w, s.robotTakeInitiative(ri, ev))
}

// handleRobotInitiativeStatus says where the robot's learning stands.
//
//	GET /api/v1/robot/initiative
func (s *Server) handleRobotInitiativeStatus(w http.ResponseWriter, r *http.Request) {
	robotWriteJSON(w, s.initiativeStatus())
}

// handleRobotInitiativeLabel is the person's own word on a decision: whether
// it was worth saying. It wins over the outcome's (robot_initiative_learn.go).
//
//	POST /api/v1/robot/initiative/label {"id": "init-…", "interrupt": "talk" | "wait"}
//
// talk is "it was news to me", wait "it was not" — the news question's label.
func (s *Server) handleRobotInitiativeLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        string `json:"id"`
		Interrupt string `json:"interrupt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" ||
		(req.Interrupt != "talk" && req.Interrupt != "wait") {
		http.Error(w, `want {"id", "interrupt": "talk" | "wait"}`, http.StatusBadRequest)
		return
	}
	gold := "routine"
	if req.Interrupt == "talk" {
		gold = "news"
	}
	l := initiativeLabel{ID: req.ID, At: time.Now().Format(time.RFC3339), Gold: map[string]string{newsQuestionID: gold},
		By: "person", Reason: "the person said so"}
	if err := appendJSONLine(initiativeHomeFile("robot-initiative-labels.jsonl"), l); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	robotWriteJSON(w, l)
}

// handleRobotInitiativeRetrain trains again now, without waiting for the count.
//
//	POST /api/v1/robot/initiative/retrain
func (s *Server) handleRobotInitiativeRetrain(w http.ResponseWriter, r *http.Request) {
	if s.initiative == nil || !s.robotRetrain(s.initiative, len(initiativeLabels())) {
		http.Error(w, "no worker that can train has asked lately, or a training is under way", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	robotWriteJSON(w, map[string]any{"training": true})
}

// handleRobotOfferReply answers the robot's offer as a reply in the chat
// would (はい / いいえ…) — for a client with buttons, and for trying it.
//
//	POST /api/v1/robot/offer/reply {"said": "はい"}
func (s *Server) handleRobotOfferReply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Said string `json:"said"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	answer, ok := s.robotOfferReply(req.Said)
	if !ok {
		http.Error(w, "no offer waiting, or that is no reply to one", http.StatusConflict)
		return
	}
	s.sayTurnAnswer(answer, nil, nil) // the chat, the bubble and the conversation
	robotWriteJSON(w, map[string]string{"answer": answer})
}
