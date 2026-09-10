package server

import (
	"fmt"
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
	matchedByType map[string][]string,
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

	// A missing WANT is answered by putting one down, not by joining two things
	// that exist — so the offer stops here, with somewhere to draw it and the
	// type to draw. Where exactly the tile goes is the board's business: it
	// knows which cells are free, and the engine does not.
	if wz.Kind == "want_type" {
		if wz.Type == "" {
			return nil
		}
		// Where the offer is drawn from.
		//
		// A plain want offer belongs beside the group it is about, so a thing
		// of that group anchors it. A RELATED one does not: "wire the route's
		// departure into a reminder" is a move made at the route, and drawn
		// beside a station it points at the wrong end of its own sentence. So
		// when the form names another want — and an earlier 所作 has already
		// settled which one counts — the offer stands next to that want.
		anchor, anchorKind := anchors[0], "thing"
		if rel := relatedWantType(wz); rel != "" {
			if ids := matchedByType[rel]; len(ids) > 0 {
				anchor, anchorKind = ids[0], "want"
			}
		}
		return &KataSuggestion{
			KataID: k.ID, Name: k.Name, Mark: k.Mark,
			Constellation: scope.Name, Lone: scope.Lone,
			Kind: wz.Kind, Type: wz.Type,
			Hint:       progress[idx].Hint,
			Anchor:     anchor,
			AnchorKind: anchorKind,
		}
	}
	if wz.Kind != "thing" || wz.Subtype == "" {
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
		AnchorKind:    "thing",
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

// importsFrom reports whether `consumer` takes one of its parameters from a
// want of the named type, out of the named state.
//
// The wire is spelled in two halves and both must be there: the provider
// declares `exposes: [{ currentState: departure, as: <key> }]`, and the
// consumer declares `imports: { <key>: event_time }`. Matching them here rather
// than trusting either side alone is the difference between "a route and an
// alarm are both on the board" and "this alarm goes off when that route says
// to leave".
//
// When a 所作 earlier in the same form has already settled which wants of the
// provider's type count — the route that arrives in THIS group, not any route
// — the wire has to run to one of those. Otherwise any want of the type will
// do, which is the right answer for a form that never pinned one.
func (s *Server) importsFrom(
	consumer *mywant.Want,
	req mywant.WazaImport,
	wantsByType map[string][]*mywant.Want,
	matchedByType map[string][]string,
) bool {
	if req.Type == "" || req.State == "" || consumer == nil || len(consumer.Spec.Imports) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, id := range matchedByType[req.Type] {
		allowed[id] = true
	}
	for globalKey, localKey := range consumer.Spec.Imports {
		// Pinned to one inlet, when the form says which.
		if req.Into != "" && fmt.Sprintf("%v", localKey) != req.Into {
			continue
		}
		for _, provider := range wantsByType[req.Type] {
			if provider == nil || provider.Metadata.ID == consumer.Metadata.ID {
				continue
			}
			if len(allowed) > 0 && !allowed[provider.Metadata.ID] {
				continue
			}
			for _, exp := range provider.Spec.Exposes {
				if exp.As == globalKey && exp.CurrentState == req.State {
					return true
				}
			}
		}
	}
	return false
}

// owns reports whether `parent` is the controlling owner of a want of the
// named type.
//
// The relation a budget is fed by: it adds up what the wants under it report,
// so "a budget and a route on the same board" is not the form and "a budget
// the route is under" is. Pinned providers narrow it the same way importsFrom
// does — the route that arrives HERE, not any route.
func (s *Server) owns(
	parent *mywant.Want,
	req mywant.WazaOwns,
	wantsByType map[string][]*mywant.Want,
	matchedByType map[string][]string,
) bool {
	if req.Type == "" || parent == nil {
		return false
	}
	allowed := map[string]bool{}
	for _, id := range matchedByType[req.Type] {
		allowed[id] = true
	}
	for _, child := range wantsByType[req.Type] {
		if child == nil || child.Metadata.ID == parent.Metadata.ID {
			continue
		}
		if len(allowed) > 0 && !allowed[child.Metadata.ID] {
			continue
		}
		for _, ref := range child.Metadata.OwnerReferences {
			if ref.Kind == "Want" && ref.Controller && ref.ID == parent.Metadata.ID {
				return true
			}
		}
	}
	return false
}

// relatedWantType is the want type a waza is tied to, if it is tied to one at
// all — the other end of a wire, or the child a parent must own.
func relatedWantType(wz mywant.Waza) string {
	switch {
	case wz.ImportFrom != nil:
		return wz.ImportFrom.Type
	case wz.ParamFrom != nil:
		return wz.ParamFrom.Type
	case wz.Owns != nil:
		return wz.Owns.Type
	}
	return ""
}

// paramsFrom is importsFrom for a parameter inlet.
//
// Same wire, different plumbing, because the board has two of them. A state
// field is fed by `imports: { <globalKey>: <stateKey> }`; a parameter is fed by
// `params: { <p>: { fromGlobalParam: <globalKey> } }`, resolved into the want's
// effective parameters — which is what a want's own code reads. Matching the
// wrong one draws a line and feeds nothing.
func (s *Server) paramsFrom(
	consumer *mywant.Want,
	req mywant.WazaImport,
	wantsByType map[string][]*mywant.Want,
	matchedByType map[string][]string,
) bool {
	if req.Type == "" || req.State == "" || consumer == nil || len(consumer.Spec.Params) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, id := range matchedByType[req.Type] {
		allowed[id] = true
	}
	for name, v := range consumer.Spec.Params {
		if req.Into != "" && name != req.Into {
			continue
		}
		ref, ok := v.(map[string]any)
		if !ok {
			continue
		}
		key, ok := ref["fromGlobalParam"].(string)
		if !ok || key == "" {
			continue
		}
		for _, provider := range wantsByType[req.Type] {
			if provider == nil || provider.Metadata.ID == consumer.Metadata.ID {
				continue
			}
			if len(allowed) > 0 && !allowed[provider.Metadata.ID] {
				continue
			}
			for _, exp := range provider.Spec.Exposes {
				if exp.AsGlobalParam == key && exp.CurrentState == req.State {
					return true
				}
			}
		}
	}
	return false
}
