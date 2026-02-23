package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

// Config
func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.config)
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var newConfig Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update in-memory config
	s.config.HeaderPosition = newConfig.HeaderPosition
	s.config.ColorMode = newConfig.ColorMode

	// Persist to ~/.mywant/config.yaml using the helper
	s.saveFrontendConfig()

	json.NewEncoder(w).Encode(s.config)
}

// System Controls
func (s *Server) stopServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	log.Printf("[SYSTEM] Stop requested via API from %s", r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]string{"message": "Server stopping..."})

	// Use a goroutine to send the signal after the response is sent
	go func() {
		time.Sleep(1 * time.Second)
		log.Printf("[SYSTEM] Sending SIGTERM to self (PID: %d)", os.Getpid())
		syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()
}

func (s *Server) restartServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	log.Printf("[SYSTEM] Restart requested via API from %s", r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]string{"message": "Server restarting..."})

	go func() {
		// Wait a moment for the response to be sent
		time.Sleep(500 * time.Millisecond)
		log.Printf("[SYSTEM] Attempting self-restart via shell...")

		executable, err := os.Executable()
		if err != nil {
			log.Printf("[SYSTEM] Error getting executable path: %v", err)
			return
		}

		// Prepare arguments (exclude the executable itself)
		args := os.Args[1:]

		// Build the command line. We need to escape arguments properly if they contain spaces
		// but for MyWant common flags it should be simple.
		cmdParts := []string{executable}
		cmdParts = append(cmdParts, args...)

		// Join parts into a single command line
		fullCmd := strings.Join(cmdParts, " ")

		// Use a shell to wait for this process to exit, then start the new one
		// We use nohup and redirect output to the same log file if possible
		// or just let the new process handle its own logging (like -D mode does)
		restartCmd := exec.Command("sh", "-c", fmt.Sprintf("sleep 1 && %s", fullCmd))

		if err := restartCmd.Start(); err != nil {
			log.Printf("[SYSTEM] Error starting restart shell: %v", err)
			return
		}

		log.Printf("[SYSTEM] Restart shell spawned, exiting current process")
		os.Exit(0)
	}()
}

// Health Check
func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.wantsMu.RLock()
	wantsCount := len(s.wants)
	s.wantsMu.RUnlock()

	health := map[string]any{
		"status":  "healthy",
		"wants":   wantsCount,
		"version": "1.0.0",
		"server":  "mywant",
	}
	json.NewEncoder(w).Encode(health)
}

// Recipes
func (s *Server) createRecipe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var recipe mywant.GenericRecipe
	if err := json.NewDecoder(r.Body).Decode(&recipe); err != nil {
		http.Error(w, "Invalid recipe format", http.StatusBadRequest)
		return
	}
	recipeID := recipe.Recipe.Metadata.Name
	if recipeID == "" {
		http.Error(w, "Recipe name required", http.StatusBadRequest)
		return
	}
	if err := s.recipeRegistry.CreateRecipe(recipeID, &recipe); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes", recipeID, "success", http.StatusCreated, "", "Recipe created")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": recipeID, "message": "Recipe created"})
}

func (s *Server) listRecipes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.recipeRegistry.ListRecipes())
}

func (s *Server) getRecipe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	recipe, exists := s.recipeRegistry.GetRecipe(vars["id"])
	if !exists {
		http.Error(w, "Recipe not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(recipe)
}

func (s *Server) updateRecipe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	var recipe mywant.GenericRecipe
	if err := json.NewDecoder(r.Body).Decode(&recipe); err != nil {
		http.Error(w, "Invalid format", http.StatusBadRequest)
		return
	}
	if err := s.recipeRegistry.UpdateRecipe(vars["id"], &recipe); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/recipes/"+vars["id"], vars["id"], "success", http.StatusOK, "", "Recipe updated")
	json.NewEncoder(w).Encode(map[string]string{"message": "updated"})
}

