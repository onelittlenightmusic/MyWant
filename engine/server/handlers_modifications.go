package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"
)

// exportableWants builds fresh, detached *Want copies (metadata/spec/status/
// history/explicit-state) of every currently-running want, stably sorted by
// orderKey then ID. System wants (gui_state etc.) are excluded unless
// includeSystemWants is true. Shared by exportWants and the world-snapshot
// save logic in handlers_worlds.go.
func (s *Server) exportableWants(includeSystemWants bool) []*mywant.Want {
	wantsByID := make(map[string]*mywant.Want)

	if s.globalBuilder != nil {
		currentStates := s.globalBuilder.GetAllWantStates()
		for _, want := range currentStates {
			wantCopy := &mywant.Want{
				Metadata:    want.Metadata,
				Spec:        want.Spec,
				Status:      want.GetStatus(),
				History:     want.BuildHistory(),
				HiddenState: want.GetHiddenState(),
			}
			mywant.StoreStateMulti(wantCopy, want.GetExplicitState())
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

	// Explicitly sort for stable ordering
	sort.Slice(allWants, func(i, j int) bool {
		keyI := allWants[i].Metadata.OrderKey
		if keyI == "" {
			keyI = allWants[i].Metadata.ID
		}
		keyJ := allWants[j].Metadata.OrderKey
		if keyJ == "" {
			keyJ = allWants[j].Metadata.ID
		}
		if keyI != keyJ {
			return keyI < keyJ
		}
		return allWants[i].Metadata.ID < allWants[j].Metadata.ID
	})

	return allWants
}

func (s *Server) exportWants(w http.ResponseWriter, r *http.Request) {
	includeSystemWants := false
	if includeSystemWantsStr := r.URL.Query().Get("includeSystemWants"); includeSystemWantsStr != "" {
		includeSystemWants = strings.ToLower(includeSystemWantsStr) == "true"
	}

	allWants := s.exportableWants(includeSystemWants)

	yamlData, err := yaml.Marshal(allWants)
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

	var importedWants []*mywant.Want
	if err := yaml.Unmarshal(data, &importedWants); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, want := range importedWants {
		if want.Metadata.ID == "" {
			http.Error(w, "Imported wants must have IDs", http.StatusBadRequest)
			return
		}
	}

	dtoWants := mywant.RuntimeWantsToDTOSlice(importedWants)
	executionID := generateWantID()
	execution := &WantExecution{
		ID:     executionID,
		Wants:  dtoWants,
		Status: "created",
	}
	s.wants[executionID] = execution

	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(importedWants)
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
		for _, want := range importedWants {
			if rw, _, found := s.globalBuilder.FindWantByID(want.Metadata.ID); found {
				mywant.StoreStateMulti(rw, want.GetAllState())
			}
		}
	}()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": executionID, "status": "created"})
}

func (s *Server) addLabelToWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]
	var req struct{ Key, Value string }
	json.NewDecoder(r.Body).Decode(&req)

	// Game mode locks canvas tile positions — reject only if the value
	// actually changes (mirrors the guard in updateWant).
	if s.config.InteractionMode == "game" && (req.Key == canvasLabelX || req.Key == canvasLabelY) {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found && want.Metadata.Labels[req.Key] != req.Value {
			http.Error(w, "Tile movement is disabled in game mode", http.StatusConflict)
			return
		}
	}

	if err := s.globalBuilder.QueueWantAddLabel(wantID, req.Key, req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"message": "queued"})
}

func (s *Server) removeLabelFromWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	// Game mode locks canvas tile positions — removing the x/y label would
	// reset placement back to auto-layout, so block it same as a move.
	if s.config.InteractionMode == "game" && (key == canvasLabelX || key == canvasLabelY) {
		http.Error(w, "Tile movement is disabled in game mode", http.StatusConflict)
		return
	}

	if err := s.globalBuilder.QueueWantRemoveLabel(vars["id"], key); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"message": "queued"})
}

func (s *Server) addUsingDependency(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]
	var req struct{ Key, Value string }
	json.NewDecoder(r.Body).Decode(&req)

	want, _, found := s.globalBuilder.FindWantByID(wantID)
	if !found {
		http.Error(w, "Not found", 404)
		return
	}

	if want.Spec.Using == nil {
		want.Spec.Using = make([]ws.UsingEntry, 0)
	}
	want.Spec.Using = append(want.Spec.Using, ws.UsingEntry{Labels: map[string]string{req.Key: req.Value}})

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

	newUsing := make([]ws.UsingEntry, 0)
	for _, dep := range want.Spec.Using {
		if _, has := dep.Labels[key]; !has {
			newUsing = append(newUsing, dep)
		}
	}
	want.Spec.Using = newUsing

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"message": "removed"})
}

