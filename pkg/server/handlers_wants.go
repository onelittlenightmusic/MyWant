package server

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	mywant "mywant/engine/src"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

func (s *Server) createWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Read request body
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	data := buf.Bytes()

	// First try to parse as a Config
	var config mywant.Config
	var configErr error

	if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		configErr = yaml.Unmarshal(data, &config)
	} else {
		configErr = json.Unmarshal(data, &config)
	}

	// If config parsing failed or has no wants, try parsing as single Want
	if configErr != nil || len(config.Wants) == 0 {
		var newWant *mywant.Want

		if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
			configErr = yaml.Unmarshal(data, &newWant)
		} else {
			configErr = json.Unmarshal(data, &newWant)
		}

		if configErr != nil || newWant == nil {
			errorMsg := fmt.Sprintf("Invalid request: must be either a Want object or Config with wants array. Error: %v", configErr)
			s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusBadRequest, errorMsg, "")
			http.Error(w, errorMsg, http.StatusBadRequest)
			return
		}
		config = mywant.Config{Wants: []*mywant.Want{newWant}}
	}
	if err := s.validateWantTypes(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want type: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusBadRequest, errorMsg, "validation")
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}
	if err := s.validateWantSpec(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want spec: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusBadRequest, errorMsg, "validation")
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Assign IDs
	for _, want := range config.Wants {
		if want.Metadata.ID == "" {
			want.Metadata.ID = generateWantID()
		}
	}

	// Assign OrderKeys to wants that don't have them (for display ordering)
	// Find the last used order key from existing wants
	allWantStates := s.globalBuilder.GetAllWantStates()
	var lastOrderKey string
	for _, existingWant := range allWantStates {
		if existingWant.Metadata.OrderKey != "" && existingWant.Metadata.OrderKey > lastOrderKey {
			lastOrderKey = existingWant.Metadata.OrderKey
		}
	}
	log.Printf("[ORDERKEY] Found %d existing wants, last order key: '%s'\n", len(allWantStates), lastOrderKey)

	// Assign sequential order keys to new wants
	for _, want := range config.Wants {
		if want.Metadata.OrderKey == "" {
			lastOrderKey = mywant.GenerateOrderKeyAfter(lastOrderKey)
			want.Metadata.OrderKey = lastOrderKey
			log.Printf("[ORDERKEY] Assigned order key '%s' to want '%s'\n", lastOrderKey, want.Metadata.Name)
		}
	}

	executionID := generateWantID()
	execution := &WantExecution{
		ID:      executionID,
		Config:  config,
		Status:  "created",
		Builder: s.globalBuilder,
	}
	s.wants[executionID] = execution
	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(config.Wants)
	if err != nil {
		delete(s.wants, executionID)
		errorMsg := fmt.Sprintf("Failed to add wants: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusConflict, errorMsg, "duplicate_name")
		s.logError(r, http.StatusConflict, errorMsg, "duplicate_name", err.Error(), "")
		http.Error(w, errorMsg, http.StatusConflict)
		return
	}

	for _, want := range config.Wants {
		InfoLog("[API:CREATE] Want queued for addition: %s (%s, ID: %s)\n", want.Metadata.Name, want.Metadata.Type, want.Metadata.ID)
	}
	w.WriteHeader(http.StatusCreated)

	wantCount := len(config.Wants)
	wantNames := []string{}
	for _, want := range config.Wants {
		wantNames = append(wantNames, want.Metadata.Name)
	}
	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", strings.Join(wantNames, ", "), "success", http.StatusCreated, "", fmt.Sprintf("Created %d want(s)", wantCount))

	response := map[string]any{
		"id":       executionID,
		"status":   execution.Status,
		"wants":    wantCount,
		"want_ids": wantIDs,
		"message":  "Wants created and added to execution queue",
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) listWants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	includeSystemWants := false
	if includeSystemWantsStr := r.URL.Query().Get("includeSystemWants"); includeSystemWantsStr != "" {
		includeSystemWants = strings.ToLower(includeSystemWantsStr) == "true"
	}

	// Build filters from query parameters
	filters := mywant.WantFilters{
		Type:         r.URL.Query().Get("type"),
		Labels:       make(map[string]string),
		UsingFilters: make(map[string]string),
	}

	// Parse label query parameters (format: key=value)
	for _, label := range r.URL.Query()["label"] {
		parts := strings.SplitN(label, "=", 2)
		if len(parts) == 2 {
			filters.Labels[parts[0]] = parts[1]
		}
	}

	// Parse using query parameters (format: key=value)
	for _, u := range r.URL.Query()["using"] {
		parts := strings.SplitN(u, "=", 2)
		if len(parts) == 2 {
			filters.UsingFilters[parts[0]] = parts[1]
		}
	}

	wantsByID := make(map[string]*mywant.Want)

	for _, execution := range s.wants {
		if execution.Builder != nil && execution.Builder != s.globalBuilder { // Avoid duplicate if globalBuilder is also in s.wants
			currentStates := execution.Builder.GetAllWantStates()
			for _, want := range currentStates {
				wantCopy := &mywant.Want{
					Metadata:    want.Metadata,
					Spec:        want.Spec,
					Status:      want.GetStatus(),
					History:     want.History,
					State:       want.GetExplicitState(),
					HiddenState: want.GetHiddenState(),
				}
				wantsByID[want.Metadata.ID] = wantCopy
			}
		}
	}

	// Always include global builder wants
	if s.globalBuilder != nil {
		currentStates := s.globalBuilder.GetAllWantStates()
		for _, want := range currentStates {
			wantCopy := &mywant.Want{
				Metadata:    want.Metadata,
				Spec:        want.Spec,
				Status:      want.GetStatus(),
				History:     want.History,
				State:       want.GetExplicitState(),
				HiddenState: want.GetHiddenState(),
			}
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}
	allWants := make([]*mywant.Want, 0, len(wantsByID))
	for _, want := range wantsByID {
		if !includeSystemWants && want.Metadata.IsSystemWant {
			continue
		}
		// Apply filters using common filtering function
		if !want.MatchesFilters(filters) {
			continue
		}
		// Calculate hash for change detection
		want.Hash = mywant.CalculateWantHash(want)
		allWants = append(allWants, want)
	}
	response := map[string]any{
		"timestamp":    time.Now().Format(time.RFC3339),
		"execution_id": fmt.Sprintf("api-dump-%d", time.Now().Unix()),
		"wants":        allWants,
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) getWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]
	groupBy := r.URL.Query().Get("groupBy")
	includeConnectivity := r.URL.Query().Get("connectivityMetadata") == "true"

	log.Printf("[DEBUG] getWant: Looking for want ID: %s (includeConnectivity=%v)\n", wantID, includeConnectivity)

	// 1. Search in runtime wants across all executions
	for i, execution := range s.wants {
		// First try searching in the builder's internal state
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				log.Printf("[DEBUG] getWant: Found %s in runtime of execution %s\n", wantID, i)

				var wantCopy *mywant.Want
				if includeConnectivity {
					wantCopy = &mywant.Want{
						Metadata:             want.Metadata,
						Spec:                 want.Spec,
						Status:               want.GetStatus(),
						History:              want.History,
						State:                want.GetExplicitState(),
						HiddenState:          want.GetHiddenState(),
						ConnectivityMetadata: want.ConnectivityMetadata,
					}
				} else {
					wantCopy = &mywant.Want{
						Metadata:    want.Metadata,
						Spec:        want.Spec,
						Status:      want.GetStatus(),
						History:     want.History,
						State:       want.GetExplicitState(),
						HiddenState: want.GetHiddenState(),
					}
				}

				if groupBy != "" {
					response := buildWantResponse(wantCopy, groupBy)
					json.NewEncoder(w).Encode(response)
					return
				}
				json.NewEncoder(w).Encode(wantCopy)
				return
			}
		}

		// Fallback: If not in runtime yet, check the execution's config wants
		for j, want := range execution.Config.Wants {
			if want.Metadata.ID == wantID {
				log.Printf("[DEBUG] getWant: Found %s in config of execution %s at index %d\n", wantID, i, j)
				// Return the config version (it's not running yet, so no status/history)
				if groupBy != "" {
					response := buildWantResponse(want, groupBy)
					json.NewEncoder(w).Encode(response)
					return
				}
				json.NewEncoder(w).Encode(want)
				return
			}
		}
	}

	// 2. Search in global builder (runtime and config)
	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			log.Printf("[DEBUG] getWant: Found %s in globalBuilder\n", wantID)

			var wantCopy *mywant.Want
			if includeConnectivity {
				wantCopy = &mywant.Want{
					Metadata:             want.Metadata,
					Spec:                 want.Spec,
					Status:               want.GetStatus(),
					History:              want.History,
					State:                want.GetExplicitState(),
					HiddenState:          want.GetHiddenState(),
					ConnectivityMetadata: want.ConnectivityMetadata,
				}
			} else {
				wantCopy = &mywant.Want{
					Metadata:    want.Metadata,
					Spec:        want.Spec,
					Status:      want.GetStatus(),
					History:     want.History,
					State:       want.GetExplicitState(),
					HiddenState: want.GetHiddenState(),
				}
			}

			if groupBy != "" {
				response := buildWantResponse(wantCopy, groupBy)
				json.NewEncoder(w).Encode(response)
				return
			}
			json.NewEncoder(w).Encode(wantCopy)
			return
		}
	}

	log.Printf("[DEBUG] getWant: Want %s NOT FOUND in %d executions or globalBuilder\n", wantID, len(s.wants))
	http.Error(w, "Want not found", http.StatusNotFound)
}

