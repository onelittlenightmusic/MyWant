package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	mywant "mywant/engine/core"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

// Config
func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, s.config)
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	var newConfig Config
	if err := DecodeRequest(r, &newConfig); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Update in-memory config
	s.config.HeaderPosition = newConfig.HeaderPosition
	s.config.ColorMode = newConfig.ColorMode

	// Persist to ~/.mywant/config.yaml using the helper
	s.saveFrontendConfig()

	s.JSONResponse(w, http.StatusOK, s.config)
}

// System Controls
func (s *Server) stopServer(w http.ResponseWriter, r *http.Request) {
	log.Printf("[SYSTEM] Stop requested via API from %s", r.RemoteAddr)
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "Server stopping..."})

	// Use a goroutine to send the signal after the response is sent
	go func() {
		time.Sleep(1 * time.Second)
		log.Printf("[SYSTEM] Sending SIGTERM to self (PID: %d)", os.Getpid())
		syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()
}

func (s *Server) restartServer(w http.ResponseWriter, r *http.Request) {
	log.Printf("[SYSTEM] Restart requested via API from %s", r.RemoteAddr)
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "Server restarting..."})

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
	s.wantsMu.RLock()
	wantsCount := len(s.wants)
	s.wantsMu.RUnlock()

	health := map[string]any{
		"status":  "healthy",
		"wants":   wantsCount,
		"version": "1.0.0",
		"server":  "mywant",
	}
	s.JSONResponse(w, http.StatusOK, health)
}

// Recipes
func (s *Server) createRecipe(w http.ResponseWriter, r *http.Request) {
	var recipe mywant.GenericRecipe
	if err := DecodeRequest(r, &recipe); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid recipe format", err.Error())
		return
	}
	// Always generate a dynamic GUID for the registry ID (non-persistent)
	recipeID := uuid.New().String()
	recipe.Recipe.Metadata.ID = recipeID

	if recipe.Recipe.Metadata.Name == "" {
		s.JSONError(w, r, http.StatusBadRequest, "Recipe name required", "")
		return
	}
	if err := s.recipeRegistry.CreateRecipe(recipeID, &recipe); err != nil {
		s.JSONError(w, r, http.StatusConflict, err.Error(), "")
		return
	}

	// Save to file for persistence and to enable use as a custom target type
	userRecipesDir := mywant.UserRecipesDir()
	os.MkdirAll(userRecipesDir, 0755)

	// Determine a meaningful filename based on custom_type or name
	fileBase := recipe.Recipe.Metadata.CustomType
	if fileBase == "" {
		fileBase = recipe.Recipe.Metadata.Name
	}
	fileBase = strings.ReplaceAll(fileBase, " ", "-")
	filename := fmt.Sprintf("%s/%s.yaml", userRecipesDir, fileBase)

	// Create a copy for saving to disk without the dynamic ID
	saveRecipe := recipe
	saveRecipe.Recipe.Metadata.ID = "" // Don't persist the dynamic GUID
	yamlData, _ := yaml.Marshal(saveRecipe)
	os.WriteFile(filename, yamlData, 0644)

	// Immediately register as custom target type if custom_type is provided
	if recipe.Recipe.Metadata.CustomType != "" {
		mywant.RegisterCustomTargetType(
			s.recipeRegistry,
			recipe.Recipe.Metadata.CustomType,
			recipe.Recipe.Metadata.Description,
			filename,
			mywant.ParameterDefsToMap(recipe.Recipe.Parameters),
		)
		log.Printf("[SERVER] 🎯 Registered custom target type '%s' from newly created recipe\n", recipe.Recipe.Metadata.CustomType)
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes", recipeID, "success", http.StatusCreated, "", "Recipe created")
	s.JSONResponse(w, http.StatusCreated, map[string]string{"id": recipeID, "message": "Recipe created"})
}

func (s *Server) listRecipes(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, s.recipeRegistry.ListRecipes())
}

