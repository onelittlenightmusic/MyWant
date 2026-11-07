package server

import (
	ws "github.com/onelittlenightmusic/want-spec"

	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
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
// it is run (/tools/call with the turn's id), closed with the answer — or posted
// whole afterwards, by a device that worked without this server and brings
// the exchange back: its steps not yet run here are run then. Either way the
// robot's chat shows it the way it shows its own: the question, the steps as
// its working log, the answer in its voice.

// robotStep is want-spec's RobotStep: one tool run.
type robotStep = ws.RobotStep

type robotTurn struct {
	ID         string      `json:"id"`
	Question   string      `json:"question"`
	Steps      []robotStep `json:"steps"`
	Answer     string      `json:"answer,omitempty"`
	Error      string      `json:"error,omitempty"`
	StartedAt  string      `json:"started_at"`
	// DeviceID and Character are who asked, when the client says: the
	// device's own id (its character is the one it is assigned to) or the
	// character itself. Whose story the turn is part of (story.go).
	DeviceID  string `json:"deviceId,omitempty"`
	Character string `json:"character,omitempty"`
	FinishedAt string      `json:"finished_at,omitempty"`
	// Where the answering model ran: "device" for one elsewhere, "robot" for
	// the Mac's own (fmtool).
	By string `json:"by,omitempty"`
	// Quiet: kept here, not written into the robot's chat. The Mac's own
	// model is asked through the robot want, which already puts the question,
	// the tool it used and the answer in the chat; writing them again would
	// say everything twice. Posted as {"chat": false}.
	Quiet bool `json:"quiet,omitempty"`
	// Cards: the wants the answer is about, as every client shows them under
	// it — settled with the answer (robotAnswerCards).
	Cards []robotCard `json:"cards,omitempty"`
	// Choices: the answer is a question with these replies (「スコアを確認しますか？」
	// → はい / いいえ), which every chat offers beside its input. Given by
	// the client, or read from the answer (robotReplyChoices).
	Choices []string `json:"choices,omitempty"`
	// Outcome: how the answer went, judged by what the person did next
	// (story_outcome.go: success, corrected, reask, ignored).
	Outcome string `json:"outcome,omitempty"`
}

const (
	turnsKey = "fm_turns"
	turnsMax = 20
	// Who asked, in the robot's chat: the on-device model's phone.
	robotDeviceSender = "fm-device"
)

// robotLastN keeps the robot's chat lists as long as the rest of the code keeps
// them (robotResponsesMax).
func robotLastN(list []any) []any {
	if len(list) > robotResponsesMax {
		return list[len(list)-robotResponsesMax:]
	}
	return list
}

// Turns are read, changed and written back whole; one at a time.
var turnsMu sync.Mutex

func (s *Server) robotWantNow() (*mywant.Want, error) {
	robot := s.findWantByIDOrName(robotWantName)
	if robot == nil {
		return nil, fmt.Errorf("no robot on this board")
	}
	return robot, nil
}

func robotLoadTurns(robot *mywant.Want) []robotTurn {
	var turns []robotTurn
	raw, _ := json.Marshal(mywant.GetCurrent(robot, turnsKey, []any{}))
	_ = json.Unmarshal(raw, &turns)
	return turns
}

func robotStoreTurns(robot *mywant.Want, turns []robotTurn) {
	if len(turns) > turnsMax {
		turns = turns[len(turns)-turnsMax:]
	}
	var list []any
	raw, _ := json.Marshal(turns)
	_ = json.Unmarshal(raw, &list)
	robot.SetCurrent(turnsKey, list)
}

// robotUpdateTurn finds a turn by id and lets change edit it in place.
func (s *Server) robotUpdateTurn(id string, change func(*robotTurn) error) (robotTurn, error) {
	turnsMu.Lock()
	defer turnsMu.Unlock()
	robot, err := s.robotWantNow()
	if err != nil {
		return robotTurn{}, err
	}
	turns := robotLoadTurns(robot)
	for i := range turns {
		if turns[i].ID == id {
			if err := change(&turns[i]); err != nil {
				return robotTurn{}, err
			}
			robotStoreTurns(robot, turns)
			return turns[i], nil
		}
	}
	return robotTurn{}, fmt.Errorf("no turn %q", id)
}

// robotRepeatTools are the tools whose same call twice in a turn can only give
// the same answer: a type's description, a want made (the second finds the
// first), the robot walked to where it stands. A look or the board can
// change between two calls (a want's answer coming in), so they run again.
var robotRepeatTools = map[string]bool{"describe_type": true, "deploy_want": true, "point": true}

// robotRepeatedStep is the earlier step of this turn that made the same call
// (tool and arguments alike, the type's name in any case), or nil.
func (s *Server) robotRepeatedStep(id, tool string, args map[string]string) *robotStep {
	if !robotRepeatTools[tool] {
		return nil
	}
	turnsMu.Lock()
	defer turnsMu.Unlock()
	robot, err := s.robotWantNow()
	if err != nil {
		return nil
	}
	for _, t := range robotLoadTurns(robot) {
		if t.ID != id {
			continue
		}
		for i := range t.Steps {
			st := t.Steps[i]
			if st.Tool == tool && st.Done && robotSameArgs(st.Arguments, args) && !strings.HasPrefix(st.Output, "Error:") {
				return &st
			}
		}
	}
	return nil
}

func robotSameArgs(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		w, ok := b[k]
		if !ok || !strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(w)) {
			return false
		}
	}
	return true
}

