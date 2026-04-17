package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

// computeCollectionHash computes a stable hash over a sorted slice of want hashes.
func computeCollectionHash(hashes []string) string {
	sorted := make([]string, len(hashes))
	copy(sorted, hashes)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, ":")))
	return fmt.Sprintf("%x", h)[:16]
}

// wantHashEntry is a lightweight representation used by the /wants/hashes endpoint.
type wantHashEntry struct {
	ID        string `json:"id"`
	Hash      string `json:"hash"`
	UpdatedAt int64  `json:"updated_at"`
}

// listWantHashes returns only the ID+hash for every visible want.
// Clients can use If-None-Match against the collection ETag to skip the call entirely.
func (s *Server) listWantHashes(w http.ResponseWriter, r *http.Request) {
	includeSystemWants := strings.ToLower(r.URL.Query().Get("includeSystemWants")) == "true"
	includeCancelled := strings.ToLower(r.URL.Query().Get("includeCancelled")) == "true"

	wantsByID := make(map[string]*mywant.Want)

	s.wantsMu.RLock()
	executions := make([]*WantExecution, 0, len(s.wants))
	for _, exec := range s.wants {
		executions = append(executions, exec)
	}
	s.wantsMu.RUnlock()

	for _, execution := range executions {
		if execution.Builder != nil && execution.Builder != s.globalBuilder {
			for _, want := range execution.Builder.GetAllWantStates() {
				wantsByID[want.Metadata.ID] = want
			}
		}
	}
	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			wantsByID[want.Metadata.ID] = want
		}
	}

	entries := make([]wantHashEntry, 0, len(wantsByID))
	rawHashes := make([]string, 0, len(wantsByID))
	for _, want := range wantsByID {
		if !includeSystemWants && want.Metadata.IsSystemWant {
			continue
		}
		if !includeCancelled && want.GetStatus() == mywant.WantStatusCancelled {
			continue
		}
		h := mywant.CalculateWantHash(want)
		entries = append(entries, wantHashEntry{
			ID:        want.Metadata.ID,
			Hash:      h,
			UpdatedAt: want.Metadata.UpdatedAt,
		})
		rawHashes = append(rawHashes, h)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	collectionHash := computeCollectionHash(rawHashes)

	ifNoneMatch := strings.Trim(r.Header.Get("If-None-Match"), `"`)
	if ifNoneMatch != "" && ifNoneMatch == collectionHash {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", `"`+collectionHash+`"`)
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"collection_hash": collectionHash,
		"wants":           entries,
	})
}

