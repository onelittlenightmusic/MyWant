package server

import (
	"math"
	"strconv"

	mywant "mywant/engine/core"
)

// suggestFor turns "this form is one 所作 short here" into a move on the board.
//
// Returns nil unless the move is one the player could actually make by hand,
// which narrows it hard and on purpose:
//
//   - the missing piece is a THING of some subtype. A missing want is a real
//     gap too, but it is answered by placing a new want rather than by joining
//     two things, and that is a different offer with a different drawing.
//   - something of this scope is ON the board, so the offer has a place to
//     start from.
//   - something that would fill the gap is on the board too, so it has a place
//     to end. A value that is merely remembered is not a move, it is a
//     shopping list.
//
// What comes back is one pair of tiles: put that one in with this one, and the
// form holds. The board can draw that as a line, which is the whole point —
// "add a city to 国分寺" is advice, and a line between two tiles is a move.
func (s *Server) suggestFor(
	k mywant.Kata,
	waza []mywant.Waza,
	progress []WazaProgress,
	scope *thingScope,
) *KataSuggestion {
	if scope == nil || s.thingStore == nil || s.thingLabels == nil {
		return nil
	}
	// The one that is missing. There is exactly one — the caller checked.
	idx := -1
	for i := range progress {
		if !progress[i].Satisfied {
			if idx >= 0 {
				return nil
			}
			idx = i
		}
	}
	if idx < 0 || idx >= len(waza) {
		return nil
	}
	wz := waza[idx]
	if wz.Kind != "thing" || wz.Subtype == "" {
		return nil
	}

	labels := s.thingLabels.All()
	onBoard := func(id string) bool {
		l := labels[id]
		return l != nil && l[thingCanvasPinLabel] == "true"
	}

	// Where the offer starts: a member of this scope that is actually drawn.
	var anchors []string
	for _, id := range scope.MemberIDs {
		if onBoard(id) {
			anchors = append(anchors, id)
		}
	}
	if len(anchors) == 0 {
		return nil
	}

	// Where it could end: a thing of the missing subtype, on the board, not
	// already part of this scope.
	inScope := map[string]bool{}
	for _, id := range scope.MemberIDs {
		inScope[id] = true
	}
	key := thingCatalogKey(wz.Subtype)
	var candidates []string
	for _, e := range s.thingStore.Entries() {
		if e.Catalog != key || inScope[e.ID] || !onBoard(e.ID) {
			continue
		}
		candidates = append(candidates, e.ID)
	}
	if len(candidates) == 0 {
		return nil
	}

	// The nearest pair, and only that one.
	//
	// A board with twenty stations and one city would otherwise offer the same
	// city twenty times over, and twenty ghost lines converging on one tile is
	// not a suggestion, it is weather. One line per form per place, drawn
	// between the two tiles that are actually closest to each other, is a
	// thing a player can look at and answer.
	bestA, bestB, bestD := "", "", math.Inf(1)
	for _, a := range anchors {
		ax, ay, ok := boardPos(labels[a])
		if !ok {
			continue
		}
		for _, b := range candidates {
			bx, by, ok := boardPos(labels[b])
			if !ok {
				continue
			}
			if d := math.Hypot(bx-ax, by-ay); d < bestD {
				bestA, bestB, bestD = a, b, d
			}
		}
	}
	if bestA == "" || bestB == "" {
		return nil
	}

	return &KataSuggestion{
		KataID:        k.ID,
		Name:          k.Name,
		Mark:          k.Mark,
		Constellation: scope.Name,
		Lone:          scope.Lone,
		Kind:          wz.Kind,
		Subtype:       wz.Subtype,
		Hint:          progress[idx].Hint,
		Anchor:        bestA,
		Candidate:     bestB,
	}
}

// boardPos reads a thing's canvas cell off its labels.
func boardPos(l map[string]string) (x, y float64, ok bool) {
	if l == nil {
		return 0, 0, false
	}
	fx, errX := strconv.ParseFloat(l["mywant.io/canvas-x"], 64)
	fy, errY := strconv.ParseFloat(l["mywant.io/canvas-y"], 64)
	if errX != nil || errY != nil {
		return 0, 0, false
	}
	return fx, fy, true
}
