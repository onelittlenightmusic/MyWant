package server

import (
	"net/http"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// getAllWants returns all runtime Want instances from all executions and the globalBuilder.
func (s *Server) getAllWants() []*mywant.Want {
	s.wantsMu.RLock()
	executions := make([]*WantExecution, 0, len(s.wants))
	for _, exec := range s.wants {
		executions = append(executions, exec)
	}
	s.wantsMu.RUnlock()

	var result []*mywant.Want
	for _, exec := range executions {
		if exec.Builder != nil {
			result = append(result, exec.Builder.GetWants()...)
		}
	}
	if s.globalBuilder != nil {
		result = append(result, s.globalBuilder.GetWants()...)
	}
	return result
}

// findWantByIDInAll finds a Want by ID across all executions and the globalBuilder.
func (s *Server) findWantByIDInAll(wantID string) *mywant.Want {
	s.wantsMu.RLock()
	executions := make([]*WantExecution, 0, len(s.wants))
	for _, exec := range s.wants {
		executions = append(executions, exec)
	}
	s.wantsMu.RUnlock()

	for _, exec := range executions {
		if exec.Builder != nil {
			if want, _, found := exec.Builder.FindWantByID(wantID); found {
				return want
			}
		}
	}
	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			return want
		}
	}
	return nil
}

// collectDescendantIDs performs a BFS over OwnerReferences to collect all descendant
// Want IDs of the given ancestorID (exclusive; the ancestor itself is not included).
func collectDescendantIDs(ancestorID string, allWants []*mywant.Want) map[string]bool {
	childrenOf := make(map[string][]string)
	for _, w := range allWants {
		for _, ref := range w.Metadata.OwnerReferences {
			if ref.Controller {
				childrenOf[ref.ID] = append(childrenOf[ref.ID], w.Metadata.ID)
			}
		}
	}

	descendants := make(map[string]bool)
	queue := childrenOf[ancestorID]
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		descendants[id] = true
		queue = append(queue, childrenOf[id]...)
	}
	return descendants
}

// labelStringForKey returns the string representation of a Want's state label for a given key.
func labelStringForKey(want *mywant.Want, key string) string {
	if label, ok := want.StateLabels[key]; ok {
		switch label {
		case mywant.LabelGoal:
			return "goal"
		case mywant.LabelCurrent:
			return "current"
		case mywant.LabelPlan:
			return "plan"
		}
	}
	return "none"
}

// buildWantStateSnapshot builds a WantStateSnapshot for a single Want, optionally
// filtering to only state fields matching the given label ("current", "goal", "plan", "").
func buildWantStateSnapshot(want *mywant.Want, labelFilter string) WantStateSnapshot {
	explicitState := want.GetExplicitState()
	current := make(map[string]any)
	goal := make(map[string]any)
	plan := make(map[string]any)
	var finalResult any

	for k, v := range explicitState {
		if k == "final_result" {
			finalResult = v
			continue
		}
		label, hasLabel := want.StateLabels[k]
		if !hasLabel {
			label = mywant.LabelNone
		}
		switch label {
		case mywant.LabelGoal:
			if labelFilter == "" || labelFilter == "goal" {
				goal[k] = v
			}
		case mywant.LabelPlan:
			if labelFilter == "" || labelFilter == "plan" {
				plan[k] = v
			}
		default:
			if labelFilter == "" || labelFilter == "current" {
				current[k] = v
			}
		}
	}

	if labelFilter != "" && labelFilter != "current" {
		finalResult = nil
	}

	return WantStateSnapshot{
		WantID:   want.Metadata.ID,
		WantName: want.Metadata.Name,
		State:    hierarchicalState{FinalResult: finalResult, Current: current, Goal: goal, Plan: plan},
	}
}

// listStates handles GET /api/v1/states
// Query params:
//
//	include_global=false  – exclude global state (default: included)
//	ancestor_id=<id>      – limit to descendants of this Want
//	label=current|goal|plan – filter by state label
func (s *Server) listStates(w http.ResponseWriter, r *http.Request) {
	includeGlobal := r.URL.Query().Get("include_global") != "false"
	ancestorID := r.URL.Query().Get("ancestor_id")
	labelFilter := r.URL.Query().Get("label")

	allWants := s.getAllWants()

	var descendants map[string]bool
	if ancestorID != "" {
		if s.findWantByIDInAll(ancestorID) == nil {
			s.JSONError(w, r, http.StatusNotFound, "ancestor want not found", "")
			return
		}
		descendants = collectDescendantIDs(ancestorID, allWants)
	}

	snapshots := make([]WantStateSnapshot, 0, len(allWants))
	for _, want := range allWants {
		if descendants != nil && !descendants[want.Metadata.ID] {
			continue
		}
		snapshots = append(snapshots, buildWantStateSnapshot(want, labelFilter))
	}

	resp := StatesListResponse{
		Wants: snapshots,
		Total: len(snapshots),
	}
	if includeGlobal && s.globalBuilder != nil {
		resp.GlobalState = s.globalBuilder.GetGlobalStateAll()
	}

	s.JSONResponse(w, http.StatusOK, resp)
}

