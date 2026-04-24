# CLAUDE.md - Quick Reference

## Project Overview

**Declarative chain programming system** - express "what you want" via YAML. Autonomous agents (Do/Monitor) collaborate to solve Wants based on Capabilities.

**Features:** YAML workflows • Agent ecosystem • CLI lifecycle mgmt • Real-time dashboard • Persistent state

## Documentation

[Want System](docs/want-system.md) • [Agent System](docs/agent-system.md) • [Execution Modes](docs/AgentExecutionModes.md) • [Developer Guide](docs/WantDeveloperGuide.md) • [Want Card Plugin System](docs/WantCardPluginSystem.md) • [Agent Catalog](AGENTS.md) • [CLI Usage](docs/MYWANT_CLI_USAGE.md) • [Examples](docs/agent-examples.md) • [Webhook (Teams/Slack)](docs/TEAMS_WEBHOOK_SPEC.md) • [Shortcuts](web/SHORTCUTS_AND_MCP_TESTING.md)

## Core Architecture

**Components:** `declarative.go` (types) • `recipe_loader_generic.go` • `*_types.go` (engine/types/) • `chain_builder.go` • `yaml/agents/`

**Types:** `Want` (Metadata/Spec/State/Status) • `WantSpec` (Params/Using/Recipe) • `ChainBuilder` • `Agent` (Capabilities)

**Want Categories:** Independent (parallel) • Dependent (pipeline via `using`) • Coordinator (orchestrates)

**Agent Types:** DoAgent (one-time tasks) • MonitorAgent (continuous polling)

**Execution Modes:** Local (goroutines, default) • Webhook (HTTP services) • RPC (gRPC/JSON-RPC, planned)

## Essential Commands

**Configuration:**
```sh
./bin/mywant config set/get/reset
./bin/mywant --config /path/to/config.yaml config get  # Custom config (~/.mywant/config.yaml default)
```

**Quick Start:**
```sh
make restart-all                                    # ⚠️ ALWAYS use this to rebuild & restart (builds CLI + frontend + starts server)
make release                                        # Build CLI only (no restart)
./bin/mywant start -D [--port 8080]                     # Start server (after manual build)
./bin/mywant start --worker -D                          # Agent service
./bin/mywant start --dev                                # Dev mode
./bin/mywant wants create -f yaml/config/config-travel.yaml
./bin/mywant wants list
./bin/mywant ps
./bin/mywant stop
```

**Management:**
```sh
# Wants
./bin/mywant wants get/delete/suspend/resume <ID>

# Recipes
./bin/mywant recipes list
./bin/mywant recipes create -f recipe.yaml          # from file
./bin/mywant recipes create --from-want <ID> --name "my-recipe"  # from want
./bin/mywant recipes create -i                      # interactive

# System
./bin/mywant types/agents/capabilities list
./bin/mywant logs
```

**Make Commands:**
```sh
make restart-all             # Start MyWant (8080) + Mock Flight (8090) [includes build]
make test-concurrent-deploy  # Concurrent deployment test
make run-travel-recipe       # Independent wants demo
make run-queue-system-recipe # Pipeline demo
make run-qnet-recipe         # Multi-stream demo
```

**Note:** Always use `make restart-all` for rebuilding and restarting — it builds `./bin/mywant` (engine/server), the frontend, and starts both the main server and mock flight server. Never use `make release` + manual start as a substitute when doing a full rebuild.

## Key Patterns

**Recipe:** Independent (no `using`, parallel) • Dependent (`using` selectors, pipeline) • Params by name only

**State:** `StoreState(key, value)` (batched) • `GetState(key)` (retrieves) • Auto-persists via reconciliation

**Dynamic Wants:**
```go
builder.AddDynamicNode(Want{...}) / AddDynamicNodes([]Want{})  // Auto-connects via selectors
```

## File Organization

`yaml/config/` (configs) • `yaml/recipes/` (templates) • `yaml/agents/` (agent definitions) • `engine/core/` (core library) • `engine/types/` (*_types.go) • `engine/server/` (HTTP API) • `engine/worker/` (agent service) • `engine/demos/` (demo programs) • `client/` (CLI client + HTTP client library) • `client/cmd/mywant/` (CLI entrypoint) • `docs/` • `web/` (React frontend + embedded assets)

## Coding Rules

1. Max 7s sleep in build/test
2. **Server modifications**: Edit `engine/server/handlers_*.go`
3. Always `StoreState(k,v)` / `GetState(k)` for state access
4. Initialize StateHistory: `if want.History.StateHistory == nil { want.History.StateHistory = make([]StateHistoryEntry, 0) }`
5. **NEVER call `AddChildWant` from within a Want's `Progress()` or `Initialize()`**. Child Want creation is exclusively the responsibility of the Target (recipe deployment / ChainBuilder). Wants must not self-spawn children.

## Want Execution Lifecycle

`BeginProgressCycle()` → `Exec()` (channel I/O) → `EndProgressCycle()` (commit state/history)

**Connectivity (`require`):** `none` (default) • `users` (output) • `providers` (input) • `providers_and_users` (both)

## Agent System

**Types:** DoAgent (one-time task → State) • MonitorAgent (continuous polling → Want updates)

**Capabilities:** `flight_api_agency` • `hotel_agency` • `command_execution` • `mcp_gmail` • `reminder_monitoring`

**YAML Definition:**
```yaml
name: "agent_premium"
type: "do"
capabilities: ["hotel_agency", "premium_services"]
description: "Luxury hotel bookings with automated upgrades."
```

**Patterns:** Monitor & Retrigger (detect change → replan) • Silencer (auto-approval during user wait)

**Commands:** `./bin/mywant agents list` • `./bin/mywant capabilities list` • `./bin/mywant wants get <ID> --history`

## System Directories

**`~/.mywant/`** - config.yaml • server.pid/log • agent-service.pid/port/log • flight-server.log

**Config (`~/.mywant/config.yaml`):**
```yaml
agent_mode: local              # local/webhook/grpc
server_host: localhost
server_port: 8080
agent_service_host: localhost
agent_service_port: 8081
mock_flight_port: 8090
```

## Server Implementation

**Server Flow:**
```
./bin/mywant start -D → client/cmd/mywant/commands/start.go → server.New() → engine/server/server.go
```

**When modifying server**:
- ✅ Edit `engine/server/handlers_*.go` (want, agent, recipe handlers)
- ✅ Edit `engine/server/server.go` (server setup)

**Build & Run:**
- `make restart-all` - Builds `./bin/mywant` (includes `engine/server`) and starts it
- `./bin/mywant start -D` - Starts server using `engine/server` implementation

## Pending Improvements

Frontend recipe deploy button • Replace `State` access with `GetState()` • Async rebooking for travel changes

## RAG Database

```bash
python3 tools/codebase_rag.py  # Interactive search

# API: CodebaseRAG('codebase_rag.db').search("ChainBuilder", entity_types=['struct'])
```

Auto-syncs via post-commit hook. See `tools/README_RAG.md` and `QUICKSTART_RAG.md`.

## Completed Tasks

**2026-01-10:** CLAUDE.md update - Agent system docs, CLI commands, patterns (Monitor & Retrigger, Silencer)

**2025-12-13:** LabelSelectorAutocomplete keyboard nav (54e8484) • Coordinator refactoring - unified `type: coordinator` (3db770d)

**Pending:** None