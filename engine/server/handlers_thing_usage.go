package server

import (
	"net/http"
	"sort"
	"strings"
)

// The memo↔want relation.
//
// It is derived, not stored: a want names a memo value exactly when one of its
// parameters carries a subType and holds that value — the same rule ThingHook
// uses to decide the value was worth remembering in the first place. Deriving it
// on read means the relation can never drift from the wants that are actually
// live, and deleting a want removes its edges for free.
//
// memo-events.yaml keeps the historical side (who named what, when); this is the
// present tense.

// ThingRefSubType marks a parameter whose value is a list of thing ids
// ("<catalog>::<value>") rather than a value to be remembered under one
// catalog. It is a reserved subType name and not a data type: nothing is ever
// filed under it, and a want type declares it to say "these are the things this
// want is about".
const ThingRefSubType = "thing_ref"

// memoUsage is one remembered value together with the live wants naming it.
type memoUsage struct {
	// ID is the memo record id, "<catalog>::<value>" — the same id the memo
	// cards use, so a client can line the two up directly.
	ID      string   `json:"id"`
	Catalog string   `json:"catalog"`
	Subtype string   `json:"subtype"`
	Value   string   `json:"value"`
	WantIDs []string `json:"wantIDs"`
}

// getThingUsage handles GET /api/v1/memo/usage
//
// Returns only values that are both remembered and currently named by a live
// want, so a caller can show the memo alongside the wants using it.
func (s *Server) getThingUsage(w http.ResponseWriter, _ *http.Request) {
	out := s.deriveThingUsage()
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"usage": out,
		"count": len(out),
	})
}

// deriveThingUsage is the derivation itself, split out so the one GET that
// returns a whole thing can reuse it instead of the caller making a second
// round trip for the same answer.
func (s *Server) deriveThingUsage() []memoUsage {
	// Everything the memo actually holds, so a parameter value that was never
	// recorded (recordMemo: false, or typed before the hook existed) does not
	// masquerade as a memo.
	remembered := map[string]bool{}
	// Also: which catalogs a value of this name is remembered in, so a field
	// can be matched against a thing filed under a different but
	// interchangeable subtype — a `from` declared as a station finding the
	// place you already named. See subtypesInterchangeable.
	catalogsByValue := map[string][]string{}
	for catalog, values := range s.thingStore.All() {
		for _, v := range values {
			remembered[catalog+"::"+v] = true
			catalogsByValue[v] = append(catalogsByValue[v], catalog)
		}
	}

	byID := map[string]*memoUsage{}
	seen := map[string]map[string]bool{} // usage id → want id set

	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			typeDef := s.globalBuilder.GetWantTypeDefinition(want.Metadata.Type)
			if typeDef == nil {
				continue
			}
			for _, pd := range typeDef.Parameters {
				// A parameter that points AT things rather than being one: its
				// value is a list of ids already in "<catalog>::<value>" form.
				// Handled ahead of the single-value rule below because the ids
				// carry their own catalogs — a list spanning an artist and an
				// event has no one subType that could stand for all of it, and
				// running these through subtypeToKey would only double the
				// catalog up ("things::artists::Dream Theater").
				if pd.SubType == ThingRefSubType {
					list, _ := want.Spec.Params[pd.Name].([]any)
					for _, el := range list {
						id, ok := el.(string)
						if !ok {
							continue
						}
						id = strings.TrimSpace(id)
						catalog, value, found := strings.Cut(id, "::")
						if !found || value == "" || !remembered[id] {
							continue
						}
						if byID[id] == nil {
							byID[id] = &memoUsage{
								ID:      id,
								Catalog: catalog,
								Subtype: keyToSubtype(catalog),
								Value:   value,
							}
							seen[id] = map[string]bool{}
						}
						if wid := want.Metadata.ID; wid != "" && !seen[id][wid] {
							seen[id][wid] = true
							byID[id].WantIDs = append(byID[id].WantIDs, wid)
						}
					}
					continue
				}
				if pd.SubType == "" {
					continue
				}
				if !pd.ShouldRecordThing() {
					continue
				}
				str, ok := want.Spec.Params[pd.Name].(string)
				if !ok {
					continue
				}
				str = strings.TrimSpace(str)
				if str == "" {
					continue
				}
				catalog := subtypeToKey(pd.SubType)
				// The declared subtype first; failing that, any catalog holding
				// this value whose subtype is interchangeable with it. The
				// field asked for a string with a meaning, and these all are.
				if !remembered[catalog+"::"+str] {
					catalog = ""
					for _, c := range catalogsByValue[str] {
						if subtypesInterchangeable(pd.SubType, keyToSubtype(c)) {
							catalog = c
							break
						}
					}
					if catalog == "" {
						continue
					}
				}
				id := catalog + "::" + str
				if byID[id] == nil {
					byID[id] = &memoUsage{
						ID:      id,
						Catalog: catalog,
						Subtype: keyToSubtype(catalog),
						Value:   str,
					}
					seen[id] = map[string]bool{}
				}
				if wid := want.Metadata.ID; wid != "" && !seen[id][wid] {
					seen[id][wid] = true
					byID[id].WantIDs = append(byID[id].WantIDs, wid)
				}
			}
		}

	}

	out := make([]memoUsage, 0, len(byID))
	for _, u := range byID {
		sort.Strings(u.WantIDs)
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
