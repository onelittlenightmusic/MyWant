# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MyWant is a Go library implementing functional chain programming patterns with channels. The project uses a **recipe-based configuration system** where config YAML files serve as the top-level user interface and recipes provide reusable component templates.

## Core Architecture

### Configuration System (User Interface)

- **Config Files**: Top-level user interface in `config/` directory (`config-*-recipe.yaml`)
- **Recipe Files**: Reusable components in `recipes/` directory
- **Demo Programs**: Entry points that load config files (`cmd/demos/demo_*_recipe.go`)

### Main Components

- **declarative.go**: Core configuration system with Config/Want/Recipe types
- **recipe_loader_generic.go**: Generic recipe loading and processing
- **recipe_loader.go**: Owner-based recipe loading for dynamic want creation
- **\*_types.go**: Want type implementations (qnet_types.go, travel_types.go, etc.)

### Key Types and Concepts

#### Configuration API
- `Config`: Complete execution configuration with array of `Want`s
- `Want`: Individual processing unit with `Metadata`, `Spec`, `Stats`, `Status`
- `WantSpec`: Want configuration with `Params`, `Using` selectors, and optional `Recipe` reference
- `ChainBuilder`: Builds and executes chains from configuration

#### Recipe System
- `GenericRecipe`: Recipe template with `Parameters`, `Wants`, and optional `Coordinator`
- `RecipeWant`: Flattened recipe format (type, labels, params, using at top level)
- `GenericRecipeLoader`: Loads and processes recipe files
- Parameter substitution: Recipe parameters referenced by name (e.g., `count: count`)

#### Want Types
- **Independent Wants**: Execute in parallel without dependencies (travel planning)
- **Dependent Wants**: Connected in pipelines using `using` selectors (queue systems)
- **Coordinator Wants**: Orchestrate independent wants (travel_coordinator)

## Development Commands

### Module Initialization
```sh
go mod init MyWant
go mod tidy
```

### Running Examples

#### Recipe-Based Examples (Recommended)
```sh
# Independent wants (travel planning)
make run-travel-recipe        # Uses config/config-travel-recipe.yaml → recipes/travel-itinerary.yaml

# Dependent wants (queue system pipeline)  
make run-queue-system-recipe  # Uses config/config-queue-system-recipe.yaml → recipes/queue-system.yaml

# Complex multi-stream systems
make run-qnet-recipe         # Uses config/config-qnet-recipe.yaml → recipes/qnet-pipeline.yaml
make run-qnet-using-recipe   # QNet with YAML-defined using connections

# Owner-based dynamic want creation
make run-sample-owner        # Dynamic wants using recipes from recipes/ directory
```

#### Direct Configuration Examples
```sh
# Direct want definitions (no recipes)
make run-qnet          # Uses config/config-qnet.yaml
make run-prime         # Uses config/config-prime.yaml
make run-fibonacci     # Uses config/config-fibonacci.yaml
make run-fibonacci-loop # Uses config/config-fibonacci-loop.yaml
make run-travel        # Uses config/config-travel.yaml
```

### Server Startup and Testing

#### Building the Server Binary
```sh
# Build MyWant server
make build-server      # Creates ./bin/mywant binary

# Build Mock Flight Server
make build-mock        # Creates ./bin/flight-server binary
```

#### Starting/Restarting Servers
```sh
# Start both MyWant server (8080) and Mock Flight Server (8081)
make restart-all       # Kills existing servers and starts fresh
                       # MyWant server listens on localhost:8080
                       # Flight server listens on localhost:8081

# Individual server start
make run-server        # Start MyWant server on port 8080
make run-mock          # Start Mock Flight Server on port 8081
```

#### Concurrent Stress Testing

```sh
# Main concurrent deployment test (recommended)
make test-concurrent-deploy  # Tests concurrent want deployments with race condition detection
                             # Deploys Travel Planner + Fibonacci Recipe concurrently
                             # Verifies no "concurrent map read/write" panics
                             # Takes ~15 seconds to complete

# Check server status
curl -s http://localhost:8080/api/v1/wants | jq '.wants | length'
```

**Test Workflow**:
1. Deploys Travel Planner recipe (independent wants: restaurant, hotel, buffet)
2. Waits 0.5 seconds
3. Deploys Fibonacci Recipe concurrently (exercises state mutations)
4. Waits 10 seconds for execution
5. Verifies both deployments succeeded and no race conditions detected

