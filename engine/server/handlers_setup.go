package server

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"mywant/engine/core"

	"gopkg.in/yaml.v3"
)

func (s *Server) setupRoutes() {
	s.router.Use(corsMiddleware)

	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Wants CRUD
	wants := api.PathPrefix("/wants").Subrouter()
	wants.HandleFunc("", s.createWant).Methods("POST")
	wants.HandleFunc("", s.listWants).Methods("GET")
	wants.HandleFunc("", s.deleteWants).Methods("DELETE")
	wants.HandleFunc("", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/validate", s.validateWant).Methods("POST")
	wants.HandleFunc("/validate", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/export", s.exportWants).Methods("POST", "OPTIONS")
	wants.HandleFunc("/import", s.importWants).Methods("POST", "OPTIONS")
	wants.HandleFunc("/{id}", s.getWant).Methods("GET")
	wants.HandleFunc("/{id}", s.updateWant).Methods("PUT")
	wants.HandleFunc("/{id}", s.deleteWant).Methods("DELETE")
	wants.HandleFunc("/{id}", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/status", s.getWantStatus).Methods("GET")
	wants.HandleFunc("/{id}/results", s.getWantResults).Methods("GET")
	wants.HandleFunc("/{id}/suspend", s.suspendWant).Methods("POST")
	wants.HandleFunc("/{id}/resume", s.resumeWant).Methods("POST")
	wants.HandleFunc("/{id}/stop", s.stopWant).Methods("POST")
	wants.HandleFunc("/{id}/start", s.startWant).Methods("POST")
	wants.HandleFunc("/suspend", s.suspendWants).Methods("POST")
	wants.HandleFunc("/resume", s.resumeWants).Methods("POST")
	wants.HandleFunc("/stop", s.stopWants).Methods("POST")
	wants.HandleFunc("/start", s.startWants).Methods("POST")
	wants.HandleFunc("/{id}/recipe-analysis", s.analyzeWantForRecipe).Methods("GET")
	wants.HandleFunc("/{id}/recipe-analysis", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/labels", s.addLabelToWant).Methods("POST")
	wants.HandleFunc("/{id}/labels/{key}", s.removeLabelFromWant).Methods("DELETE")
	wants.HandleFunc("/{id}/labels", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/order", s.updateWantOrder).Methods("PUT", "OPTIONS")
	wants.HandleFunc("/{id}/using", s.addUsingDependency).Methods("POST")
	wants.HandleFunc("/{id}/using/{key}", s.removeUsingDependency).Methods("DELETE")
	wants.HandleFunc("/{id}/using", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/using/{key}", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/labels/{key}", s.handleOptions).Methods("OPTIONS")

	// Agents CRUD
	agents := api.PathPrefix("/agents").Subrouter()
	agents.HandleFunc("", s.createAgent).Methods("POST")
	agents.HandleFunc("", s.listAgents).Methods("GET")
	agents.HandleFunc("/{name}", s.getAgent).Methods("GET")
	agents.HandleFunc("/{name}", s.deleteAgent).Methods("DELETE")

	// Capabilities CRUD
	capabilities := api.PathPrefix("/capabilities").Subrouter()
	capabilities.HandleFunc("", s.createCapability).Methods("POST")
	capabilities.HandleFunc("", s.listCapabilities).Methods("GET")
	capabilities.HandleFunc("/{name}", s.getCapability).Methods("GET")
	capabilities.HandleFunc("/{name}", s.deleteCapability).Methods("DELETE")
	capabilities.HandleFunc("/{name}/agents", s.findAgentsByCapability).Methods("GET")

	// Recipe CRUD
	recipes := api.PathPrefix("/recipes").Subrouter()
	s.router.HandleFunc("/api/v1/recipes", s.createRecipe).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/v1/recipes/from-want", s.saveRecipeFromWant).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/v1/recipes", s.listRecipes).Methods("GET", "OPTIONS")
	recipes.HandleFunc("/{id}", s.getRecipe).Methods("GET")
	recipes.HandleFunc("/{id}", s.updateRecipe).Methods("PUT")
	recipes.HandleFunc("/{id}", s.deleteRecipe).Methods("DELETE")

	// Want Type endpoints
	wantTypes := api.PathPrefix("/want-types").Subrouter()
	wantTypes.HandleFunc("", s.listWantTypes).Methods("GET")
	wantTypes.HandleFunc("", s.handleOptions).Methods("OPTIONS")
	wantTypes.HandleFunc("/{name}", s.getWantType).Methods("GET")
	wantTypes.HandleFunc("/{name}", s.handleOptions).Methods("OPTIONS")
	wantTypes.HandleFunc("/{name}/examples", s.getWantTypeExamples).Methods("GET")
	wantTypes.HandleFunc("/{name}/examples", s.handleOptions).Methods("OPTIONS")

	// Labels endpoints
	labels := api.PathPrefix("/labels").Subrouter()
	labels.HandleFunc("", s.getLabels).Methods("GET")
	labels.HandleFunc("", s.addLabel).Methods("POST")
	labels.HandleFunc("", s.handleOptions).Methods("OPTIONS")

	// Utilities
	errors := api.PathPrefix("/errors").Subrouter()
	errors.HandleFunc("", s.listErrorHistory).Methods("GET")
	errors.HandleFunc("/{id}", s.getErrorHistoryEntry).Methods("GET")
	errors.HandleFunc("/{id}", s.updateErrorHistoryEntry).Methods("PUT")
	errors.HandleFunc("/{id}", s.deleteErrorHistoryEntry).Methods("DELETE")

	logs := api.PathPrefix("/logs").Subrouter()
	logs.HandleFunc("", s.getLogs).Methods("GET")
	logs.HandleFunc("", s.clearLogs).Methods("DELETE")
	logs.HandleFunc("", s.handleOptions).Methods("OPTIONS")

	llm := api.PathPrefix("/llm").Subrouter()
	llm.HandleFunc("/query", s.queryLLM).Methods("POST")
	llm.HandleFunc("/query", s.handleOptions).Methods("OPTIONS")

	// Interactive want creation endpoints
	interact := api.PathPrefix("/interact").Subrouter()
	// Session management
	interact.HandleFunc("", s.interactCreate).Methods("POST")
	interact.HandleFunc("", s.handleOptions).Methods("OPTIONS")
	// Session operations
	interact.HandleFunc("/{id}", s.interactMessage).Methods("POST")
	interact.HandleFunc("/{id}", s.interactDelete).Methods("DELETE")
	interact.HandleFunc("/{id}", s.handleOptions).Methods("OPTIONS")
	// Deployment
	interact.HandleFunc("/{id}/deploy", s.interactDeploy).Methods("POST")
	interact.HandleFunc("/{id}/deploy", s.handleOptions).Methods("OPTIONS")

	// OpenAPI Spec
	api.HandleFunc("/spec", s.getSpec).Methods("GET")

	// System controls
	api.HandleFunc("/system/stop", s.stopServer).Methods("POST", "OPTIONS")
	api.HandleFunc("/system/restart", s.restartServer).Methods("POST", "OPTIONS")

	// Config endpoint
	api.HandleFunc("/config", s.getConfig).Methods("GET", "OPTIONS")
	api.HandleFunc("/config", s.updateConfig).Methods("PUT", "OPTIONS")

	// Reactions endpoints
	reactions := api.PathPrefix("/reactions").Subrouter()
	reactions.HandleFunc("/", s.createReactionQueue).Methods("POST")
	reactions.HandleFunc("/", s.listReactionQueues).Methods("GET")
	reactions.HandleFunc("/", s.handleOptions).Methods("OPTIONS")
	reactions.HandleFunc("/{id}", s.getReactionQueue).Methods("GET")
	reactions.HandleFunc("/{id}", s.addReactionToQueue).Methods("PUT")
	reactions.HandleFunc("/{id}", s.deleteReactionQueue).Methods("DELETE")
	reactions.HandleFunc("/{id}", s.handleOptions).Methods("OPTIONS")

	// Webhooks
	webhooks := api.PathPrefix("/webhooks").Subrouter()
	webhooks.HandleFunc("/{id}", s.receiveWebhook).Methods("POST")
	webhooks.HandleFunc("/{id}", s.handleOptions).Methods("OPTIONS")
	webhooks.HandleFunc("", s.listWebhookEndpoints).Methods("GET")
	webhooks.HandleFunc("", s.handleOptions).Methods("OPTIONS")

	// Screenshots (replay)
	api.HandleFunc("/screenshots/{filename}", s.serveReplayScreenshot).Methods("GET")

	// Health check
	s.router.HandleFunc("/health", s.healthCheck).Methods("GET")

	// Static files (embedded React GUI)
	if s.config.WebFS != nil {
		s.router.PathPrefix("/").Handler(http.FileServer(s.config.WebFS)).Methods("GET", "HEAD")
	}
}

// loadRecipeFilesIntoRegistry loads recipe YAML files into the recipe registry for the API
func loadRecipeFilesIntoRegistry(recipeDir string, registry *mywant.CustomTargetTypeRegistry) error {
	if _, err := os.Stat(recipeDir); os.IsNotExist(err) {
		log.Printf("[SERVER] Recipe directory '%s' does not exist, skipping recipe loading\n", recipeDir)
		return nil
	}
	loader := mywant.NewGenericRecipeLoader(recipeDir)

	// List all recipe files
	recipes, err := loader.ListRecipes()
	if err != nil {
		return fmt.Errorf("failed to list recipes: %v", err)
	}

	log.Printf("[SERVER] Loading %d recipe files into registry...\n", len(recipes))

	// Load each recipe file
	loadedCount := 0
	for _, relativePath := range recipes {
		fullPath := fmt.Sprintf("%s/%s", recipeDir, relativePath)

		// Read and parse the recipe file directly
		data, err := os.ReadFile(fullPath)
		if err != nil {
			log.Printf("[SERVER] Warning: Failed to read recipe %s: %v\n", relativePath, err)
			continue
		}

		var recipe mywant.GenericRecipe
		if err := yaml.Unmarshal(data, &recipe); err != nil {
			log.Printf("[SERVER] Warning: Failed to parse recipe %s: %v\n", relativePath, err)
			continue
		}

		// Generate a dynamic GUID for the recipe ID (non-persistent)
		recipeID := uuid.New().String()
		recipe.Recipe.Metadata.ID = recipeID

		if err := registry.CreateRecipe(recipeID, &recipe); err != nil {
			log.Printf("[SERVER] Warning: Failed to register recipe %s: %v\n", recipeID, err)
			continue
		}

		log.Printf("[SERVER] ✅ Loaded recipe: %s (ID: %s)\n", recipe.Recipe.Metadata.Name, recipeID)
		loadedCount++
	}

	log.Printf("[SERVER] Successfully loaded %d/%d recipe files\n", loadedCount, len(recipes))
	return nil
}