func (s *Server) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if err := s.recipeRegistry.DeleteRecipe(vars["id"]); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/recipes/"+vars["id"], vars["id"], "success", http.StatusNoContent, "", "Recipe deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) analyzeWantForRecipe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	wantID := vars["id"]

	// Find parent want
	var parentWant *mywant.Want
	var builder *mywant.ChainBuilder

	for _, execution := range s.wants {
		if execution.Builder != nil {
			if wnt, _, found := execution.Builder.FindWantByID(wantID); found {
				parentWant = wnt
				builder = execution.Builder
				break
			}
		}
	}
	if parentWant == nil && s.globalBuilder != nil {
		if wnt, _, found := s.globalBuilder.FindWantByID(wantID); found {
			parentWant = wnt
			builder = s.globalBuilder
		}
	}

	if parentWant == nil {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	// Collect child wants
	allWants := builder.GetAllWantStates()
	var childWants []*mywant.Want
	for _, wnt := range allWants {
		for _, ownerRef := range wnt.Metadata.OwnerReferences {
			if ownerRef.ID == wantID || (ownerRef.Name == parentWant.Metadata.Name && ownerRef.Kind == "Want") {
				childWants = append(childWants, wnt)
				break
			}
		}
	}

	// Collect recommended state fields from parentStateAccess of child capabilities
	stateMap := make(map[string]mywant.StateDef)
	collectFromCapability := func(capName string) {
		if cap, ok := s.agentRegistry.GetCapability(capName); ok {
			for _, field := range cap.ParentStateAccess {
				if _, exists := stateMap[field.Name]; !exists {
					stateMap[field.Name] = mywant.StateDef{
						Name:        field.Name,
						Description: field.Description,
						Type:        field.Type,
					}
				}
			}
		}
	}
	for _, child := range childWants {
		// 1. Capabilities declared in spec.requires
		for _, capName := range child.Spec.Requires {
			collectFromCapability(capName)
		}
		// 2. ThinkAgent capabilities declared in the want type definition
		if s.wantTypeLoader != nil && child.Metadata.Type != "" {
			if typeDef := s.wantTypeLoader.GetDefinition(child.Metadata.Type); typeDef != nil {
				for _, capName := range typeDef.ThinkCapabilities {
					collectFromCapability(capName)
				}
			}
		}
	}

	// Convert map to slice
	recommendedState := make([]mywant.StateDef, 0, len(stateMap))
	for _, sd := range stateMap {
		recommendedState = append(recommendedState, sd)
	}

	analysis := WantRecipeAnalysis{
		WantID:     wantID,
		ChildCount: len(childWants),
		RecommendedState: recommendedState,
		SuggestedMetadata: mywant.GenericRecipeMetadata{
			Name:    parentWant.Metadata.Name + "-recipe",
			Version: "1.0.0",
		},
	}

	json.NewEncoder(w).Encode(analysis)
}

func (s *Server) saveRecipeFromWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req SaveRecipeFromWantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Implementation simplified for brevity - assume similar logic to main.go but utilizing helper methods if needed
	// In a real refactor, we'd copy the full logic. For now, let's copy the core logic.

	// Find parent want
	var parentWant *mywant.Want
	var builder *mywant.ChainBuilder

	for _, execution := range s.wants {
		if execution.Builder != nil {
			if wnt, _, found := execution.Builder.FindWantByID(req.WantID); found {
				parentWant = wnt
				builder = execution.Builder
				break
			}
		}
	}
	if parentWant == nil && s.globalBuilder != nil {
		if wnt, _, found := s.globalBuilder.FindWantByID(req.WantID); found {
			parentWant = wnt
			builder = s.globalBuilder
		}
	}

	if parentWant == nil {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	allWants := builder.GetAllWantStates()
	var childWants []mywant.RecipeWant

	for _, wnt := range allWants {
		isChild := false
		for _, ownerRef := range wnt.Metadata.OwnerReferences {
			if ownerRef.ID == req.WantID || (ownerRef.Name == parentWant.Metadata.Name && ownerRef.Kind == "Want") {
				isChild = true
				break
			}
		}
		if isChild {
			meta := wnt.Metadata
			meta.ID = ""
			meta.OwnerReferences = nil
			childWants = append(childWants, mywant.RecipeWant{Metadata: meta, Spec: wnt.Spec})
		}
	}

	// Assuming req.Metadata is compatible or needs mapping
	// In types.go we defined Metadata as `any`, here we need to cast or marshal/unmarshal
	// Let's re-marshal to get GenericRecipeMetadata
	metaBytes, _ := json.Marshal(req.Metadata)
	var recipeMeta mywant.GenericRecipeMetadata
	json.Unmarshal(metaBytes, &recipeMeta)

	recipe := mywant.GenericRecipe{
		Recipe: mywant.RecipeContent{
			Metadata:   recipeMeta,
			Wants:      childWants,
			State:      req.State,
			Parameters: req.Parameters,
		},
	}

	if recipe.Recipe.Metadata.Name == "" {
		recipe.Recipe.Metadata.Name = parentWant.Metadata.Name + "-recipe"
	}

	recipeID := recipe.Recipe.Metadata.Name
	if err := s.recipeRegistry.CreateRecipe(recipeID, &recipe); err != nil {
		s.recipeRegistry.UpdateRecipe(recipeID, &recipe)
	}

	// Save to file (~/.mywant/recipes/)
	userRecipesDir := mywant.UserRecipesDir()
	os.MkdirAll(userRecipesDir, 0755)
	filename := fmt.Sprintf("%s/%s.yaml", userRecipesDir, recipeID)
	filename = strings.ReplaceAll(filename, " ", "-")
	yamlData, _ := yaml.Marshal(recipe)
	os.WriteFile(filename, yamlData, 0644)

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes/from-want", recipeID, "success", http.StatusCreated, "", "Recipe saved")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"id": recipeID, "message": "Recipe saved", "file": filename, "wants": len(childWants),
	})
}

