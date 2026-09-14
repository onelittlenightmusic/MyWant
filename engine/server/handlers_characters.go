package server

import (
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"strings"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// listCharacters returns all characters.
func (s *Server) listCharacters(w http.ResponseWriter, r *http.Request) {
	characters := mywant.ListCharacters()
	if characters == nil {
		characters = []mywant.Character{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"characters": characters,
		"count":      len(characters),
	})
}

// getCharacter returns a single character by ID.
func (s *Server) getCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	c, ok := mywant.GetCharacter(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, c)
}

// createCharacter adds a new character.
func (s *Server) createCharacter(w http.ResponseWriter, r *http.Request) {
	var c mywant.Character
	if err := DecodeRequest(r, &c); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if c.Name == "" {
		s.JSONError(w, r, http.StatusBadRequest, "name is required", "")
		return
	}
	created := mywant.AddCharacter(c)
	// A character arrives with a mouth. Made here rather than left for the next
	// restart so somebody can be talked to the moment they exist.
	s.ensureCharacterChatWant(created.ID)
	// And legs to walk with — see character_motion_wants.go.
	s.ensureCharacterMotionWant(created.ID)
	go broadcastSSE("character_changed", created.ID)
	s.JSONResponse(w, http.StatusCreated, created)
}

// updateCharacter replaces a character's editable fields (name, avatar, color).
// AssignedDeviceIDs are preserved server-side and not overwritten by this endpoint.
func (s *Server) updateCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updated mywant.Character
	if err := DecodeRequest(r, &updated); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if !mywant.UpdateCharacter(id, updated) {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	c, _ := mywant.GetCharacter(id)
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// deleteCharacter removes a character. Assigned devices become unassigned automatically
// because AssignedDeviceIDs are stored on the character record.
func (s *Server) deleteCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !mywant.DeleteCharacter(id) {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	// Their mouth goes with them. It is a protected system want, so nothing else
	// would ever remove it — a deleted character would leave a chat want behind
	// that no character claims and no API call can clear.
	s.removeCharacterChatWant(id)
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "deleted", "id": id})
}