func (s *Server) updateWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	var targetExecution *WantExecution
	var targetWantIndex int = -1
	var foundWant *mywant.Want

	for i, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				log.Printf("[API:UPDATE] Found want in execution %s\n", i)
				targetExecution = execution
				foundWant = want

				for j, configWant := range execution.Config.Wants {
					if configWant.Metadata.ID == wantID {
						targetWantIndex = j
						break
					}
				}
				break
			}
		}

		// Fallback: search in config wants directly
		for j, configWant := range execution.Config.Wants {
			if configWant.Metadata.ID == wantID {
				log.Printf("[API:UPDATE] Found want in config of execution %s\n", i)
				targetExecution = execution
				foundWant = configWant
				targetWantIndex = j
				break
			}
		}
		if foundWant != nil {
			break
		}
	}

	if targetExecution == nil || foundWant == nil {
		if s.globalBuilder != nil {
			if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
				foundWant = want
			}
		}
	}

	if foundWant == nil {
		errorMsg := "Want not found"
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}

	bodyData, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusBadRequest)
		return
	}

	var updatedWant *mywant.Want
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") {
		if err := yaml.Unmarshal(bodyData, &updatedWant); err != nil {
			http.Error(w, fmt.Sprintf("Invalid YAML: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		if err := json.Unmarshal(bodyData, &updatedWant); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
	}

	if updatedWant == nil {
		http.Error(w, "Want object required", http.StatusBadRequest)
		return
	}

	tempConfig := mywant.Config{Wants: []*mywant.Want{updatedWant}}
	if err := s.validateWantTypes(tempConfig); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.validateWantSpec(tempConfig); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Preserve the original want ID
	updatedWant.Metadata.ID = foundWant.Metadata.ID

	// Safety: Preserve ownerReferences if not provided in the request
	if updatedWant.Metadata.OwnerReferences == nil && len(foundWant.Metadata.OwnerReferences) > 0 {
		updatedWant.Metadata.OwnerReferences = foundWant.Metadata.OwnerReferences
	}

	if targetExecution != nil {
		// Update config in execution
		if targetWantIndex >= 0 && targetWantIndex < len(targetExecution.Config.Wants) {
			targetExecution.Config.Wants[targetWantIndex] = updatedWant
		} else {
			targetExecution.Config.Wants = append(targetExecution.Config.Wants, updatedWant)
		}

		if targetExecution.Builder != nil {
			targetExecution.Builder.UpdateWant(updatedWant)
		}
		targetExecution.Status = "updated"
	} else if s.globalBuilder != nil {
		s.globalBuilder.UpdateWant(updatedWant)
	}

	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "success", http.StatusOK, "", fmt.Sprintf("Updated want: %s", updatedWant.Metadata.Name))
	json.NewEncoder(w).Encode(updatedWant)
}