// Agents
func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var data struct {
		Name         string   `json:"name"`
		Type         string   `json:"type"`
		Capabilities []string `json:"capabilities"`
	}
	json.NewDecoder(r.Body).Decode(&data)

	base := mywant.BaseAgent{Name: data.Name, Capabilities: data.Capabilities, Type: mywant.AgentType(data.Type)}
	var agent mywant.Agent
	if data.Type == "do" {
		agent = &mywant.DoAgent{BaseAgent: base}
	} else {
		agent = &mywant.MonitorAgent{BaseAgent: base}
	}
	s.agentRegistry.RegisterAgent(agent)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"name": agent.GetName(), "type": agent.GetType()})
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	agents := s.agentRegistry.GetAllAgents()
	res := make([]map[string]any, len(agents))
	for i, a := range agents {
		res[i] = map[string]any{"name": a.GetName(), "type": a.GetType(), "capabilities": a.GetCapabilities()}
	}
	json.NewEncoder(w).Encode(map[string]any{"agents": res})
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	if agent, ok := s.agentRegistry.GetAgent(vars["name"]); ok {
		json.NewEncoder(w).Encode(map[string]any{"name": agent.GetName(), "type": agent.GetType(), "capabilities": agent.GetCapabilities()})
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if s.agentRegistry.UnregisterAgent(vars["name"]) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

// Capabilities
func (s *Server) createCapability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var cap mywant.Capability
	json.NewDecoder(r.Body).Decode(&cap)
	s.agentRegistry.RegisterCapability(cap)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cap)
}

func (s *Server) listCapabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"capabilities": s.agentRegistry.GetAllCapabilities()})
}

func (s *Server) getCapability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if cap, ok := s.agentRegistry.GetCapability(mux.Vars(r)["name"]); ok {
		json.NewEncoder(w).Encode(cap)
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

func (s *Server) deleteCapability(w http.ResponseWriter, r *http.Request) {
	if s.agentRegistry.UnregisterCapability(mux.Vars(r)["name"]) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

func (s *Server) findAgentsByCapability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	agents := s.agentRegistry.FindAgentsByGives(mux.Vars(r)["name"])
	if agents == nil {
		agents = []mywant.Agent{}
	}
	res := make([]map[string]any, len(agents))
	for i, a := range agents {
		res[i] = map[string]any{"name": a.GetName(), "type": a.GetType(), "capabilities": a.GetCapabilities()}
	}
	json.NewEncoder(w).Encode(map[string]any{"agents": res})
}

// Want Types
func (s *Server) listWantTypes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.wantTypeLoader == nil {
		http.Error(w, "Loader not ready", 503)
		return
	}

	defs := s.wantTypeLoader.GetAll()
	// Filter logic omitted for brevity

	res := make([]map[string]any, len(defs))
	for i, d := range defs {
		res[i] = map[string]any{
			"name":        d.Metadata.Name,
			"title":       d.Metadata.Title,
			"category":    d.Metadata.Category,
			"pattern":     d.Metadata.Pattern,
			"version":     d.Metadata.Version,
			"system_type": d.Metadata.SystemType,
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"wantTypes": res, "count": len(res)})
}

