package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// The two neighbour kinds that live outside correlationPhase.
//
// correlationPhase (engine core) only knows want↔want data dependencies —
// import/expose/state access. The canvas' jump overlay also wants to reach a
// want's constellation co-members and the things it named through a parameter.
// Both are derivable on read from the label store and the live want set, the
// same way getConstellations and getThingUsage already derive their answers, so
// they are computed here and folded into the want's correlation list under
// Kind "constellation" / "parameter" rather than being pushed onto
// want.Metadata.Correlation (which is want_spec's type and has no room for a
// thing target).

// constellationRelationRate and parameterRelationRate are the baseline coupling
// strengths for the derived kinds — below any real data dependency (which
// scores +2 for state access plus one per shared label), so the overlay places
// a genuine relation ahead of a shared constellation when a direction is
// contested and distance ties.
const (
	constellationRelationRate = 2
	parameterRelationRate     = 1
)

// thingRelationFingerprint is a hash of everything the derived
// parameter/constellation correlations are computed from — which live wants
// name which things, and the constellation/colour labels on things. Mixed into
// the /wants collection ETag so that a thing becoming named (or a constellation
// changing) busts the client's cache: correlationPhase never touched
// want.Metadata.Correlation for these kinds, so CalculateWantHash cannot see
// them change on its own.
func (s *Server) thingRelationFingerprint() string {
	h := sha256.New()
	for _, u := range s.deriveThingUsage() {
		fmt.Fprintf(h, "%s|%s|%v\n", u.ID, u.Subtype, u.WantIDs)
	}
	all := s.thingLabels.All()
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "L:%s=%v\n", k, all[k])
	}
	return fmt.Sprintf("things:%x", h.Sum(nil)[:8])
}

// thingUUIDByPair maps "<catalog>::<value>" — the key deriveThingUsage /
// deriveThingDefinitions work in — to the thing's real id, which is a UUID and
// is what the canvas keys tiles, placements and jump targets by. A value a want
// names but that was never persisted as a thing has no entry here.
func (s *Server) thingUUIDByPair() map[string]string {
	m := make(map[string]string)
	for _, e := range s.thingStore.Entries() {
		if e.Value == "" {
			continue
		}
		m[e.Catalog+"::"+e.Value] = e.ID
	}
	return m
}

// constellationMember is one member of a constellation, either kind.
type constellationMember struct {
	id   string
	kind string // "want" | "thing"
}

