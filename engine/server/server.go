package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	mywant "mywant/engine/core"
	types "mywant/engine/types"
	_ "mywant/engine/types"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

var GlobalDebugEnabled bool

// Server represents the MyWant server
type Server struct {
	config               Config
	wants                map[string]*WantExecution        // Store active want executions
	wantsMu              sync.RWMutex                     // Protects wants map
	globalBuilder        *mywant.ChainBuilder             // Global builder with running reconcile loop for server mode
	agentRegistry        *mywant.AgentRegistry            // Agent and capability registry
	recipeRegistry       *mywant.CustomTargetTypeRegistry // Recipe registry
	wantTypeLoader       *mywant.WantTypeLoader           // Want type definitions loader
	errorHistory         []ErrorHistoryEntry              // Store error history
	errorMu              sync.Mutex                       // Protects errorHistory slice
	router               *mux.Router
	reactionQueueManager *types.ReactionQueueManager      // Reaction queue manager for reminder wants
	interactionManager   *mywant.InteractionManager       // Interactive want creation manager
	httpServer           *http.Server                     // HTTP server instance
	otelShutdown         func(context.Context) error      // OpenTelemetry shutdown hook
}

// WantExecutionTyped overrides the one in types.go to use proper mywant types if possible
// Go doesn't support struct overriding in same package easily, so we update the one in types.go or use this one internally.
// Let's reuse the one in types.go but cast Builder/Config when needed, OR import types.go into server.go properly.
// Since they are in same package 'server', they share types.
// We should update pkg/server/types.go to import mywant/engine/core and use proper types.

