# CLAUDE.md - Quick Reference

## Project Overview

MyWant is a **declarative chain programming system** where you express "what you want" through YAML configuration. Autonomous **agents** (Do/Monitor types) collaborate to solve your Wants based on their Capabilities.

**Key Features:**
- 📝 YAML-driven workflows with recipe-based configuration
- 🤖 Autonomous agent ecosystem (Do/Monitor agents)
- 💻 Full-stack CLI (`mywant`) for complete lifecycle management
- 📊 Interactive dashboard with real-time monitoring
- 💾 Persistent memory with continuous state reconciliation

## Documentation

**Official Docs:**
- [Want System](docs/want-system.md) - Core Want/Recipe concepts
- [Agent System](docs/agent-system.md) - Agent architecture
- [Agent Execution Modes](docs/AgentExecutionModes.md) - Local/Webhook/RPC execution
- [Want Developer Guide](docs/WantDeveloperGuide.md) - Custom Want type development (state management, exec cycle, achieving_percentage)
- [Agent Catalog](AGENTS.md) - Available agents and capabilities
- [CLI Usage Guide](docs/WANT_CLI_USAGE.md) - Complete mywant reference
- [Examples](docs/agent-examples.md) - Agent usage examples
- [Keyboard Shortcuts & MCP Testing](web/SHORTCUTS_AND_MCP_TESTING.md) - Frontend shortcuts

## Core Architecture

**Main Components:**
- `declarative.go` - Core Config/Want/Recipe/Agent types
- `recipe_loader_generic.go` - Generic recipe loading
- `*_types.go` - Want type implementations (in `engine/cmd/types/`)
- `engine/src/chain_builder.go` - Chain execution engine
- `yaml/agents/` - Agent YAML definitions

**Key Types:**
- `Want` - Processing unit with Metadata, Spec, State, Status
- `WantSpec` - Config with Params, Using selectors, Recipe reference
- `ChainBuilder` - Builds and executes chains
- `Agent` - Autonomous executor with Capabilities

**Want Categories:**
- **Independent** - Execute in parallel (travel planning: restaurant, hotel, buffet)
- **Dependent** - Pipeline via `using` selectors (queue: generator→queue→sink)
- **Coordinator** - Orchestrates other wants via input channels

**Agent Types:**
- **DoAgent** - Execute specific tasks once (e.g., book flight)
- **MonitorAgent** - Continuously poll external state (e.g., flight status monitor)

**Agent Execution Modes:**
- **Local** - In-process goroutines (default, fastest)
- **Webhook** - External HTTP services (language-agnostic, scalable)
- **RPC** - gRPC/JSON-RPC services (high-performance, planned)

## Essential Commands

**Configuration Management:**
```sh
# Interactive configuration setup
./mywant config set

# View current configuration
./mywant config get

# Reset to defaults
./mywant config reset

# Use custom config file
./mywant --config /path/to/config.yaml config get
./mywant --config ./dev-config.yaml start -D

# Configuration file location: ~/.mywant/config.yaml (default)
```

**CLI Quick Start:**
```sh
# Build CLI with embedded GUI
make release

# Start backend API and embedded GUI (uses config file)
./mywant start -D

# Start with specific port (overrides config)
./mywant start -D --port 8080

# Start Agent Service (worker mode)
./mywant start --worker -D

# Or run in development mode with hot reload
./mywant start --dev

# Deploy a want from file
./mywant wants create -f yaml/config/config-travel.yaml

# List all wants
./mywant wants list

# Check process status
./mywant ps

# Stop server
./mywant stop
```

**Want-CLI Management:**
```sh
# Want lifecycle
./mywant wants get <WANT_ID>           # Get detailed status
./mywant wants delete <WANT_ID>        # Delete want
./mywant wants suspend/resume <ID>     # Lifecycle control

# Recipe operations
./mywant recipes list                  # List available recipes
./mywant recipes create -f recipe.yaml # Create new recipe
./mywant recipes from-want <ID> --name "my-recipe"  # Generate from existing want

# System inspection
./mywant types list                    # List want types
./mywant agents list                   # List registered agents
./mywant capabilities list             # List capabilities
./mywant logs                          # View operation logs
```

**Legacy Make Commands:**
```sh
make restart-all             # Start servers: MyWant (8080) + Mock Flight (8090)
make test-concurrent-deploy  # Test concurrent deployments with race detection
make run-travel-recipe       # Independent wants (restaurant, hotel, buffet)
make run-queue-system-recipe # Pipeline (generator→queue→sink)
make run-qnet-recipe         # Complex multi-stream system
```

**Important**: Never use `make build-server` directly; `make restart-all` includes building.

## Key Patterns

**Recipe Structure:**
- **Independent wants** (no `using`) - Execute in parallel, need Coordinator
- **Dependent wants** (with `using`) - Form pipelines via label matching
- **Parameters** - Referenced by name, no Go templating

**State Management:**
- Use `StoreState(key, value)` to update state (batched during execution)
- Use `GetState(key)` to retrieve values
- State auto-persists across executions via memory reconciliation

**Dynamic Wants:**
```go
builder.AddDynamicNode(Want{...})  // Add single want
builder.AddDynamicNodes([]Want{})  // Add multiple
// Auto-connected via label selectors
```

## File Organization

