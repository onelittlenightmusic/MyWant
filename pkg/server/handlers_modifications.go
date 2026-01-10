package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	mywant "mywant/engine/src"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

func (s *Server) exportWants(w http.ResponseWriter, r *http.Request) {
	includeSystemWants := false
	if includeSystemWantsStr := r.URL.Query().Get("includeSystemWants"); includeSystemWantsStr != "" {
		includeSystemWants = strings.ToLower(includeSystemWantsStr) == "true"
	}

	wantsByID := make(map[string]*mywant.Want)

	for _, execution := range s.wants {
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

	if len(wantsByID) == 0 && s.globalBuilder != nil {
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
		allWants = append(allWants, want)
	}

	config := mywant.Config{Wants: allWants}
	yamlData, err := yaml.Marshal(&config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"wants-export-%d.yaml\"", time.Now().Unix()))
	w.WriteHeader(http.StatusOK)
	w.Write(yamlData)
}

func (s *Server) importWants(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	data := buf.Bytes()

	var config mywant.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, want := range config.Wants {
		if want.Metadata.ID == "" {
			http.Error(w, "Imported wants must have IDs", http.StatusBadRequest)
			return
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
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	go func() {
		for i := 0; i < 20; i++ {
			if s.globalBuilder.AreWantsAdded(wantIDs) {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		for _, want := range config.Wants {
			if importedWant, _, found := s.globalBuilder.FindWantByID(want.Metadata.ID); found {
				if want.State != nil {
					for k, v := range want.State {
						importedWant.StoreState(k, v)
					}
				}
			}
		}
	}()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": executionID, "status": "created"})
}

func (s *Server) addLabelToWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]
	var req struct{Key, Value string}
	json.NewDecoder(r.Body).Decode(&req)

	if err := s.globalBuilder.QueueWantAddLabel(wantID, req.Key, req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"message": "queued"})
}

func (s *Server) removeLabelFromWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if err := s.globalBuilder.QueueWantRemoveLabel(vars["id"], vars["key"]); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"message": "queued"})
}

func (s *Server) addUsingDependency(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]
	var req struct{Key, Value string}
	json.NewDecoder(r.Body).Decode(&req)

	want, _, found := s.globalBuilder.FindWantByID(wantID)
	if !found {
		http.Error(w, "Not found", 404)
		return
	}

	if want.Spec.Using == nil {
		want.Spec.Using = make([]map[string]string, 0)
	}
	want.Spec.Using = append(want.Spec.Using, map[string]string{req.Key: req.Value})
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"message": "added"})
}

func (s *Server) removeUsingDependency(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]
	key := vars["key"]

	want, _, found := s.globalBuilder.FindWantByID(wantID)
	if !found {
		http.Error(w, "Not found", 404)
		return
	}

	newUsing := make([]map[string]string, 0)
	for _, dep := range want.Spec.Using {
		if _, has := dep[key]; !has {
			newUsing = append(newUsing, dep)
		}
	}
	want.Spec.Using = newUsing
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"message": "removed"})
}