**Success Indicators**:
- Both deployments return HTTP 201 (Created)
- Server remains responsive throughout
- No "concurrent map read/write" panic in logs
- Execution completes without crashes

**Race Condition Context**:
The test specifically validates the fix for concurrent map access during:
- Multiple goroutines writing to `Want.State` map
- `AggregateChanges()` batching state updates
- `addAggregatedStateHistory()` reading state snapshots
- Concurrent execution of independent wants

**Logs Location**: `logs/mywant-backend.log`

## Code Patterns

### Recipe-Based Configuration Usage

#### Step 1: Load Recipe with Config
```go
// Load config that references a recipe
config, params, err := LoadRecipeWithConfig("config/config-travel-recipe.yaml")
builder := NewChainBuilder(config)
RegisterTravelWantTypes(builder)
builder.Execute()
```

#### Step 2: Direct Configuration Loading
```go
// Load config with direct want definitions
config, err := loadConfigFromYAML("config/config-travel.yaml")
builder := NewChainBuilder(config)
RegisterTravelWantTypes(builder)
builder.Execute()
```

### Want Configuration Examples

#### Independent Wants (Travel Recipe)
```yaml
# recipes/travel-itinerary.yaml
recipe:
  parameters:
    prefix: "travel"
    restaurant_type: "fine dining"
    hotel_type: "luxury"

  wants:
    # No using selectors - independent execution
    - type: restaurant
      params:
        restaurant_type: restaurant_type
    - type: hotel
      params:
        hotel_type: hotel_type

  coordinator:
    type: travel_coordinator
    params:
      display_name: display_name
```

#### Dependent Wants (Queue System Recipe)
```yaml
# recipes/queue-system.yaml
recipe:
  parameters:
    count: 1000
    rate: 10.0
    service_time: 0.1

  wants:
    # Generator (no dependencies)
    - type: sequence
      labels:
        role: source
        category: queue-producer
      params:
        count: count
        rate: rate
        
    # Queue (depends on generator)
    - type: queue
      labels:
        role: processor
        category: queue-processor  
      params:
        service_time: service_time
      using:
        - category: queue-producer  # Connect to generator
        
    # Sink (depends on queue)
    - type: sink
      labels:
        role: collector
      using:
        - role: processor  # Connect to queue
```

### Dynamic Want Addition

Wants can be added to a running chain dynamically:

```go
// Add a single dynamic want
builder.AddDynamicNode(Want{
    Metadata: Metadata{
        Name: "dynamic-processor",
        Type: "queue",
        Labels: map[string]string{
            "role": "processor",
            "stage": "dynamic",
        },
    },
    Spec: WantSpec{
        Params: map[string]interface{}{
            "service_time": 0.4,
        },
        Using: []map[string]string{{
            "role": "source",
        }},
    },
})

// Add multiple dynamic wants
builder.AddDynamicNodes([]Want{want1, want2, want3})
```

Dynamic wants are automatically connected based on their label selectors and integrate seamlessly with the existing chain topology.

### Memory Reconciliation

The system supports memory reconciliation for persistent state management across chain executions:

```go
// Memory is automatically loaded from YAML files at startup
// and saved during chain execution for state persistence

// Example memory configuration structure:
// - Want states and statistics
// - Connection topology  
// - Processing parameters
// - Runtime metrics
```

Memory reconciliation enables:
- **State Persistence**: Want states survive chain restarts
- **Configuration Recovery**: Automatic reload of previous configurations
- **Statistics Continuity**: Processing metrics maintained across executions
- **Dynamic Topology**: Preserved connections for dynamically added wants

## File Organization

### Configuration Layer (User Interface)
- `config/config-*-recipe.yaml`: Config files that reference recipes
- `config/config-*.yaml`: Config files with direct want definitions
- `cmd/demos/demo_*_recipe.go`: Demo programs that load recipe-based configs
- `cmd/demos/demo_*.go`: Demo programs that load direct configs

### Recipe Layer (Reusable Components)
- `recipes/*.yaml`: Recipe template files with parameters
- `recipe_loader_generic.go`: Generic recipe processing
- `recipe_loader.go`: Owner-based recipe loading