// New creates a new MyWant server
func New(config Config) *Server {
	// Initialise OpenTelemetry (best-effort; non-fatal on failure)
	otelShutdown, err := mywant.InitOTEL(context.Background(), config.OTELEndpoint)
	if err != nil {
		log.Printf("[SERVER] Warning: OTEL init failed: %v\n", err)
		otelShutdown = func(context.Context) error { return nil }
	}
	_ = otelShutdown // stored below after Server is constructed

	agentRegistry := mywant.NewAgentRegistry()

	// Load capabilities and agents from directories if they exist
	if err := agentRegistry.LoadCapabilities(mywant.CapabilitiesDir + "/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load capabilities: %v\n", err)
	}

	if err := agentRegistry.LoadAgents(mywant.AgentsDir + "/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load agents: %v\n", err)
	}

	// Load MRS plugin agents from ~/.mywant/custom-types/ (best-effort)
	if err := agentRegistry.LoadUserCustomAgents(mywant.UserCustomTypesDir()); err != nil {
		log.Printf("[SERVER] Warning: Failed to load user custom agents: %v\n", err)
	}

	recipeRegistry := mywant.NewCustomTargetTypeRegistry()

	// Load recipes from recipes/ directory as custom types
	_ = mywant.ScanAndRegisterCustomTypes(mywant.RecipesDir, recipeRegistry)

	// Also load the recipe files themselves into the recipe registry
	_ = loadRecipeFilesIntoRegistry(mywant.RecipesDir, recipeRegistry)

	// Load user-saved recipes from ~/.mywant/recipes/
	userRecipesDir := mywant.UserRecipesDir()
	_ = mywant.ScanAndRegisterCustomTypes(userRecipesDir, recipeRegistry)
	_ = loadRecipeFilesIntoRegistry(userRecipesDir, recipeRegistry)

	// Load want type definitions
	wantTypesDir := mywant.WantTypesDir
	if config.WantTypesDir != "" {
		wantTypesDir = config.WantTypesDir
	}
	wantTypeLoader := mywant.NewWantTypeLoader(wantTypesDir)
	if err := wantTypeLoader.LoadAllWantTypes(); err != nil {
		log.Printf("[WARN] Failed to load want types: %v", err)
	}

	// Load data type definitions (JSON Schema)
	dataTypeLoader := mywant.NewDataTypeLoader()
	if err := dataTypeLoader.LoadFromDir(mywant.DataTypesDir); err != nil {
		log.Printf("[WARN] Failed to load data types: %v", err)
	}
	mywant.SetGlobalDataTypeLoader(dataTypeLoader)

	globalBuilder := mywant.NewChainBuilderWithPaths(config.ConfigPath, config.MemoryPath)
	mywant.SetGlobalServerConfig(config)

	// Load initial configuration from memory file if it exists (persistence)
	initialConfig := mywant.Config{Wants: []*mywant.Want{}}
	if config.MemoryPath != "" {
		if _, err := os.Stat(config.MemoryPath); err == nil {
			if loadedConfig, err := mywant.LoadConfigFromYAML(config.MemoryPath); err == nil {
				initialConfig = loadedConfig
				log.Printf("[SERVER] Restored %d wants from %s\n", len(initialConfig.Wants), config.MemoryPath)
			}
		}
	}

	// Remove any stale gui_state wants (they should not persist across restarts)
	// and inject a fresh one so it is always available from the start.
	filtered := initialConfig.Wants[:0]
	for _, w := range initialConfig.Wants {
		if w.Metadata.Type != "gui_state" {
			filtered = append(filtered, w)
		}
	}
	initialConfig.Wants = append(filtered, &mywant.Want{
		Metadata: mywant.Metadata{
			ID:           "system-gui-state",
			Name:         "system-gui-state",
			Type:         "gui_state",
			IsSystemWant: true,
		},
	})

	globalBuilder.SetConfigInternal(initialConfig)
	globalBuilder.SetServerMode(true)
	globalBuilder.SetAgentRegistry(agentRegistry)
	globalBuilder.SetCustomTargetRegistry(recipeRegistry) // Set custom types from recipes

	// Register the global ChainBuilder so wants can access it for the retrigger mechanism
	mywant.SetGlobalChainBuilder(globalBuilder)
	mywant.SetGlobalGoalThinkerUseStub(config.GoalThinker.UseStub)

	// Register all agent implementations (auto-registered via init() functions)
	mywant.RegisterAllKnownAgentImplementations(agentRegistry)

	// Derive MonitorCapabilities cache for all want type definitions by
	// cross-referencing their requires list against the now-populated agent registry
	if wantTypeLoader != nil {
		wantTypeLoader.EnrichMonitorCapabilities(agentRegistry)
	}

	// Register core system want types
	mywant.RegisterMonitorWantTypes(globalBuilder)
	mywant.RegisterOwnerWantTypes(globalBuilder)
	mywant.RegisterSchedulerWantTypes(globalBuilder)

	// Transfer loaded want type definitions to global builder for state initialization.
	// This must happen in New() (not just Start()) so tests that call setupRoutes()
	// directly also have StateLabels populated before ExecuteWithMode runs.
	if wantTypeLoader != nil {
		allDefs := wantTypeLoader.GetAll()
		for _, def := range allDefs {
			globalBuilder.StoreWantTypeDefinition(def)
		}
	}

	// Record want type load warnings into the API log so they are visible via
	// GET /api/v1/logs and `./bin/mywant logs`.
	if wantTypeLoader != nil {
		for _, w := range wantTypeLoader.GetLoadWarnings() {
			globalBuilder.LogAPIOperation("STARTUP", "/want-types/load", "", "warning", 0, w, "")
		}
	}

	// Create reaction queue manager for reminders (multi-queue system)
	reactionQueueManager := types.NewReactionQueueManager()

	// Initialize internal HTTP client for agents
	baseURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
	globalBuilder.SetHTTPClient(mywant.NewHTTPClient(baseURL))

	// Create interaction manager for interactive want creation
	gooseManager, err := types.GetGooseManager(context.Background())
	if err != nil {
		log.Printf("[WARN] Failed to initialize GooseManager for InteractionManager: %v", err)
		log.Printf("[WARN] Interactive want creation will not be available")
	}
	interactionManager := mywant.NewInteractionManager(wantTypeLoader, recipeRegistry, gooseManager)

	GlobalDebugEnabled = config.Debug
	mywant.DebugLoggingEnabled = config.Debug

	return &Server{
		config:               config,
		wants:                make(map[string]*WantExecution),
		globalBuilder:        globalBuilder,
		agentRegistry:        agentRegistry,
		recipeRegistry:       recipeRegistry,
		wantTypeLoader:       wantTypeLoader,
		errorHistory:         make([]ErrorHistoryEntry, 0),
		router:               mux.NewRouter(),
		reactionQueueManager: reactionQueueManager,
		interactionManager:   interactionManager,
		otelShutdown:         otelShutdown,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.setupRoutes()

	// Start pprof profiling server only in debug mode
	if s.config.Debug {
		go func() {
			pprofAddr := "localhost:6060"
			log.Printf("📊 pprof profiling server starting on http://%s/debug/pprof/\n", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				log.Printf("⚠️  pprof server error: %v\n", err)
			}
		}()
	}

	// Start global builder's reconcile loop for server mode (runs indefinitely)
	go s.globalBuilder.ExecuteWithMode(true)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	// Handle signals for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-stop
		log.Printf("Received signal: %v. Shutting down server gracefully...\n", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.Shutdown(ctx); err != nil {
			log.Printf("Error during shutdown: %v\n", err)
		}
	}()

	log.Printf("🚀 MyWant server starting on %s\n", addr)

	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}

	log.Println("Server stopped")
	return nil
}