// assignDevicesToCharacter sets the device assignment for a character atomically.
// Any device in the request body is removed from its previous character first.
// Body: { "deviceIds": ["device-uuid-1", ...] }
func (s *Server) assignDevicesToCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		DeviceIDs []string `json:"deviceIds"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if req.DeviceIDs == nil {
		req.DeviceIDs = []string{}
	}
	c, ok := mywant.AssignDevicesToCharacter(id, req.DeviceIDs)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// setCharacterAuraDefault marks (or, with an empty value, clears) an aura mark
// for a character. It accepts either of two shapes:
//
//   - BINDING (want-field shorthand): { wantId, section, key, value, mode }.
//     The caller names the want it is marking; what gets stored is that want's
//     *type* plus section/key — an address that means the same thing in any
//     install. The instance ID is only used to look the type up here.
//
//   - DEFINITION (explicit target): { target: {kind, name, path}, value }.
//     value may be an object — the definition the target's name resolves to
//     (e.g. a place → {lat, lng, radius}). Nothing is applied to a want.
//
// value is decoded as an arbitrary JSON value so a definition can carry an
// object while a binding carries a scalar. An empty value clears the mark.
func (s *Server) setCharacterAuraDefault(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		// binding shorthand
		WantID  string `json:"wantId"`
		Section string `json:"section"`
		Key     string `json:"key"`
		Mode    string `json:"mode"`
		// explicit target (definitions, and any non-want-field mark)
		Target *mywant.AuraTarget `json:"target"`
		// value: scalar for a binding, object for a definition
		Value any `json:"value"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	var target mywant.AuraTarget
	switch {
	case req.Target != nil:
		target = *req.Target
	case req.WantID != "":
		want, _, found := s.globalBuilder.FindWantByID(req.WantID)
		if !found {
			s.JSONError(w, r, http.StatusNotFound, "Want not found", req.WantID)
			return
		}
		if want.Metadata.Type == "" {
			s.JSONError(w, r, http.StatusBadRequest, "Want has no type to address the mark to", req.WantID)
			return
		}
		target = mywant.AuraTarget{
			Kind: mywant.AuraTargetKindWantType,
			Name: want.Metadata.Type,
			Path: req.Section + "/" + req.Key,
		}
	default:
		s.JSONError(w, r, http.StatusBadRequest, "Either target or wantId is required", "")
		return
	}
	if !target.Valid() {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid aura target (kind and name are required)", "")
		return
	}

	// A definition is written to the ledger only — the thing is the record, and
	// a second copy on the character is what made the two drift apart. An empty
	// value still goes through SetCharacterAuraDefault so that definitions
	// written before this change are cleared from the character as they are
	// un-named. Bindings are the character's own and stay where they are.
	mark := mywant.AuraMark{Target: target, Value: req.Value, Mode: req.Mode}
	if !target.IsBinding() {
		mark.Value = nil
	}
	c, ok := mywant.SetCharacterAuraDefault(id, mark)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	// A definition mark (a catalog kind, not a want-type binding) names a value.
	// Both giving and taking back a name are written to the ledger: it is the
	// record of what has been named, so it has to hear about both, and an empty
	// value is how this endpoint spells "un-name it".
	if !target.IsBinding() {
		if req.Value != nil && req.Value != "" {
			if err := s.thingStore.Record(target.Kind, target.Name); err != nil {
				mywant.WarnLog("[aura] failed to record memo %s=%q: %v", target.Kind, target.Name, err)
			}
			s.recordThingEvent(target.Kind, target.Name, ThingSourceAuraDefinition, req.WantID, c, req.Value)
		} else {
			s.recordThingEvent(target.Kind, target.Name, ThingSourceAuraDefinitionRemoved, req.WantID, c, nil)
		}
	}
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// recordThingEvent appends one line to the thing ledger.
//
// The ledger is append-only and every character's marks land in it, so reading
// it back gives the whole story of what has been named, by whom, when and out
// of which want — rather than the current state of one character's marks.
// subtype is the catalog kind; value is the NAME; namedValue is what that name
// was given to; wantID, when present, is resolved to its want name and type.
// A nil event store is a no-op.
func (s *Server) recordThingEvent(subtype, value, source, wantID string, c *mywant.Character, namedValue any) {
	if s.thingEvents == nil || value == "" {
		return
	}
	ev := ThingEvent{
		Catalog:    subtypeToKey(subtype),
		Subtype:    subtype,
		Value:      value,
		Source:     source,
		NamedValue: namedValue,
	}
	if c != nil {
		ev.CharacterID = c.ID
		ev.CharacterName = c.Name
	}
	if wantID != "" {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			ev.WantID = want.Metadata.ID
			ev.WantType = want.Metadata.Type
		}
	}
	_ = s.thingEvents.Record(ev)
}

