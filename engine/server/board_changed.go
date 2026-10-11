package server

import (
	"sort"
	"sync"
	"time"
)

// What changed on the board, said once: one event, `board_changed`, for the
// wants and the things alike.
//
// There used to be two, `want_changed` (ids of wants created, updated,
// deleted) and `thing_changed` (a thing added, labelled, pinned, moved to
// rest), each answered on its own by whoever listened — and a thing a want
// names changes with the want (the board draws who uses it), which neither
// event said: the board re-read every thing whenever any want changed, just
// in case. Now a change says both halves:
//
//	{"wants": [...], "things": [...]}   or   {"all": true}
//
// and a want's change carries the things it names — before and after, so a
// thing it stopped naming is told too. Changes arriving close together are
// one event (boardChangeWindow), so a burst — a cluster dragged, a share of
// ten photos — is read once.
//
// `all` is a change no list can say: a world opened, every thing replaced.

// boardChangeWindow is how long changes gather before they are sent.
const boardChangeWindow = 150 * time.Millisecond

type boardChange struct {
	Wants  []string `json:"wants,omitempty"`
	Things []string `json:"things,omitempty"`
	All    bool     `json:"all,omitempty"`
}

type boardChangeHub struct {
	mu     sync.Mutex
	server *Server
	wants  map[string]bool
	things map[string]bool
	all    bool
	timer  *time.Timer
	// The things each want named when it was last told, so a want that
	// stops naming a thing tells that thing too.
	named map[string][]string
}

var boardChanges = &boardChangeHub{}

// attachBoardChanges gives the hub the server it reads usage from.
func (s *Server) attachBoardChanges() {
	boardChanges.mu.Lock()
	boardChanges.server = s
	boardChanges.mu.Unlock()
}

// boardWantsChanged says these wants changed (created, updated, deleted).
func boardWantsChanged(ids ...string) { boardChanges.add(ids, nil, false) }

// boardThingsChanged says these things changed.
func boardThingsChanged(ids ...string) { boardChanges.add(nil, ids, false) }

// boardAllChanged says everything may have changed (a world opened).
func boardAllChanged() { boardChanges.add(nil, nil, true) }

func (h *boardChangeHub) add(wants, things []string, all bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.wants == nil {
		h.wants, h.things = map[string]bool{}, map[string]bool{}
	}
	for _, id := range wants {
		if id != "" {
			h.wants[id] = true
		}
	}
	for _, id := range things {
		if id != "" {
			h.things[id] = true
		}
	}
	h.all = h.all || all
	if h.timer == nil {
		h.timer = time.AfterFunc(boardChangeWindow, h.flush)
	}
}

func (h *boardChangeHub) flush() {
	h.mu.Lock()
	wants, things, all, s := h.wants, h.things, h.all, h.server
	h.wants, h.things, h.all, h.timer = map[string]bool{}, map[string]bool{}, false, nil
	h.mu.Unlock()

	// The things the changed wants name, now and before.
	if len(wants) > 0 && s != nil && !all {
		now := s.thingsNamedByWants(wants)
		h.mu.Lock()
		if h.named == nil {
			h.named = map[string][]string{}
		}
		for w := range wants {
			for _, t := range h.named[w] {
				things[t] = true
			}
			for _, t := range now[w] {
				things[t] = true
			}
			if len(now[w]) > 0 {
				h.named[w] = now[w]
			} else {
				delete(h.named, w)
			}
		}
		h.mu.Unlock()
	}

	change := boardChange{Wants: boardSortedIDs(wants), Things: boardSortedIDs(things), All: all}
	if !change.All && len(change.Wants) == 0 && len(change.Things) == 0 {
		return
	}
	broadcastSSE("board_changed", change)
}

// thingsNamedBy is, for each of these wants, the ids of the things it names —
// from the same derivation the board's roads are drawn from (deriveThingUsage).
func (s *Server) thingsNamedByWants(wants map[string]bool) map[string][]string {
	out := map[string][]string{}
	if s.thingStore == nil {
		return out
	}
	idOf := map[string]string{}
	for _, e := range s.thingStore.Entries() {
		idOf[e.Catalog+"::"+e.Value] = e.ID
	}
	for _, u := range s.deriveThingUsage() {
		id := idOf[u.Catalog+"::"+u.Value]
		if id == "" {
			id = u.ID
		}
		for _, w := range u.WantIDs {
			if wants[w] {
				out[w] = append(out[w], id)
			}
		}
	}
	return out
}

func boardSortedIDs(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