- `yaml/config/` - Config YAML files (user interface)
- `yaml/recipes/` - Recipe templates (reusable components)
- `yaml/agents/` - Agent YAML definitions (Do/Monitor agents)
- `engine/src/` - Core system: declarative.go, chain_builder.go
- `engine/cmd/types/` - Want implementations: *_types.go files
- `engine/cmd/server/` - HTTP server and API endpoints
- `docs/` - Documentation (want-system.md, agent-system.md, etc.)
- `web/` - React frontend (embedded in binary)

## Coding Rules

1. **Sleep timings**: Max 7 seconds in build/test
2. **Never** call `make build-server` directly - use `make restart-all`
3. Use `StoreState()` for all state updates (enables proper batching)
4. Use `GetState()` for all state reads (returns value, exists)
5. Initialize `StateHistory` before appending:
   ```go
   if want.History.StateHistory == nil {
       want.History.StateHistory = make([]StateHistoryEntry, 0)
   }
   ```

## Want Execution Lifecycle

1. **BeginProgressCycle()** - Start batching state changes
2. **Exec()** - Main execution (channel I/O)
3. **EndProgressCycle()** - Commit state changes and history

**Connectivity Requirements (`require` field):**
- `none` - No connections needed (default)
- `users` - Needs output connections (generators, independent wants)
- `providers` - Needs input connections (sinks, collectors)
- `providers_and_users` - Needs both (pipelines: queue, processors)

## Agent System

**Agent Types:**
- **DoAgent** - Executes specific tasks once and writes results to State
  - Examples: `agent_flight_api` (flight booking), `execution_command` (shell commands)
- **MonitorAgent** - Continuously polls external state and updates Want
  - Examples: `monitor_flight_api` (flight status), `user_reaction_monitor` (approval monitoring)

**Capabilities:**
Agents declare what they can do via capabilities:
- `flight_api_agency` - Flight API integration
- `hotel_agency` - Hotel booking services
- `command_execution` - Shell command execution
- `mcp_gmail` - Gmail operations via MCP
- `reminder_monitoring` - User reaction tracking

**Agent Definition (YAML):**
```yaml
# yaml/agents/agent-example.yaml
name: "agent_premium"
type: "do"
capabilities:
  - "hotel_agency"
  - "premium_services"
description: "Handles luxury hotel bookings with automated upgrades."
```

**Common Agent Patterns:**
1. **Monitor & Retrigger**: MonitorAgent detects state change (e.g., flight delay) → triggers Coordinator to replan
2. **Silencer Pattern**: Auto-approval agent responds while user input is pending

**Agent Management:**
```sh
./mywant agents list           # Show registered agents
./mywant capabilities list     # Show available capabilities
./mywant wants get <ID> --history  # View agent execution history
```

## System Directories

**Configuration and Logs:**
- `~/.mywant/` - All configuration, PID files, logs, and runtime data
  - `config.yaml` - CLI configuration (agent mode, hosts, ports)
  - `server.pid` - Backend server process ID
  - `server.log` - Backend server logs
  - `agent-service.pid` - Agent service process ID
  - `agent-service.port` - Agent service port number
  - `agent-service.log` - Agent service logs
  - `flight-server.log` - Mock flight server logs

**Configuration Options (`~/.mywant/config.yaml`):**
```yaml
agent_mode: local              # local, webhook, or grpc
server_host: localhost         # Main server host
server_port: 8080              # Main server port
agent_service_host: localhost  # Agent service host (webhook/grpc)
agent_service_port: 8081       # Agent service port (webhook/grpc)
mock_flight_port: 8090         # Mock flight server port
```

## Pending Improvements

- Frontend recipe cards need deploy button for direct recipe launch
- Replace direct `State` field access with `GetState()` everywhere
- Consider async rebooking in dynamic travel changes

## RAG Database (Code Search)

**Quick Usage:**
```bash
# Interactive search
python3 tools/codebase_rag.py

# Python API
from tools.codebase_rag import CodebaseRAG
rag = CodebaseRAG('codebase_rag.db')
results = rag.search("ChainBuilder", entity_types=['struct'])
rag.close()
```

**Auto-sync**: RAG database auto-updates via post-commit hook. Just commit normally.

**Resources**: See `tools/README_RAG.md` and `QUICKSTART_RAG.md` for details.

## Completed Tasks

### 2026-01-10: Documentation Update
✅ **Updated CLAUDE.md with latest system information**
- Added Agent System documentation (DoAgent/MonitorAgent, Capabilities)
- Integrated mywant commands and usage patterns
- Updated project overview to reflect declarative agent-based architecture
- Added comprehensive CLI quick start and management commands
- Documented agent patterns (Monitor & Retrigger, Silencer)
- Updated file organization with agents/, docs/, web/ directories

### 2025-12-13: UI and Coordinator Improvements
✅ **Fixed LabelSelectorAutocomplete keyboard navigation** (Commit 54e8484)
- Added arrow key (Up/Down) navigation through dropdown options
- Tab key now confirms selection and moves to next field
- Visual highlight and auto-focus management

✅ **Coordinator System Refactoring**
- Unified all recipes to use `type: coordinator`
- Removed legacy backward-compatibility code (Commit 3db770d)
- Simplified `getCoordinatorConfig()` logic

## Pending Tasks
None - all major refactoring complete!