func (s *Server) getRecipe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	recipe, exists := s.recipeRegistry.GetRecipe(vars["id"])
	if !exists {
		s.JSONError(w, r, http.StatusNotFound, "Recipe not found", "")
		return
	}
	s.JSONResponse(w, http.StatusOK, recipe)
}

func (s *Server) updateRecipe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var recipe mywant.GenericRecipe
	if err := DecodeRequest(r, &recipe); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid format", err.Error())
		return
	}
	if err := s.recipeRegistry.UpdateRecipe(vars["id"], &recipe); err != nil {
		s.JSONError(w, r, http.StatusNotFound, err.Error(), "")
		return
	}

	// Persist changes to file
	userRecipesDir := mywant.UserRecipesDir()
	os.MkdirAll(userRecipesDir, 0755)

	// Determine filename based on custom_type or name
	fileBase := recipe.Recipe.Metadata.CustomType
	if fileBase == "" {
		fileBase = recipe.Recipe.Metadata.Name
	}
	fileBase = strings.ReplaceAll(fileBase, " ", "-")
	filename := fmt.Sprintf("%s/%s.yaml", userRecipesDir, fileBase)

	// Create a copy for saving to disk without the dynamic ID
	saveRecipe := recipe
	saveRecipe.Recipe.Metadata.ID = "" // Don't persist the dynamic GUID
	yamlData, _ := yaml.Marshal(saveRecipe)
	os.WriteFile(filename, yamlData, 0644)

	// Re-register as custom target type if custom_type is provided
	if recipe.Recipe.Metadata.CustomType != "" {
		mywant.RegisterCustomTargetType(
			s.recipeRegistry,
			recipe.Recipe.Metadata.CustomType,
			recipe.Recipe.Metadata.Description,
			filename,
			mywant.ParameterDefsToMap(recipe.Recipe.Parameters),
		)
		log.Printf("[SERVER] 🎯 Updated custom target type '%s' registration\n", recipe.Recipe.Metadata.CustomType)
	}

	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/recipes/"+vars["id"], vars["id"], "success", http.StatusOK, "", "Recipe updated")
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (s *Server) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	recipeID := vars["id"]

	recipe, exists := s.recipeRegistry.GetRecipe(recipeID)
	if !exists {
		s.JSONError(w, r, http.StatusNotFound, "Recipe not found", "")
		return
	}

	if err := s.recipeRegistry.DeleteRecipe(recipeID); err != nil {
		s.JSONError(w, r, http.StatusNotFound, err.Error(), "")
		return
	}

	// Also delete the file if it's in the user recipes directory
	userRecipesDir := mywant.UserRecipesDir()

	// Determine filename based on custom_type or name as saved
	fileBase := recipe.Recipe.Metadata.CustomType
	if fileBase == "" {
		fileBase = recipe.Recipe.Metadata.Name
	}
	fileBase = strings.ReplaceAll(fileBase, " ", "-")
	filename := fmt.Sprintf("%s/%s.yaml", userRecipesDir, fileBase)

	if _, err := os.Stat(filename); err == nil {
		os.Remove(filename)
		log.Printf("[SERVER] 🗑️ Deleted recipe file: %s\n", filename)
	}

	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/recipes/"+recipeID, recipeID, "success", http.StatusNoContent, "", "Recipe deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) analyzeWantForRecipe(w http.ResponseWriter, r *http.Request) {
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
		s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
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
		// Collect from all capabilities declared in spec.requires (includes ThinkAgent capabilities)
		for _, capName := range child.Spec.Requires {
			collectFromCapability(capName)
		}
	}

	// Convert map to slice
	recommendedState := make([]mywant.StateDef, 0, len(stateMap))
	for _, sd := range stateMap {
		recommendedState = append(recommendedState, sd)
	}

	analysis := WantRecipeAnalysis{
		WantID:           wantID,
		ChildCount:       len(childWants),
		RecommendedState: recommendedState,
		SuggestedMetadata: mywant.GenericRecipeMetadata{
			Name:    parentWant.Metadata.Name + "-recipe",
			Version: "1.0.0",
		},
	}

	s.JSONResponse(w, http.StatusOK, analysis)
}

