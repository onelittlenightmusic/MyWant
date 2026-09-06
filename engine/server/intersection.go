package server

import (
	"math"
	"strconv"
	"sync"
	"time"

	mywant "mywant/engine/core"
)

// Intersection — what happens when something that moves ends up on the same
// cell as a want.
//
// There were two of these, one for characters and one for things, and they
// were the same twenty lines twice: round the position, find the want at that
// cell, work out whether that is a change from last time, and act on the
// change. The only genuinely different part was what "act" meant. So the
// transition is written once here, and what happens on arrival is a rule.
//
// Not Fliction. Fliction is two tiles the user is holding together — a
// suggestion the board makes and a person accepts. An intersection is nobody's
// suggestion: something moved, it is now standing somewhere, and the board says
// what that means. A bin swallows what walks into it whether or not anyone was
// watching.
//
// Evaluated on the mover's side rather than the want's, and from the one place
// each kind of mover's position is written — the cursor handler and the drive
// engine for a character, the motion tick for a thing. A want does not have to
// know it is being stood on, or poll to find out.
//
// Three things a rule must get right, all of them learned from the version
// that only handled characters:
//
//   - A cell can hold more than one want. The old code took the first want at
//     the cell and stopped, which was invisible while only buttons counted and
//     wrong the moment anything else did — a bin sharing a cell with a note is
//     an ordinary board, and one of the two would never fire.
//   - Which wants a rule cares about is the rule's business. The old code
//     filtered to `form-type: button` before any rule saw the want, so a type
//     without that label — trash has no reason to carry it, it is not a
//     pressed thing — could not be reacted to at all.
//   - Arriving and leaving are different events, and most rules only have the
//     first. Stepping off a going plate does not un-start you.

type moverKind int

const (
	moverCharacter moverKind = iota
	moverThing
)

// mover is whatever just moved. A character and a thing are the same kind of
// participant here — something with a position that arrived somewhere — and
// this is the whole of what an intersection needs to know about either.
type mover struct {
	kind moverKind
	id   string
}

// targetField names the want array that lists this kind of mover.
func (m mover) targetField() string {
	if m.kind == moverThing {
		return targetsThings
	}
	return targetsCharacters
}

// key distinguishes movers of different kinds in the one map. Ids are already
// prefixed by kind in practice ("chr-…", "thg-…"), but the map is not the place
// to rely on that.
func (m mover) key() string {
	return strconv.Itoa(int(m.kind)) + "\x00" + m.id
}

// intersectionContext is what a rule is allowed to reach for: the server, when
// it needs to write something outside the want, and the form-type test, which
// is passed in rather than read off the server so the transition logic can be
// exercised without standing one up.
type intersectionContext struct {
	s        *Server
	isButton func(typeName string) bool
}

// intersectionRule is one meaning a shared cell can have.
//
// `applies` is asked of every want the mover is standing on, so a rule that
// cares about one want type says so here and nowhere else. `leave` is optional:
// most arrivals are events rather than states.
type intersectionRule struct {
	name    string
	applies func(ctx intersectionContext, want *mywant.Want, m mover) bool
	enter   func(ctx intersectionContext, want *mywant.Want, m mover)
	leave   func(ctx intersectionContext, want *mywant.Want, m mover)
}

var (
	intersectionMu sync.Mutex
	// Which wants each mover was standing on as of its last position. Touched
	// from HTTP handler goroutines (ordinary movement), each character's own
	// motion want (driven movement) and the thing motion ticker, so it needs
	// its own lock.
	intersectionOn = map[string][]string{}
)

// syncButtonOccupancy re-evaluates a character's intersections after their
// position is written. Called from the cursor PUT handler and from the drive
// engine's moveDrivenCharacter.
//
// Keeps its old name because that is what those two call it, and because from
// their side nothing has changed: they report a move, and this decides what the
// move meant.
func (s *Server) syncButtonOccupancy(characterID string, newX, newY float64) {
	if s.globalBuilder == nil {
		return
	}
	applyIntersections(s, mover{moverCharacter, characterID}, newX, newY,
		s.globalBuilder.GetWants(), s.isButtonFormType)
}

// syncThingOccupancy is the same for a thing, called from the motion tick.
func (s *Server) syncThingOccupancy(thingID string, newX, newY float64) {
	if s.globalBuilder == nil {
		return
	}
	applyIntersections(s, mover{moverThing, thingID}, newX, newY,
		s.globalBuilder.GetWants(), s.isButtonFormType)
}