// addRelation adds an expose entry to a want's Spec.Exposes.
//
// POST /api/v1/wants/{id}/relations
// Body: { "as": "temperature", "currentState": "temperature" }
//
//	or: { "as": "temperature", "param": "temperature" }
func (s *Server) addRelation(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	var req struct {
		As           string `json:"as"`
		Param        string `json:"param,omitempty"`
		CurrentState string `json:"currentState,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.As == "" {
		http.Error(w, `{"error":"as is required"}`, http.StatusBadRequest)
		return
	}

	want, _, found := s.globalBuilder.FindWantByID(wantID)
	if !found {
		http.Error(w, `{"error":"want not found"}`, http.StatusNotFound)
		return
	}

	entry := mywant.ExposeEntry{
		As:           req.As,
		Param:        req.Param,
		CurrentState: req.CurrentState,
	}
	want.Spec.Exposes = append(want.Spec.Exposes, entry)
	s.globalBuilder.UpdateWant(want)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"message": "added"})
}

// removeRelation removes an expose entry from a want's Spec.Exposes.
//
// DELETE /api/v1/wants/{id}/relations
// Body: { "label": "expose/temperature" }
//
// The label must start with "expose/". Removes the ExposeEntry from the provider want
// and also removes the matching import key from all consumer wants that reference it.
func (s *Server) removeRelation(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Label == "" {
		http.Error(w, `{"error":"label is required"}`, http.StatusBadRequest)
		return
	}
	asKey := strings.TrimPrefix(req.Label, "expose/")
	if asKey == req.Label {
		http.Error(w, `{"error":"label must start with expose/"}`, http.StatusBadRequest)
		return
	}

	want, _, found := s.globalBuilder.FindWantByID(wantID)
	if !found {
		http.Error(w, `{"error":"want not found"}`, http.StatusNotFound)
		return
	}

	// Remove the ExposeEntry from the provider want.
	newExposes := make([]mywant.ExposeEntry, 0, len(want.Spec.Exposes))
	removed := false
	for _, e := range want.Spec.Exposes {
		if e.As == asKey {
			removed = true
		} else {
			newExposes = append(newExposes, e)
		}
	}
	if !removed {
		http.Error(w, `{"error":"expose entry not found"}`, http.StatusNotFound)
		return
	}
	want.Spec.Exposes = newExposes
	s.globalBuilder.UpdateWant(want)

	// Remove the matching import key from all consumer wants that reference asKey.
	for _, w := range s.globalBuilder.GetWants() {
		if _, ok := w.Spec.Imports[asKey]; !ok {
			continue
		}
		newImports := make(map[string]string, len(w.Spec.Imports)-1)
		for k, v := range w.Spec.Imports {
			if k != asKey {
				newImports[k] = v
			}
		}
		w.Spec.Imports = newImports
		s.globalBuilder.UpdateWant(w)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"message": "removed"})
}

// getWantCluster returns all wants in the same expose-based cluster as the given want.
//
// GET /api/v1/wants/{id}/cluster
//
// A cluster is the connected component reachable via expose-based stateAccess
// correlations (labels prefixed "stateAccess/consumer:expose/" or
// "stateAccess/provider:expose/").  The response includes the root want itself.
func (s *Server) getWantCluster(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]

	// BFS through expose-linked correlations to find all cluster members.
	visited := map[string]bool{wantID: true}
	queue := []string{wantID}
	var members []ClusterMember

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		want, _, found := s.globalBuilder.FindWantByID(current)
		if !found {
			continue
		}

		x, _ := strconv.Atoi(want.Metadata.Labels["mywant.io/canvas-x"])
		y, _ := strconv.Atoi(want.Metadata.Labels["mywant.io/canvas-y"])
		members = append(members, ClusterMember{
			ID:   current,
			Name: want.Metadata.Name,
			X:    x,
			Y:    y,
		})

		for _, corr := range want.Metadata.Correlation {
			if visited[corr.WantID] {
				continue
			}
			for _, l := range corr.Labels {
				if strings.HasPrefix(l, "stateAccess/consumer:expose/") ||
					strings.HasPrefix(l, "stateAccess/provider:expose/") {
					visited[corr.WantID] = true
					queue = append(queue, corr.WantID)
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ClusterResponse{
		RootID:  wantID,
		Members: members,
	})
}