func (s *Server) saveRecipeFromWant(w http.ResponseWriter, r *http.Request) {
	var req SaveRecipeFromWantRequest
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

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
		s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
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
			meta.Name = "" // clear runtime name; recipe loader auto-generates on deploy
			meta.OwnerReferences = nil
			childWants = append(childWants, mywant.RecipeWant{Metadata: meta, Spec: wnt.Spec})
		}
	}

	// Parameterize child want params
	generatedParams, parameterizedWants := buildParameterizedRecipe(childWants, s.wantTypeLoader)

	// Map metadata
	metaBytes, _ := json.Marshal(req.Metadata)
	var recipeMeta mywant.GenericRecipeMetadata
	json.Unmarshal(metaBytes, &recipeMeta)

	recipe := mywant.GenericRecipe{
		Recipe: mywant.RecipeContent{
			Metadata:   recipeMeta,
			Wants:      parameterizedWants,
			State:      req.State,
			Parameters: generatedParams,
		},
	}

	if recipe.Recipe.Metadata.Name == "" {
		recipe.Recipe.Metadata.Name = parentWant.Metadata.Name + "-recipe"
	}

	// Always generate a dynamic GUID for the registry ID
	recipeID := uuid.New().String()
	recipe.Recipe.Metadata.ID = recipeID

	if err := s.recipeRegistry.CreateRecipe(recipeID, &recipe); err != nil {
		s.recipeRegistry.UpdateRecipe(recipeID, &recipe)
	}

	// Save to file (~/.mywant/recipes/)
	userRecipesDir := mywant.UserRecipesDir()
	os.MkdirAll(userRecipesDir, 0755)

	fileBase := recipe.Recipe.Metadata.CustomType
	if fileBase == "" {
		fileBase = recipe.Recipe.Metadata.Name
	}
	fileBase = strings.ReplaceAll(fileBase, " ", "-")
	filename := fmt.Sprintf("%s/%s.yaml", userRecipesDir, fileBase)

	saveRecipe := recipe
	saveRecipe.Recipe.Metadata.ID = ""
	yamlData, _ := yaml.Marshal(saveRecipe)
	os.WriteFile(filename, yamlData, 0644)

	if recipe.Recipe.Metadata.CustomType != "" {
		mywant.RegisterCustomTargetType(
			s.recipeRegistry,
			recipe.Recipe.Metadata.CustomType,
			recipe.Recipe.Metadata.Description,
			filename,
			mywant.ParameterDefsToMap(recipe.Recipe.Parameters),
		)
		log.Printf("[SERVER] 🎯 Registered custom target type '%s' from newly saved recipe\n", recipe.Recipe.Metadata.CustomType)
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes/from-want", recipeID, "success", http.StatusCreated, "", "Recipe saved")
	s.JSONResponse(w, http.StatusCreated, map[string]any{
		"id": recipeID, "message": "Recipe saved", "file": filename, "wants": len(childWants),
	})
}

// buildParameterizedRecipe converts child want params from runtime-resolved values
// back into recipe-level parameter references. Only params declared in each want
// type's definition are included; runtime-injected or undeclared keys are dropped.
//
// Naming rules for recipe-level parameter names:
//   - Key appears in exactly one child → use key as-is (e.g. "budget")
//   - Key appears in multiple children → prefix with want type (e.g. "restaurant_cost")
//   - After prefixing, still conflicts (same type twice) → append _1, _2, …
//
// Returns:
//   - parameters: map of recipe-level param name → current value (used as defaults)
//   - wants: copy of childWants with Spec.Params replaced by parameter references
func inferParamType(v any) string {
	switch v.(type) {
	case int, int64:
		return "int"
	case float64:
		return "float64"
	case bool:
		return "bool"
	default:
		return "string"
	}
}

