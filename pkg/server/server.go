package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	types "mywant/engine/cmd/types"
	mywant "mywant/engine/src"

	"github.com/gorilla/mux"
)

var GlobalDebugEnabled bool

// Server represents the MyWant server
type Server struct {
	config               Config
	wants                map[string]*WantExecution        // Store active want executions
	globalBuilder        *mywant.ChainBuilder             // Global builder with running reconcile loop for server mode
	agentRegistry        *mywant.AgentRegistry            // Agent and capability registry
	recipeRegistry       *mywant.CustomTargetTypeRegistry // Recipe registry
	wantTypeLoader       *mywant.WantTypeLoader           // Want type definitions loader
	errorHistory         []ErrorHistoryEntry              // Store error history
	router               *mux.Router
	reactionQueueManager *types.ReactionQueueManager // Reaction queue manager for reminder wants
	interactionManager   *mywant.InteractionManager  // Interactive want creation manager
}

// WantExecutionTyped overrides the one in types.go to use proper mywant types if possible
// Go doesn't support struct overriding in same package easily, so we update the one in types.go or use this one internally.
// Let's reuse the one in types.go but cast Builder/Config when needed, OR import types.go into server.go properly.
// Since they are in same package 'server', they share types.
// We should update pkg/server/types.go to import mywant/engine/src and use proper types.

// New creates a new MyWant server
func New(config Config) *Server {
	agentRegistry := mywant.NewAgentRegistry()

	// Load capabilities and agents from directories if they exist
	if err := agentRegistry.LoadCapabilities(mywant.CapabilitiesDir + "/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load capabilities: %v\n", err)
	}

	// Register mock server capability (ensure it exists before agent registration)
	agentRegistry.RegisterCapability(mywant.Capability{
		Name:  "mock_server_management",
		Gives: []string{"mock_server_management"},
	})

	if err := agentRegistry.LoadAgents(mywant.AgentsDir + "/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load agents: %v\n", err)
	}
	recipeRegistry := mywant.NewCustomTargetTypeRegistry()

	// Load recipes from recipes/ directory as custom types
	_ = mywant.ScanAndRegisterCustomTypes(mywant.RecipesDir, recipeRegistry)

	// Also load the recipe files themselves into the recipe registry
	_ = loadRecipeFilesIntoRegistry(mywant.RecipesDir, recipeRegistry)

	// Load want type definitions
	wantTypeLoader := mywant.NewWantTypeLoader(mywant.WantTypesDir)
	if err := wantTypeLoader.LoadAllWantTypes(); err != nil {
		log.Printf("[WARN] Failed to load want types: %v", err)
	}
	globalBuilder := mywant.NewChainBuilderWithPaths("", "engine/memory/memory-0000-latest.yaml")
	globalBuilder.SetConfigInternal(mywant.Config{Wants: []*mywant.Want{}})
	globalBuilder.SetServerMode(true)
	globalBuilder.SetAgentRegistry(agentRegistry)
	globalBuilder.SetCustomTargetRegistry(recipeRegistry) // Set custom types from recipes

	// Register the global ChainBuilder so wants can access it for the retrigger mechanism
	mywant.SetGlobalChainBuilder(globalBuilder)

	// Register Gmail dynamic want type factory
	types.RegisterGmailDynamicWant(globalBuilder)

	tempServer := &Server{}

	// Register dynamic agent implementations on global registry This provides the actual Action/Monitor functions for YAML-loaded agents
	tempServer.registerDynamicAgents(agentRegistry)
	types.RegisterExecutionAgents(agentRegistry)
	types.RegisterMCPAgent(agentRegistry)
	types.RegisterDynamicMCPAgents(agentRegistry) // Register Discovery, Developer, Compiler, Validator agents

	// Create reaction queue manager for reminders (multi-queue system)
	reactionQueueManager := types.NewReactionQueueManager()

	// Register reminder queue management agent (DoAgent for queue lifecycle)
	if err := types.RegisterReminderQueueAgent(agentRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to register reminder queue agent: %v\n", err)
	}

	// Register mock server management agent (DoAgent for server lifecycle)
	if err := types.RegisterMockServerAgent(agentRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to register mock server agent: %v\n", err)
	}

	// Register user reaction monitor agent (MonitorAgent using HTTP API)
	if err := types.RegisterUserReactionMonitorAgent(agentRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to register user reaction monitor agent: %v\n", err)
	}

	// Register user reaction do agent
	if err := types.RegisterUserReactionDoAgent(agentRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to register user reaction do agent: %v\n", err)
	}

	// Register knowledge management agent
	types.RegisterKnowledgeAgents(agentRegistry)

	// Register MCP server process management agent
	types.RegisterMCPServerProcessAgent(agentRegistry)

	// Register Dynamic MCP Agents
	types.RegisterDynamicMCPAgents(agentRegistry)

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

	// Register all want types on global builder before starting reconcile loop
	types.RegisterQNetWantTypes(s.globalBuilder)
	types.RegisterFibonacciWantTypes(s.globalBuilder)
	types.RegisterPrimeWantTypes(s.globalBuilder)
	types.RegisterTravelWantTypesWithAgents(s.globalBuilder, s.agentRegistry)
	types.RegisterApprovalWantTypes(s.globalBuilder)
	types.RegisterExecutionResultWantType(s.globalBuilder)
	types.RegisterReminderWantType(s.globalBuilder)
	types.RegisterSilencerWantType(s.globalBuilder)
	types.RegisterGmailWantType(s.globalBuilder)
	types.RegisterGmailDynamicWantType(s.globalBuilder)
	types.RegisterKnowledgeWantType(s.globalBuilder)
	types.RegisterFlightMockServerWantType(s.globalBuilder)
	types.RegisterDraftWantType(s.globalBuilder)
	mywant.RegisterMonitorWantTypes(s.globalBuilder)
	mywant.RegisterOwnerWantTypes(s.globalBuilder)
	mywant.RegisterSchedulerWantTypes(s.globalBuilder)

	// Transfer loaded want type definitions to global builder for state initialization
	if s.wantTypeLoader != nil {
		allDefs := s.wantTypeLoader.GetAll()
		for _, def := range allDefs {
			s.globalBuilder.StoreWantTypeDefinition(def)
		}
	}

	// Start global builder's reconcile loop for server mode (runs indefinitely)
	go s.globalBuilder.ExecuteWithMode(true)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	log.Printf("🚀 MyWant server starting on %s\n", addr)
	// (Omitted detailed endpoint logging for brevity in this file, or we can add it back)

	return http.ListenAndServe(addr, s.router)
}

// registerDynamicAgents registers implementations for special agents loaded from YAML
func (s *Server) registerDynamicAgents(agentRegistry *mywant.AgentRegistry) {
	setupFlightAPIAgents(agentRegistry)
}

func setupFlightAPIAgents(agentRegistry *mywant.AgentRegistry) {
	if agent, exists := agentRegistry.GetAgent("agent_flight_api"); exists {
		if doAgent, ok := agent.(*mywant.DoAgent); ok {
			flightAgent := types.NewAgentFlightAPI(
				"agent_flight_api",
				[]string{"flight_api_agency"},
				[]string{},
				"http://localhost:8081",
			)
			doAgent.Action = func(ctx context.Context, want *mywant.Want) error {
				_, err := flightAgent.Exec(ctx, want)
				return err
			}
		}
	}
}

// corsMiddleware adds CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
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
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
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
