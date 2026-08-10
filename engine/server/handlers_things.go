package server

import (
	"errors"
	"net/http"
	"os"
	"sort"
	"strconv"

	"github.com/gorilla/mux"

	mywant "mywant/engine/core"
)

// GET /api/v1/memo/suggestions/{subtype}
// Returns recorded values for a subtype, most-recent first.
// Query param: limit (default 20)
func (s *Server) getThingSuggestions(w http.ResponseWriter, r *http.Request) {
	subtype := mux.Vars(r)["subtype"]
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	suggestions := s.thingStore.Suggestions(subtype, limit)
	if suggestions == nil {
		suggestions = []string{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"subtype":     subtype,
		"suggestions": suggestions,
	})
}

// ThingFull is everything about one remembered value, in one place.
//
// The catalogs on disk hold only names, so for a long time a client wanting to
// draw a thing had to fetch the names, then the data types for its icon and
// colour, then the labels for where it sits, then the usage for which wants
// name it, then the stats — five round trips and a catalog→type reverse lookup
// that ended up reimplemented in three different files. All of that is derived
// from state this server already holds, so it belongs in the one GET.
type ThingFull struct {
	// ID is "<catalog>::<value>" — the id used everywhere a thing is referred to.
	ID      string `json:"id"`
	Catalog string `json:"catalog"`
	Subtype string `json:"subtype"`
	Value   string `json:"value"`

	Icon  string `json:"icon"`
	Color string `json:"color"`

	// Definitions are the names given to this value, by every character — the
	// ledger's word, not any one character's copy.
	Definitions []ThingDefinition `json:"definitions,omitempty"`
	Stats       *MemoStat         `json:"stats,omitempty"`
	// WantIDs are the live wants naming this value right now.
	WantIDs []string          `json:"wantIDs,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// GET /api/v1/things
// Every remembered value, whole: name, type, who named it, where it sits, and
// which wants are naming it.
func (s *Server) getThings(w http.ResponseWriter, _ *http.Request) {
	types := DataTypeDefinitions()

	usageByID := map[string][]string{}
	for _, u := range s.deriveThingUsage() {
		usageByID[u.ID] = u.WantIDs
	}

	// Definitions are keyed by the name they gave, which is the catalog entry
	// itself: naming a value is what puts it in the catalog.
	defsByID := map[string][]ThingDefinition{}
	for _, d := range s.deriveThingDefinitions() {
		id := d.Catalog + "::" + d.Name
		defsByID[id] = append(defsByID[id], d)
	}

	stats := map[string]map[string]MemoStat{}
	if s.thingEvents != nil {
		stats = s.thingEvents.Stats()
	}
	labels := s.thingLabels.All()

	out := []ThingFull{}
	for _, e := range s.thingStore.Entries() {
		if e.Value == "" {
			continue
		}
		subtype := keyToSubtype(e.Catalog)
		info := types[subtype]
		// Labels and usage hang off the thing's own id; definitions, stats and
		// usage are derived from what a want NAMED, which is a catalog and a
		// value, so those are still looked up by the pair.
		pair := e.Catalog + "::" + e.Value
		t := ThingFull{
			ID:          e.ID,
			Catalog:     e.Catalog,
			Subtype:     subtype,
			Value:       e.Value,
			Icon:        info.Icon,
			Color:       info.Color,
			Definitions: defsByID[pair],
			WantIDs:     usageByID[pair],
			Labels:      labels[e.ID],
		}
		if t.Icon == "" {
			t.Icon = "Type"
		}
		if t.Color == "" {
			t.Color = "#64748b"
		}
		if st, ok := stats[e.Catalog][e.Value]; ok {
			t.Stats = &st
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Catalog != out[j].Catalog {
			return out[i].Catalog < out[j].Catalog
		}
		return out[i].Value < out[j].Value
	})

	s.JSONResponse(w, http.StatusOK, map[string]any{"things": out})
}

// GET /api/v1/memo/subtypes
// Backward-compat alias — redirects callers to /api/v1/datatypes response shape.
func (s *Server) getThingSubtypes(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"subtypes": DataTypeDefinitions(),
	})
}

// GET /api/v1/memo/events
// Returns memo provenance events, most-recent first.
// Query params: limit (default 200); catalog + value to filter to one named value.
func (s *Server) getThingEvents(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	catalog := r.URL.Query().Get("catalog")
	value := r.URL.Query().Get("value")

	var events []ThingEvent
	if catalog != "" && value != "" {
		events = s.thingEvents.ForValue(catalog, value, limit)
	} else {
		events = s.thingEvents.All(limit)
	}
	if events == nil {
		events = []ThingEvent{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"events": events})
}

// getThingDefinitions handles GET /api/v1/things/definitions
//
// Every name in force, from every character — the ledger read back. This is
// what the frontend should ask for rather than walking each character's
// auraDefaults: a name belongs to the thing it names, not to the character
// who happened to give it.
//
// Marks made before the ledger carried NamedValue are folded in from
// characters.yaml, so nothing that was already named goes missing while both
// stores are in use. The ledger wins wherever they overlap.
func (s *Server) getThingDefinitions(w http.ResponseWriter, _ *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{"definitions": s.deriveThingDefinitions()})
}

// deriveThingDefinitions is the derivation itself, shared with getThings so a
// caller never has to ask twice for the same answer.
func (s *Server) deriveThingDefinitions() []ThingDefinition {
	defs := []ThingDefinition{}
	if s.thingEvents != nil {
		defs = s.thingEvents.Definitions()
	}

	at := make(map[string]int, len(defs))
	for i, d := range defs {
		at[d.CharacterID+"\x00"+d.Subtype+"\x00"+d.Name] = i
	}
	for _, c := range mywant.ListCharacters() {
		for _, mark := range c.AuraDefaults {
			if mark.Target.IsBinding() || mark.Target.Name == "" {
				continue
			}
			k := c.ID + "\x00" + mark.Target.Kind + "\x00" + mark.Target.Name
			if i, ok := at[k]; ok {
				// The ledger knows this name but was written before it carried
				// the value — fill it in rather than skipping, or a name that
				// predates NamedValue would read as pointing at nothing.
				if defs[i].Value == nil {
					defs[i].Value = mark.Value
				}
				continue
			}
			defs = append(defs, ThingDefinition{
				Catalog:       subtypeToKey(mark.Target.Kind),
				Subtype:       mark.Target.Kind,
				Name:          mark.Target.Name,
				Value:         mark.Value,
				CharacterID:   c.ID,
				CharacterName: c.Name,
			})
		}
	}

	// A name with nothing behind it is not a definition. These are names given
	// before the ledger recorded values and since taken back, so neither store
	// has the value any more — they would show up as a name pointing at
	// nothing. Dropped from what is in force, NOT from the ledger: the lines
	// recording that they were once named stay where they are.
	inForce := make([]ThingDefinition, 0, len(defs))
	for _, d := range defs {
		if d.Value == nil {
			continue
		}
		inForce = append(inForce, d)
	}
	return inForce
}

// GET /api/v1/memo/stats
// Per-value usage stats (count + lastUsed), derived from the event log, keyed
// by catalog then value.
func (s *Server) getThingStats(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{"stats": s.thingEvents.Stats()})
}

// POST /api/v1/things   body: {catalog, value}
// Remembers one value. Returns the thing, identity included.
func (s *Server) createThing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Catalog string `json:"catalog"`
		Value   string `json:"value"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if body.Catalog == "" || body.Value == "" {
		s.JSONError(w, r, http.StatusBadRequest, "catalog and value are required", "")
		return
	}
	catalog := body.Catalog
	if _, isSubtype := DataTypeDefinitions()[catalog]; isSubtype {
		catalog = subtypeToKey(catalog)
	}
	entry, err := s.thingStore.Add(catalog, body.Value)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to save thing", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, entry)
}