func buildParameterizedRecipe(childWants []mywant.RecipeWant, loader *mywant.WantTypeLoader) ([]mywant.ParameterDef, []mywant.RecipeWant) {
	type entry struct {
		wantIdx  int
		wantType string
		key      string
		value    any
	}

	// Step 1: collect only declared params per child, counting cross-child key usage
	var entries []entry
	keyCount := map[string]int{}

	for i, cw := range childWants {
		var declaredKeys map[string]bool
		if loader != nil {
			if typeDef := loader.GetDefinition(cw.Metadata.Type); typeDef != nil {
				declaredKeys = make(map[string]bool, len(typeDef.Parameters))
				for _, p := range typeDef.Parameters {
					declaredKeys[p.Name] = true
				}
			}
		}
		for key, value := range cw.Spec.Params {
			if declaredKeys != nil && !declaredKeys[key] {
				continue // skip params not declared in the want type definition
			}
			entries = append(entries, entry{i, cw.Metadata.Type, key, value})
			keyCount[key]++
		}
	}

	// Step 2: determine recipe-level parameter name for each entry
	type namedEntry struct {
		entry
		paramName string
	}
	paramNameCount := map[string]int{}
	var named []namedEntry
	for _, e := range entries {
		pName := e.key
		if keyCount[e.key] > 1 {
			pName = e.wantType + "_" + e.key
		}
		named = append(named, namedEntry{e, pName})
		paramNameCount[pName]++
	}

	// Step 3: disambiguate still-conflicting names (same type appears twice)
	paramNameSeen := map[string]int{}
	var parameters []mywant.ParameterDef
	childRefs := make([]map[string]string, len(childWants))
	for i := range childWants {
		childRefs[i] = map[string]string{}
	}
	for _, ne := range named {
		pName := ne.paramName
		if paramNameCount[ne.paramName] > 1 {
			paramNameSeen[ne.paramName]++
			pName = fmt.Sprintf("%s_%d", ne.paramName, paramNameSeen[ne.paramName])
		}
		parameters = append(parameters, mywant.ParameterDef{
			Name:    pName,
			Type:    inferParamType(ne.value),
			Default: ne.value,
		})
		childRefs[ne.wantIdx][ne.key] = pName
	}

	// Step 4: rebuild child wants with parameter references instead of literal values
	result := make([]mywant.RecipeWant, len(childWants))
	for i, cw := range childWants {
		newCW := cw
		newParams := make(map[string]any, len(childRefs[i]))
		for key, ref := range childRefs[i] {
			newParams[key] = ref // string reference → substituted at deploy time
		}
		newCW.Spec.SetParamsFromMap(newParams)
		result[i] = newCW
	}

	return parameters, result
}

// Agents
func (s *Server) registerAgentYAML(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Failed to read body", err.Error())
		return
	}
	// Use a placeholder path so relative script paths resolve from custom-types dir
	yamlPath := mywant.UserCustomTypesDir() + "/agent.yaml"
	if err := s.agentRegistry.RegisterMRSAgentFromYAML(body, yamlPath); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid agent YAML", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "agent registered successfully"})
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Name         string   `json:"name"`
		Type         string   `json:"type"`
		Capabilities []string `json:"capabilities"`
	}
	if err := DecodeRequest(r, &data); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	base := mywant.BaseAgent{Name: data.Name, Capabilities: data.Capabilities, Type: mywant.AgentType(data.Type)}
	var agent mywant.Agent
	if data.Type == "do" {
		agent = &mywant.DoAgent{BaseAgent: base}
	} else {
		agent = &mywant.MonitorAgent{BaseAgent: base}
	}
	s.agentRegistry.RegisterAgent(agent)
	s.JSONResponse(w, http.StatusCreated, map[string]any{"name": agent.GetName(), "type": agent.GetType()})
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	agents := s.agentRegistry.GetAllAgents()
	res := make([]map[string]any, len(agents))
	for i, a := range agents {
		res[i] = map[string]any{"name": a.GetName(), "type": a.GetType(), "capabilities": a.GetCapabilities()}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"agents": res})
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if agent, ok := s.agentRegistry.GetAgent(vars["name"]); ok {
		s.JSONResponse(w, http.StatusOK, map[string]any{"name": agent.GetName(), "type": agent.GetType(), "capabilities": agent.GetCapabilities()})
		return
	}
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if s.agentRegistry.UnregisterAgent(vars["name"]) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
}

