package server

import (
	"log"
	"strings"
)

// The robot goes to what an answer was read off.
//
// Asked 「今朝のゴルフのスコアは？」, the answer is read off a photo — the score
// card from this morning, joined to this morning's reservation. Said in a
// chat, it is a sentence with nothing to check it against; the photo it came
// from is on the board, and the robot walks over and stands on it, the way it
// stands on what point was asked to find. So a person sees where the answer
// came from, and can open it.
//
// What it goes to is the evidence, most specific first: the photo whose field
// was the answer, else the want whose answer it was. One of a want's answers
// (`<want id>/result/<key>`) stands for its want: the robot walks on cells,
// and the answer's ball is beside its want's tile.

// walkRobotToEvidence stands the robot on the thing or want an answer was
// read off. Quietly nothing when it is not on the board.
func (s *Server) walkRobotToEvidence(id string) {
	if i := strings.Index(id, answerSep); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		return
	}
	tiles, err := s.fmTiles()
	if err != nil {
		return
	}
	for _, t := range tiles {
		if t.id != id {
			continue
		}
		if err := s.fmWalkRobot(t.x, t.y); err != nil {
			log.Printf("[robot] could not walk to %s: %v", t.name, err)
		}
		return
	}
}