func (s *Server) createWant(w http.ResponseWriter, r *http.Request) {
	// Read request body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Failed to read body", err.Error())
		return
	}

	// First try to parse as a Config
	var config mywant.Config
	var configErr error

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") {
		configErr = yaml.Unmarshal(data, &config)
	} else {
		configErr = json.Unmarshal(data, &config)
	}

	// If config parsing failed or has no wants, try parsing as single Want
	if configErr != nil || len(config.Wants) == 0 {
		var newWant *mywant.Want
		if strings.Contains(contentType, "yaml") {
			configErr = yaml.Unmarshal(data, &newWant)
		} else {
			configErr = json.Unmarshal(data, &newWant)
		}

		if configErr != nil || newWant == nil {
			s.JSONError(w, r, http.StatusBadRequest, "Invalid request: must be either a Want object or Config with wants array", configErr.Error())
			return
		}
		config = mywant.Config{Wants: []*mywant.Want{newWant}}
	}

	if err := s.validateWantTypes(config); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid want type", err.Error())
		return
	}
	if err := s.validateWantSpec(config); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid want spec", err.Error())
		return
	}

	// Assign IDs and apply want type defaults
	for _, want := range config.Wants {
		if want.Metadata.ID == "" {
			want.Metadata.ID = generateWantID()
		}

		// Apply want type definition defaults
		typeDef := s.globalBuilder.GetWantTypeDefinition(want.Metadata.Type)
		if typeDef != nil {
			if len(want.Spec.Requires) == 0 && len(typeDef.Requires) > 0 {
				want.Spec.Requires = typeDef.Requires
			}
		}
	}

	// Assign OrderKeys
	allWantStates := s.globalBuilder.GetAllWantStates()
	var lastOrderKey string
	for _, existingWant := range allWantStates {
		if existingWant.Metadata.OrderKey != "" && existingWant.Metadata.OrderKey > lastOrderKey {
			lastOrderKey = existingWant.Metadata.OrderKey
		}
	}

	for _, want := range config.Wants {
		if want.Metadata.OrderKey == "" {
			lastOrderKey = mywant.GenerateOrderKeyAfter(lastOrderKey)
			want.Metadata.OrderKey = lastOrderKey
		}
	}

	executionID := generateWantID()
	execution := &WantExecution{
		ID:      executionID,
		Config:  config,
		Status:  "created",
		Builder: s.globalBuilder,
	}
	s.wantsMu.Lock()
	s.wants[executionID] = execution
	s.wantsMu.Unlock()

	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(config.Wants)
	if err != nil {
		s.wantsMu.Lock()
		delete(s.wants, executionID)
		s.wantsMu.Unlock()
		s.JSONError(w, r, http.StatusConflict, "Failed to add wants", err.Error())
		return
	}

	wantNames := []string{}
	for _, want := range config.Wants {
		wantNames = append(wantNames, want.Metadata.Name)
	}
	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", strings.Join(wantNames, ", "), "success", http.StatusCreated, "", fmt.Sprintf("Created %d want(s)", len(config.Wants)))

	s.JSONResponse(w, http.StatusCreated, map[string]any{
		"id":       executionID,
		"status":   execution.Status,
		"wants":    len(config.Wants),
		"want_ids": wantIDs,
		"message":  "Wants created and added to execution queue",
	})
}

func (s *Server) listWants(w http.ResponseWriter, r *http.Request) {
	includeSystemWants := strings.ToLower(r.URL.Query().Get("includeSystemWants")) == "true"
	includeCancelled := strings.ToLower(r.URL.Query().Get("includeCancelled")) == "true"

	filters := mywant.WantFilters{
		Type:         r.URL.Query().Get("type"),
		Labels:       make(map[string]string),
		UsingFilters: make(map[string]string),
	}

	for _, label := range r.URL.Query()["label"] {
		parts := strings.SplitN(label, "=", 2)
		if len(parts) == 2 {
			filters.Labels[parts[0]] = parts[1]
		}
	}

	for _, u := range r.URL.Query()["using"] {
		parts := strings.SplitN(u, "=", 2)
		if len(parts) == 2 {
			filters.UsingFilters[parts[0]] = parts[1]
		}
	}

	wantsByID := make(map[string]*mywant.Want)

	s.wantsMu.RLock()
	executions := make([]*WantExecution, 0, len(s.wants))
	for _, exec := range s.wants {
		executions = append(executions, exec)
	}
	s.wantsMu.RUnlock()

	for _, execution := range executions {
		if execution.Builder != nil && execution.Builder != s.globalBuilder {
			for _, want := range execution.Builder.GetAllWantStates() {
				wantsByID[want.Metadata.ID] = want
			}
		}
	}

	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			wantsByID[want.Metadata.ID] = want
		}
	}

	// Compute collection ETag before building full responses
	rawHashes := make([]string, 0, len(wantsByID))
	for _, want := range wantsByID {
		if !includeSystemWants && want.Metadata.IsSystemWant { continue }
		if !includeCancelled && want.GetStatus() == mywant.WantStatusCancelled { continue }
		if !want.MatchesFilters(filters) { continue }
		rawHashes = append(rawHashes, mywant.CalculateWantHash(want))
	}
	collectionHash := computeCollectionHash(rawHashes)
	w.Header().Set("ETag", `"`+collectionHash+`"`)

	ifNoneMatch := strings.Trim(r.Header.Get("If-None-Match"), `"`)
	if ifNoneMatch != "" && ifNoneMatch == collectionHash {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	allWants := make([]wantAPIResponse, 0, len(wantsByID))
	for _, want := range wantsByID {
		if !includeSystemWants && want.Metadata.IsSystemWant { continue }
		if !includeCancelled && want.GetStatus() == mywant.WantStatusCancelled { continue }
		if !want.MatchesFilters(filters) { continue }
		allWants = append(allWants, buildWantAPIResponse(want, false))
	}

	sort.Slice(allWants, func(i, j int) bool {
		keyI, keyJ := allWants[i].Metadata.OrderKey, allWants[j].Metadata.OrderKey
		if keyI == "" { keyI = allWants[i].Metadata.ID }
		if keyJ == "" { keyJ = allWants[j].Metadata.ID }
		if keyI != keyJ { return keyI < keyJ }
		return allWants[i].Metadata.ID < allWants[j].Metadata.ID
	})

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"timestamp":    time.Now().Format(time.RFC3339),
		"execution_id": fmt.Sprintf("api-dump-%d", time.Now().Unix()),
		"wants":        allWants,
	})
}

