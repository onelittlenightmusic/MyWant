package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	want_spec "github.com/onelittlenightmusic/want-spec"
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

	contentType := r.Header.Get("Content-Type")

	// Try to parse as []*want_spec.Want (bare array)
	var dtoWants []*want_spec.Want
	var parseErr error
	if strings.Contains(contentType, "yaml") {
		parseErr = yaml.Unmarshal(data, &dtoWants)
	} else {
		parseErr = json.Unmarshal(data, &dtoWants)
	}

	// Fallback: try parsing as a single want_spec.Want
	if parseErr != nil || len(dtoWants) == 0 {
		var singleDTO *want_spec.Want
		if strings.Contains(contentType, "yaml") {
			parseErr = yaml.Unmarshal(data, &singleDTO)
		} else {
			parseErr = json.Unmarshal(data, &singleDTO)
		}
		if parseErr != nil || singleDTO == nil {
			s.JSONError(w, r, http.StatusBadRequest, "Invalid request: must be a Want object or array of Wants", parseErr.Error())
			return
		}
		dtoWants = []*want_spec.Want{singleDTO}
	}

	// Convert DTOs to runtime wants
	runtimeWants := mywant.WantDTOSliceToRuntime(dtoWants)

	// Resolve fromGlobalParam references in spec.when using parameters.yaml values
	if err := mywant.ResolveFromGlobalParams(runtimeWants); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Failed to resolve fromGlobalParam", err.Error())
		return
	}

	if err := s.validateWantTypes(runtimeWants); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid want type", err.Error())
		return
	}
	if err := s.validateWantSpec(runtimeWants); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid want spec", err.Error())
		return
	}

	// Assign IDs
	for i, want := range runtimeWants {
		if want.Metadata.ID == "" {
			want.Metadata.ID = generateWantID()
			dtoWants[i].Metadata.ID = want.Metadata.ID
		}
	}

	// Run all creation hooks (OrderKey, WantTypeDefaults, CanvasCoordinate, …).
	// Hooks mutate runtimeWants in place; sync changes back to dtoWants afterward.
	allWantStates := s.globalBuilder.GetAllWantStates()
	allWantSlice := make([]*mywant.Want, 0, len(allWantStates))
	for _, w := range allWantStates {
		allWantSlice = append(allWantSlice, w)
	}
	if err := s.runWantCreationHooks(runtimeWants, allWantSlice); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Want creation hook failed", err.Error())
		return
	}
	for i, want := range runtimeWants {
		dtoWants[i].Metadata.Labels = want.Metadata.Labels
		dtoWants[i].Metadata.OrderKey = want.Metadata.OrderKey
		dtoWants[i].Spec.Requires = want.Spec.Requires
	}

	executionID := generateWantID()
	execution := &WantExecution{
		ID:     executionID,
		Wants:  dtoWants,
		Status: "created",
	}
	s.wantsMu.Lock()
	s.wants[executionID] = execution
	s.wantsMu.Unlock()

	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(runtimeWants)
	if err != nil {
		s.wantsMu.Lock()
		delete(s.wants, executionID)
		s.wantsMu.Unlock()
		s.JSONError(w, r, http.StatusConflict, "Failed to add wants", err.Error())
		return
	}

	wantNames := []string{}
	for _, want := range runtimeWants {
		wantNames = append(wantNames, want.Metadata.Name)
	}
	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", strings.Join(wantNames, ", "), "success", http.StatusCreated, "", fmt.Sprintf("Created %d want(s)", len(runtimeWants)))
	s.notifyWantCreated(runtimeWants)

	s.JSONResponse(w, http.StatusCreated, map[string]any{
		"id":       executionID,
		"status":   execution.Status,
		"wants":    len(runtimeWants),
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

	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			wantsByID[want.Metadata.ID] = want
		}
	}

	// Compute collection ETag before building full responses
	rawHashes := make([]string, 0, len(wantsByID))
	for _, want := range wantsByID {
		if !includeSystemWants && want.Metadata.IsSystemWant {
			continue
		}
		if !includeCancelled && want.GetStatus() == mywant.WantStatusCancelled {
			continue
		}
		if !want.MatchesFilters(filters) {
			continue
		}
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
		if !includeSystemWants && want.Metadata.IsSystemWant {
			continue
		}
		if !includeCancelled && want.GetStatus() == mywant.WantStatusCancelled {
			continue
		}
		if !want.MatchesFilters(filters) {
			continue
		}
		allWants = append(allWants, s.buildWantAPIResponse(want, false))
	}

	sort.Slice(allWants, func(i, j int) bool {
		keyI, keyJ := allWants[i].Metadata.OrderKey, allWants[j].Metadata.OrderKey
		if keyI == "" {
			keyI = allWants[i].Metadata.ID
		}
		if keyJ == "" {
			keyJ = allWants[j].Metadata.ID
		}
		if keyI != keyJ {
			return keyI < keyJ
		}
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
	Internal    map[string]any `json:"internal,omitempty"`
}

// ExposableFieldInfo describes a state field that the want type has declared as exposable.
type ExposableFieldInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	SubType string `json:"subType,omitempty"`
}