// robotRepeatOutput is what the model hears for a call it already made.
func robotRepeatOutput(prev robotStep) string {
	first := prev.Output
	if i := strings.IndexAny(first, "\n"); i > 0 {
		first = first[:i]
	}
	return "Already done in this turn; calling it again changes nothing. What it gave: " + first +
		" — Call no more tools. Answer the person now, in one short sentence, from what this turn already found."
}

// turnQuestion is what a turn was asked, or "".
func (s *Server) turnQuestion(id string) string {
	turnsMu.Lock()
	defer turnsMu.Unlock()
	robot, err := s.robotWantNow()
	if err != nil {
		return ""
	}
	for _, t := range robotLoadTurns(robot) {
		if t.ID == id {
			return t.Question
		}
	}
	return ""
}

func (s *Server) robotAddTurn(turn robotTurn) error {
	turnsMu.Lock()
	defer turnsMu.Unlock()
	robot, err := s.robotWantNow()
	if err != nil {
		return err
	}
	robotStoreTurns(robot, append(robotLoadTurns(robot), turn))
	return nil
}

// robotAsked puts the question in the robot's chat, where what was said to it goes.
func (s *Server) robotAsked(question string) {
	robot, err := s.robotWantNow()
	if err != nil || question == "" {
		return
	}
	messages := mywant.GetCurrent(robot, "cc_messages", []any{})
	messages = append(messages, map[string]any{
		"sender": robotDeviceSender, "text": question,
		"timestamp": time.Now().Format(time.RFC3339), "channel_id": robotDeviceSender,
	})
	robot.SetCurrent("cc_messages", robotLastN(messages))
}

// sayTurnAnswer puts the answer in the robot's chat and in its mouth — as any
// answer of its own is (CharacterSpeaks: the bubble and the speech log).
// Nothing here asks the robot's agent anything: webhook_auto_request, which
// sets that going, is left alone.
//
// The wants the turn's steps brought the robot to go with it, as cards: the
// chat shows each under the answer, so what the answer is about is there at a
// glance (and a press away), as on the phone.
func (s *Server) sayTurnAnswer(answer string, cards []robotCard, choices []string) {
	robot, err := s.robotWantNow()
	if err != nil || answer == "" {
		return
	}
	entry := map[string]any{
		"text": answer, "timestamp": time.Now().Format(time.RFC3339), "subtype": "fm",
	}
	if len(cards) > 0 {
		entry["cards"] = cards
	}
	if len(choices) > 0 {
		entry["choices"] = choices
	}
	responses := mywant.GetCurrent(robot, "cc_responses", []any{})
	responses = append(responses, entry)
	robot.SetCurrent("cc_responses", robotLastN(responses))
	mywant.CharacterSpeaks(robotWantName, answer, "agent")
}