func (s *Server) deleteWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]

	for executionID, execution := range s.wants {
		var foundInBuilder bool

		if execution.Builder != nil {
			currentStates := execution.Builder.GetAllWantStates()
			for _, want := range currentStates {
				if want.Metadata.ID == wantID {
					foundInBuilder = true
					break
				}
			}
		}

		if foundInBuilder {
			if execution.Builder != nil {
				execution.Builder.DeleteWantsAsyncWithTracking([]string{wantID})
				s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}", wantID, "success", http.StatusNoContent, "", "Deletion queued")
			}

			// Clean up config if empty
			if len(execution.Config.Wants) == 0 {
				delete(s.wants, executionID)
			}

			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Immediately set status to deleting for quick UI feedback
			want.SetStatus(mywant.WantStatusDeleting)

			s.globalBuilder.DeleteWantsAsyncWithTracking([]string{wantID})
			s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}", wantID, "success", http.StatusNoContent, "", "Deletion queued from global builder")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	http.Error(w, "Want not found", http.StatusNotFound)
}

func (s *Server) deleteWants(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	if idsParam != "" {
		w.Header().Set("Content-Type", "application/json")
		wantIDs := strings.Split(idsParam, ",")
		if err := s.globalBuilder.QueueWantDelete(wantIDs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
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
	w.Header().Set("Content-Type", "application/json")

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
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.globalBuilder.LogAPIOperation("POST", fmt.Sprintf("/api/v1/wants/{id}/%s", operation), wantID, "success", http.StatusAccepted, "", operation+" queued")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
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

func (s *Server) handleBatchOperation(w http.ResponseWriter, r *http.Request, operation string) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
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
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/"+operation, strings.Join(body.IDs, ","), "success", http.StatusAccepted, "", "Batch "+operation+" queued")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"message": "Batch " + operation + " operation queued",
		"count":   len(body.IDs),
	})
}

