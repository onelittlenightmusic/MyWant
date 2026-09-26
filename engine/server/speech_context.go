package server

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// What the asker can see, sent along with what they said.
//
// "これ消して" and "ここに置いて" are how people point at a board while talking
// about it, and until now the robot was handed the words alone: the sender and
// the text, nothing else (see forwardToRobotIfAddressed). So a demonstrative
// had nothing to bind to — the one thing the speaker was obviously indicating,
// the tile they are standing on, was known to this server and thrown away on
// the way out.
//
// Now the utterance carries where the speaker is standing and what is under and
// around them. It is a short line, not a board dump: the board itself is one
// `mywant board` away, and a small on-device model spends its whole window on a
// preamble that tries to pre-answer the question.

// speechContextRadius is how far around the speaker counts as "next to me".
// One cell: a neighbour you could touch. Two already names half a district.
const speechContextRadius = 1

// tileNear is one tile standing at or beside the speaker.
type tileNear struct {
	name  string
	kind  string // "want" | "thing"
	what  string // type, or subtype
	x, y  int
	under bool // standing on it, rather than beside it
}

// contextForSpeaker describes where the speaker is and what is around them, or
// returns "" when nothing is known — an unknown position is not worth a
// sentence saying so.
func (s *Server) contextForSpeaker(speakerID string) string {
	x, y, ok := speakerCell(speakerID)
	if !ok {
		return ""
	}

	var under, beside []string
	for _, t := range s.tilesAround(x, y) {
		line := fmt.Sprintf("%s (%s %s)", t.name, t.kind, t.what)
		if t.under {
			under = append(under, line)
		} else {
			beside = append(beside, line)
		}
	}

	parts := []string{fmt.Sprintf("the person asking is standing at (%d, %d) on the canvas", x, y)}
	if len(under) > 0 {
		parts = append(parts, "on that cell: "+strings.Join(under, ", "))
	}
	if len(beside) > 0 {
		parts = append(parts, "next to them: "+strings.Join(beside, ", "))
	}
	if len(under) > 0 || len(beside) > 0 {
		parts = append(parts, `"this"/"これ"/"here"/"ここ" most likely means one of those`)
	}
	return "(context: " + strings.Join(parts, "; ") + ")"
}

// speakerCell is the cell a character is standing on.
//
// Their live cursor first, then the last place they were seen: somebody who
// typed a question and stopped moving is still standing where they were, and
// treating a gone-quiet cursor as no position at all would drop the context
// exactly for the person who paused to type.
func speakerCell(characterID string) (int, int, bool) {
	cursorsMu.RLock()
	defer cursorsMu.RUnlock()
	if e, ok := cursors[characterID]; ok {
		return int(math.Round(e.X)), int(math.Round(e.Y)), true
	}
	if e, ok := lastCursorPos[characterID]; ok {
		return int(math.Round(e.X)), int(math.Round(e.Y)), true
	}
	return 0, 0, false
}

// tilesAround lists the wants and things standing on or beside one cell.
func (s *Server) tilesAround(x, y int) []tileNear {
	var near []tileNear
	consider := func(t tileNear) {
		dx, dy := absInt(t.x-x), absInt(t.y-y)
		if dx > speechContextRadius || dy > speechContextRadius {
			return
		}
		t.under = dx == 0 && dy == 0
		near = append(near, t)
	}

	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			if want == nil || want.Metadata.IsSystemWant {
				continue
			}
			labels := want.GetLabels() // a live want's map is written under its lock
			wx, errX := strconv.Atoi(labels[canvasXLabelKey])
			wy, errY := strconv.Atoi(labels[canvasYLabelKey])
			if errX != nil || errY != nil {
				continue
			}
			consider(tileNear{name: want.Metadata.Name, kind: "want", what: want.Metadata.Type, x: wx, y: wy})
		}
	}

	if s.thingStore != nil && s.thingLabels != nil {
		labels := s.thingLabels.All()
		for _, thing := range s.thingStore.Entries() {
			l := labels[thing.ID]
			if l[canvasOnLabelKey] != "true" {
				continue
			}
			tx, errX := strconv.Atoi(l[canvasXLabelKey])
			ty, errY := strconv.Atoi(l[canvasYLabelKey])
			if errX != nil || errY != nil {
				continue
			}
			what := thing.Catalog
			consider(tileNear{name: thing.Value, kind: "thing", what: what, x: tx, y: ty})
		}
	}
	return near
}

// The labels a tile's place is kept in, named here so this file reads without
// chasing them: the same keys the canvas writes when a tile is dragged.
const (
	canvasOnLabelKey = "mywant.io/canvas"
	canvasXLabelKey  = "mywant.io/canvas-x"
	canvasYLabelKey  = "mywant.io/canvas-y"
)
