package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"sort"

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

// constellationCorrelationEntries returns one entry per (want-or-thing) member
// of every constellation this want belongs to.
func (s *Server) constellationCorrelationEntries(want *mywant.Want) []enrichedCorrelationEntry {
	if want == nil {
		return nil
	}
	var out []enrichedCorrelationEntry
	seen := map[string]bool{}
	for key := range want.Metadata.Labels {
		name := constellationNameFromKey(key)
		if name == "" {
			continue
		}
		label := constellationLabelPrefix + name
		for _, m := range s.membersOfConstellation("want", name) {
			if m == want.Metadata.ID || seen["want\x00"+m] {
				continue
			}
			seen["want\x00"+m] = true
			out = append(out, enrichedCorrelationEntry{
				Kind:       "constellation",
				WantID:     m,
				TargetKind: "want",
				TargetID:   m,
				Labels:     []string{label},
				Rate:       constellationRelationRate,
			})
		}
		for _, t := range s.membersOfConstellation("thing", name) {
			if seen["thing\x00"+t] {
				continue
			}
			seen["thing\x00"+t] = true
			out = append(out, enrichedCorrelationEntry{
				Kind:       "constellation",
				TargetKind: "thing",
				TargetID:   t,
				Labels:     []string{label},
				Rate:       constellationRelationRate,
			})
		}
	}
	return out
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

	var out []enrichedCorrelationEntry
	seen := map[string]bool{}

	for key := range s.thingLabels.Get(id) {
		name := constellationNameFromKey(key)
		if name == "" {
			continue
		}
		label := constellationLabelPrefix + name
		for _, t := range s.membersOfConstellation("thing", name) {
			if t == id || seen["thing\x00"+t] {
				continue
			}
			seen["thing\x00"+t] = true
			out = append(out, enrichedCorrelationEntry{
				Kind:       "constellation",
				TargetKind: "thing",
				TargetID:   t,
				Labels:     []string{label},
				Rate:       constellationRelationRate,
			})
		}
		for _, m := range s.membersOfConstellation("want", name) {
			if seen["want\x00"+m] {
				continue
			}
			seen["want\x00"+m] = true
			out = append(out, enrichedCorrelationEntry{
				Kind:       "constellation",
				WantID:     m,
				TargetKind: "want",
				TargetID:   m,
				Labels:     []string{label},
				Rate:       constellationRelationRate,
			})
		}
	}

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
