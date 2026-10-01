package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
	types "mywant/engine/types"
)

// A turn: one go of talking to the robot from elsewhere — what was asked,
// what the robot did about it, and what it answered.
//
// The model that answers may be on a phone, but the robot is here, and so is
// its record. A question that ends in the robot walking to 荻窪 is not a
// question and an answer with a walk somewhere off the record: it is one
// exchange, kept whole on the robot's want (fm_turns), so any screen can read
// it and it can be played again.
//
// A turn is built live — opened with the question, each tool call recorded as
// it is run (/fm/call with the turn's id), closed with the answer — or posted
// whole afterwards, by a device that worked without this server and brings
// the exchange back: its steps not yet run here are run then. Either way the
// robot's chat shows it the way it shows its own: the question, the steps as
// its working log, the answer in its voice.

type fmStep struct {
	Tool      string            `json:"tool"`
	Arguments map[string]string `json:"arguments"`
	Output    string            `json:"output,omitempty"`
	// Done: run against this server already. A step posted without it is
	// run when the turn arrives.
	Done bool   `json:"done"`
	At   string `json:"at,omitempty"`
}

type fmTurn struct {
	ID         string   `json:"id"`
	Question   string   `json:"question"`
	Steps      []fmStep `json:"steps"`
	Answer     string   `json:"answer,omitempty"`
	Error      string   `json:"error,omitempty"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at,omitempty"`
	// Where the answering model ran: "device" for one elsewhere, "robot" for
	// the Mac's own (fmtool).
	By string `json:"by,omitempty"`
	// Quiet: kept here, not written into the robot's chat. The Mac's own
	// model is asked through the robot want, which already puts the question,
	// the tool it used and the answer in the chat; writing them again would
	// say everything twice. Posted as {"chat": false}.
	Quiet bool `json:"quiet,omitempty"`
}

const (
	fmTurnsKey = "fm_turns"
	fmTurnsMax = 20
	// Who asked, in the robot's chat: the on-device model's phone.
	fmDeviceSender = "fm-device"
)

// fmLastN keeps the robot's chat lists as long as the rest of the code keeps
// them (robotResponsesMax).
func fmLastN(list []any) []any {
	if len(list) > robotResponsesMax {
		return list[len(list)-robotResponsesMax:]
	}
	return list
}

// Turns are read, changed and written back whole; one at a time.
var fmTurnsMu sync.Mutex

func (s *Server) fmRobot() (*mywant.Want, error) {
	robot := s.findWantByIDOrName(fmRobotWant)
	if robot == nil {
		return nil, fmt.Errorf("no robot on this board")
	}
	return robot, nil
}

func fmLoadTurns(robot *mywant.Want) []fmTurn {
	var turns []fmTurn
	raw, _ := json.Marshal(mywant.GetCurrent(robot, fmTurnsKey, []any{}))
	_ = json.Unmarshal(raw, &turns)
	return turns
}

func fmStoreTurns(robot *mywant.Want, turns []fmTurn) {
	if len(turns) > fmTurnsMax {
		turns = turns[len(turns)-fmTurnsMax:]
	}
	var list []any
	raw, _ := json.Marshal(turns)
	_ = json.Unmarshal(raw, &list)
	robot.SetCurrent(fmTurnsKey, list)
}

// fmUpdateTurn finds a turn by id and lets change edit it in place.
func (s *Server) fmUpdateTurn(id string, change func(*fmTurn) error) (fmTurn, error) {
	fmTurnsMu.Lock()
	defer fmTurnsMu.Unlock()
	robot, err := s.fmRobot()
	if err != nil {
		return fmTurn{}, err
	}
	turns := fmLoadTurns(robot)
	for i := range turns {
		if turns[i].ID == id {
			if err := change(&turns[i]); err != nil {
				return fmTurn{}, err
			}
			fmStoreTurns(robot, turns)
			return turns[i], nil
		}
	}
	return fmTurn{}, fmt.Errorf("no turn %q", id)
}

func (s *Server) fmAddTurn(turn fmTurn) error {
	fmTurnsMu.Lock()
	defer fmTurnsMu.Unlock()
	robot, err := s.fmRobot()
	if err != nil {
		return err
	}
	fmStoreTurns(robot, append(fmLoadTurns(robot), turn))
	return nil
}

// fmAsked puts the question in the robot's chat, where what was said to it goes.
func (s *Server) fmAsked(question string) {
	robot, err := s.fmRobot()
	if err != nil || question == "" {
		return
	}
	messages := mywant.GetCurrent(robot, "cc_messages", []any{})
	messages = append(messages, map[string]any{
		"sender": fmDeviceSender, "text": question,
		"timestamp": time.Now().Format(time.RFC3339), "channel_id": fmDeviceSender,
	})
	robot.SetCurrent("cc_messages", fmLastN(messages))
}

// fmAnswered puts the answer in the robot's chat and in its mouth — as any
// answer of its own is (CharacterSpeaks: the bubble and the speech log).
// Nothing here asks the robot's agent anything: webhook_auto_request, which
// sets that going, is left alone.
func (s *Server) fmAnswered(answer string) {
	robot, err := s.fmRobot()
	if err != nil || answer == "" {
		return
	}
	responses := mywant.GetCurrent(robot, "cc_responses", []any{})
	responses = append(responses, map[string]any{
		"text": answer, "timestamp": time.Now().Format(time.RFC3339), "subtype": "fm",
	})
	robot.SetCurrent("cc_responses", fmLastN(responses))
	mywant.CharacterSpeaks(fmRobotWant, answer, "agent")
}

