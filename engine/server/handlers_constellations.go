package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"
)

// constellationLabelPrefix is the reserved label-key prefix that expresses
// constellation membership. A constellation named "近い" is the label
// "constellation/近い"="true" carried by each member — a memo value (via
// ThingLabelStore) or a want (via metadata.labels). There is no separate
// constellation ledger; a constellation exists exactly as long as some member
// still carries its label.
const constellationLabelPrefix = "constellation/"

// constellationColorPrefix is the reserved label-key prefix that records the
// colour a constellation is drawn in — "constellation-color/近い"="#38bdf8",
// carried by every current member alongside its membership label. It is a
// presentation choice, not a fact about the members, but there is no
// constellation ledger to hang it on, so it rides on the members the same way
// membership does and travels with the world's things for free.
const constellationColorPrefix = "constellation-color/"

func constellationColorKey(name string) string { return constellationColorPrefix + name }

// constellationColorNameFromKey returns the constellation name for a
// "constellation-color/<name>" key, or "" if the key is not a colour label.
func constellationColorNameFromKey(key string) string {
	if name, ok := strings.CutPrefix(key, constellationColorPrefix); ok {
		return name
	}
	return ""
}

// legacyConstellationLabelPrefix is what the same relation was stored under
// before the rename. Still read, so constellations named before the change keep
// working, but never written: membership set from here on uses the new prefix,
// and a legacy membership converts the first time it is edited.
const legacyConstellationLabelPrefix = "group/"

func constellationKey(name string) string { return constellationLabelPrefix + name }

// constellationNameFromKey returns the constellation name for a
// "constellation/<name>" (or legacy "group/<name>") key, or "" if the key is
// not a constellation label.
func constellationNameFromKey(key string) string {
	if name, ok := strings.CutPrefix(key, constellationLabelPrefix); ok {
		return name
	}
	if name, ok := strings.CutPrefix(key, legacyConstellationLabelPrefix); ok {
		return name
	}
	return ""
}

// legacyConstellationKey is the pre-rename key for a name — only ever removed,
// never written.
func legacyConstellationKey(name string) string { return legacyConstellationLabelPrefix + name }

// constellationDTO is the API shape (id == name; kind namespaces memo vs want).
type constellationDTO struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Color   string   `json:"color,omitempty"`
	Members []string `json:"members"`
}

// collectThingConstellations aggregates constellation/* labels across all memo values.
func (s *Server) collectThingConstellations() []constellationDTO {
	byName := map[string][]string{}
	colorByName := map[string]string{}
	for valueID, labels := range s.thingLabels.All() {
		for key, val := range labels {
			if name := constellationNameFromKey(key); name != "" {
				byName[name] = append(byName[name], valueID)
			}
			if name := constellationColorNameFromKey(key); name != "" && val != "" {
				colorByName[name] = val
			}
		}
	}
	return constellationsFromMap(byName, colorByName, "thing")
}

// collectWantConstellations aggregates constellation/* labels across all live wants.
func (s *Server) collectWantConstellations() []constellationDTO {
	byName := map[string][]string{}
	colorByName := map[string]string{}
	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			for key, val := range want.Metadata.Labels {
				if name := constellationNameFromKey(key); name != "" {
					byName[name] = append(byName[name], want.Metadata.ID)
				}
				if name := constellationColorNameFromKey(key); name != "" && val != "" {
					colorByName[name] = val
				}
			}
		}
	}
	return constellationsFromMap(byName, colorByName, "want")
}