// hierarchicalState groups state fields by their label classification.
// FinalResult is promoted to the top level for convenient access.
type hierarchicalState struct {
	FinalResult any            `json:"final_result,omitempty"`
	Current     map[string]any `json:"current,omitempty"`
	Goal        map[string]any `json:"goal,omitempty"`
	Plan        map[string]any `json:"plan,omitempty"`
}

// wantAPIResponse is the API-level representation of a Want with hierarchical state.
type wantAPIResponse struct {
	Metadata             mywant.Metadata             `json:"metadata"`
	Spec                 mywant.WantSpec             `json:"spec"`
	Status               mywant.WantStatus           `json:"status,omitempty"`
	State                hierarchicalState           `json:"state"`
	StateTimestamps      map[string]time.Time        `json:"state_timestamps,omitempty"`
	HiddenState          map[string]any              `json:"hidden_state,omitempty"`
	History              mywant.WantHistory          `json:"history"`
	ConnectivityMetadata mywant.ConnectivityMetadata `json:"connectivity_metadata,omitempty"`
	Hash                 string                      `json:"hash,omitempty"`
}

// buildWantAPIResponse constructs a wantAPIResponse from a live Want, grouping state fields
// into current/goal/plan buckets. Unlabeled explicit state fields (including system-reserved
// fields like final_result) fall into the current bucket.
func buildWantAPIResponse(want *mywant.Want, includeConnectivity bool) wantAPIResponse {
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
			goal[k] = v
		case mywant.LabelPlan:
			plan[k] = v
		default:
			// LabelCurrent, LabelNone (including system-reserved fields) → current
			current[k] = v
		}
	}

	resp := wantAPIResponse{
		Metadata:        want.Metadata,
		Spec:            want.Spec,
		Status:          want.GetStatus(),
		State:           hierarchicalState{FinalResult: finalResult, Current: current, Goal: goal, Plan: plan},
		StateTimestamps: want.GetStateTimestamps(),
		HiddenState:     want.GetHiddenState(),
		History:         want.BuildHistory(),
		Hash:            mywant.CalculateWantHash(want),
	}
	if includeConnectivity {
		resp.ConnectivityMetadata = want.ConnectivityMetadata
	}
	return resp
}

func (s *Server) getWant(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	includeConnectivity := r.URL.Query().Get("connectivityMetadata") == "true"

	s.wantsMu.RLock()
	executions := make([]*WantExecution, 0, len(s.wants))
	for _, exec := range s.wants { executions = append(executions, exec) }
	s.wantsMu.RUnlock()

	serveWantResponse := func(want *mywant.Want) {
		resp := buildWantAPIResponse(want, includeConnectivity)
		w.Header().Set("ETag", `"`+resp.Hash+`"`)
		ifNoneMatch := strings.Trim(r.Header.Get("If-None-Match"), `"`)
		if ifNoneMatch != "" && ifNoneMatch == resp.Hash {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		s.JSONResponse(w, http.StatusOK, resp)
	}

	for _, execution := range executions {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				serveWantResponse(want)
				return
			}
		}
		for _, want := range execution.Config.Wants {
			if want.Metadata.ID == wantID {
				s.JSONResponse(w, http.StatusOK, want)
				return
			}
		}
	}

	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			serveWantResponse(want)
			return
		}
	}

	s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
}