### Want Implementation Layer
- `*_types.go`: Want type implementations (qnet_types.go, travel_types.go, etc.)
- Registration functions: `Register*WantTypes(builder)`

### Core System
- `declarative.go`: Core types and configuration loading
- `chain/chain.go`: Low-level chain operations

## Dependencies

- Go 1.21+
- `gopkg.in/yaml.v3` for YAML configuration support

## Important Notes

### Recipe System
- **Config files are the user interface** - what you execute with make targets
- **Recipe files are reusable components** - consumed by config files
- Parameter substitution uses simple references (e.g., `count: count`)
- **No Go templating** - uses direct YAML parameter substitution
- Recipe wants use flattened structure (type, labels, params at top level)

### Want Connectivity
- **Independent wants**: No `using` selectors, execute in parallel
- **Dependent wants**: Use `using` selectors to form processing pipelines
- **Label-based connections**: Flexible topology without hardcoded names
- **Automatic name generation**: Recipe loader generates want names if missing

### Legacy Support
- Old template-based system archived in `archive/` directory
- Legacy `Template` field deprecated in favor of `Recipe` field
- Both imperative and declarative APIs supported for different use cases

### File Structure
- Use `declarative.go` for configuration-based chains
- Want types in separate `*_types.go` files
- Recipe files in `recipes/` directory
- Config files in `config/` directory with `config-*` naming

## System Requirements Specification

### Want Requirements

#### Core Structure
- **Metadata**: `name`, `type`, `labels` for identification and connectivity
- **Spec**: Configuration with `params`, `using` selectors, optional `Recipe` reference
- **State**: Runtime data storage via `StoreState()` method (private access)
- **History**: `ParameterHistory` and `StateHistory` for tracking changes
- **Status**: Execution state (`idle`, `running`, `completed`, `failed`)

#### Execution Lifecycle
1. **BeginExecCycle()** - Start batching state changes
2. **Exec()** - Main execution logic with channel I/O
3. **EndExecCycle()** - Commit batched changes to state and history
4. **State Persistence** - Survives across executions via memory reconciliation

#### Key Methods
- `StoreState(key, value)` - Store state changes (batched during execution)
- `GetState(key)` - Retrieve state values (returns value, exists)
- `SetSchedule()` - Apply schedule data and complete execution cycle

#### State Management
- **Batching Mode**: During execution cycle, `StoreState()` calls are batched in `pendingStateChanges`
- **Immediate Mode**: Outside execution cycle, `StoreState()` immediately updates `want.State`
- **History Recording**: State changes recorded in `StateHistory` with timestamps
- **Memory Dumps**: State persisted to YAML files for reconciliation across executions

### Agent Requirements

#### Agent Types
- **DoAgent**: Action-based agents that execute specific tasks
- **MonitorAgent**: State monitoring and data loading from external sources

#### Core Structure
- **BaseAgent**: `Name`, `Capabilities`, `Uses`, `Type`
- **Execution**: `Exec(ctx, want)` method that operates on want state
- **State Management**: Must use `want.StoreState()` for state persistence

#### Specialized Implementations
- **AgentRestaurant**: Returns `RestaurantSchedule` with restaurant-specific timing (6-9 PM, 1.5-3 hours)
- **AgentBuffet**: Returns `BuffetSchedule` with buffet-specific scheduling (lunch/dinner, 2-4 hours)
- **MonitorRestaurant**: Reads initial state from YAML files (`restaurant0.yaml`, `restaurant1.yaml`)

#### Execution Context
- Agents execute **outside** the want's execution cycle context
- `StoreState()` calls are **immediate** (not batched)
- Results stored in `agent_result` state key
- Two-step execution: MonitorAgent first, then ActionAgent conditionally

#### Agent Integration Flow
```go
// Step 1: Execute MonitorRestaurant to check existing state
monitorAgent := NewMonitorRestaurant(...)
monitorAgent.Exec(ctx, &want)

// Step 2: Only if no existing schedule found, execute AgentRestaurant
if result, exists := want.GetState("agent_result"); !exists {
    agentRestaurant.Exec(ctx, &want)
}
```

### Recipe Requirements