// enrichedCorrelationEntry mirrors want_spec.CorrelationEntry with an added RelationID.
type enrichedCorrelationEntry struct {
	RelationID string   `json:"relationID,omitempty"`
	WantID     string   `json:"wantID"`
	Labels     []string `json:"labels"`
	Rate       int      `json:"rate"`
	DataType   string   `json:"dataType,omitempty"`
}

// apiMetadata mirrors mywant.Metadata for the API response, replacing Correlation with
// enrichedCorrelationEntry so each entry carries a RelationID.
type apiMetadata struct {
	mywant.Metadata
	Correlation []enrichedCorrelationEntry `json:"correlation,omitempty"`
}

// wantAPIResponse is the API-level representation of a Want with hierarchical state.
type wantAPIResponse struct {
	Metadata             apiMetadata                 `json:"metadata"`
	Spec                 mywant.WantSpec             `json:"spec"`
	Status               mywant.WantStatus           `json:"status,omitempty"`
	State                hierarchicalState           `json:"state"`
	StateTimestamps      map[string]time.Time        `json:"state_timestamps,omitempty"`
	HiddenState          map[string]any              `json:"hidden_state,omitempty"`
	History              mywant.WantHistory          `json:"history"`
	ConnectivityMetadata mywant.ConnectivityMetadata `json:"connectivity_metadata,omitempty"`
	Hash                 string                      `json:"hash,omitempty"`
	ExposableFields      []ExposableFieldInfo        `json:"exposable_fields,omitempty"`
}

// buildWantAPIResponse constructs a wantAPIResponse from a live Want, grouping state fields
// into current/goal/plan buckets. Unlabeled explicit state fields (including system-reserved
// fields like final_result) fall into the current bucket.
func (s *Server) buildWantAPIResponse(want *mywant.Want, includeConnectivity bool) wantAPIResponse {
	explicitState := want.GetExplicitState()
	current := make(map[string]any)
	goal := make(map[string]any)
	plan := make(map[string]any)
	internal := make(map[string]any)
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
		case mywant.LabelInternal:
			internal[k] = v
		default:
			// LabelCurrent, LabelNone (including system-reserved fields) → current
			current[k] = v
		}
	}

	exposableFields := s.exposableFieldsCache[want.Metadata.Type]

	// Build enriched correlation entries with RelationIDs.
	enrichedCorr := make([]enrichedCorrelationEntry, 0, len(want.Metadata.Correlation))
	for _, ce := range want.Metadata.Correlation {
		var relationID string
		for _, l := range ce.Labels {
			if strings.HasPrefix(l, "stateAccess/consumer:expose/") {
				// This want is the provider; compute ID from own ID + field name.
				fn := strings.TrimPrefix(l, "stateAccess/consumer:expose/")
				relationID = computeRelationID(want.Metadata.ID, fn)
				break
			}
			if strings.HasPrefix(l, "stateAccess/provider:expose/") {
				// This want is the consumer; provider is ce.WantID.
				fn := strings.TrimPrefix(l, "stateAccess/provider:expose/")
				relationID = computeRelationID(ce.WantID, fn)
				break
			}
		}
		enrichedCorr = append(enrichedCorr, enrichedCorrelationEntry{
			RelationID: relationID,
			WantID:     ce.WantID,
			Labels:     ce.Labels,
			Rate:       ce.Rate,
			DataType:   ce.DataType,
		})
	}
	meta := apiMetadata{Metadata: want.Metadata, Correlation: enrichedCorr}

	resp := wantAPIResponse{
		Metadata:        meta,
		Spec:            want.Spec,
		Status:          want.GetStatus(),
		State:           hierarchicalState{FinalResult: finalResult, Current: current, Goal: goal, Plan: plan, Internal: internal},
		StateTimestamps: want.GetStateTimestamps(),
		HiddenState:     want.GetHiddenState(),
		History:         want.BuildHistory(),
		Hash:            mywant.CalculateWantHash(want),
		ExposableFields: exposableFields,
	}
	if includeConnectivity {
		resp.ConnectivityMetadata = want.ConnectivityMetadata
	}
	return resp
}