func constellationsFromMap(byName map[string][]string, colorByName map[string]string, kind string) []constellationDTO {
	out := make([]constellationDTO, 0, len(byName))
	for name, members := range byName {
		sort.Strings(members)
		out = append(out, constellationDTO{ID: name, Name: name, Kind: kind, Color: colorByName[name], Members: members})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// setConstellationMembership applies (add=true) or clears (add=false) the group label on
// one member of the given kind. The colour label follows the same move: a
// member joining a coloured constellation picks the colour up, and one leaving
// drops it, so the "any member carries it" rule collectConstellations relies on
// stays true through membership changes.
func (s *Server) setConstellationMembership(kind, name, member string, add bool) {
	key := constellationKey(name)
	colorKey := constellationColorKey(name)
	if kind == "want" {
		if s.globalBuilder == nil {
			return
		}
		if add {
			_ = s.globalBuilder.QueueWantAddLabel(member, key, "true")
			if c := s.constellationColor(kind, name); c != "" {
				_ = s.globalBuilder.QueueWantAddLabel(member, colorKey, c)
			}
		} else {
			_ = s.globalBuilder.QueueWantRemoveLabel(member, key)
			_ = s.globalBuilder.QueueWantRemoveLabel(member, legacyConstellationKey(name))
			_ = s.globalBuilder.QueueWantRemoveLabel(member, colorKey)
		}
		return
	}
	// thing
	if add {
		_ = s.thingLabels.Set(member, key, "true")
		if c := s.constellationColor(kind, name); c != "" {
			_ = s.thingLabels.Set(member, colorKey, c)
		}
	} else {
		_ = s.thingLabels.Remove(member, key)
		_ = s.thingLabels.Remove(member, legacyConstellationKey(name))
		_ = s.thingLabels.Remove(member, colorKey)
	}
}

// constellationColor returns the colour currently recorded for a constellation
// (any one member carrying the label answers for the whole), or "" if none.
func (s *Server) constellationColor(kind, name string) string {
	colorKey := constellationColorKey(name)
	if kind == "want" {
		if s.globalBuilder != nil {
			for _, want := range s.globalBuilder.GetAllWantStates() {
				if c, ok := want.Metadata.Labels[colorKey]; ok && c != "" {
					return c
				}
			}
		}
		return ""
	}
	for _, labels := range s.thingLabels.All() {
		if c, ok := labels[colorKey]; ok && c != "" {
			return c
		}
	}
	return ""
}

// setConstellationColor writes (or, with color=="", clears) the colour label on
// every current member of the constellation.
func (s *Server) setConstellationColor(kind, name, color string) {
	colorKey := constellationColorKey(name)
	for _, m := range s.membersOfConstellation(kind, name) {
		if kind == "want" {
			if s.globalBuilder == nil {
				continue
			}
			if color != "" {
				_ = s.globalBuilder.QueueWantAddLabel(m, colorKey, color)
			} else {
				_ = s.globalBuilder.QueueWantRemoveLabel(m, colorKey)
			}
			continue
		}
		if color != "" {
			_ = s.thingLabels.Set(m, colorKey, color)
		} else {
			_ = s.thingLabels.Remove(m, colorKey)
		}
	}
}

// normalizeConstellationKind accepts the pre-rename name for the thing side.
func normalizeConstellationKind(kind string) string {
	if kind == "memo" {
		return "thing"
	}
	return kind
}

// GET /api/v1/constellations?kind=thing|want ("memo" still accepted for thing)
func (s *Server) getConstellations(w http.ResponseWriter, r *http.Request) {
	kind := normalizeConstellationKind(r.URL.Query().Get("kind"))
	var groups []constellationDTO
	switch kind {
	case "want":
		groups = s.collectWantConstellations()
	case "thing":
		groups = s.collectThingConstellations()
	default:
		groups = append(s.collectThingConstellations(), s.collectWantConstellations()...)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"groups": groups})
}

// POST /api/v1/constellations   body: {name, kind, members, color?}
func (s *Server) createConstellation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string   `json:"name"`
		Kind    string   `json:"kind"`
		Members []string `json:"members"`
		Color   string   `json:"color"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := validateConstellationName(body.Name); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid constellation name", err.Error())
		return
	}
	body.Kind = normalizeConstellationKind(body.Kind)
	if body.Kind != "thing" && body.Kind != "want" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid kind", "kind must be thing or want")
		return
	}
	for _, m := range body.Members {
		s.setConstellationMembership(body.Kind, body.Name, m, true)
	}
	if body.Color != "" {
		s.setConstellationColor(body.Kind, body.Name, body.Color)
	}
	s.JSONResponse(w, http.StatusOK, constellationDTO{ID: body.Name, Name: body.Name, Kind: body.Kind, Color: body.Color, Members: body.Members})
}

// PUT /api/v1/constellations/{name}   body: {name?, members?, kind, color?}
// Reconciles membership to the provided set and optionally renames the constellation
// or recolours it (color:"" clears the colour, back to the default starlight).
func (s *Server) updateConstellation(w http.ResponseWriter, r *http.Request) {
	oldName := mux.Vars(r)["name"]
	var body struct {
		Name    *string   `json:"name"`
		Members *[]string `json:"members"`
		Kind    string    `json:"kind"`
		Color   *string   `json:"color"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	body.Kind = normalizeConstellationKind(body.Kind)
	if body.Kind != "thing" && body.Kind != "want" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid kind", "kind must be thing or want")
		return
	}
	newName := oldName
	if body.Name != nil {
		if err := validateConstellationName(*body.Name); err != nil {
			s.JSONError(w, r, http.StatusBadRequest, "invalid constellation name", err.Error())
			return
		}
		newName = *body.Name
	}

	// The colour, captured before any membership move so a rename can carry it
	// over even when the client did not restate it.
	carriedColor := s.constellationColor(body.Kind, oldName)

	// Current members of the old group.
	current := s.membersOfConstellation(body.Kind, oldName)

	if body.Members != nil {
		want := map[string]bool{}
		for _, m := range *body.Members {
			want[m] = true
		}
		// Remove members no longer wanted (from the OLD name).
		for _, m := range current {
			if !want[m] {
				s.setConstellationMembership(body.Kind, oldName, m, false)
			}
		}
		// Under a rename, everything moves to the new key below; otherwise add
		// the newly-wanted members to the existing name here.
		if newName == oldName {
			for m := range want {
				s.setConstellationMembership(body.Kind, oldName, m, true)
			}
		}
	}

	// Rename: move every (final) member from the old key to the new key.
	if newName != oldName {
		final := *orDefaultMembers(body.Members, current)
		for _, m := range final {
			s.setConstellationMembership(body.Kind, oldName, m, false)
			s.setConstellationMembership(body.Kind, newName, m, true)
		}
	}

	// Colour: an explicit value in the body wins (including "" to clear);
	// otherwise a rename still keeps whatever colour the old name had.
	if body.Color != nil {
		s.setConstellationColor(body.Kind, newName, *body.Color)
	} else if newName != oldName && carriedColor != "" {
		s.setConstellationColor(body.Kind, newName, carriedColor)
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "group updated", "name": newName})
}