#### Recipe Structure
```yaml
recipe:
  parameters:          # Input parameters for template substitution
    count: 1000
    rate: 10.0

  wants:              # Array of want templates
    - type: sequence   # Want type
      labels:          # Label selectors for connectivity
        role: source
      params:          # Parameters (can reference recipe parameters)
        count: count   # Simple parameter substitution
      using:           # Optional connectivity selectors
        - category: producer

  coordinator:        # Optional orchestrating want
    type: travel_coordinator
    params:
      display_name: display_name
```

#### Recipe Types

##### Independent Wants (Travel Planning)
- **No `using` selectors** - Execute in parallel
- **Coordinator Want** - Orchestrates independent wants via input channels
- Example: Restaurant, Hotel, Buffet reservations coordinated by TravelCoordinator

##### Dependent Wants (Pipeline Processing)
- **`using` selectors** - Form processing pipelines via label matching
- **Label-based connectivity** - Flexible topology without hardcoded names
- Example: Generator → Queue → Processor → Sink pipeline

#### Recipe Processing
1. **Parameter Substitution**: Simple reference by name (e.g., `count: count`)
2. **Want Generation**: Recipe loader creates want instances with resolved parameters
3. **Connectivity**: Automatic connection based on `using` label selectors
4. **Name Generation**: Auto-generated if not specified in recipe

#### Configuration Hierarchy
- **Config Files** (`config-*-recipe.yaml`) - User interface, references recipes
- **Recipe Files** (`recipes/*.yaml`) - Reusable component templates
- **Want Types** (`*_types.go`) - Implementation layer
- **Demo Programs** - Entry points that load and execute configs

#### Key Features
- **Memory Reconciliation**: State persistence across executions
- **Dynamic Want Addition**: Runtime topology modification
- **Owner References**: Parent-child relationships for lifecycle management
- **Validation**: OpenAPI spec validation for configuration integrity

### State Management Requirements

#### State History Initialization
All `StateHistory` fields must be properly initialized to prevent null reference errors:

```go
// Required in all state history append operations
if want.History.StateHistory == nil {
    want.History.StateHistory = make([]StateHistoryEntry, 0)
}
want.History.StateHistory = append(want.History.StateHistory, entry)
```

#### Critical Locations Requiring StateHistory Initialization
- `addToStateHistory()` method - Individual state change tracking
- `addAggregatedStateHistory()` method - Bulk state change tracking
- Agent state commit operations - Batch agent state changes
- Want creation/loading - Initial setup phase

#### State Access Patterns
- **Private State**: `want.state` field should not be accessed directly
- **Controlled Access**: All state changes must use `StoreState()` method
- **State Retrieval**: Use `GetState()` method which returns `(value, exists)`
- **Encapsulation**: Maintains proper separation between internal state and public API
- make run-dynamic-travel-change didn't wait until rebooking.
- frontend recipe card should have control bar to have deploy button of the recipe so that selected recipe can be deployed in one click as want.
- 直接のStateへのアクセスも全部GetStateに変換したい。\
    provided, _ := d.State["description_provided"].(bool)


# Coding rule

- ここのビルド、テストではsleep は7秒で良い。それ位以上のsleepは禁止する。
- ビルドはいちいちmake build-serverを呼ぶのは禁止する。make restart-allに全て含まれている。

## Debugging Travel Itinerary Coordinator Issue (2025-11-16)

### Problem Statement
The Travel Itinerary recipe's child wants (Restaurant, Hotel, Buffet) cannot send their generated schedule data through output channels to the Coordinator want. As a result:
- Coordinator want gets stuck in "Running" state with 0 inputs received
- Child wants report "Output channel not available" and skip sending
- Expected: Coordinator should receive all 3 schedules and reach "Completed" state

### Root Cause Analysis
**Issue Chain:**
1. Paths are generated by `generatePathsFromConnections()` in `cb.pathMap` but aren't synchronized to individual Want structs
2. Child wants execute before checking if output channels are properly set
3. Child wants can't find output channels because `Want.paths.Out` is empty at execution time

**Previous Session Fix:** Added path synchronization code in chain_builder.go lines 670-671:
```go
// Synchronize generated paths to individual Want structs
for wantName, paths := range cb.pathMap {
    if runtimeWant, exists := cb.wants[wantName]; exists {
        runtimeWant.want.paths.In = paths.In
        runtimeWant.want.paths.Out = paths.Out
    }
}
```

### Logging Configuration

**Log Output Location:** `/Users/hiroyukiosaki/work/MyWant/logs/mywant-backend.log`