func (s *Server) updateWant(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	var targetExecution *WantExecution
	var targetWantIndex int = -1
	var foundWant *mywant.Want

	s.wantsMu.RLock()
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				targetExecution, foundWant = execution, want
				for j, cw := range execution.Config.Wants {
					if cw.Metadata.ID == wantID { targetWantIndex = j; break }
				}
				break
			}
		}
		for j, cw := range execution.Config.Wants {
			if cw.Metadata.ID == wantID { targetExecution, foundWant, targetWantIndex = execution, cw, j; break }
		}
		if foundWant != nil { break }
	}
	s.wantsMu.RUnlock()

	if foundWant == nil && s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found { foundWant = want }
	}

	if foundWant == nil {
		s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
		return
	}

	var updatedWant *mywant.Want
	if err := DecodeRequest(r, &updatedWant); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	updatedWant.Metadata.ID = foundWant.Metadata.ID
	if updatedWant.Metadata.OwnerReferences == nil {
		updatedWant.Metadata.OwnerReferences = foundWant.Metadata.OwnerReferences
	}

	if targetExecution != nil {
		if targetWantIndex >= 0 { targetExecution.Config.Wants[targetWantIndex] = updatedWant
		} else { targetExecution.Config.Wants = append(targetExecution.Config.Wants, updatedWant) }
		if targetExecution.Builder != nil { targetExecution.Builder.UpdateWant(updatedWant) }
	} else if s.globalBuilder != nil {
		s.globalBuilder.UpdateWant(updatedWant)
	}

	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "success", http.StatusOK, "", fmt.Sprintf("Updated want: %s", updatedWant.Metadata.Name))
	s.JSONResponse(w, http.StatusOK, updatedWant)
}

func (s *Server) deleteWant(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	var targetBuilder *mywant.ChainBuilder
	var targetExecutionID string

	s.wantsMu.RLock()
	for eid, exec := range s.wants {
		if exec.Builder != nil {
			for _, want := range exec.Builder.GetAllWantStates() {
				if want.Metadata.ID == wantID { targetBuilder, targetExecutionID = exec.Builder, eid; break }
			}
		}
		if targetBuilder != nil { break }
	}
	s.wantsMu.RUnlock()

	if targetBuilder != nil {
		targetBuilder.DeleteWantsAsyncWithTracking([]string{wantID})
		s.wantsMu.Lock()
		if exec, ok := s.wants[targetExecutionID]; ok && len(exec.Config.Wants) == 0 { delete(s.wants, targetExecutionID) }
		s.wantsMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			want.SetStatus(mywant.WantStatusDeleting)
			s.globalBuilder.DeleteWantsAsyncWithTracking([]string{wantID})
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
}

func (s *Server) deleteWants(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	if idsParam != "" {
		wantIDs := strings.Split(idsParam, ",")
		if err := s.globalBuilder.QueueWantDelete(wantIDs); err != nil {
			s.JSONError(w, r, http.StatusInternalServerError, "Batch deletion failed", err.Error())
			return
		}
		s.JSONResponse(w, http.StatusAccepted, map[string]any{
			"message": "Batch deletion queued",
			"ids":     wantIDs,
		})
		return
	}
	s.handleBatchOperation(w, r, "delete")
}

// Batch operations
func (s *Server) suspendWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	s.handleSingleLifecycle(w, r, vars["id"], "suspend")
}
func (s *Server) resumeWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	s.handleSingleLifecycle(w, r, vars["id"], "resume")
}
func (s *Server) stopWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	s.handleSingleLifecycle(w, r, vars["id"], "stop")
}
func (s *Server) startWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	s.handleSingleLifecycle(w, r, vars["id"], "start")
}

