package server

// robot_body.go — who is acting as a robot.
//
// The robot was one character, and everything it did named it: it walked by
// moving the want "robot", spoke into that want's mouth, wrote that want's
// chat, and its story was "robot"'s. Any character can be one now: a
// character labelled robot=true has a want of type robot for its chat and its
// body (character_chat_wants.go), is answered by the same model with the same
// tools, and moves the same way — the server stands it on cells.
//
// So what a robot does is done as somebody: its body. A body is a character
// and the want that is its mouth and where it stands. It travels the way who
// asked (robotWhoArg) and what they asked (robotQuestionArg) do — on the turn
// (robotTurn.Body) and in a tool's arguments (robotBodyArg) — and every walk,
// line and story entry is that body's. Nothing said: the robot, as before.

import (
	"fmt"
	"strings"

	mywant "mywant/engine/core"
)

// robotBodyArg carries, in a tool's arguments, which robot is running it (a
// character id); robotRunTool takes it out once the tool has run.
const robotBodyArg = "_body"

// robotBody is a character acting as a robot, and the want it acts through.
type robotBody struct {
	Character string
	Want      string
}

// theRobot is the robot there has always been.
var theRobot = robotBody{Character: robotCharacterID, Want: robotWantName}

// isRobotCharacter: the character acts as a robot — the robot itself, or one
// labelled so.
func isRobotCharacter(c mywant.Character) bool {
	return c.ID == robotCharacterID || mywant.IsRobotCharacter(c)
}

// bodyOf is the body of a character, the robot when it is none.
func bodyOf(character string) robotBody {
	if character == "" || character == robotCharacterID {
		return theRobot
	}
	c, ok := mywant.GetCharacter(character)
	if !ok || !isRobotCharacter(*c) {
		return theRobot
	}
	return robotBody{Character: c.ID, Want: characterChatWantName(c.ID)}
}

// bodyOfWant is the body whose want this is (a robot want's character_id).
func bodyOfWant(w *mywant.Want) robotBody {
	if w == nil || w.Metadata.Name == robotWantName {
		return theRobot
	}
	if id, _ := w.Spec.Params["character_id"].(string); id != "" {
		return bodyOf(id)
	}
	return theRobot
}

// bodyFromArgs is the body a tool runs as.
func bodyFromArgs(args map[string]string) robotBody {
	return bodyOf(args[robotBodyArg])
}

// withBody puts the body into a tool's arguments; the robot needs no mention.
func withBody(args map[string]string, b robotBody) map[string]string {
	if b.Character != "" && b.Character != robotCharacterID {
		args[robotBodyArg] = b.Character
	}
	return args
}

// withBodyID is the body as an act names it: "" for the robot.
func withBodyID(b robotBody) string {
	if b.Character == robotCharacterID {
		return ""
	}
	return b.Character
}

// carryBody copies the body from one tool's arguments to another's — a tool
// that walks by pointing (robotPoint) walks as the one it was run as.
func carryBody(from, to map[string]string) map[string]string {
	if b, ok := from[robotBodyArg]; ok {
		to[robotBodyArg] = b
	}
	return to
}

// robotBodies is every character acting as a robot now.
func robotBodies() []robotBody {
	out := []robotBody{theRobot}
	for _, c := range mywant.ListCharacters() {
		if c.ID != robotCharacterID && isRobotCharacter(c) {
			out = append(out, bodyOf(c.ID))
		}
	}
	return out
}

// isRobotBodyWant: the want is some robot's body, not a thing on the board.
func isRobotBodyWant(w *mywant.Want) bool {
	return w != nil && (w.Metadata.Name == robotWantName || w.Metadata.Type == "robot")
}

// bodyWant is the body's want.
func (s *Server) bodyWant(b robotBody) (*mywant.Want, error) {
	w := s.findWantByIDOrName(b.Want)
	if w == nil {
		return nil, fmt.Errorf("no %s on this board", b.Want)
	}
	return w, nil
}

// walkBody stands the body on a cell: x and y in one step.
func (s *Server) walkBody(b robotBody, x, y int) error {
	walk := map[string]any{"metadata": map[string]any{"labels": map[string]string{
		robotCanvasX: fmt.Sprint(x), robotCanvasY: fmt.Sprint(y),
	}}}
	return s.backend("PATCH", "/api/v1/wants/"+b.Want, walk, nil)
}

// sayBody puts words in the body's mouth (its bubble) and its speech record.
func (s *Server) sayBody(b robotBody, words, kind string) error {
	words = strings.TrimSpace(words)
	if words == "" {
		return nil
	}
	recordSpeechAs(b.Character, words, "say", kind)
	return s.backend("PUT", "/api/v1/states/"+b.Want, map[string]any{"say": words}, nil)
}

// bodyStands is the cell the body stands on.
func (s *Server) bodyStands(b robotBody) (x, y int, ok bool) {
	w := s.findWantByIDOrName(b.Want)
	if w == nil {
		return 0, 0, false
	}
	return wantCell(w)
}

// turnBody is the body a turn was answered by.
func (s *Server) turnBody(id string) robotBody {
	if id == "" {
		return theRobot
	}
	turnsMu.Lock()
	defer turnsMu.Unlock()
	robot, err := s.robotWantNow()
	if err != nil {
		return theRobot
	}
	for _, t := range robotLoadTurns(robot) {
		if t.ID == id {
			return bodyOf(t.Body)
		}
	}
	return theRobot
}
