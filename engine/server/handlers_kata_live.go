package server

import (
	"net/http"
	"sort"
	"strings"

	mywant "mywant/engine/core"
)

// A live kata is a constellation that is currently burning: the wants and memo
// values satisfying it are all on the board at this moment. The canvas and the
// minimap both draw from this one payload — the canvas uses all of it, the
// minimap keeps the want half, since it has no memo layer to place stars on.
//
// This is deliberately not folded into GET /api/v1/wants. Drawing a
// constellation needs the EDGES, and a per-want label cannot say who to join to
// without the client re-deriving the whole evaluation. The wants do carry a
// `kata/<id>` label for filtering (see annotateKataLabels); the shape lives here.

// kataEdge joins two members of a live constellation.
type kataEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Kind is "relation" when a value actually flows between the two — the road
	// already drawn on the ground — and "waza" when they are merely part of the
	// same form. The client can then light an existing road rather than laying a
	// second line beside it.
	Kind string `json:"kind"`
	// Field names the exposed state a "relation" edge carries.
	Field string `json:"field,omitempty"`
}

// kataConstellationDTO is one live kata, ready to draw.
type kataConstellationDTO struct {
	ID            string `json:"id"`
	KataID        string `json:"kataID"`
	Name          string `json:"name"`
	Reading       string `json:"reading,omitempty"`
	Level         string `json:"level"`
	LevelName     string `json:"levelName,omitempty"`
	Constellation string `json:"constellation,omitempty"`
	// Color is the belt's colour: a live kata is drawn in the colour of the belt
	// it belongs to, which is what tells two burning forms apart at a glance.
	Color  string `json:"color,omitempty"`
	Accent string `json:"accent,omitempty"`

	Satisfied int  `json:"satisfied"`
	Total     int  `json:"total"`
	Locked    bool `json:"locked"`
	Recorded  bool `json:"recorded"`
	Mastery   int  `json:"mastery"`

	WantIDs []string   `json:"wantIDs"`
	MemoIDs []string   `json:"memoIDs"`
	Edges   []kataEdge `json:"edges"`
}

// listLiveKata returns every kata standing right now, as constellations.
func (s *Server) listLiveKata(w http.ResponseWriter, r *http.Request) {
	_, kata := s.evaluateKata()

	levelByID := map[string]mywant.KataLevel{}
	for _, lv := range mywant.ListKataLevels() {
		levelByID[lv.ID] = lv
	}

	out := make([]kataConstellationDTO, 0, 4)
	for _, k := range kata {
		if !k.Live || k.Masked {
			continue
		}
		// A form with no want in it is not burning on the board — it is a set of
		// names, which the memo constellation already draws in its own pale
		// white. Lighting it again in the belt's colour would say that naming
		// three stations is the same event as a kata coming together.
		if len(k.LiveWantIDs) == 0 {
			continue
		}
		lv := levelByID[k.Level]
		out = append(out, kataConstellationDTO{
			ID:            k.KataID + "\x00" + k.Constellation,
			KataID:        k.KataID,
			Name:          k.Name,
			Reading:       k.Reading,
			Level:         k.Level,
			LevelName:     lv.Name,
			Constellation: k.Constellation,
			Color:         lv.Color,
			Accent:        lv.Accent,
			Satisfied:     k.Satisfied,
			Total:         k.Total,
			Locked:        k.Locked,
			Recorded:      k.Recorded,
			Mastery:       k.Mastery,
			WantIDs:       orEmpty(k.LiveWantIDs),
			MemoIDs:       orEmpty(k.LiveMemo),
			Edges:         s.kataEdges(k.LiveMemo, k.LiveWantIDs),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].ID < out[j].ID
	})

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"constellations": out,
		"count":          len(out),
	})
}

// orEmpty keeps a list a list in JSON, so the client never has to tell an empty
// constellation from a null one.
func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// kataEdges joins the members of one live kata.
//
// Where a relation already exists between two member wants, that relation is the
// edge — the form rides on the road that is already there instead of drawing a
// second line beside it. Whatever is left over is chained memo-first, one link
// per neighbour: a complete graph turns four stars into a blot, which is why the
// memo constellations on the canvas are chained too.
func (s *Server) kataEdges(memoIDs, wantIDs []string) []kataEdge {
	members := append(append([]string{}, memoIDs...), wantIDs...)
	if len(members) < 2 {
		return nil
	}

	inKata := map[string]bool{}
	for _, id := range wantIDs {
		inKata[id] = true
	}

	edges := make([]kataEdge, 0, len(members))
	joined := map[string]bool{} // unordered pair key → already has an edge
	pairKey := func(a, b string) string {
		if a > b {
			a, b = b, a
		}
		return a + "\x00" + b
	}

	for _, rel := range s.buildRelations(func(providerID, consumerID string) bool {
		return inKata[providerID] && inKata[consumerID]
	}) {
		key := pairKey(rel.ProviderID, rel.ConsumerID)
		if joined[key] {
			continue
		}
		joined[key] = true
		edges = append(edges, kataEdge{
			From:  rel.ProviderID,
			To:    rel.ConsumerID,
			Kind:  "relation",
			Field: rel.FieldName,
		})
	}

	for i := 1; i < len(members); i++ {
		a, b := members[i-1], members[i]
		key := pairKey(a, b)
		if joined[key] {
			continue
		}
		joined[key] = true
		edges = append(edges, kataEdge{From: a, To: b, Kind: "waza"})
	}

	return edges
}

// kataLabelPrefix marks a label as derived from a live kata rather than set by
// the user. The value is the constellation the kata stands in, so "which form"
// is the key and "about what" is the value.
const kataLabelPrefix = "kata/"

// kataLabelsByWant maps want id → the derived kata labels it should carry.
//
// These are never written onto the live wants: they are facts about the board at
// this instant, and persisting them would leave stale forms behind in state.yaml
// that would then have to be stripped on the way back in.
func (s *Server) kataLabelsByWant() map[string]map[string]string {
	_, kata := s.evaluateKata()

	byWant := map[string]map[string]string{}
	for _, k := range kata {
		if !k.Live || k.Masked {
			continue
		}
		for _, id := range k.LiveWantIDs {
			if byWant[id] == nil {
				byWant[id] = map[string]string{}
			}
			byWant[id][kataLabelPrefix+k.KataID] = k.Constellation
		}
	}
	return byWant
}

// kataLabelFingerprint is a stable digest of the whole annotation, for folding
// into the want collection's ETag. Without it a kata catching fire changes
// nothing the ETag can see, and the client keeps serving the unlit board.
func kataLabelFingerprint(byWant map[string]map[string]string) string {
	if len(byWant) == 0 {
		return ""
	}
	parts := make([]string, 0, len(byWant))
	for wantID, labels := range byWant {
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		line := wantID
		for _, k := range keys {
			line += "\x00" + k + "=" + labels[k]
		}
		parts = append(parts, line)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x01")
}