// robotAnswerCards is the turn's cards, each filled with what its want says now
// — its type, how it is doing and its result in a line. Settled once, with the
// answer, so every client shows the same card under the same answer.
func (s *Server) robotAnswerCards(turn robotTurn) []robotCard {
	cards := turnCards(turn)
	var things []struct {
		ID      string `json:"id"`
		Value   string `json:"value"`
		Subtype string `json:"subtype"`
	}
	for i, c := range cards {
		if c.Kind == "thing" {
			// A thing has no status or result: its name and what kind of
			// thing it is (a city, a station) are what its card says.
			if things == nil {
				var resp struct {
					Things []struct {
						ID      string `json:"id"`
						Value   string `json:"value"`
						Subtype string `json:"subtype"`
					} `json:"things"`
				}
				if s.backend("GET", "/api/v1/things", nil, &resp) == nil {
					things = resp.Things
				}
			}
			for _, t := range things {
				if t.ID == c.ID {
					cards[i].Name, cards[i].Type = t.Value, t.Subtype
				}
			}
			continue
		}
		var w struct {
			Metadata struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"metadata"`
			Status string `json:"status"`
			State  struct {
				FinalResult any `json:"final_result"`
			} `json:"state"`
		}
		if err := s.backend("GET", "/api/v1/wants/"+c.ID, nil, &w); err != nil {
			continue
		}
		if w.Metadata.Name != "" {
			cards[i].Name = w.Metadata.Name
		}
		cards[i].Type = w.Metadata.Type
		cards[i].Status = w.Status
		cards[i].Summary = robotCardSummary(w.State.FinalResult)
	}
	return cards
}

// robotCardSummary is a want's result in a line: text as it is; an object as its
// first few plain values (not the machine copies, *_rfc3339); a list as its
// count. The one rule every client's card follows.
func robotCardSummary(v any) string {
	const max = 140
	cut := func(s string) string {
		if r := []rune(s); len(r) > max {
			return string(r[:max])
		}
		return s
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return cut(x)
	case bool, float64, int:
		return fmt.Sprint(x)
	case []any:
		if len(x) == 0 {
			return ""
		}
		// A list of records (a route's legs, a day's reservations): the first
		// one says what the list is; the count says how much more there is.
		// "1 件" alone said nothing at all.
		if _, isRecord := x[0].(map[string]any); isRecord {
			first := robotCardSummary(x[0])
			if len(x) == 1 {
				return first
			}
			return cut(fmt.Sprintf("%s（ほか %d 件）", first, len(x)-1))
		}
		return fmt.Sprintf("%d 件", len(x))
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			if strings.HasSuffix(k, "_rfc3339") {
				continue
			}
			switch x[k].(type) {
			case map[string]any, []any, nil:
				continue
			}
			parts = append(parts, fmt.Sprintf("%s: %v", k, x[k]))
			if len(parts) == 4 {
				break
			}
		}
		return cut(strings.Join(parts, " · "))
	}
	return ""
}

// turnCards is every want the turn's steps brought the robot to, once each.
func turnCards(turn robotTurn) []robotCard {
	var out []robotCard
	seen := map[string]bool{}
	for _, st := range turn.Steps {
		if st.Card != nil && !seen[st.Card.ID] {
			seen[st.Card.ID] = true
			out = append(out, *st.Card)
		}
	}
	return out
}

// robotShowStep writes a step into the robot's working log, the line its chat
// shows between a question and its answer.
func (s *Server) robotShowStep(step robotStep, prefix string) {
	robot, err := s.robotWantNow()
	if err != nil {
		return
	}
	var args []string
	for _, p := range robotToolByName(step.Tool).Arguments {
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

// robotAlignPlace keeps what the robot does in step with what it says.
//
// An answer that says 「ここに『荻窪→中野坂上』があります」 is a gesture as
// much as words: the person looks where the robot stands. On Fly the robot
// said it standing on 荻窪 station — it had walked there for the question
// before, and this turn's deploy left it where it was. So when the answer
// says here (ここ, こちら, here) and nothing in this turn walked the robot, the
// thing or want it names — the longest name of the board's found in the
// answer, so 「荻窪→中野坂上」 is not taken for 荻窪 — is where it goes, as a
// step of the turn like any other. A turn that did walk it is left alone: the
// walk is what the model chose to show, and the answer is about that.
func (s *Server) robotAlignPlace(turn robotTurn) *robotStep {
	if !robotSaysHere(turn.Answer) {
		return nil
	}
	for _, st := range turn.Steps {
		if strings.Contains(st.Output, "The robot is standing on") {
			return nil
		}
	}
	tiles, err := s.robotTiles()
	if err != nil {
		return nil
	}
	named := -1
	for i, t := range tiles {
		n := []rune(t.name)
		if len(n) < 2 || !strings.Contains(turn.Answer, t.name) {
			continue
		}
		if named < 0 || len(n) > len([]rune(tiles[named].name)) {
			named = i
		}
	}
	if named < 0 {
		return nil
	}
	robot, err := s.robotWantNow()
	if err != nil {
		return nil
	}
	labels := robot.GetLabels()
	if labels[robotCanvasX] == fmt.Sprint(tiles[named].x) && labels[robotCanvasY] == fmt.Sprint(tiles[named].y) {
		return nil
	}
	step := robotStep{Tool: "point", Arguments: map[string]string{"name": tiles[named].name}}
	s.robotRunStep(&step)
	step.Card = robotTakeCard(step.Arguments)
	log.Printf("[fm] turn %s said here of %q away from it: walked there", turn.ID, tiles[named].name)
	return &step
}

var robotHereEnglish = regexp.MustCompile(`(?i)\bhere\b`)

// robotSaysHere: the answer points at where the robot stands.
func robotSaysHere(answer string) bool {
	return strings.Contains(answer, "ここ") || strings.Contains(answer, "こちら") || robotHereEnglish.MatchString(answer)
}

// robotRunStep runs a step here and notes what came of it.
func (s *Server) robotRunStep(step *robotStep) {
	step.Output = s.robotRunTool(step.Tool, step.Arguments)
	step.Done = true
	step.At = time.Now().Format(time.RFC3339)
}

// ── Handlers ────────────────────────────────────────────────────────────────

// POST /api/v1/turns
//
//	{question}                     opens a turn; answer it with /answer
//	{question, steps, answer}      a whole turn, brought back afterwards:
//	                               steps not done here are run now
func (s *Server) handleTurnPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		robotTurn
		Chat *bool `json:"chat"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	turn := body.robotTurn
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
		s.robotAsked(turn.Question)
	}
	for i := range turn.Steps {
		if turn.Steps[i].Arguments == nil {
			turn.Steps[i].Arguments = map[string]string{}
		}
		if !turn.Steps[i].Done {
			if robotHearsQuestion(turn.Steps[i].Tool) {
				turn.Steps[i].Arguments[robotQuestionArg] = turn.Question
			}
			s.robotRunStep(&turn.Steps[i])
		}
		// The want the step brought the robot to, as its card (see robotTakeCard).
		if turn.Steps[i].Card == nil {
			turn.Steps[i].Card = robotTakeCard(turn.Steps[i].Arguments)
		}
		if !turn.Quiet {
			s.robotShowStep(turn.Steps[i], "")
		}
	}
	if turn.Answer = strings.TrimSpace(turn.Answer); turn.Answer != "" {
		turn.FinishedAt = time.Now().Format(time.RFC3339)
		if step := s.robotAlignPlace(turn); step != nil {
			turn.Steps = append(turn.Steps, *step)
			if !turn.Quiet {
				s.robotShowStep(*step, "")
			}
		}
		turn.Cards = s.robotAnswerCards(turn)
		if len(turn.Choices) == 0 {
			turn.Choices = robotReplyChoices(turn.Answer)
		}
		if !turn.Quiet {
			s.sayTurnAnswer(turn.Answer, turn.Cards, turn.Choices)
		}
	}
	if err := s.robotAddTurn(turn); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	// Into the asker's story (story.go): asked, and answered when posted whole.
	story.ask(story.characterOf(turn.Character, turn.DeviceID, turn.Question, storyDeviceCharacter), turn.ID, turn.DeviceID, turn.Question)
	if turn.FinishedAt != "" {
		story.answer(turn.ID, turn.Cards, turn.Error != "")
	}
	log.Printf("[fm] turn %s: %q (%d steps)", turn.ID, firstLine(turn.Question, 80), len(turn.Steps))
	robotWriteJSON(w, turn)
}

// POST /api/v1/turns/{id}/answer  {answer, choices?} or {error}
// POST /api/v1/turns/{id}/feedback — the person's word on an answer:
// {"reaction": "good"} (👍) or {"reaction": "wrong"} (👎). Said on its own,
// with nothing asked after it; it is the turn's outcome (success, wrong) in
// the asker's story, by person, over what was judged from what they did.
func (s *Server) handleTurnFeedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reaction string `json:"reaction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	outcome := map[string]string{"good": ws.TurnOutcomeSuccess, "wrong": ws.TurnOutcomeWrong}[strings.TrimSpace(req.Reaction)]
	if outcome == "" {
		http.Error(w, "reaction must be good or wrong", http.StatusBadRequest)
		return
	}
	turn, err := s.robotUpdateTurn(mux.Vars(r)["id"], func(t *robotTurn) error {
		t.Outcome = outcome
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	story.said(turn.ID, turn.Character, turn.Question, outcome)
	robotWriteJSON(w, map[string]string{"turn": turn.ID, "outcome": outcome})
}

func (s *Server) handleTurnAnswer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Answer  string   `json:"answer"`
		Error   string   `json:"error"`
		Choices []string `json:"choices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	turn, err := s.robotUpdateTurn(mux.Vars(r)["id"], func(t *robotTurn) error {
		if t.FinishedAt != "" {
			return fmt.Errorf("turn %s is already answered", t.ID)
		}
		t.Answer = strings.TrimSpace(req.Answer)
		t.Error = strings.TrimSpace(req.Error)
		t.Choices = req.Choices
		if len(t.Choices) == 0 {
			t.Choices = robotReplyChoices(t.Answer)
		}
		t.FinishedAt = time.Now().Format(time.RFC3339)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if turn.Answer != "" {
		if step := s.robotAlignPlace(turn); step != nil {
			turn.Steps = append(turn.Steps, *step)
			_, _ = s.robotUpdateTurn(turn.ID, func(t *robotTurn) error { t.Steps = append(t.Steps, *step); return nil })
			if !turn.Quiet {
				s.robotShowStep(*step, "")
			}
		}
		turn.Cards = s.robotAnswerCards(turn)
		cards := turn.Cards
		_, _ = s.robotUpdateTurn(turn.ID, func(t *robotTurn) error { t.Cards = cards; return nil })
		if !turn.Quiet {
			s.sayTurnAnswer(turn.Answer, turn.Cards, turn.Choices)
		}
	}
	if turn.Error != "" && !turn.Quiet {
		if robot, err := s.robotWantNow(); err == nil {
			types.RecordCCActivityDetail(robot, "error", firstLine(turn.Error, 120), turn.Error)
		}
	}
	story.answer(turn.ID, turn.Cards, turn.Error != "")
	log.Printf("[fm] turn %s answered: %q", turn.ID, firstLine(turn.Answer+turn.Error, 120))
	robotWriteJSON(w, turn)
}