// Capabilities
func (s *Server) createCapability(w http.ResponseWriter, r *http.Request) {
	var cap mywant.Capability
	if err := DecodeRequest(r, &cap); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}
	s.agentRegistry.RegisterCapability(cap)
	s.JSONResponse(w, http.StatusCreated, cap)
}

func (s *Server) listCapabilities(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{"capabilities": s.agentRegistry.GetAllCapabilities()})
}

func (s *Server) getCapability(w http.ResponseWriter, r *http.Request) {
	if cap, ok := s.agentRegistry.GetCapability(mux.Vars(r)["name"]); ok {
		s.JSONResponse(w, http.StatusOK, cap)
		return
	}
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
}

func (s *Server) deleteCapability(w http.ResponseWriter, r *http.Request) {
	if s.agentRegistry.UnregisterCapability(mux.Vars(r)["name"]) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
}

func (s *Server) findAgentsByCapability(w http.ResponseWriter, r *http.Request) {
	agents := s.agentRegistry.FindAgentsByGives(mux.Vars(r)["name"])
	if agents == nil {
		agents = []mywant.Agent{}
	}
	res := make([]map[string]any, len(agents))
	for i, a := range agents {
		res[i] = map[string]any{"name": a.GetName(), "type": a.GetType(), "capabilities": a.GetCapabilities()}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"agents": res})
}

// Want Types
func (s *Server) listWantTypes(w http.ResponseWriter, r *http.Request) {
	if s.wantTypeLoader == nil {
		s.JSONError(w, r, 503, "Loader not ready", "")
		return
	}

	defs := s.wantTypeLoader.GetAll()
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
	s.JSONResponse(w, http.StatusOK, map[string]any{"wantTypes": res, "count": len(res)})
}

func (s *Server) getWantType(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	name := parts[len(parts)-1]
	if def := s.wantTypeLoader.GetDefinition(name); def != nil {
		s.JSONResponse(w, http.StatusOK, def)
		return
	}
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
}

func (s *Server) getRecipeExamples(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	recipe, ok := s.recipeRegistry.GetRecipe(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Recipe not found", "")
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"id":       id,
		"name":     recipe.Recipe.Metadata.Name,
		"examples": recipe.Recipe.Examples,
	})
}

func (s *Server) getWantTypeExamples(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	name := parts[len(parts)-2]
	if def := s.wantTypeLoader.GetDefinition(name); def != nil {
		s.JSONResponse(w, http.StatusOK, map[string]any{"name": name, "examples": def.Examples})
		return
	}
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
}

// Labels
func (s *Server) getLabels(w http.ResponseWriter, r *http.Request) {
	if s.globalBuilder == nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Global builder not initialized", "")
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

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"labelKeys":   keys,
		"labelValues": values,
	})
}

func (s *Server) addLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Key == "" || req.Value == "" {
		s.JSONError(w, r, http.StatusBadRequest, "Key and Value are required", "")
		return
	}

	if s.globalBuilder != nil {
		s.globalBuilder.AddLabelToRegistry(req.Key, req.Value)
	}

	s.JSONResponse(w, http.StatusCreated, map[string]string{
		"message": "Label registered (v2-verified)",
		"key":     req.Key,
		"value":   req.Value,
		"status":  "success",
	})
}

