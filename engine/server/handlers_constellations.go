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

// constellationDTO is the API shape (id == name).
//
// `kind` used to namespace the whole constellation: a thing one and a want one
// were different constellations that merely shared a name. Nothing in the
// STORAGE ever said so — membership is a `constellation/<name>` label, carried
// by a want's metadata or by a thing's labels, and a name has always been able
// to have both — so the split was an artefact of the two collectors and of an
// API that made the caller declare which pile it meant.
//
// A constellation is a name someone gave to a handful of things on the board,
// and the board has two kinds of things on it. So the kind moved onto the
// MEMBER, where it was always a fact, and `kind` is now a summary of what is
// in there: "thing", "want", or "mixed". Kept, rather than dropped, because
// every existing caller reads it and because "what sort of constellation is
// this" is still a fair question.
type constellationDTO struct {
	ID   string `json:"id" yaml:"id"`
	Name string `json:"name" yaml:"name"`
	// "thing", "want", or "mixed" — derived from the members.
	Kind  string `json:"kind" yaml:"kind"`
	Color string `json:"color,omitempty" yaml:"color,omitempty"`
	// Art is the URL of a picture drawn for it (constellation_art.go), laid
	// on the board's floor under it — carried by its members as the colour is.
	Art string `json:"art,omitempty" yaml:"art,omitempty"`
	// Capture is the tile a capture constellation was caught into — drawn from
	// it out to each member; "" is an order, drawn as a chain
	// (constellation_capture.go).
	Capture string `json:"capture,omitempty" yaml:"capture,omitempty"`
	// CaptureAt is where its hub was last seen, "x,y" — its ring stays there
	// when the hub is gone.
	CaptureAt string   `json:"captureAt,omitempty" yaml:"captureAt,omitempty"`
	Members   []string `json:"members" yaml:"members"`
	// Which kind each member is, keyed by the same ids as Members. The list
	// stays a list of ids so that every caller that only asks "is this id in
	// here" keeps working unchanged.
	MemberKinds map[string]string `json:"memberKinds,omitempty" yaml:"memberKinds,omitempty"`
}

// collectThingConstellations aggregates constellation/* labels across all memo values.
func (s *Server) collectThingConstellations() []constellationDTO {
	byName := map[string][]string{}
	colorByName := map[string]string{}
	artByName := map[string]string{}
	captureByName := map[string]string{}
	captureAtByName := map[string]string{}
	for valueID, labels := range s.thingLabels.All() {
		for key, val := range labels {
			if name := constellationNameFromKey(key); name != "" {
				byName[name] = append(byName[name], valueID)
			}
			if name := constellationColorNameFromKey(key); name != "" && val != "" {
				colorByName[name] = val
			}
			if name := constellationArtNameFromKey(key); name != "" && val != "" {
				artByName[name] = val
			}
			if name := constellationCaptureNameFromKey(key); name != "" && val != "" {
				captureByName[name] = val
			}
			if name := constellationCaptureAtNameFromKey(key); name != "" && val != "" {
				captureAtByName[name] = val
			}
		}
	}
	return constellationsFromMap(byName, colorByName, artByName, captureByName, captureAtByName, "thing")
}

// collectWantConstellations aggregates constellation/* labels across all live wants.
func (s *Server) collectWantConstellations() []constellationDTO {
	byName := map[string][]string{}
	colorByName := map[string]string{}
	artByName := map[string]string{}
	captureByName := map[string]string{}
	captureAtByName := map[string]string{}
	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			// GetLabels, not Metadata.Labels: a label change is applied to the
			// live want's own map under its metadata lock, and ranging over it
			// unlocked is a concurrent map read the runtime kills the process for.
			for key, val := range want.GetLabels() {
				if name := constellationNameFromKey(key); name != "" {
					byName[name] = append(byName[name], want.Metadata.ID)
				}
				if name := constellationColorNameFromKey(key); name != "" && val != "" {
					colorByName[name] = val
				}
				if name := constellationArtNameFromKey(key); name != "" && val != "" {
					artByName[name] = val
				}
				if name := constellationCaptureNameFromKey(key); name != "" && val != "" {
					captureByName[name] = val
				}
				if name := constellationCaptureAtNameFromKey(key); name != "" && val != "" {
					captureAtByName[name] = val
				}
			}
		}
	}
	return constellationsFromMap(byName, colorByName, artByName, captureByName, captureAtByName, "want")
}