// applyButtonOccupancy is the character path with no server behind it — the
// seam the transition tests drive.
func applyButtonOccupancy(characterID string, newX, newY float64, allWants []*mywant.Want, isButton func(typeName string) bool) {
	applyIntersections(nil, mover{moverCharacter, characterID}, newX, newY, allWants, isButton)
}

// applyIntersections works out what changed about where a mover is standing,
// and runs the rules over the difference.
func applyIntersections(
	s *Server, m mover, newX, newY float64,
	allWants []*mywant.Want, isButton func(typeName string) bool,
) {
	rx := strconv.Itoa(int(math.Round(newX)))
	ry := strconv.Itoa(int(math.Round(newY)))
	ctx := intersectionContext{s: s, isButton: isButton}

	intersectionMu.Lock()
	defer intersectionMu.Unlock()

	was := intersectionOn[m.key()]
	now := wantsAtCell(rx, ry, allWants)

	nowIDs := make(map[string]bool, len(now))
	for _, w := range now {
		nowIDs[w.Metadata.ID] = true
	}
	wasIDs := make(map[string]bool, len(was))
	for _, id := range was {
		wasIDs[id] = true
	}

	// Left first, then arrived: a mover that steps from one plate straight onto
	// another should be off the first before it is on the second, or a rule
	// reading "who is standing on me" sees it in two places at once.
	for _, id := range was {
		if nowIDs[id] {
			continue
		}
		want := wantByID(allWants, id)
		if want == nil {
			// Gone from the board while it was being stood on. Nothing to take
			// it off of; dropping it from the record below is the whole job.
			continue
		}
		for _, rule := range intersectionRules {
			if rule.leave != nil && rule.applies(ctx, want, m) {
				rule.leave(ctx, want, m)
				announce(rule, want, m, "leave")
			}
		}
	}
	for _, want := range now {
		if wasIDs[want.Metadata.ID] {
			continue // still standing where it was; not an arrival
		}
		for _, rule := range intersectionRules {
			if rule.applies(ctx, want, m) {
				rule.enter(ctx, want, m)
				announce(rule, want, m, "enter")
			}
		}
	}

	if len(now) == 0 {
		delete(intersectionOn, m.key())
		return
	}
	ids := make([]string, 0, len(now))
	for _, w := range now {
		ids = append(ids, w.Metadata.ID)
	}
	intersectionOn[m.key()] = ids
}

// intersectionEvent is a rule having fired, told to whoever is watching.
//
// A rule does something to the board — a bin swallows a thing, a plate starts
// it moving — and the board has a way of SHOWING that: the bin's lid opens and
// something drops into it. Until now the only way in was the drag gesture, so
// the browser knew because it had just done it. A thing that slid into a bin on
// its own was archived in silence: the tile simply stopped existing, with the
// lid shut.
//
// So the rules say what they did. Deliberately about the rule rather than about
// the bin — "trash fired here, on this thing" — because that is the general
// fact, and the next rule with something to show gets its animation without a
// second event being invented for it.
type intersectionEvent struct {
	Rule  string `json:"rule"`
	Phase string `json:"phase"` // "enter" | "leave"
	// The want that reacted, and what arrived at it.
	WantID    string `json:"wantId"`
	WantType  string `json:"wantType"`
	MoverKind string `json:"moverKind"` // "character" | "thing"
	MoverID   string `json:"moverId"`
}

func announce(rule intersectionRule, want *mywant.Want, m mover, phase string) {
	kind := "character"
	if m.kind == moverThing {
		kind = "thing"
	}
	go broadcastSSE("intersection", intersectionEvent{
		Rule:      rule.name,
		Phase:     phase,
		WantID:    want.Metadata.ID,
		WantType:  want.Metadata.Type,
		MoverKind: kind,
		MoverID:   m.id,
	})
}

// forgetIntersections drops a mover's record without running any leave rules —
// for a mover that is no longer on the board at all, where "stepped off" would
// be the wrong story.
func forgetIntersections(m mover) {
	intersectionMu.Lock()
	delete(intersectionOn, m.key())
	intersectionMu.Unlock()
}

// wantsAtCell returns every want whose canvas labels put it on (x, y).
//
// Every want, not the first, and not only the ones some rule will end up
// caring about: which of them matter is decided per rule, and a cell with two
// wants on it is an ordinary board.
func wantsAtCell(x, y string, allWants []*mywant.Want) []*mywant.Want {
	var out []*mywant.Want
	for _, want := range allWants {
		if want.GetLabel(canvasLabelX) == x && want.GetLabel(canvasLabelY) == y {
			out = append(out, want)
		}
	}
	return out
}