// Errors & Logs
func (s *Server) listErrorHistory(w http.ResponseWriter, r *http.Request) {
	sorted := make([]ErrorHistoryEntry, len(s.errorHistory))
	copy(sorted, s.errorHistory)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp > sorted[j].Timestamp })
	s.JSONResponse(w, http.StatusOK, map[string]any{"errors": sorted, "total": len(sorted)})
}

func (s *Server) getErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	for _, e := range s.errorHistory {
		if e.ID == id {
			s.JSONResponse(w, http.StatusOK, e)
			return
		}
	}
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
}

func (s *Server) updateErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	for i, e := range s.errorHistory {
		if e.ID == id {
			s.errorHistory[i].Resolved = true
			s.JSONResponse(w, http.StatusOK, s.errorHistory[i])
			return
		}
	}
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
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
	s.JSONError(w, r, http.StatusNotFound, "Not found", "")
}

func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	var logs []mywant.APILogEntry
	if s.globalBuilder != nil {
		logs = s.globalBuilder.GetAPILogs()
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"logs": logs, "count": len(logs)})
}

func (s *Server) clearLogs(w http.ResponseWriter, r *http.Request) {
	if s.globalBuilder != nil {
		s.globalBuilder.ClearAPILogs()
	}
	w.WriteHeader(http.StatusOK)
}

// LLM
func (s *Server) queryLLM(w http.ResponseWriter, r *http.Request) {
	var req LLMRequest
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	model := req.Model
	if model == "" {
		model = "gpt-oss:20b"
	}

	resp, err := s.callOllamaLLM(model, req.Message)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "LLM query failed", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, resp)
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
	id, err := s.reactionQueueManager.CreateQueue()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to create reaction queue", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusCreated, map[string]string{"queue_id": id})
}

func (s *Server) listReactionQueues(w http.ResponseWriter, r *http.Request) {
	queues := s.reactionQueueManager.ListQueues()
	s.JSONResponse(w, http.StatusOK, map[string]any{"queues": queues, "count": len(queues)})
}

func (s *Server) getReactionQueue(w http.ResponseWriter, r *http.Request) {
	queue, err := s.reactionQueueManager.GetQueue(mux.Vars(r)["id"])
	if err != nil {
		s.JSONError(w, r, http.StatusNotFound, "Queue not found", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"queue_id": queue.ID, "reactions": queue.GetReactions()})
}

func (s *Server) addReactionToQueue(w http.ResponseWriter, r *http.Request) {
	var req ReactionRequest
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}
	id, err := s.reactionQueueManager.AddReactionToQueue(mux.Vars(r)["id"], req.Approved, req.Comment)
	if err != nil {
		s.JSONError(w, r, http.StatusNotFound, "Queue not found", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]string{"reaction_id": id})
}

func (s *Server) deleteReactionQueue(w http.ResponseWriter, r *http.Request) {
	if err := s.reactionQueueManager.DeleteQueue(mux.Vars(r)["id"]); err != nil {
		s.JSONError(w, r, http.StatusNotFound, "Queue not found", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]bool{"deleted": true})
}

// OpenAPI Spec
func (s *Server) getSpec(w http.ResponseWriter, r *http.Request) {
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
		s.JSONError(w, r, http.StatusNotFound, "OpenAPI specification not found", err.Error())
		return
	}

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		var body any
		if err := yaml.Unmarshal(data, &body); err != nil {
			s.JSONError(w, r, http.StatusInternalServerError, "Failed to parse specification", err.Error())
			return
		}
		s.JSONResponse(w, http.StatusOK, body)
	} else {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(data)
	}
}

