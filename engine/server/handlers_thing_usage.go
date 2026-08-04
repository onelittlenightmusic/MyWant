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
func (s *Server) getThingUsage(w http.ResponseWriter, r *http.Request) {
	// Everything the memo actually holds, so a parameter value that was never
	// recorded (recordMemo: false, or typed before the hook existed) does not
	// masquerade as a memo.
	remembered := map[string]bool{}
	for catalog, values := range s.thingStore.All() {
		for _, v := range values {
			remembered[catalog+"::"+v] = true
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
				id := catalog + "::" + str
				if !remembered[id] {
					continue
				}
				if byID[id] == nil {
					byID[id] = &memoUsage{
						ID:      id,
						Catalog: catalog,
						Subtype: pd.SubType,
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

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"usage": out,
		"count": len(out),
	})
}