// constellationMemberRank is the position a member carries inside a
// constellation — the number in the value of its membership label (see the
// GUI's utils/thingOrder). "true", a member with no place yet, returns
// ok=false.
func (s *Server) constellationMemberRank(kind, name, id string) (int, bool) {
	key, legacy := constellationKey(name), legacyConstellationKey(name)
	var raw string
	if kind == "want" {
		if s.globalBuilder != nil {
			if wnt, _, found := s.globalBuilder.FindWantByID(id); found && wnt != nil {
				if v, ok := wnt.Metadata.Labels[key]; ok {
					raw = v
				} else {
					raw = wnt.Metadata.Labels[legacy]
				}
			}
		}
	} else {
		lbls := s.thingLabels.Get(id)
		if v, ok := lbls[key]; ok {
			raw = v
		} else {
			raw = lbls[legacy]
		}
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

// constellationChainNeighbors returns only the members sitting immediately
// before and after the subject in the constellation's order — the ±1 hop. A
// jump is meant to step along the chain, not skip down it, so anything two or
// more places away is dropped. Members with no place in the order fall off the
// chain once the subject has one; when the subject itself has no place there is
// no chain to walk, so every member stays a candidate.
func (s *Server) constellationChainNeighbors(name, subjectKind, subjectID string) []constellationMember {
	var all []constellationMember
	for _, m := range s.membersOfConstellation("want", name) {
		all = append(all, constellationMember{m, "want"})
	}
	for _, t := range s.membersOfConstellation("thing", name) {
		all = append(all, constellationMember{t, "thing"})
	}

	type rankedMember struct {
		constellationMember
		rank int
	}
	var ranked []rankedMember
	for _, m := range all {
		if r, ok := s.constellationMemberRank(m.kind, name, m.id); ok {
			ranked = append(ranked, rankedMember{m, r})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].rank != ranked[j].rank {
			return ranked[i].rank < ranked[j].rank
		}
		if ranked[i].kind != ranked[j].kind {
			return ranked[i].kind < ranked[j].kind
		}
		return ranked[i].id < ranked[j].id
	})

	si := -1
	for i, m := range ranked {
		if m.kind == subjectKind && m.id == subjectID {
			si = i
			break
		}
	}
	if si < 0 {
		out := make([]constellationMember, 0, len(all))
		for _, m := range all {
			if m.kind == subjectKind && m.id == subjectID {
				continue
			}
			out = append(out, m)
		}
		return out
	}

	var out []constellationMember
	if si-1 >= 0 {
		out = append(out, ranked[si-1].constellationMember)
	}
	if si+1 < len(ranked) {
		out = append(out, ranked[si+1].constellationMember)
	}
	return out
}

// constellationEntriesFor emits the ±1-hop constellation neighbours of a
// subject (a want or a thing) as correlation entries, de-duplicated via seen.
func (s *Server) constellationEntriesFor(subjectKind, subjectID string, labels map[string]string, seen map[string]bool) []enrichedCorrelationEntry {
	var out []enrichedCorrelationEntry
	for key := range labels {
		name := constellationNameFromKey(key)
		if name == "" {
			continue
		}
		label := constellationLabelPrefix + name
		for _, m := range s.constellationChainNeighbors(name, subjectKind, subjectID) {
			sk := m.kind + "\x00" + m.id
			if seen[sk] {
				continue
			}
			seen[sk] = true
			e := enrichedCorrelationEntry{
				Kind:       "constellation",
				TargetKind: m.kind,
				TargetID:   m.id,
				Labels:     []string{label},
				Rate:       constellationRelationRate,
			}
			if m.kind == "want" {
				e.WantID = m.id
			}
			out = append(out, e)
		}
	}
	return out
}

// constellationCorrelationEntries returns the ±1-hop constellation neighbours
// of a want across every constellation it belongs to.
func (s *Server) constellationCorrelationEntries(want *mywant.Want) []enrichedCorrelationEntry {
	if want == nil {
		return nil
	}
	return s.constellationEntriesFor("want", want.Metadata.ID, want.Metadata.Labels, map[string]bool{})
}

// parameterCorrelationEntries returns one entry per thing this want currently
// names through a catalog-typed parameter (`at: 国分寺` → the thing
// "cities::国分寺"). Derived from the same live usage getThingUsage serves.
func (s *Server) parameterCorrelationEntries(want *mywant.Want) []enrichedCorrelationEntry {
	if want == nil || want.Metadata.ID == "" {
		return nil
	}
	wid := want.Metadata.ID
	uuidByPair := s.thingUUIDByPair()
	var out []enrichedCorrelationEntry
	for _, u := range s.deriveThingUsage() {
		for _, id := range u.WantIDs {
			if id != wid {
				continue
			}
			// The canvas needs the thing's UUID; u.ID is the "catalog::value"
			// pair. Fall back to the pair for a named value with no thing
			// record — the overlay can still show it, it just can't be jumped to.
			target := uuidByPair[u.ID]
			if target == "" {
				target = u.ID
			}
			out = append(out, enrichedCorrelationEntry{
				Kind:       "parameter",
				TargetKind: "thing",
				TargetID:   target,
				Labels:     []string{"parameter/" + u.Subtype},
				Rate:       parameterRelationRate,
				DataType:   u.Subtype,
			})
			break
		}
	}
	return out
}

// listThingRelations handles GET /api/v1/things/{id}/relations.
//
// A thing is not a want and carries no correlation of its own; this is the
// mirror of a want's constellation/parameter entries — its constellation
// co-members (things and wants) and the wants that name it.
func (s *Server) listThingRelations(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		s.JSONError(w, r, http.StatusBadRequest, "thing id required", "")
		return
	}

	seen := map[string]bool{}
	out := s.constellationEntriesFor("thing", id, s.thingLabels.Get(id), seen)

	// `id` is a thing UUID; deriveThingUsage keys by the "catalog::value" pair.
	uuidByPair := s.thingUUIDByPair()
	for _, u := range s.deriveThingUsage() {
		if uuidByPair[u.ID] != id {
			continue
		}
		for _, wid := range u.WantIDs {
			if seen["want\x00"+wid] {
				continue
			}
			seen["want\x00"+wid] = true
			out = append(out, enrichedCorrelationEntry{
				Kind:       "parameter",
				WantID:     wid,
				TargetKind: "want",
				TargetID:   wid,
				Labels:     []string{"parameter/" + u.Subtype},
				Rate:       parameterRelationRate,
				DataType:   u.Subtype,
			})
		}
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{"relations": out})
}