// Screenshots
func (s *Server) serveReplayScreenshot(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filename := vars["filename"]
	for _, c := range filename {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			s.JSONError(w, r, http.StatusBadRequest, "invalid filename", "")
			return
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "server error", err.Error())
		return
	}
	filePath := home + "/.mywant/screenshots/" + filename
	data, err := os.ReadFile(filePath)
	if err != nil {
		s.JSONError(w, r, http.StatusNotFound, "not found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Write(data)
}

// GlobalState
func hashJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)[:16]
}

func (s *Server) getGlobalState(w http.ResponseWriter, r *http.Request) {
	var stateMap map[string]any
	if s.globalBuilder != nil {
		stateMap = s.globalBuilder.GetGlobalStateAll()
	}
	if stateMap == nil {
		stateMap = make(map[string]any)
	}

	etag := hashJSON(stateMap)
	w.Header().Set("ETag", `"`+etag+`"`)
	ifNoneMatch := strings.Trim(r.Header.Get("If-None-Match"), `"`)
	if ifNoneMatch != "" && ifNoneMatch == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"state":     stateMap,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) getGlobalParameters(w http.ResponseWriter, r *http.Request) {
	params := mywant.GetAllGlobalParameters()

	etag := hashJSON(params)
	w.Header().Set("ETag", `"`+etag+`"`)
	ifNoneMatch := strings.Trim(r.Header.Get("If-None-Match"), `"`)
	if ifNoneMatch != "" && ifNoneMatch == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"parameters": params,
		"count":      len(params),
		"types":      mywant.GetGlobalParamTypes(),
	})
}

func (s *Server) updateGlobalParameters(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Parameters map[string]any `json:"parameters"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if body.Parameters == nil {
		body.Parameters = make(map[string]any)
	}
	if err := mywant.SetAllGlobalParameters(body.Parameters); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to save parameters", err.Error())
		return
	}
	params := mywant.GetAllGlobalParameters()
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"parameters": params,
		"count":      len(params),
		"types":      mywant.GetGlobalParamTypes(),
	})
}

func (s *Server) deleteGlobalState(w http.ResponseWriter, r *http.Request) {
	if s.globalBuilder != nil {
		s.globalBuilder.ClearGlobalState()
		s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/global-state", "", "success", http.StatusNoContent, "", "Global state cleared")
	}
	w.WriteHeader(http.StatusNoContent)
}

const guiStateWantID = "system-gui-state"

// guiStateSeq is a monotonically increasing counter incremented on every PUT to /api/v1/gui/state.
// Clients use it to detect changes without relying on the source field, enabling multi-tab sync.
var guiStateSeq int64
var guiStateSeqMu sync.Mutex

func nextGUIStateSeq() int64 {
	guiStateSeqMu.Lock()
	defer guiStateSeqMu.Unlock()
	guiStateSeq++
	return guiStateSeq
}

func currentGUIStateSeq() int64 {
	guiStateSeqMu.Lock()
	defer guiStateSeqMu.Unlock()
	return guiStateSeq
}

// guiStateResponse wraps the GUI state with a monotonic seq number for multi-tab sync.
type guiStateResponse struct {
	Seq   int64          `json:"seq"`
	State map[string]any `json:"state"`
}

// getGUIState handles GET /api/v1/gui/state
func (s *Server) getGUIState(w http.ResponseWriter, r *http.Request) {
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "gui_state want not found", "")
		return
	}
	s.JSONResponse(w, http.StatusOK, guiStateResponse{
		Seq:   currentGUIStateSeq(),
		State: guiFields(want),
	})
}

// updateGUIState handles PUT /api/v1/gui/state
// Merges the request body into the gui_state want's state and increments seq.
func (s *Server) updateGUIState(w http.ResponseWriter, r *http.Request) {
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "gui_state want not found", "")
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
	s.JSONResponse(w, http.StatusOK, guiStateResponse{
		Seq:   nextGUIStateSeq(),
		State: guiFields(want),
	})
}

// guiFields returns the declared GUI state fields via ProvidedStateFields.
func guiFields(want *mywant.Want) map[string]any {
	return want.GetExplicitState()
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