// GET /api/v1/turns — the latest turns, oldest first.
func (s *Server) handleTurnList(w http.ResponseWriter, r *http.Request) {
	robot, err := s.robotWantNow()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	turnsMu.Lock()
	turns := robotLoadTurns(robot)
	turnsMu.Unlock()
	robotWriteJSON(w, map[string]any{"turns": turns})
}

// GET /api/v1/turns/{id}
func (s *Server) handleTurnGet(w http.ResponseWriter, r *http.Request) {
	turn, err := s.robotUpdateTurn(mux.Vars(r)["id"], func(*robotTurn) error { return nil })
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	robotWriteJSON(w, turn)
}

// POST /api/v1/turns/{id}/replay
//
// Plays the turn again: the steps that show something (a walk to a tile) are
// run again, and the robot says its answer again. Steps that make something
// (deploy_want) are not — a second want is not the same exchange again — and
// reading steps change nothing to see. The chat gets the replayed steps as
// working-log lines; the question and the answer are not written twice.
func (s *Server) handleTurnReplay(w http.ResponseWriter, r *http.Request) {
	turn, err := s.robotUpdateTurn(mux.Vars(r)["id"], func(*robotTurn) error { return nil })
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var replayed []robotStep
	for _, step := range turn.Steps {
		if !robotToolByName(step.Tool).replays {
			continue
		}
		again := robotStep{Tool: step.Tool, Arguments: step.Arguments}
		s.robotRunStep(&again)
		s.robotShowStep(again, "↻ ")
		replayed = append(replayed, again)
	}
	if turn.Answer != "" {
		mywant.CharacterSpeaks(robotWantName, turn.Answer, "agent")
	}
	log.Printf("[fm] turn %s replayed (%d steps)", turn.ID, len(replayed))
	robotWriteJSON(w, map[string]any{"turn": turn, "replayed": replayed})
}

// robotYesNo is what a yes-or-no question is answered with.
var robotYesNo = []string{"はい", "いいえ"}

// robotAskWords make a question one that is not answered yes or no
// (「どこに置きますか？」).
var robotAskWords = []string{"どこ", "何", "なに", "なん", "いつ", "誰", "だれ", "どれ", "どう", "どの", "どちら", "どっち", "いくつ", "いくら", "なぜ",
	"what", "where", "when", "who", "which", "how", "why"}

// robotReplyChoices: the replies an answer that asks something takes — はい /
// いいえ when its last sentence is a question asking yes or no
// (「アーカイブした「新宿御苑前→銀座」があります。復元しますか？」), none otherwise.
func robotReplyChoices(answer string) []string {
	a := strings.TrimSpace(answer)
	if !strings.HasSuffix(a, "？") && !strings.HasSuffix(a, "?") {
		return nil
	}
	last := strings.TrimRight(a, "？?")
	if i := strings.LastIndexAny(last, "。！!？?\n"); i >= 0 {
		last = last[i:]
	}
	lower := strings.ToLower(last)
	for _, w := range robotAskWords {
		if strings.Contains(lower, w) {
			return nil
		}
	}
	return robotYesNo
}