func wantByID(allWants []*mywant.Want, id string) *mywant.Want {
	for _, w := range allWants {
		if w.Metadata.ID == id {
			return w
		}
	}
	return nil
}

// ── the rules ────────────────────────────────────────────────────────────────

// intersectionRules is the whole list. Order matters only where two rules touch
// the same state; occupancy runs first so that a rule firing on arrival can
// already see the mover in the want's own target array.
var intersectionRules = []intersectionRule{
	occupancyRule,
	goingRule,
	trashRule,
}

// occupancyRule keeps a want's target array equal to who is standing on it.
//
// A want type opts in by carrying `form-type: button` (the same label the GUI
// reads to draw it as a round, sinkable tile — see wantForm.ts) and declaring a
// `characters` or `things` array. Nothing here knows the names of the types
// that do: direction, going and gear today, whatever carries the label
// tomorrow.
var occupancyRule = intersectionRule{
	name: "occupancy",
	applies: func(ctx intersectionContext, want *mywant.Want, _ mover) bool {
		return ctx.isButton != nil && ctx.isButton(want.Metadata.Type)
	},
	enter: func(_ intersectionContext, want *mywant.Want, m mover) {
		addTargetToWant(want, m.targetField(), m.id)
	},
	leave: func(_ intersectionContext, want *mywant.Want, m mover) {
		removeTargetFromWant(want, m.targetField(), m.id)
	},
}

// goingRule is the pressure plate: stepping onto a going want flips the
// stepper's own moving flag, and stepping on it again flips it back. Stepping
// off does nothing — going, once set, stays set wherever the mover wanders
// next, which is why this rule has no leave.
//
// The two kinds are told apart only in how the flag is written, and the
// difference is real rather than a shortcut. A character's going lives on their
// own "character_motion" want, so the instruction has to reach that want's
// Progress to be carried out at all; queueing it as the same webhook payload
// the card sends is how a footstep and a card click stay one implementation
// rather than two that disagree. A thing's is a label, and a label is written
// by writing it.
var goingRule = intersectionRule{
	name: "going",
	applies: func(_ intersectionContext, want *mywant.Want, m mover) bool {
		return want.Metadata.Type == "going" && m.id != ""
	},
	enter: func(ctx intersectionContext, want *mywant.Want, m mover) {
		if m.kind == moverCharacter {
			want.AppendState("webhook_queue", map[string]any{
				"payload":    map[string]any{"action": "toggle", "character_id": m.id},
				"receivedAt": time.Now().Format(time.RFC3339Nano),
			})
			return
		}
		if ctx.s == nil || ctx.s.thingLabels == nil {
			return
		}
		next := "true"
		if ctx.s.thingLabels.Get(m.id)[thingMovingLabel] == "true" {
			next = "false"
		}
		_ = ctx.s.thingLabels.Set(m.id, thingMovingLabel, next)
		go broadcastSSE("thing_changed", m.id)
	},
}

// trashRule is the bin swallowing what walks into it.
//
// The board could already do this to a tile you DRAGGED onto the bin, decided
// on the frontend at the end of the gesture. That was the whole of it, which
// meant a bin was something you aimed at rather than something that was there:
// a thing carried onto it by its own motion went straight over the top. Same
// bin, same outcome, decided where the position is actually written.
//
// Things only. A character standing on a bin is a person standing on a bin;
// there is nothing to archive, and "you have been filed away" is not a thing to
// do to somebody without asking.
//
// Archive, not delete — a bin you cannot reach back into is a shredder. For a
// thing the archive IS the pin: `mywant.io/canvas: false` takes it off the
// board and the Thing list's archive drawer is where it comes back from.
var trashRule = intersectionRule{
	name: "trash",
	applies: func(ctx intersectionContext, want *mywant.Want, m mover) bool {
		return want.Metadata.Type == "trash" && m.kind == moverThing && ctx.s != nil
	},
	enter: func(ctx intersectionContext, _ *mywant.Want, m mover) {
		if ctx.s.thingLabels == nil {
			return
		}
		_ = ctx.s.thingLabels.Set(m.id, thingCanvasPinLabel, "false")
		// Stopped as well as archived. A thing that kept its speed while off
		// the board would go on travelling in the dark, and reappear at
		// whatever cell it had drifted to whenever someone un-archived it.
		_ = ctx.s.thingLabels.Set(m.id, thingMovingLabel, "false")
		// It left the board rather than moved on it, so this is the catalog
		// event, not the position one.
		go broadcastSSE("thing_changed", m.id)
	},
}