// setCharacterAuraCardWant sets (or, with an empty wantId, clears) the want a
// character has bookmarked as their aura card.
// Body: { "wantId": "want-id" }
func (s *Server) setCharacterAuraCardWant(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		WantID string `json:"wantId"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	c, ok := mywant.SetCharacterAuraCardWant(id, req.WantID)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// cardNameKindValue resolves, for the field a want's aura mark is ABOUT, the
// catalog kind its value is NAMED into and the field's current value. The kind
// comes from the value's self-described `type` (or the field's declared
// subType), mapped through the data type's catalog relationship (e.g.
// location_coordinate → place). The client names only the want; the server owns
// which field and which catalog.
//
// The field is the want's final result unless the type names another with
// `aura_mark_field`. What a card is ABOUT and what it ANSWERS are not always
// the same: Spotify answers with the track playing, but the thing worth marking
// while it plays is the album — the track is one of the things that were true
// of the album at that moment, not the subject. A type that says nothing here
// keeps the behaviour it always had.
func (s *Server) cardNameKindValue(wantID string) (kind string, value any, ok bool) {
	want, _, found := s.globalBuilder.FindWantByID(wantID)
	if !found || want.Metadata.Type == "" {
		return "", nil, false
	}
	def := s.globalBuilder.GetWantTypeDefinition(want.Metadata.Type)
	if def == nil {
		return "", nil, false
	}
	frf := want.GetStringParam("aura_mark_field", def.FinalResultField)
	if frf == "" {
		return "", nil, false
	}
	value, _ = want.GetCurrent(frf)

	sub := ""
	if m, isMap := value.(map[string]any); isMap {
		if t, isStr := m["type"].(string); isStr {
			sub = t
		}
	}
	if sub == "" {
		for _, sd := range def.State {
			if sd.Name == frf {
				sub = sd.SubType
				break
			}
		}
	}
	if sub == "" {
		sub = "value"
	}
	kind = sub
	if info, has := DataTypeDefinitions()[sub]; has && info.Catalog != "" {
		kind = info.Catalog
	}
	return kind, value, true
}

// cardAuraName names a want's final-result value into its catalog — pressing X
// on a card, the same as naming a field. The server resolves the catalog kind;
// the name is also recorded in the memo. PUT /api/v1/characters/{id}/card-aura-name
// Body: { "wantId": "want-id", "name": "自宅" }
func (s *Server) cardAuraName(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		WantID string `json:"wantId"`
		Name   string `json:"name"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	kind, value, ok := s.cardNameKindValue(req.WantID)
	if !ok {
		s.JSONError(w, r, http.StatusBadRequest, "Want has no final-result field to name", req.WantID)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		s.JSONError(w, r, http.StatusBadRequest, "name is required", "")
		return
	}
	mark := mywant.AuraMark{Target: mywant.AuraTarget{Kind: kind, Name: name}, Value: value}
	c, ok := mywant.SetCharacterAuraDefault(id, mark)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	if err := s.thingStore.Record(kind, name); err != nil {
		mywant.WarnLog("[aura] failed to record memo %s=%q: %v", kind, name, err)
	}
	s.recordThingEvent(kind, name, MemoSourceCardName, req.WantID, c, value)
	snapped := s.snapshotAuraLabels(req.WantID, kind, name)
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"character": c, "kind": kind, "name": name, "labels": snapped,
	})
}

// snapshotAuraLabels writes what was true about a thing at the moment it was
// marked onto the thing itself.
//
// This is the other half of what an aura mark is for. Marking used to be only
// an act of naming — this value, called that — and the rest of what the card
// was showing went nowhere, even though it was the reason the moment was worth
// marking at all. Pressing X while an album plays is not really "the album is
// called X": it is "this album, and here is its cover, its artist, the track
// that was playing". Those are facts about the album that arrived together and
// were lost separately.
//
// So the type says which fields are those facts:
//
//	aura_mark_labels  space-separated state fields, copied onto the marked
//	                  thing as labels of the same name.
//
// Space-separated field names in a param is the form this codebase already
// uses for exactly this (transit's skill_args_keys). Labels are the right home
// because a thing's labels are already what it keeps about itself
// (thing-labels.yaml), they travel with it to every card, and the Thing card
// already reads one of them for its background
// (`background: "@album_art_url"`).
//
// A field that is empty is skipped rather than written blank: an album whose
// cover has not loaded is still worth marking, and a label nobody wrote reads
// as "not known" where an empty one reads as "known to be nothing".
func (s *Server) snapshotAuraLabels(wantID, kind, name string) []string {
	want := s.findWantByIDOrName(wantID)
	if want == nil {
		return nil
	}
	keys := strings.Fields(want.GetStringParam("aura_mark_labels", ""))
	if len(keys) == 0 {
		return nil
	}

	// The labels hang off the thing's id, which is what the label store has
	// moved to — so the entry naming just made or confirmed has to be found.
	id := ""
	for _, e := range s.thingStore.Entries() {
		if e.Value == name && e.Catalog == subtypeToKey(kind) {
			id = e.ID
			break
		}
	}
	if id == "" {
		return nil
	}

	state := want.GetAllState()
	written := []string{}
	for _, key := range keys {
		v, _ := state[key].(string)
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		if err := s.thingLabels.Set(id, key, v); err != nil {
			mywant.WarnLog("[aura] failed to label %s %s: %v", id, key, err)
			continue
		}
		written = append(written, key)
	}
	if len(written) > 0 {
		mywant.InfoLog("[aura] %s: marked %s %q with %v\n", want.Metadata.Name, kind, name, written)
		go broadcastSSE("thing_changed", id)
	}
	return written
}

