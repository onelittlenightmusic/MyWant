package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	mywant "mywant/engine/core"
)

// A live kata is a constellation that is currently burning: the wants and things
// satisfying it are all on the board at this moment. The canvas and the
// minimap both draw from this one payload — the canvas uses all of it, the
// minimap keeps the want half, since it has no thing layer to place stars on.
//
// This is deliberately not folded into GET /api/v1/wants. Drawing a
// constellation needs the EDGES, and a per-want label cannot say who to join to
// without the client re-deriving the whole evaluation. The wants do carry a
// `kata/<id>` label for filtering (see kataLabelsByWant); the shape lives here.

// kataEdge joins two members of a live constellation.
//
// Only thing↔thing and thing↔want edges are ever drawn. A constellation is not a
// graph of everything that touches: the remembered values are what the form is
// ABOUT, so they are its hubs, and joining two wants directly would say they
// have something to do with each other when all they share is the place.
type kataEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
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
	ThingIDs []string   `json:"thingIDs"`
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
		// names, which the thing constellation already draws in its own pale
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
			ThingIDs:       orEmpty(k.LiveThings),
			Edges:         s.kataEdges(k.LiveThings, k.LiveWantIDs),
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

// kataEdges joins the members of one live kata: the things chain to each
// other, and every want hangs off the thing it names.
//
// Chained rather than fully connected — a complete graph turns four stars into a
// blot, which is why the thing constellations on the canvas are chained too — and
// spoked from the thing side because a remembered thing is what a form is about.
// Two wants are never joined to each other: they share a place, not a thread.
func (s *Server) kataEdges(thingIDs, wantIDs []string) []kataEdge {
	if len(thingIDs)+len(wantIDs) < 2 {
		return nil
	}

	edges := make([]kataEdge, 0, len(thingIDs)+len(wantIDs))

	for i := 1; i < len(thingIDs); i++ {
		edges = append(edges, kataEdge{From: thingIDs[i-1], To: thingIDs[i]})
	}

	// A kata with no thing 所作 (糧: a restaurant and a budget, joined by nothing
	// the system can check) has no hub to hang from. Chaining its wants is the
	// only way it reads as one form rather than as unrelated lights.
	if len(thingIDs) == 0 {
		for i := 1; i < len(wantIDs); i++ {
			edges = append(edges, kataEdge{From: wantIDs[i-1], To: wantIDs[i]})
		}
		return edges
	}

	thingByValue := map[string]string{}
	for _, id := range thingIDs {
		if _, value, ok := strings.Cut(id, "::"); ok {
			thingByValue[value] = id
		}
	}

	for _, wantID := range wantIDs {
		hubs := s.thingsNamedBy(wantID, thingByValue)
		if len(hubs) == 0 {
			// It satisfied the form inside this constellation, so it belongs to
			// it even when the naming is indirect (a venue resolved at runtime,
			// say). Hang it off the first value rather than leaving it adrift.
			hubs = []string{thingIDs[0]}
		}
		for _, hub := range hubs {
			edges = append(edges, kataEdge{From: hub, To: wantID})
		}
	}

	return edges
}

// thingsNamedBy returns the member ids of the things this want names in its
// parameters — the values it is pointed at, which is what makes it part of the
// form rather than merely present.
func (s *Server) thingsNamedBy(wantID string, thingByValue map[string]string) []string {
	if s.globalBuilder == nil {
		return nil
	}
	var want *mywant.Want
	for _, w := range s.globalBuilder.GetAllWantStates() {
		if w.Metadata.ID == wantID {
			want = w
			break
		}
	}
	if want == nil || want.Spec.Params == nil {
		return nil
	}

	seen := map[string]bool{}
	var out []string
	for _, v := range want.Spec.Params {
		value := strings.TrimSpace(fmt.Sprintf("%v", v))
		if value == "" {
			continue
		}
		if id, ok := thingByValue[value]; ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
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