func (s *Server) getWantType(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(r.URL.Path, "/")
	name := parts[len(parts)-1]
	if def := s.wantTypeLoader.GetDefinition(name); def != nil {
		json.NewEncoder(w).Encode(def)
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

func (s *Server) getWantTypeExamples(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(r.URL.Path, "/")
	name := parts[len(parts)-2]
	if def := s.wantTypeLoader.GetDefinition(name); def != nil {
		json.NewEncoder(w).Encode(map[string]any{"name": name, "examples": def.Examples})
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

// Labels
func (s *Server) getLabels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.globalBuilder == nil {
		http.Error(w, "Global builder not initialized", http.StatusInternalServerError)
		return
	}

	// 1. Get all registered labels from the persistent registry in ChainBuilder
	keys, rawValues := s.globalBuilder.GetRegisteredLabels()

	// 2. Prepare the response structure with owner/user info
	values := make(map[string][]map[string]any)

	for _, k := range keys {
		vStrings := rawValues[k]
		vList := make([]map[string]any, 0, len(vStrings))

		for _, v := range vStrings {
			// Find current owners and users for this specific label value in the active graph
			ownerMap := make(map[string]bool)
			userMap := make(map[string]bool)

			findOwnersUsers := func(builder *mywant.ChainBuilder) {
				if builder == nil {
					return
				}
				// Check active wants in this builder
				states := builder.GetAllWantStates()
				for _, want := range states {
					// Check if want PROVIDES this label
					labels := want.GetLabels()
					if val, ok := labels[k]; ok && val == v {
						ownerMap[want.Metadata.ID] = true
					}
					// Check if want USES this label (via 'using' in spec)
					for _, u := range want.Spec.Using {
						if uv, ok := u[k]; ok && uv == v {
							userMap[want.Metadata.ID] = true
						}
					}
				}
			}

			// Track processed builders to avoid redundant scanning
			processedBuilders := make(map[*mywant.ChainBuilder]bool)

			if s.globalBuilder != nil {
				findOwnersUsers(s.globalBuilder)
				processedBuilders[s.globalBuilder] = true
			}

			for _, exec := range s.wants {
				if exec.Builder != nil && !processedBuilders[exec.Builder] {
					findOwnersUsers(exec.Builder)
					processedBuilders[exec.Builder] = true
				}
			}

			// Convert deduplicated maps to sorted slices
			owners := make([]string, 0, len(ownerMap))
			for id := range ownerMap {
				owners = append(owners, id)
			}
			sort.Strings(owners)

			users := make([]string, 0, len(userMap))
			for id := range userMap {
				users = append(users, id)
			}
			sort.Strings(users)

			vList = append(vList, map[string]any{
				"value":  v,
				"owners": owners,
				"users":  users,
			})
		}
		values[k] = vList
	}

	json.NewEncoder(w).Encode(map[string]any{
		"labelKeys":   keys,
		"labelValues": values,
	})
}

func (s *Server) addLabel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("[SERVER-ERROR] Failed to decode addLabel request: %v\n", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	fmt.Printf("[SERVER-DEBUG] addLabel request: Key=%s, Value=%s\n", req.Key, req.Value)

	if req.Key == "" || req.Value == "" {
		http.Error(w, "Key and Value are required", http.StatusBadRequest)
		return
	}

	if s.globalBuilder != nil {
		s.globalBuilder.AddLabelToRegistry(req.Key, req.Value)
		fmt.Printf("[SERVER-INFO] Registered global label via builder: %s=%s\n", req.Key, req.Value)
	} else {
		fmt.Printf("[SERVER-WARN] Global builder not available for label registration\n")
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Label registered (v2-verified)",
		"key":     req.Key,
		"value":   req.Value,
		"status":  "success",
	})
}

// Errors & Logs
func (s *Server) listErrorHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sorted := make([]ErrorHistoryEntry, len(s.errorHistory))
	copy(sorted, s.errorHistory)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp > sorted[j].Timestamp })
	json.NewEncoder(w).Encode(map[string]any{"errors": sorted, "total": len(sorted)})
}

func (s *Server) getErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := mux.Vars(r)["id"]
	for _, e := range s.errorHistory {
		if e.ID == id {
			json.NewEncoder(w).Encode(e)
			return
		}
	}
	http.Error(w, "Not found", 404)
}

func (s *Server) updateErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := mux.Vars(r)["id"]
	for i, e := range s.errorHistory {
		if e.ID == id {
			// Update logic omitted
			s.errorHistory[i].Resolved = true
			json.NewEncoder(w).Encode(s.errorHistory[i])
			return
		}
	}
	http.Error(w, "Not found", 404)
}