// PATCH /api/v1/things/{id}   body: {catalog?, value?}
//
// The operation the old whole-catalog PUT could not express: change what a
// thing is filed under, or what it is called, WITHOUT changing which thing it
// is. Everything pointing at it — where it sits on the board, which group it
// joined — is keyed by the id and so simply keeps pointing.
func (s *Server) patchThing(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Catalog string `json:"catalog"`
		Value   string `json:"value"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	// Callers say either the catalog key ("landmarks", what the GUI has in
	// hand) or the subtype name ("landmark", what a person types). Normalise,
	// or the same move made from two places files the thing under two
	// different catalogs.
	catalog := body.Catalog
	if _, isSubtype := DataTypeDefinitions()[catalog]; isSubtype {
		catalog = subtypeToKey(catalog)
	}
	entry, err := s.thingStore.Update(id, catalog, body.Value)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.JSONError(w, r, http.StatusNotFound, "thing not found", id)
		return
	case errors.Is(err, os.ErrExist):
		s.JSONError(w, r, http.StatusConflict, "that catalog already holds that value", id)
		return
	case err != nil:
		s.JSONError(w, r, http.StatusInternalServerError, "failed to update thing", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, entry)
}

// DELETE /api/v1/things/{id}
func (s *Server) deleteThing(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.thingStore.Delete(id); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to delete thing", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "thing deleted", "id": id})
}

// PUT /api/v1/memo
// Replaces the entire memo with the provided map[string][]string.
//
// Kept for callers that still speak in whole catalogs. It cannot express
// identity — a value that moves between catalogs across two of these looks
// like a delete and an unrelated create — so the store preserves the id of any
// catalog/value pair that survives the write, and anything genuinely new gets
// a fresh one. Prefer POST/PATCH/DELETE above.
func (s *Server) putThings(w http.ResponseWriter, r *http.Request) {
	var data map[string][]string
	if err := DecodeRequest(r, &data); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := s.thingStore.Replace(data); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to save memo", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "memo updated"})
}
