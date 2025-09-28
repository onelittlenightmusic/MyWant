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