// searchStates handles GET /api/v1/states/search?field=<name>
// Required query param: field
// Optional: include_global, ancestor_id
func (s *Server) searchStates(w http.ResponseWriter, r *http.Request) {
	field := r.URL.Query().Get("field")
	if field == "" {
		s.JSONError(w, r, http.StatusBadRequest, "field query parameter is required", "")
		return
	}
	includeGlobal := r.URL.Query().Get("include_global") != "false"
	ancestorID := r.URL.Query().Get("ancestor_id")

	allWants := s.getAllWants()

	var descendants map[string]bool
	if ancestorID != "" {
		if s.findWantByIDInAll(ancestorID) == nil {
			s.JSONError(w, r, http.StatusNotFound, "ancestor want not found", "")
			return
		}
		descendants = collectDescendantIDs(ancestorID, allWants)
	}

	results := make([]StateSearchResult, 0)
	for _, want := range allWants {
		if descendants != nil && !descendants[want.Metadata.ID] {
			continue
		}
		allState := want.GetAllState()
		val, ok := allState[field]
		if !ok {
			continue
		}
		results = append(results, StateSearchResult{
			WantID:   want.Metadata.ID,
			WantName: want.Metadata.Name,
			Field:    field,
			Value:    val,
			Label:    labelStringForKey(want, field),
			Source:   "want",
		})
	}

	if includeGlobal && s.globalBuilder != nil {
		if val, ok := s.globalBuilder.GetGlobalStateValue(field); ok {
			results = append(results, StateSearchResult{
				Field:  field,
				Value:  val,
				Label:  "none",
				Source: "global",
			})
		}
	}

	s.JSONResponse(w, http.StatusOK, StateSearchResponse{
		Field:   field,
		Results: results,
		Total:   len(results),
	})
}

// getWantState handles GET /api/v1/states/{id}
func (s *Server) getWantState(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	want := s.findWantByIDInAll(wantID)
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", "")
		return
	}
	s.JSONResponse(w, http.StatusOK, buildWantStateSnapshot(want, ""))
}

// updateWantState handles PUT /api/v1/states/{id}
// Body: map of key-value pairs to merge into the want's state.
func (s *Server) updateWantState(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	want := s.findWantByIDInAll(wantID)
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", "")
		return
	}

	var updates map[string]any
	if err := DecodeRequest(r, &updates); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	for key, val := range updates {
		want.StoreState(key, val)
	}

	s.JSONResponse(w, http.StatusOK, buildWantStateSnapshot(want, ""))
}

// setWantStateKey handles PUT /api/v1/states/{id}/{key}
// Body: any JSON value to store under the given key.
func (s *Server) setWantStateKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]
	key := vars["key"]

	want := s.findWantByIDInAll(wantID)
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", "")
		return
	}

	var value any
	if err := DecodeRequest(r, &value); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	want.StoreState(key, value)
	s.JSONResponse(w, http.StatusOK, map[string]any{"key": key, "value": value})
}

// deleteWantStateKey handles DELETE /api/v1/states/{id}/{key}
func (s *Server) deleteWantStateKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]
	key := vars["key"]

	want := s.findWantByIDInAll(wantID)
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", "")
		return
	}

	want.DeleteState(key)
	w.WriteHeader(http.StatusNoContent)
}

// setGlobalStateKey handles PUT /api/v1/global-state/{key}
// Body: any JSON value to store under the given key.
func (s *Server) setGlobalStateKey(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	var value any
	if err := DecodeRequest(r, &value); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if s.globalBuilder != nil {
		s.globalBuilder.StoreGlobalState(key, value)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"key": key, "value": value})
}

// deleteGlobalStateKey handles DELETE /api/v1/global-state/{key}
func (s *Server) deleteGlobalStateKey(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	if s.globalBuilder != nil {
		s.globalBuilder.DeleteGlobalStateKey(key)
	}
	w.WriteHeader(http.StatusNoContent)
}