**Server Configuration (engine/cmd/server/main.go:2067-2074):**
```go
// Configure logging to a file
logFile, err := os.OpenFile("./logs/mywant-backend.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
if err != nil {
	log.Fatalf("Failed to open log file: %v", err)
}
defer logFile.Close()
log.SetOutput(logFile)
log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
```

**How to Access Logs:**
1. Logs are written to file at runtime
2. After test execution completes, read the log file with: `cat /Users/hiroyukiosaki/work/MyWant/logs/mywant-backend.log`
3. Search logs with: `grep "RESTAURANT\|HOTEL\|BUFFET\|COORDINATOR" /Users/hiroyukiosaki/work/MyWant/logs/mywant-backend.log`
4. For path generation debugging: `grep "RECONCILE:CONNECT" /Users/hiroyukiosaki/work/MyWant/logs/mywant-backend.log`

### Current Investigation Plan (All Steps Must Be Completed)

#### Step 1: Stop All Background Processes
Kill all running background bash processes to avoid interference with fresh test run

#### Step 2: Verify travel_types.go Structure
Examine RestaurantWant, HotelWant, and BuffetWant to identify public getter methods for accessing output channels
- Look for methods like `GetOutputChannels()`, `GetFirstOutputChannel()`, or similar
- Must use **getter methods** (public API), not direct field access like `r.paths`

#### Step 3: Add Debug Logging to RestaurantWant.Exec()
Use proper getter methods to log:
- Whether output channels are available at execution start
- Details of each output channel (name, state, etc.)
- Whether data was successfully sent to the channel
```
Example expected logs:
[RESTAURANT] Exec() start - Output channels: available=true, count=1
[RESTAURANT] Sending schedule to output channel...
[RESTAURANT] Schedule sent successfully
```

#### Step 4: Add Similar Logging to HotelWant and BuffetWant
Replicate the debugging approach from Step 3 for consistency

#### Step 5: Rebuild and Restart Server
Use `make restart-all` only (no separate build command)

#### Step 6: Clear Log File and Deploy Travel Recipe
1. Clear `/Users/hiroyukiosaki/work/MyWant/logs/mywant-backend.log`
2. Deploy Travel Itinerary recipe via API
3. Wait 7 seconds for execution

#### Step 7: Analyze Logs for Path Generation
Look for logs indicating:
- How many paths were generated in `cb.pathMap`
- For each child want: how many In/Out paths were set
- Whether path synchronization succeeded for each want
- Example: `[RECONCILE:CONNECT] Synchronized paths for 'restaurant_want': Set In=0, Out=1`

#### Step 8: Analyze Logs for Execution Output
Look for logs from child wants showing:
- Whether output channels were available during Exec()
- Whether data was successfully sent
- If "Output channel not available" is still reported
- Example: `[RESTAURANT] Exec() - Output channels available: true`

#### Step 9: Analyze Logs for Coordinator Status
Check whether Coordinator received all 3 inputs:
- Expected: `[COORDINATOR] All 3 inputs connected` → `Received 3 packets` → `Status: Completed`
- If not: Determine what packets it actually received

### Expected Success Criteria
When all steps are completed correctly:
1. Logs show path generation and synchronization succeeded
2. Child wants report output channels are available
3. Child wants log successful packet transmission
4. Coordinator logs show receipt of all 3 schedules
5. Coordinator status changes from "Running" to "Completed"

### Files Involved
- `engine/src/chain_builder.go` - Path generation and synchronization (already has debug logs from previous session)
- `engine/cmd/types/travel_types.go` - RestaurantWant, HotelWant, BuffetWant implementations
- `recipes/travel-itinerary.yaml` - Travel recipe definition with execution ordering
- `logs/mywant-backend.log` - Output logs for analysis

## Codebase RAG Database System

A **Retrieval Augmented Generation (RAG)** database is available for semantic code search and architecture understanding across the entire MyWant repository.

### Database Status

- **Location**: `codebase_rag.db` (632KB SQLite database)
- **Indexed Entities**: 760 total
  - Functions: 409
  - Structs/Types: 198
  - Interfaces: 15
  - Files: 138
- **Top Components**:
  - `engine/cmd/server/main.go` (56 entities)
  - `engine/src/chain_builder.go` (38 entities)
  - `engine/src/want.go` (30 entities)