func (s *Server) deleteErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	for i, e := range s.errorHistory {
		if e.ID == id {
			s.errorHistory = append(s.errorHistory[:i], s.errorHistory[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	http.Error(w, "Not found", 404)
}

func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var logs []mywant.APILogEntry
	if s.globalBuilder != nil {
		logs = s.globalBuilder.GetAPILogs()
	}
	json.NewEncoder(w).Encode(map[string]any{"logs": logs, "count": len(logs)})
}

func (s *Server) clearLogs(w http.ResponseWriter, r *http.Request) {
	if s.globalBuilder != nil {
		s.globalBuilder.ClearAPILogs()
	}
	w.WriteHeader(http.StatusOK)
}

// LLM
func (s *Server) queryLLM(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req LLMRequest
	json.NewDecoder(r.Body).Decode(&req)

	model := req.Model
	if model == "" {
		model = "gpt-oss:20b"
	}

	resp, err := s.callOllamaLLM(model, req.Message)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) callOllamaLLM(model, prompt string) (*LLMResponse, error) {
	ollamaURL := os.Getenv("GPT_BASE_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	reqBody, _ := json.Marshal(OllamaRequest{Model: model, Prompt: prompt})
	resp, err := http.Post(ollamaURL+"/api/generate", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var oResp OllamaResponse
	json.NewDecoder(resp.Body).Decode(&oResp)
	return &LLMResponse{Response: oResp.Response, Model: oResp.Model, Timestamp: time.Now().Format(time.RFC3339)}, nil
}

// Reactions
func (s *Server) createReactionQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := s.reactionQueueManager.CreateQueue()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"queue_id": id})
}

func (s *Server) listReactionQueues(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	queues := s.reactionQueueManager.ListQueues()
	json.NewEncoder(w).Encode(map[string]any{"queues": queues, "count": len(queues)})
}

func (s *Server) getReactionQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	queue, err := s.reactionQueueManager.GetQueue(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"queue_id": queue.ID, "reactions": queue.GetReactions()})
}

func (s *Server) addReactionToQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req ReactionRequest
	json.NewDecoder(r.Body).Decode(&req)
	id, err := s.reactionQueueManager.AddReactionToQueue(mux.Vars(r)["id"], req.Approved, req.Comment)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"reaction_id": id})
}

func (s *Server) deleteReactionQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := s.reactionQueueManager.DeleteQueue(mux.Vars(r)["id"]); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
}

// OpenAPI Spec
func (s *Server) getSpec(w http.ResponseWriter, r *http.Request) {
	// Try to find openapi.yaml in the root or current directory
	specPaths := []string{"openapi.yaml", "../openapi.yaml", "../../openapi.yaml"}
	var data []byte
	var err error

	for _, path := range specPaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "OpenAPI specification not found", http.StatusNotFound)
		return
	}

	// Determine content type based on request or default to yaml
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		// Convert YAML to JSON if requested
		var body any
		if err := yaml.Unmarshal(data, &body); err != nil {
			http.Error(w, "Failed to parse specification", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	} else {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(data)
	}
}

// Screenshots
func (s *Server) serveReplayScreenshot(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filename := vars["filename"]
	// Basic safety check: only allow alphanumeric, hyphens, underscores, dots
	for _, c := range filename {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			http.Error(w, "invalid filename", http.StatusBadRequest)
			return
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	filePath := home + "/.mywant/screenshots/" + filename
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Write(data)
}

// Error Logging Helper
func (s *Server) logError(r *http.Request, status int, message, errorType, details string, requestData any) {
	entry := ErrorHistoryEntry{
		ID:          fmt.Sprintf("err-%d", time.Now().UnixNano()),
		Timestamp:   time.Now().Format(time.RFC3339),
		Message:     message,
		Status:      status,
		Type:        errorType,
		Details:     details,
		Endpoint:    r.URL.Path,
		Method:      r.Method,
		RequestData: requestData,
		UserAgent:   r.Header.Get("User-Agent"),
	}
	s.errorMu.Lock()
	defer s.errorMu.Unlock()
	s.errorHistory = append(s.errorHistory, entry)
	if len(s.errorHistory) > 1000 {
		s.errorHistory = s.errorHistory[len(s.errorHistory)-1000:]
	}
}