// getCardAuraMark reports the catalog names a want's final-result value carries
// — the definitions of its kind that match its current value (coordinates by
// proximity, else exact) — so a card shows them without knowing which field or
// kind it is. GET /api/v1/characters/{id}/card-aura-mark?wantId=want-id
func (s *Server) getCardAuraMark(w http.ResponseWriter, r *http.Request) {
	wantID := r.URL.Query().Get("wantId")
	kind, value, ok := s.cardNameKindValue(wantID)
	if !ok {
		s.JSONResponse(w, http.StatusOK, map[string]any{"names": []any{}})
		return
	}
	type named struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	names := []named{}
	for _, ch := range mywant.ListCharacters() {
		for _, m := range ch.AuraDefaults {
			if m.Target.Kind != kind || m.Target.Path != "" {
				continue
			}
			if auraValuesMatch(m.Value, value) {
				names = append(names, named{Name: m.Target.Name, Color: ch.Color})
			}
		}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"names": names})
}

// auraValuesMatch compares a definition value to a field value: coordinates by
// proximity (GPS jitter never reproduces the captured value exactly), else by
// deep equality.
func auraValuesMatch(a, b any) bool {
	if ca, cb := coordOf(a), coordOf(b); ca != nil && cb != nil {
		return metersBetween(ca, cb) <= placeMatchRadiusM
	}
	return reflect.DeepEqual(a, b)
}

type latLng struct{ lat, lng float64 }

const placeMatchRadiusM = 150.0

func coordOf(v any) *latLng {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	lat, okLat := floatFrom(m["lat"])
	lng, okLng := floatFrom(m["lng"])
	if !okLat || !okLng {
		return nil
	}
	return &latLng{lat, lng}
}

func floatFrom(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func metersBetween(a, b *latLng) float64 {
	const R, rad = 6371000.0, math.Pi / 180
	dLat, dLng := (b.lat-a.lat)*rad, (b.lng-a.lng)*rad
	s := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(a.lat*rad)*math.Cos(b.lat*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return R * 2 * math.Atan2(math.Sqrt(s), math.Sqrt(1-s))
}

// setCharacterDesign sets the canvas preferences a character owns.
// Body: { "tile_design": "forest", "aura_design": "forest", "move_speed": 2, "speed": 1.5 }
// — either design may be "" (inherit the canvas design), move_speed 0 or 1 is
// the normal animation pace, and speed 0 falls back to the engine's default
// real movement rate (baseSpeedCellsPerSec).
func (s *Server) setCharacterDesign(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		TileDesign string  `json:"tile_design"`
		AuraDesign string  `json:"aura_design"`
		MoveSpeed  int     `json:"move_speed"`
		Speed      float64 `json:"speed"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	c, ok := mywant.SetCharacterDesign(id, req.TileDesign, req.AuraDesign, req.MoveSpeed, req.Speed)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// setCharacterDisplay replaces a character's look-and-feel choices — the
// settings that used to be global and are now one person's own.
//
// Body is the whole CharacterDisplay; an omitted field means "the built-in
// default", not "leave whatever was there" (see SetDisplay).
func (s *Server) setCharacterDisplay(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req mywant.CharacterDisplay
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	c, ok := mywant.SetCharacterDisplay(id, req)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// pruneCharacterDevices removes stale device IDs from all character assignments.
// Called by the frontend's device heartbeat cleanup path.
// Body: { "deviceIds": ["stale-device-id", ...] }
func (s *Server) pruneCharacterDevices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceIDs []string `json:"deviceIds"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	mywant.PruneCharacterDevices(req.DeviceIDs)
	go broadcastSSE("character_changed", req.DeviceIDs)
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "pruned"})
}