// fmShowStep writes a step into the robot's working log, the line its chat
// shows between a question and its answer.
func (s *Server) fmShowStep(step fmStep, prefix string) {
	robot, err := s.fmRobot()
	if err != nil {
		return
	}
	var args []string
	for _, p := range fmToolByName(step.Tool).Arguments {
		if v := step.Arguments[p.Name]; v != "" {
			args = append(args, v)
		}
	}
	summary := prefix + step.Tool
	if len(args) > 0 {
		summary += " " + strings.Join(args, " ")
	}
	types.RecordCCActivityDetail(robot, "tool", firstLine(summary, 120), step.Output)
}

// fmRunStep runs a step here and notes what came of it.
func (s *Server) fmRunStep(step *fmStep) {
	step.Output = s.fmRunTool(step.Tool, step.Arguments)
	step.Done = true
	step.At = time.Now().Format(time.RFC3339)
}

// ── Handlers ────────────────────────────────────────────────────────────────

// POST /api/v1/fm/turns
//
//	{question}                     opens a turn; answer it with /answer
//	{question, steps, answer}      a whole turn, brought back afterwards:
//	                               steps not done here are run now
func (s *Server) handleFMTurnPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		fmTurn
		Chat *bool `json:"chat"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	turn := body.fmTurn
	turn.Quiet = body.Chat != nil && !*body.Chat
	turn.Question = strings.TrimSpace(turn.Question)
	if turn.Question == "" {
		http.Error(w, "question is required", http.StatusBadRequest)
		return
	}
	turn.ID = fmt.Sprintf("turn-%d", time.Now().UnixNano())
	turn.StartedAt = time.Now().Format(time.RFC3339)
	if turn.By == "" {
		turn.By = "device"
	}
	if !turn.Quiet {
		s.fmAsked(turn.Question)
	}
	for i := range turn.Steps {
		if turn.Steps[i].Arguments == nil {
			turn.Steps[i].Arguments = map[string]string{}
		}
		if !turn.Steps[i].Done {
			s.fmRunStep(&turn.Steps[i])
		}
		if !turn.Quiet {
			s.fmShowStep(turn.Steps[i], "")
		}
	}
	if turn.Answer = strings.TrimSpace(turn.Answer); turn.Answer != "" {
		turn.FinishedAt = time.Now().Format(time.RFC3339)
		if !turn.Quiet {
			s.fmAnswered(turn.Answer)
		}
	}
	if err := s.fmAddTurn(turn); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Printf("[fm] turn %s: %q (%d steps)", turn.ID, firstLine(turn.Question, 80), len(turn.Steps))
	fmWriteJSON(w, turn)
}

// POST /api/v1/fm/turns/{id}/answer  {answer} or {error}
func (s *Server) handleFMTurnAnswer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Answer string `json:"answer"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	turn, err := s.fmUpdateTurn(mux.Vars(r)["id"], func(t *fmTurn) error {
		if t.FinishedAt != "" {
			return fmt.Errorf("turn %s is already answered", t.ID)
		}
		t.Answer = strings.TrimSpace(req.Answer)
		t.Error = strings.TrimSpace(req.Error)
		t.FinishedAt = time.Now().Format(time.RFC3339)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if turn.Answer != "" && !turn.Quiet {
		s.fmAnswered(turn.Answer)
	}
	if turn.Error != "" && !turn.Quiet {
		if robot, err := s.fmRobot(); err == nil {
			types.RecordCCActivityDetail(robot, "error", firstLine(turn.Error, 120), turn.Error)
		}
	}
	log.Printf("[fm] turn %s answered: %q", turn.ID, firstLine(turn.Answer+turn.Error, 120))
	fmWriteJSON(w, turn)
}

// GET /api/v1/fm/turns — the latest turns, oldest first.
func (s *Server) handleFMTurnList(w http.ResponseWriter, r *http.Request) {
	robot, err := s.fmRobot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	fmTurnsMu.Lock()
	turns := fmLoadTurns(robot)
	fmTurnsMu.Unlock()
	fmWriteJSON(w, map[string]any{"turns": turns})
}

// GET /api/v1/fm/turns/{id}
func (s *Server) handleFMTurnGet(w http.ResponseWriter, r *http.Request) {
	turn, err := s.fmUpdateTurn(mux.Vars(r)["id"], func(*fmTurn) error { return nil })
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	fmWriteJSON(w, turn)
}

// POST /api/v1/fm/turns/{id}/replay
//
// Plays the turn again: the steps that show something (a walk to a tile) are
// run again, and the robot says its answer again. Steps that make something
// (deploy_want) are not — a second want is not the same exchange again — and
// reading steps change nothing to see. The chat gets the replayed steps as
// working-log lines; the question and the answer are not written twice.
func (s *Server) handleFMTurnReplay(w http.ResponseWriter, r *http.Request) {
	turn, err := s.fmUpdateTurn(mux.Vars(r)["id"], func(*fmTurn) error { return nil })
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var replayed []fmStep
	for _, step := range turn.Steps {
		if !fmToolByName(step.Tool).replays {
			continue
		}
		again := fmStep{Tool: step.Tool, Arguments: step.Arguments}
		s.fmRunStep(&again)
		s.fmShowStep(again, "↻ ")
		replayed = append(replayed, again)
	}
	if turn.Answer != "" {
		mywant.CharacterSpeaks(fmRobotWant, turn.Answer, "agent")
	}
	log.Printf("[fm] turn %s replayed (%d steps)", turn.ID, len(replayed))
	fmWriteJSON(w, map[string]any{"turn": turn, "replayed": replayed})
}