### Quick Usage

#### Python API (Recommended for Claude Code)

```python
from tools.codebase_rag import CodebaseRAG

# Initialize
rag = CodebaseRAG("codebase_rag.db")

# Search for code
results = rag.search("Want execution", limit=10)
results = rag.search("GetOutputChannel", entity_types=['function'])
results = rag.search("ChainBuilder", entity_types=['struct'])

# Get architecture overview
overview = rag.get_architecture_overview()
print(f"Total entities: {overview['total_entities']}")
print(f"By type: {overview['by_type']}")
print(f"Top files: {overview['top_files']}")

# Close when done
rag.close()
```

#### Interactive CLI

```bash
python3 tools/codebase_rag.py

# Inside interactive mode:
# "Want execution" - keyword search
# "func:GetOutputChannel" - search functions
# "struct:ChainBuilder" - search structs
# "file:chain_builder.go" - search files
# "arch" - show architecture overview
# "help" - show all options
```

### Common Search Patterns

#### Understanding Want Execution
```python
results = rag.search("Exec", entity_types=['function'])  # Find Exec implementations
results = rag.search("BeginExecCycle", entity_types=['function'])  # State management
results = rag.search("GetState", entity_types=['function'])  # State access methods
```

#### Channel Communication
```python
results = rag.search("GetInputChannel", entity_types=['function'])
results = rag.search("GetOutputChannel", entity_types=['function'])
results = rag.search("Paths", entity_types=['struct'])  # Path information
```

#### Chain Building
```python
results = rag.search("ChainBuilder", entity_types=['struct'])
results = rag.search("AddDynamicNode", entity_types=['function'])
results = rag.search("generatePathsFromConnections", entity_types=['function'])
```

#### Recipe System
```python
results = rag.search("GenericRecipe", entity_types=['struct'])
results = rag.search("LoadRecipe", entity_types=['function'])
results = rag.search("RegisterWantType", entity_types=['function'])
```

#### Travel Planning
```python
results = rag.search("RestaurantWant", entity_types=['struct'])
results = rag.search("TravelCoordinator", entity_types=['struct'])
results = rag.search("GetSchedule", entity_types=['function'])
```

#### Queue System
```python
results = rag.search("Numbers", entity_types=['struct'])  # Generator
results = rag.search("Queue", entity_types=['struct'])  # Processor
results = rag.search("Sink", entity_types=['struct'])  # Collector
```

### Rebuilding the Database

When codebase changes significantly:

```bash
# Option 1: Direct rebuild
python3 tools/codebase_rag.py index

# Option 2: Using bash wrapper
bash tools/rag index        # Rebuild index
bash tools/rag reset        # Delete and rebuild
bash tools/rag arch         # Show architecture
```

### Files and Resources

- **Main RAG System**: `tools/codebase_rag.py` (760 lines, fully documented)
- **Documentation**: `tools/README_RAG.md` (comprehensive guide)
- **Quick Start**: `QUICKSTART_RAG.md` (quick reference)
- **Bash Wrapper**: `tools/rag` (convenient command-line interface)
- **Database**: `codebase_rag.db` (SQLite, 632KB)
- **Requirements**: `tools/requirements-rag.txt` (`pip install` compatible)

### Integration with Claude Code

The RAG system can be used to:

1. **Find Code Patterns**: Search for similar implementations
2. **Understand Architecture**: View codebase structure and relationships
3. **Locate Functionality**: Quickly find where features are implemented
4. **Code Analysis**: Gather context for bug fixes and refactoring
5. **Documentation**: Extract code organization and component descriptions

### Performance

- **Indexing**: ~2 seconds for full codebase
- **Search**: <100ms typical queries
- **Database Size**: ~632KB (small, fits in memory)
- **Memory Usage**: Minimal (only loads on search)

### Limitations

- **Keyword-based search** by default (text matching)
- **Semantic search** requires: `pip install sentence-transformers`
- **Updates**: Requires manual rebuild when code changes significantly

### Troubleshooting

**Database not found**: Run `python3 tools/codebase_rag.py index`

**No results**: Try simpler search terms or check spelling

**Want better results**: Install embeddings model:
```bash
pip install sentence-transformers
python3 tools/codebase_rag.py index  # Rebuild with embeddings
```