func constellationsFromMap(byName map[string][]string, colorByName, artByName, captureByName, captureAtByName map[string]string, kind string) []constellationDTO {
	out := make([]constellationDTO, 0, len(byName))
	for name, members := range byName {
		sort.Strings(members)
		kinds := make(map[string]string, len(members))
		for _, m := range members {
			kinds[m] = kind
		}
		out = append(out, constellationDTO{
			ID: name, Name: name, Kind: kind,
			Color: colorByName[name], Art: artByName[name], Capture: captureByName[name], CaptureAt: captureAtByName[name],
			Members: members, MemberKinds: kinds,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// collectAllConstellations is the whole sky: one constellation per name,
// holding whatever carries its label — wants, things, or both.
//
// The two collectors stay as they are and this folds their output together,
// rather than a third walk over both stores. They each know how to read their
// own side, and a name that appears in both is one constellation that happens
// to have members of two kinds; there is nothing to reconcile beyond saying so.
func (s *Server) collectAllConstellations() []constellationDTO {
	byName := map[string]*constellationDTO{}
	var order []string
	for _, g := range append(s.collectThingConstellations(), s.collectWantConstellations()...) {
		existing, ok := byName[g.Name]
		if !ok {
			copied := g
			copied.MemberKinds = map[string]string{}
			for k, v := range g.MemberKinds {
				copied.MemberKinds[k] = v
			}
			copied.Members = append([]string{}, g.Members...)
			byName[g.Name] = &copied
			order = append(order, g.Name)
			continue
		}
		existing.Members = append(existing.Members, g.Members...)
		for k, v := range g.MemberKinds {
			existing.MemberKinds[k] = v
		}
		// Both piles answered to this name, so it is neither one of them.
		if existing.Kind != g.Kind {
			existing.Kind = "mixed"
		}
		// A colour is recorded on every member, so either side's answer is the
		// same answer — except when one side has none, which is what a
		// constellation that only just gained a member of that kind looks like.
		if existing.Color == "" {
			existing.Color = g.Color
		}
		if existing.Art == "" {
			existing.Art = g.Art
		}
		if existing.Capture == "" {
			existing.Capture = g.Capture
		}
		if existing.CaptureAt == "" {
			existing.CaptureAt = g.CaptureAt
		}
	}
	sort.Strings(order)
	out := make([]constellationDTO, 0, len(order))
	for _, name := range order {
		g := byName[name]
		sort.Strings(g.Members)
		out = append(out, *g)
	}
	return out
}

// memberKindOf answers what an id names, by asking rather than by reading its
// prefix: ids are shaped predictably today and a shape is not a guarantee.
//
// This is what lets the API stop making the caller declare a kind. A
// constellation with a want and a thing in it cannot be described by one kind
// at the door, and the server is the side that already knows which is which.
func (s *Server) memberKindOf(id string) string {
	if s.globalBuilder != nil {
		// Both views of the want list, because they are not the same list.
		// GetAllWantStates is what the constellation collectors read and
		// GetWants is what the board reads, and a want present in one and not
		// the other came back as a thing — which then wrote its membership
		// into the thing store, where the want side would never find it again.
		// Asking both is the only answer that cannot be wrong in one
		// direction.
		for _, want := range s.globalBuilder.GetAllWantStates() {
			if want.Metadata.ID == id {
				return "want"
			}
		}
		for _, want := range s.globalBuilder.GetWants() {
			if want.Metadata.ID == id {
				return "want"
			}
		}
	}
	return "thing"
}

// setConstellationMembership applies (add=true) or clears (add=false) the group label on
// one member of the given kind. The colour label follows the same move: a
// member joining a coloured constellation picks the colour up, and one leaving
// drops it, so the "any member carries it" rule collectConstellations relies on
// stays true through membership changes.
func (s *Server) setConstellationMembership(kind, name, member string, add bool) {
	key := constellationKey(name)
	// What a constellation looks like rides on every member: a member joining
	// takes it, one leaving drops it.
	looks := []string{constellationColorKey(name), constellationArtKey(name), constellationCaptureKey(name), constellationCaptureAtKey(name)}
	if kind == "want" {
		if s.globalBuilder == nil {
			return
		}
		if add {
			_ = s.globalBuilder.QueueWantAddLabel(member, key, "true")
			for _, k := range looks {
				if v := s.anyConstellationLabel(k); v != "" {
					_ = s.globalBuilder.QueueWantAddLabel(member, k, v)
				}
			}
		} else {
			_ = s.globalBuilder.QueueWantRemoveLabel(member, key)
			_ = s.globalBuilder.QueueWantRemoveLabel(member, legacyConstellationKey(name))
			for _, k := range looks {
				_ = s.globalBuilder.QueueWantRemoveLabel(member, k)
			}
		}
		return
	}
	// thing
	if add {
		_ = s.thingLabels.Set(member, key, "true")
		for _, k := range looks {
			if v := s.anyConstellationLabel(k); v != "" {
				_ = s.thingLabels.Set(member, k, v)
			}
		}
	} else {
		_ = s.thingLabels.Remove(member, key)
		_ = s.thingLabels.Remove(member, legacyConstellationKey(name))
		// Which of the want's answers it was for goes with it (constellation_answers.go).
		_ = s.thingLabels.Remove(member, constellationAnswerKey(name))
		for _, k := range looks {
			_ = s.thingLabels.Remove(member, k)
		}
	}
}

// constellationColor returns the colour currently recorded for a constellation
// (any one member carrying the label answers for the whole), or "" if none.
func (s *Server) constellationColor(kind, name string) string {
	return s.constellationLabel(kind, constellationColorKey(name))
}

// anyConstellationLabel is the value members of either kind carry under key —
// what a member joining picks up, whichever kind the ones already in are (a
// thing caught into a want's constellation takes the want's capture).
func (s *Server) anyConstellationLabel(key string) string {
	if v := s.constellationLabel("thing", key); v != "" {
		return v
	}
	return s.constellationLabel("want", key)
}

// constellationLabel is the value a constellation's members of one kind
// carry under key (its colour, its art), or "" if none does.
func (s *Server) constellationLabel(kind, key string) string {
	if kind == "want" {
		if s.globalBuilder != nil {
			for _, want := range s.globalBuilder.GetAllWantStates() {
				// A copy under the lock — see collectWantConstellations. Joining a
				// group queues the membership label and then reads the colour
				// here, while the reconcile loop is writing that same map.
				if c, ok := want.GetLabels()[key]; ok && c != "" {
					return c
				}
			}
		}
		return ""
	}
	for _, labels := range s.thingLabels.All() {
		if c, ok := labels[key]; ok && c != "" {
			return c
		}
	}
	return ""
}

// setConstellationColor writes (or, with color=="", clears) the colour label on
// every current member of the constellation.
func (s *Server) setConstellationColor(kind, name, color string) {
	s.setConstellationLabel(kind, name, constellationColorKey(name), color)
}

// setConstellationLabel writes (or, with value=="", clears) key on every
// current member of the constellation of one kind.
func (s *Server) setConstellationLabel(kind, name, key, value string) {
	for _, m := range s.membersOfConstellation(kind, name) {
		if kind == "want" {
			if s.globalBuilder == nil {
				continue
			}
			if value != "" {
				_ = s.globalBuilder.QueueWantAddLabel(m, key, value)
			} else {
				_ = s.globalBuilder.QueueWantRemoveLabel(m, key)
			}
			continue
		}
		if value != "" {
			_ = s.thingLabels.Set(m, key, value)
		} else {
			_ = s.thingLabels.Remove(m, key)
		}
	}
}

// GET /api/v1/constellations?kind=thing|want
func (s *Server) getConstellations(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	var groups []constellationDTO
	switch kind {
	case "want":
		groups = s.collectWantConstellations()
	case "thing":
		groups = s.collectThingConstellations()
	default:
		// One constellation per name, whatever kinds are in it. The two
		// filtered cases stay for callers that genuinely want one side (the
		// Thing page's own list), but a name with both is one constellation.
		groups = s.collectAllConstellations()
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
		// The tile it is a capture into (one of its members); none, an order.
		Capture string `json:"capture"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := validateConstellationName(body.Name); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid constellation name", err.Error())
		return
	}
	// `kind` is no longer required, and no longer decides anything: each member
	// is written to whichever store it lives in, so one call can name a want
	// and a thing together. It is still accepted — every existing caller sends
	// it — and still validated when present, so a typo is an error rather than
	// silently ignored.
	if body.Kind != "" && body.Kind != "thing" && body.Kind != "want" && body.Kind != "mixed" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid kind", "kind must be thing, want or mixed")
		return
	}
	// One of a want's answers among them: the rest join the want's own
	// constellation, marked for it (constellation_answers.go).
	if hub, ok := s.joinAnswersIn(body.Members); ok {
		s.JSONResponse(w, http.StatusOK, constellationDTO{
			ID: hub, Name: hub, Kind: "thing", Capture: s.anyConstellationCapture(hub),
			Members: s.allMembersOfConstellation(hub),
		})
		return
	}
	kinds := make(map[string]string, len(body.Members))
	for _, m := range body.Members {
		k := s.memberKindOf(m)
		kinds[m] = k
		s.setConstellationMembership(k, body.Name, m, true)
	}
	if body.Color != "" {
		s.setConstellationColorForMembers(body.Name, body.Color, kinds)
	}
	if body.Capture != "" {
		// The ones joining, and — a name already there folds in — the rest.
		s.setConstellationLabelForMembers(constellationCaptureKey(body.Name), body.Capture, kinds)
		s.setConstellationCaptureEverywhere(body.Name, body.Capture)
	}
	s.JSONResponse(w, http.StatusOK, constellationDTO{
		ID: body.Name, Name: body.Name, Kind: summariseKinds(kinds),
		Color: body.Color, Capture: body.Capture, Members: body.Members, MemberKinds: kinds,
	})
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
		// The tile it is a capture into; "" makes it an order.
		Capture *string `json:"capture"`
		// Where its hub was last seen, "x,y"; "" forgets it.
		CaptureAt *string `json:"captureAt"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	// As on create, `kind` no longer decides anything — each member is written
	// where it lives. Still validated when present so a typo is not silence.
	if body.Kind != "" && body.Kind != "thing" && body.Kind != "want" && body.Kind != "mixed" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid kind", "kind must be thing, want or mixed")
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
	carriedColor := s.anyConstellationColor(oldName)
	carriedArt := s.anyConstellationArt(oldName)
	carriedCapture := s.anyConstellationCapture(oldName)
	carriedCaptureAt := s.anyConstellationCaptureAt(oldName)

	// Current members of the old group — of both kinds, or reconciling a mixed
	// constellation would silently drop everything of the kind not named.
	current := s.allMembersOfConstellation(oldName)

	// One of a want's answers among the members asked for: they join the
	// want's own constellation instead (constellation_answers.go).
	if body.Members != nil {
		if hub, ok := s.joinAnswersIn(*body.Members); ok {
			s.JSONResponse(w, http.StatusOK, map[string]any{"message": "group updated", "name": hub})
			return
		}
	}
	if body.Members != nil {
		want := map[string]bool{}
		for _, m := range *body.Members {
			want[m] = true
		}
		// Remove members no longer wanted (from the OLD name).
		for _, m := range current {
			if !want[m] {
				s.setConstellationMembership(s.memberKindOf(m), oldName, m, false)
			}
		}
		// Under a rename, everything moves to the new key below; otherwise add
		// the newly-wanted members to the existing name here.
		if newName == oldName {
			for m := range want {
				s.setConstellationMembership(s.memberKindOf(m), oldName, m, true)
			}
		}
	}

	// Rename: move every (final) member from the old key to the new key.
	if newName != oldName {
		final := *orDefaultMembers(body.Members, current)
		for _, m := range final {
			k := s.memberKindOf(m)
			// The answer it is for, carried to the new name.
			answer := s.thingLabels.All()[m][constellationAnswerKey(oldName)]
			s.setConstellationMembership(k, oldName, m, false)
			s.setConstellationMembership(k, newName, m, true)
			if answer != "" {
				_ = s.thingLabels.Set(m, constellationAnswerKey(newName), answer)
			}
		}
	}

	// Colour: an explicit value in the body wins (including "" to clear);
	// otherwise a rename still keeps whatever colour the old name had.
	if body.Color != nil {
		s.setConstellationColorEverywhere(newName, *body.Color)
	} else if newName != oldName && carriedColor != "" {
		s.setConstellationColorEverywhere(newName, carriedColor)
	}
	// Its art goes with it to the new name, as its colour does.
	if newName != oldName && carriedArt != "" {
		s.setConstellationArtEverywhere(newName, carriedArt)
	}
	// Capture or order: as asked, else what the old name was.
	if body.Capture != nil {
		s.setConstellationCaptureEverywhere(newName, *body.Capture)
	} else if newName != oldName && carriedCapture != "" {
		s.setConstellationCaptureEverywhere(newName, carriedCapture)
	}
	if body.CaptureAt != nil {
		s.setConstellationCaptureAtEverywhere(newName, *body.CaptureAt)
	} else if newName != oldName && carriedCaptureAt != "" {
		s.setConstellationCaptureAtEverywhere(newName, carriedCaptureAt)
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "group updated", "name": newName})
}

// GET /api/v1/constellations/{name}
// One constellation, as GET /constellations gives each.
func (s *Server) getConstellation(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	for _, c := range s.collectAllConstellations() {
		if c.Name == name {
			s.JSONResponse(w, http.StatusOK, c)
			return
		}
	}
	s.JSONError(w, r, http.StatusNotFound, "no such constellation", name)
}

// DELETE /api/v1/constellations/{name}?kind=memo|want
func (s *Server) deleteConstellation(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	// Its art goes with it — once its members have let go of it (a file
	// still worn is kept: removeArtFile).
	art := s.anyConstellationArt(name)
	// The ?kind= is accepted and ignored. Deleting a constellation means the
	// name stops existing, and a name that had a want and a thing in it would
	// otherwise half-survive whichever kind the caller happened to send.
	for _, m := range s.allMembersOfConstellation(name) {
		s.setConstellationMembership(s.memberKindOf(m), name, m, false)
	}
	s.removeArtFile(art)
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "group deleted"})
}

// allMembersOfConstellation is every id carrying the name's label, whichever
// store it lives in. What "the members of this constellation" means now that a
// constellation is not partitioned by kind.
func (s *Server) allMembersOfConstellation(name string) []string {
	return append(s.membersOfConstellation("thing", name), s.membersOfConstellation("want", name)...)
}

// anyConstellationColor is the colour recorded for a name by any member of
// either kind. The colour is written to every member, so any one of them
// answers for the whole — this only widens which members get asked.
func (s *Server) anyConstellationColor(name string) string {
	if c := s.constellationColor("thing", name); c != "" {
		return c
	}
	return s.constellationColor("want", name)
}

// setConstellationColorEverywhere paints every current member, of either kind.
func (s *Server) setConstellationColorEverywhere(name, color string) {
	s.setConstellationColor("thing", name, color)
	s.setConstellationColor("want", name, color)
}

// setConstellationLabelForMembers writes key on a specific set, each in its
// own store — for a constellation being created, whose members are not yet
// findable by their labels.
func (s *Server) setConstellationLabelForMembers(key, value string, kinds map[string]string) {
	for m, k := range kinds {
		if k == "want" {
			if s.globalBuilder != nil {
				_ = s.globalBuilder.QueueWantAddLabel(m, key, value)
			}
			continue
		}
		_ = s.thingLabels.Set(m, key, value)
	}
}

// setConstellationColorForMembers paints a specific set, each in its own store
// — for a constellation being created, whose members are not yet findable by
// their labels.
func (s *Server) setConstellationColorForMembers(name, color string, kinds map[string]string) {
	colorKey := constellationColorKey(name)
	for m, k := range kinds {
		if k == "want" {
			if s.globalBuilder != nil {
				_ = s.globalBuilder.QueueWantAddLabel(m, colorKey, color)
			}
			continue
		}
		_ = s.thingLabels.Set(m, colorKey, color)
	}
}

// summariseKinds reduces what is in a constellation to the one word `kind`
// still reports: "thing", "want", or "mixed" when it holds both.
func summariseKinds(kinds map[string]string) string {
	seen := ""
	for _, k := range kinds {
		if seen == "" {
			seen = k
			continue
		}
		if seen != k {
			return "mixed"
		}
	}
	return seen
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
				labels := want.GetLabels() // see collectWantConstellations
				_, has := labels[key]
				_, hadLegacy := labels[legacy]
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




// As a want is a resource (handlers_wants.go), so is a constellation: besides
// list, create, update and delete above, one read (getConstellation), PATCH as
// PUT (what is given changes), several deleted at once, and the lot written
// out as YAML and read back in (made or joined by name, with its colour, art
// and hub):
//
//	GET    /api/v1/constellations/{name}
//	PATCH  /api/v1/constellations/{name}
//	DELETE /api/v1/constellations?names=a,b
//	POST   /api/v1/constellations/export[?names=a,b]  YAML
//	POST   /api/v1/constellations/import              YAML

// DELETE /api/v1/constellations?names=a,b
func (s *Server) deleteConstellations(w http.ResponseWriter, r *http.Request) {
	names := splitList(r.URL.Query().Get("names"))
	if len(names) == 0 {
		s.JSONError(w, r, http.StatusBadRequest, "names are required", "?names=a,b")
		return
	}
	var deleted, missing []string
	for _, name := range names {
		members := s.allMembersOfConstellation(name)
		if len(members) == 0 {
			missing = append(missing, name)
			continue
		}
		art := s.anyConstellationArt(name)
		for _, m := range members {
			s.setConstellationMembership(s.memberKindOf(m), name, m, false)
		}
		s.removeArtFile(art)
		deleted = append(deleted, name)
	}
	boardThingsChanged()
	s.JSONResponse(w, http.StatusOK, map[string]any{"deleted": deleted, "missing": missing})
}

// POST /api/v1/constellations/export[?names=a,b]
func (s *Server) exportConstellations(w http.ResponseWriter, r *http.Request) {
	only := map[string]bool{}
	for _, n := range splitList(r.URL.Query().Get("names")) {
		only[n] = true
	}
	out := []constellationDTO{}
	for _, c := range s.collectAllConstellations() {
		if len(only) == 0 || only[c.Name] {
			out = append(out, c)
		}
	}
	writeYAML(w, out)
}

// POST /api/v1/constellations/import
func (s *Server) importConstellations(w http.ResponseWriter, r *http.Request) {
	var in []constellationDTO
	if err := readYAML(r, &in); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid YAML", err.Error())
		return
	}
	var names []string
	for _, c := range in {
		name := c.Name
		if name == "" {
			name = c.ID
		}
		if err := validateConstellationName(name); err != nil {
			s.JSONError(w, r, http.StatusBadRequest, "invalid constellation name", err.Error())
			return
		}
		for _, m := range c.Members {
			s.setConstellationMembership(s.memberKindOf(m), name, m, true)
		}
		if c.Color != "" {
			s.setConstellationColorEverywhere(name, c.Color)
		}
		if c.Art != "" {
			s.setConstellationArtEverywhere(name, c.Art)
		}
		if c.Capture != "" {
			s.setConstellationCaptureEverywhere(name, c.Capture)
		}
		if c.CaptureAt != "" {
			s.setConstellationCaptureAtEverywhere(name, c.CaptureAt)
		}
		names = append(names, name)
	}
	boardThingsChanged()
	s.JSONResponse(w, http.StatusOK, map[string]any{"imported": names})
}