// DELETE /api/v1/constellations/{name}?kind=memo|want
func (s *Server) deleteConstellation(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	kind := r.URL.Query().Get("kind")
	kind = normalizeConstellationKind(kind)
	if kind != "thing" && kind != "want" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid kind", "kind query param must be memo or want")
		return
	}
	for _, m := range s.membersOfConstellation(kind, name) {
		s.setConstellationMembership(kind, name, m, false)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "group deleted"})
}

// membersOfConstellation returns the current member ids of a group of the given kind.
// membersOfConstellation looks under both the current and the legacy prefix, so
// a constellation named before the rename still reports its members.
func (s *Server) membersOfConstellation(kind, name string) []string {
	key, legacy := constellationKey(name), legacyConstellationKey(name)
	if kind == "want" {
		var out []string
		if s.globalBuilder != nil {
			for _, want := range s.globalBuilder.GetAllWantStates() {
				_, has := want.Metadata.Labels[key]
				_, hadLegacy := want.Metadata.Labels[legacy]
				if has || hadLegacy {
					out = append(out, want.Metadata.ID)
				}
			}
		}
		return out
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range append(s.thingLabels.ValuesWithLabel(key), s.thingLabels.ValuesWithLabel(legacy)...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func orDefaultMembers(members *[]string, fallback []string) *[]string {
	if members != nil {
		return members
	}
	return &fallback
}

func validateConstellationName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errConstellationName("constellation name must not be empty")
	}
	if strings.Contains(name, "/") {
		return errConstellationName("group name must not contain '/'")
	}
	return nil
}

type errConstellationName string

func (e errConstellationName) Error() string { return string(e) }

// ── Raw memo-label endpoints (general facility; groups ride on top) ──────────

// GET /api/v1/memo/labels
func (s *Server) getThingLabels(w http.ResponseWriter, _ *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{"labels": s.thingLabels.All()})
}

// POST /api/v1/memo/labels   body: {value_id, key, value}
func (s *Server) setThingLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ValueID string `json:"value_id"`
		Key     string `json:"key"`
		Value   string `json:"value"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := s.thingLabels.Set(body.ValueID, body.Key, body.Value); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to set memo label", err.Error())
		return
	}
	// A label is how a value gets pinned to the canvas, which is a change to
	// what the city knows about it — the same kind of news as being named.
	go broadcastSSE("thing_changed", body.ValueID)
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "label set"})
}

// POST /api/v1/memo/labels/remove   body: {value_id, key}
func (s *Server) removeThingLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ValueID string `json:"value_id"`
		Key     string `json:"key"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := s.thingLabels.Remove(body.ValueID, body.Key); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to remove memo label", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "label removed"})
}