func (s *Server) handleSingleLifecycle(w http.ResponseWriter, r *http.Request, wantID, operation string) {
	var err error
	switch operation {
	case "suspend": err = s.globalBuilder.QueueWantSuspend([]string{wantID})
	case "resume":  err = s.globalBuilder.QueueWantResume([]string{wantID})
	case "stop":    err = s.globalBuilder.QueueWantStop([]string{wantID})
	case "start":   err = s.globalBuilder.QueueWantStart([]string{wantID})
	case "restart":
		if err = s.globalBuilder.QueueWantStop([]string{wantID}); err == nil {
			err = s.globalBuilder.QueueWantStart([]string{wantID})
		}
	}

	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Operation failed", err.Error())
		return
	}

	s.globalBuilder.LogAPIOperation("POST", fmt.Sprintf("/api/v1/wants/{id}/%s", operation), wantID, "success", http.StatusAccepted, "", operation+" queued")
	s.JSONResponse(w, http.StatusAccepted, map[string]any{
		"message": operation + " operation queued",
		"wantId":  wantID,
	})
}

func (s *Server) suspendWants(w http.ResponseWriter, r *http.Request) { s.handleBatchOperation(w, r, "suspend") }
func (s *Server) resumeWants(w http.ResponseWriter, r *http.Request)  { s.handleBatchOperation(w, r, "resume") }
func (s *Server) stopWants(w http.ResponseWriter, r *http.Request)    { s.handleBatchOperation(w, r, "stop") }
func (s *Server) startWants(w http.ResponseWriter, r *http.Request)   { s.handleBatchOperation(w, r, "start") }
func (s *Server) restartWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	s.handleSingleLifecycle(w, r, vars["id"], "restart")
}
func (s *Server) restartWants(w http.ResponseWriter, r *http.Request) { s.handleBatchOperation(w, r, "restart") }

func (s *Server) handleBatchOperation(w http.ResponseWriter, r *http.Request, operation string) {
	var body struct { IDs []string `json:"ids"` }
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	var err error
	switch operation {
	case "delete":  err = s.globalBuilder.QueueWantDelete(body.IDs)
	case "suspend": err = s.globalBuilder.QueueWantSuspend(body.IDs)
	case "resume":  err = s.globalBuilder.QueueWantResume(body.IDs)
	case "stop":    err = s.globalBuilder.QueueWantStop(body.IDs)
	case "start":   err = s.globalBuilder.QueueWantStart(body.IDs)
	case "restart":
		if err = s.globalBuilder.QueueWantStop(body.IDs); err == nil {
			err = s.globalBuilder.QueueWantStart(body.IDs)
		}
	}

	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Batch operation failed", err.Error())
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/"+operation, strings.Join(body.IDs, ","), "success", http.StatusAccepted, "", "Batch "+operation+" queued")
	s.JSONResponse(w, http.StatusAccepted, map[string]any{
		"message": "Batch " + operation + " operation queued",
		"count":   len(body.IDs),
	})
}

func (s *Server) getWantStatus(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
		s.JSONResponse(w, http.StatusOK, map[string]any{
			"id":     wantID,
			"status": string(want.GetStatus()),
		})
		return
	}
	s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
}

func (s *Server) getWantResults(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
		s.JSONResponse(w, http.StatusOK, map[string]any{ "data": want.GetAllState() })
		return
	}
	s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
}

// updateWantOrder handles PUT /api/v1/wants/{id}/order
func (s *Server) updateWantOrder(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	var req struct {
		PreviousWantID string `json:"previousWantId"`
		NextWantID     string `json:"nextWantId"`
	}

	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	want, _, found := s.globalBuilder.FindWantByID(wantID)
	if !found {
		s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
		return
	}

	var prevKey, nextKey string
	if req.PreviousWantID != "" {
		if pw, _, found := s.globalBuilder.FindWantByID(req.PreviousWantID); found && pw != nil { prevKey = pw.Metadata.OrderKey }
	}
	if req.NextWantID != "" {
		if nw, _, found := s.globalBuilder.FindWantByID(req.NextWantID); found && nw != nil { nextKey = nw.Metadata.OrderKey }
	}

	newOrderKey := mywant.GenerateOrderKeyBetween(prevKey, nextKey)
	want.Metadata.OrderKey = newOrderKey
	s.globalBuilder.UpdateWant(want)

	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}/order", wantID, "success", http.StatusOK, "", fmt.Sprintf("Updated order key for want: %s", want.Metadata.Name))
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"success":  true,
		"orderKey": newOrderKey,
		"wantId":   wantID,
	})
}
