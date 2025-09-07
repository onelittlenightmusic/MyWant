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