func (s *Server) getWantStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	wantID := vars["id"]

	if _, _, found := s.globalBuilder.FindWantByID(wantID); found {
		// Simpler lookup via global builder which covers all
		// Need actual want object to get status
		want, _, _ := s.globalBuilder.FindWantByID(wantID)
		json.NewEncoder(w).Encode(map[string]any{
			"id":     wantID,
			"status": string(want.GetStatus()),
		})
		return
	}
	http.Error(w, "Want not found", http.StatusNotFound)
}

func (s *Server) getWantResults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	wantID := vars["id"]

	if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
		json.NewEncoder(w).Encode(map[string]any{
			"data": want.GetAllState(),
		})
		return
	}
	http.Error(w, "Want not found", http.StatusNotFound)
}

func generateWantID() string {
	uuid := make([]byte, 16)
	rand.Read(uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("want-%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// WantResponseWithGroupedAgents wraps a Want with grouped agent history
type WantResponseWithGroupedAgents struct {
	Metadata mywant.Metadata    `json:"metadata"`
	Spec     mywant.WantSpec    `json:"spec"`
	Status   mywant.WantStatus  `json:"status"`
	History  mywant.WantHistory `json:"history"`
	State    map[string]any     `json:"state"`
}

func buildWantResponse(want *mywant.Want, groupBy string) any {
	response := &WantResponseWithGroupedAgents{
		Metadata: want.Metadata,
		Spec:     want.Spec,
		Status:   want.Status,
		History:  want.History,
		State:    want.State,
	}

	if groupBy == "name" {
		response.History.GroupedAgentHistory = want.GetAgentHistoryGroupedByName()
	} else if groupBy == "type" {
		response.History.GroupedAgentHistory = want.GetAgentHistoryGroupedByType()
	}

	return response
}

// updateWantOrder handles PUT /api/v1/wants/{id}/order - updates the order key of a want for drag-and-drop reordering
func (s *Server) updateWantOrder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Parse request body
	var req struct {
		PreviousWantID string `json:"previousWantId"` // ID of want before target position (empty = first)
		NextWantID     string `json:"nextWantId"`     // ID of want after target position (empty = last)
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorMsg := fmt.Sprintf("Invalid request body: %v", err)
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}/order", wantID, "error", http.StatusBadRequest, errorMsg, "invalid_request")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Find the want to be reordered
	want, _, found := s.globalBuilder.FindWantByID(wantID)
	if !found {
		errorMsg := "Want not found"
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}/order", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}

	// Get order keys of surrounding wants
	var prevKey, nextKey string

	if req.PreviousWantID != "" {
		prevWant, _, found := s.globalBuilder.FindWantByID(req.PreviousWantID)
		if found && prevWant != nil {
			prevKey = prevWant.Metadata.OrderKey
		}
	}

	if req.NextWantID != "" {
		nextWant, _, found := s.globalBuilder.FindWantByID(req.NextWantID)
		if found && nextWant != nil {
			nextKey = nextWant.Metadata.OrderKey
		}
	}

	// Generate new order key between the two
	newOrderKey := mywant.GenerateOrderKeyBetween(prevKey, nextKey)
	want.Metadata.OrderKey = newOrderKey

	// Update the want (this will trigger persistence)
	s.globalBuilder.UpdateWant(want)

	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}/order", wantID, "success", http.StatusOK, "", fmt.Sprintf("Updated order key for want: %s", want.Metadata.Name))

	// Return success response
	response := map[string]interface{}{
		"success":  true,
		"orderKey": newOrderKey,
		"wantId":   wantID,
	}
	json.NewEncoder(w).Encode(response)
}