func (s *Server) getWant(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	includeConnectivity := r.URL.Query().Get("connectivityMetadata") == "true"

	serveWantResponse := func(want *mywant.Want) {
		resp := s.buildWantAPIResponse(want, includeConnectivity)
		w.Header().Set("ETag", `"`+resp.Hash+`"`)
		ifNoneMatch := strings.Trim(r.Header.Get("If-None-Match"), `"`)
		if ifNoneMatch != "" && ifNoneMatch == resp.Hash {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		s.JSONResponse(w, http.StatusOK, resp)
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
	var foundWant *mywant.Want

	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			foundWant = want
		}
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

	// Detect unknown WantSpec fields — signals the client was built against a
	// newer want-spec than this engine.  Log a warning so operators can upgrade.
	if unknown := updatedWant.Spec.UnknownFields; len(unknown) > 0 {
		log.Printf("[WARN][want-spec drift] PUT /api/v1/wants/%s: unknown spec fields received: %v — client may be using a newer want-spec version than this engine", wantID, unknown)
	}

	updatedWant.Metadata.ID = foundWant.Metadata.ID
	// Preserve metadata fields not supplied in the PUT body so partial updates don't
	// accidentally strip e.g. the name, type, labels, or ownerReferences.
	if updatedWant.Metadata.Name == "" {
		updatedWant.Metadata.Name = foundWant.Metadata.Name
	}
	if updatedWant.Metadata.Type == "" {
		updatedWant.Metadata.Type = foundWant.Metadata.Type
	}
	if updatedWant.Metadata.Labels == nil {
		updatedWant.Metadata.Labels = foundWant.Metadata.Labels
	}
	if updatedWant.Metadata.OwnerReferences == nil {
		updatedWant.Metadata.OwnerReferences = foundWant.Metadata.OwnerReferences
	}
	// Preserve spec fields not supplied in the PUT body so partial param updates don't
	// strip structural fields like Requires, Exposes, Imports, Using, FinalResultField, etc.
	// For slice/map fields we cannot distinguish "client sent []" from "client omitted the field"
	// (both deserialize to nil/len==0 due to omitempty).
	// Preserve only when the client sent a completely absent params map; for Exposes/Using/etc.
	// trust the client — an empty slice means "clear this field" (e.g. global param removal).
	if len(updatedWant.Spec.Requires) == 0 && foundWant.Spec.Requires != nil {
		updatedWant.Spec.Requires = foundWant.Spec.Requires
	}
	// Exposes: always trust the client value. An empty [] means intentional removal.
	// (Preserving would break global param drag-out which sends filtered/empty exposes.)
	if updatedWant.Spec.Imports == nil {
		updatedWant.Spec.Imports = foundWant.Spec.Imports
	}
	if len(updatedWant.Spec.Using) == 0 && foundWant.Spec.Using != nil {
		updatedWant.Spec.Using = foundWant.Spec.Using
	}
	if updatedWant.Spec.FinalResultField == "" {
		updatedWant.Spec.FinalResultField = foundWant.Spec.FinalResultField
	}

	if s.globalBuilder != nil {
		s.globalBuilder.UpdateWant(updatedWant)
		s.globalBuilder.TriggerSave()
	}

	// Record memo entries for SubType params (same as creation hook) so that
	// editing an existing want also updates suggestion history.
	for _, hook := range s.wantCreationHooks {
		if hook.Name() == "memo" {
			_ = hook.Run(updatedWant, nil, nil)
			break
		}
	}

	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "success", http.StatusOK, "", fmt.Sprintf("Updated want: %s", updatedWant.Metadata.Name))
	s.JSONResponse(w, http.StatusOK, updatedWant)
}

func (s *Server) deleteWant(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]

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
	case "suspend":
		err = s.globalBuilder.QueueWantSuspend([]string{wantID})
	case "resume":
		err = s.globalBuilder.QueueWantResume([]string{wantID})
	case "stop":
		err = s.globalBuilder.QueueWantStop([]string{wantID})
	case "start":
		err = s.globalBuilder.QueueWantStart([]string{wantID})
	case "restart":
		err = s.globalBuilder.QueueWantRestart([]string{wantID})
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

func (s *Server) suspendWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "suspend")
}
func (s *Server) resumeWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "resume")
}
func (s *Server) stopWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "stop")
}
func (s *Server) startWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "start")
}
func (s *Server) restartWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	s.handleSingleLifecycle(w, r, vars["id"], "restart")
}
func (s *Server) restartWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "restart")
}

func (s *Server) handleBatchOperation(w http.ResponseWriter, r *http.Request, operation string) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	var err error
	switch operation {
	case "delete":
		err = s.globalBuilder.QueueWantDelete(body.IDs)
	case "suspend":
		err = s.globalBuilder.QueueWantSuspend(body.IDs)
	case "resume":
		err = s.globalBuilder.QueueWantResume(body.IDs)
	case "stop":
		err = s.globalBuilder.QueueWantStop(body.IDs)
	case "start":
		err = s.globalBuilder.QueueWantStart(body.IDs)
	case "restart":
		err = s.globalBuilder.QueueWantRestart(body.IDs)
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
		s.JSONResponse(w, http.StatusOK, map[string]any{"data": want.GetAllState()})
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
		if pw, _, found := s.globalBuilder.FindWantByID(req.PreviousWantID); found && pw != nil {
			prevKey = pw.Metadata.OrderKey
		}
	}
	if req.NextWantID != "" {
		if nw, _, found := s.globalBuilder.FindWantByID(req.NextWantID); found && nw != nil {
			nextKey = nw.Metadata.OrderKey
		}
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