// Shutdown performs a graceful shutdown of the server and its components
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down components...")

	// 1. Shutdown ChainBuilder (this triggers OnDelete for all wants)
	if s.globalBuilder != nil {
		s.globalBuilder.Shutdown()
	}

	// 2. Flush and shut down OpenTelemetry exporters
	if s.otelShutdown != nil {
		if err := s.otelShutdown(ctx); err != nil {
			log.Printf("[SERVER] Warning: OTEL shutdown error: %v\n", err)
		}
	}

	// 3. Shutdown HTTP server
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}

	return nil
}

// corsMiddleware adds CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, If-None-Match")
		w.Header().Set("Access-Control-Expose-Headers", "ETag")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, If-None-Match")
	w.Header().Set("Access-Control-Expose-Headers", "ETag")
	w.WriteHeader(http.StatusOK)
}

// InfoLog prints logs if debug is enabled
func InfoLog(format string, v ...any) {
	if GlobalDebugEnabled {
		log.Printf(format, v...)
	}
}

// ErrorLog always prints logs
func ErrorLog(format string, v ...any) {
	log.Printf(format, v...)
}

// saveFrontendConfig saves the UI settings to the CLI config file
func (s *Server) saveFrontendConfig() {
	if s.config.ConfigPath == "" {
		return
	}

	// 1. Read existing config to preserve other settings
	data, err := os.ReadFile(s.config.ConfigPath)
	var fullConfig map[string]any
	if err == nil {
		_ = yaml.Unmarshal(data, &fullConfig)
	}
	if fullConfig == nil {
		fullConfig = make(map[string]any)
	}

	// 2. Update only the frontend sections
	fullConfig["header_position"] = s.config.HeaderPosition
	fullConfig["color_mode"] = s.config.ColorMode

	// 3. Save back to file
	newData, err := yaml.Marshal(fullConfig)
	if err != nil {
		log.Printf("[SERVER] Error marshaling config for saving: %v\n", err)
		return
	}

	err = os.WriteFile(s.config.ConfigPath, newData, 0644)
	if err != nil {
		log.Printf("[SERVER] Error saving config to %s: %v\n", s.config.ConfigPath, err)
	} else {
		log.Printf("[SERVER] Saved frontend settings to %s\n", s.config.ConfigPath)
	}
}
