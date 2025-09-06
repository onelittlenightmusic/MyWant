# MyWant

A Go library implementing functional programming patterns with channels, supporting declarative configuration-based approaches for flexible processing topologies and data flow architectures. MyWant introduces a declarative programming paradigm that eliminates the need for prior knowledge beyond YAML configuration files, removing prerequisites for understanding individual components or their internal implementations.

## Installation

```bash
git clone https://github.com/onelittlenightmusic/MyWant.git
cd MyWant
go mod tidy
```

## Why Choose MyWant's Declarative Framework?

### **Zero Learning Curve**
- **YAML-Only Configuration**: No need to learn programming languages, APIs, or complex frameworks
- **Self-Documenting**: Configuration files serve as both specification and documentation
- **Instant Productivity**: Start building complex systems immediately without studying component internals

### **Component Agnostic**
- **Black Box Approach**: Use any processing component without understanding its implementation
- **Mix and Match**: Combine components from different domains seamlessly
- **Focus on What, Not How**: Declare desired outcomes rather than implementation details

### **Effortless Scalability**
- **Recipe Reusability**: Define patterns once, reuse across multiple configurations
- **Parameter Substitution**: Adapt existing recipes with different parameters
- **Dynamic Composition**: Add or modify components at runtime through configuration

### **Maintenance Freedom**
- **Configuration-Driven Changes**: Modify system behavior without code changes
- **Version Control Friendly**: Track system evolution through YAML file changes
- **Environment Adaptation**: Same recipes, different parameters for dev/staging/production

## Features

- **Recipe-based Configuration**: Define complex processing topologies using YAML recipe files
- **Independent & Dependent Wants**: Support both parallel processing and sequential pipelines
- **Dynamic Want Addition**: Add wants to running systems at runtime
- **Memory Reconciliation**: Persistent state management across system executions
- **Label-based Connectivity**: Flexible want connections using label selectors
- **Multi-flow Processing**: Support for parallel processing flows with combiners

## Core Concepts

### Config YAML - Top-Level User Interface
Config YAML files are the main interface for running MyWant programs. They specify what to execute and how:

```yaml
# Option 1: Direct want definitions
wants:
  - metadata: {...}
    spec: {...}

# Option 2: Reference reusable recipes  
recipe:
  path: "recipes/queue-system.yaml"
  parameters:
    count: 500
    rate: 20.0
```

### Wants
A "want" is a processing unit that performs a specific task. Wants can be:
- **Independent**: Execute in parallel without dependencies (e.g., travel bookings)
- **Dependent**: Connected in processing pipelines using `using` selectors (e.g., data processing flows)

### Recipes - Reusable Components
Recipes are YAML templates stored in the `recipes/` directory that define reusable want configurations with parameters. They are consumed by config files to avoid duplication and enable parameterization.

## Quick Start

### Example 1: Independent Wants (Travel Planning)

Independent wants execute in parallel without dependencies - perfect for orchestrated tasks.

**Step 1: Create the recipe** (reusable component in `recipes/travel-itinerary.yaml`):
```yaml
recipe:
  # Recipe parameters - these are the configurable values
  parameters:
    prefix: "travel"
    display_name: "One Day Travel Itinerary" 
    restaurant_type: "fine dining"
    hotel_type: "luxury"
    buffet_type: "international"
    dinner_duration: 2.0

  wants:
    # Dinner restaurant reservation (independent)
    - type: restaurant
      params:
        restaurant_type: restaurant_type
        duration_hours: dinner_duration

    # Hotel accommodation booking (independent)
    - type: hotel  
      params:
        hotel_type: hotel_type

    # Morning breakfast buffet (independent)
    - type: buffet
      params:
        buffet_type: buffet_type

  coordinator:
    type: travel_coordinator
    params:
      display_name: display_name
    using:
      - role: scheduler
```

**Step 2: Create the config file** (top-level user interface in `config/config-travel-recipe.yaml`):
```yaml
recipe:
  path: "recipes/travel-itinerary.yaml"
  parameters:
    prefix: "vacation"
    display_name: "Weekend Vacation Itinerary"
    restaurant_type: "michelin starred"
    hotel_type: "boutique"
    buffet_type: "continental"
    dinner_duration: 2.5
```

**Step 3: Run with the config file:**
```sh
make run-travel-recipe  # Uses config/config-travel-recipe.yaml
```

### Example 2: Dependent Wants (Queue System)

Dependent wants form processing pipelines using `using` selectors to connect outputs to inputs.

**Step 1: Create the recipe** (reusable component in `recipes/queue-system.yaml`):
```yaml
recipe:
  # Recipe parameters
  parameters:
    prefix: "queue-system"
    count: 1000
    rate: 10.0
    service_time: 0.1
    deterministic: false

  wants:
    # Generator want (no dependencies)
    - type: sequence
      labels:
        role: source
        category: queue-producer
        component: generator
      params:
        count: count
        rate: rate
        deterministic: deterministic

    # Queue want (depends on generator)
    - type: queue
      labels:
        role: processor
        category: queue-processor
        component: main-queue
      params:
        service_time: service_time
        deterministic: deterministic
      using:
        - category: queue-producer  # Connect to generator

    # Sink want (depends on queue)
    - type: sink
      labels:
        role: collector
        category: result-display
        component: final-sink
      params:
        display_format: "Number: %d"
      using:
        - role: processor  # Connect to queue
```

**Step 2: Create the config file** (top-level user interface in `config/config-queue-system-recipe.yaml`):
```yaml
recipe:
  path: "recipes/queue-system.yaml"
  parameters:
    count: 500
    rate: 20.0
    service_time: 0.05
    deterministic: true
```

**Step 3: Run with the config file:**
```sh
make run-queue-system-recipe  # Uses config/config-queue-system-recipe.yaml
```

## Usage

### Installation & Setup

```bash
git clone https://github.com/onelittlenightmusic/MyWant.git
cd MyWant
go mod tidy
make build
```

### Running Examples

MyWant provides three main usage patterns through make targets:

#### 1. Recipe-Based Examples (Recommended)
Recipe-based examples use reusable YAML templates with parameterization:

```sh
# Independent wants - parallel execution with coordination
make run-travel-recipe        # Travel planning from recipe (restaurant, hotel, buffet)

# Dependent wants - sequential pipeline processing  
make run-queue-system-recipe  # Queue system pipeline (generator → queue → sink)

# Complex multi-stream processing
make run-qnet-recipe         # Multi-generator QNet with parallel processing
make run-qnet-using-recipe   # QNet with YAML-defined using connections
```

**Pattern**: `demo_*_recipe.go` + `config/config-*-recipe.yaml` + `recipes/*.yaml`

#### 2. Direct Configuration Examples
Direct configuration defines wants inline without recipes:

```sh
# Simple processing chains
make run-qnet          # QNet simulation (direct YAML config)
make run-travel        # Travel planning (direct want definitions)

# Advanced processing patterns
make run-fibonacci-loop # Fibonacci with feedback loop architecture
```

**Pattern**: `demo_*.go` + `config/config-*.yaml`

#### 3. Owner-Based Dynamic Examples
Owner-based examples demonstrate dynamic want creation at runtime:

```sh
# Dynamic want generation using recipes
make run-sample-owner                    # Basic target with recipe loading
make run-sample-owner-config            # Target with configuration parameters
make run-sample-owner-high-throughput   # High-performance processing target
make run-sample-owner-dry               # Fast processing with minimal service time
make run-sample-owner-input             # Target with custom input processing
make run-qnet-target                    # QNet processing via target want
make run-travel-target                  # Travel planning via target want
```

**Pattern**: `demo_*_owner.go` or `demo_*_target.go` + config + recipe loading

#### 4. Build & Test
```sh
make build      # Build the MyWant library
make test-build # Test build with dependency check
```

### Usage Pattern Selection Guide

| Pattern | Best For | Configuration | Runtime Behavior |
|---------|----------|---------------|------------------|
| **Recipe-Based** | Reusable, parameterized systems | YAML recipe templates | Static want topology |
| **Direct Config** | Simple, one-off configurations | Inline YAML definitions | Static want topology |
| **Owner-Based** | Dynamic, adaptive systems | Config + runtime recipe loading | Dynamic want creation |

### Complete Example: Queue System with MyWant APIs

Here's a complete example showing how to use MyWant APIs to build a queue processing system:

#### 1. Create Want Types (`queue_types.go`)

```go
package main

import (
    . "mywant"
    "mywant/chain"
)

// Numbers generates packets and sends them downstream
type Numbers struct {
    Want
    Rate  float64
    Count int
    paths Paths
}

func (n *Numbers) GetPaths() *Paths { return &n.paths }

func (n *Numbers) ProcessInput(input *chain.Msg) {
    // Generate packets at specified rate
    for i := 0; i < n.Count; i++ {
        packet := &QueuePacket{Num: i, Time: float64(i)/n.Rate}
        n.paths.Outputs[0] <- &chain.Msg{Data: packet}
    }
    close(n.paths.Outputs[0])
}

// Queue processes packets with service time
type Queue struct {
    Want
    ServiceTime float64
    paths       Paths
}

func (q *Queue) GetPaths() *Paths { return &q.paths }

func (q *Queue) ProcessInput(input *chain.Msg) {
    packet := input.Data.(*QueuePacket)
    // Simulate processing time
    time.Sleep(time.Duration(q.ServiceTime * 1000) * time.Millisecond)
    q.paths.Outputs[0] <- input
}

// Register want types with the builder
func RegisterQueueWantTypes(builder *ChainBuilder) {
    builder.RegisterWantType("numbers", func(metadata Metadata, spec WantSpec) interface{} {
        return &Numbers{
            Want:  Want{Metadata: metadata, Spec: spec},
            Rate:  spec.Params["rate"].(float64),
            Count: int(spec.Params["count"].(float64)),
        }
    })
    
    builder.RegisterWantType("queue", func(metadata Metadata, spec WantSpec) interface{} {
        return &Queue{
            Want:        Want{Metadata: metadata, Spec: spec},
            ServiceTime: spec.Params["service_time"].(float64),
        }
    })
    
    builder.RegisterWantType("sink", func(metadata Metadata, spec WantSpec) interface{} {
        return NewSink(metadata, spec)
    })
}
```

#### 2. Create Configuration (`config/config-queue.yaml`)

```yaml
wants:
  - metadata:
      name: generator
      type: numbers
      labels:
        role: source
    spec:
      params:
        rate: 10.0
        count: 1000

  - metadata:
      name: processor
      type: queue
      labels:
        role: processor
    spec:
      params:
        service_time: 0.05
      using:
        - role: source

  - metadata:
      name: collector
      type: sink
      labels:
        role: collector
    spec:
      params: {}
      using:
        - role: processor
```

#### 3. Create Main Program (`main.go`)

```go
package main

import (
    "fmt"
    . "mywant"
)

func main() {
    // Load configuration from YAML
    config, err := LoadConfigFromYAML("config/config-queue.yaml")
    if err != nil {
        fmt.Printf("Error loading config: %v\n", err)
        return
    }

    // Create chain builder with the configuration
    builder := NewChainBuilder(config)
    
    // Register your custom want types
    RegisterQueueWantTypes(builder)
    
    // Execute the chain
    fmt.Println("Starting queue processing system...")
    builder.Execute()
    
    // Get final states
    states := builder.GetAllWantStates()
    for name, state := range states {
        fmt.Printf("Want %s: %s (processed: %v)\n", 
            name, state.Status, state.Stats["total_processed"])
    }
}
```

#### 4. Run the Example

```bash
go run main.go queue_types.go
```

This example demonstrates:
- **Want Creation**: Custom want types implementing the Want interface
- **Configuration Loading**: Using `LoadConfigFromYAML()` to load YAML configs
- **Chain Building**: Using `NewChainBuilder()` to create processing chains
- **Type Registration**: Using `RegisterWantType()` to register custom want types
- **Execution**: Using `Execute()` to run the processing chain
- **State Management**: Using `GetAllWantStates()` to retrieve results

## Configuration System

### Config Files (Top-Level Interface)
Config files are what you run - they specify the execution parameters:

```yaml
# Direct want definition approach
wants:
  - metadata:
      name: my-generator
      type: sequence
    spec:
      params:
        count: 1000
        rate: 10.0

# Recipe-based approach (recommended)
recipe:
  path: "recipes/queue-system.yaml"  # Reference reusable component
  parameters:                       # Override recipe defaults
    count: 500
    rate: 20.0
```

### Config vs Recipe Relationship
- **Config files**: Top-level user interface, what you execute
- **Recipe files**: Reusable components, consumed by multiple configs
- **Benefits**: Share recipes across projects, parameterize common patterns

## Recipe System

### Recipe Structure
```yaml
recipe:
  parameters:        # Configurable values
    param_name: default_value

  wants:            # Want definitions
    - type: want_type
      labels:       # Optional labels for connections
        key: value
      params:       # Parameters (can reference recipe parameters)
        param: param_name
      using:        # Optional dependencies
        - label_selector

  coordinator:      # Optional coordinator want
    type: coordinator_type
    params: {...}
    using: [...]
```

### Using Selectors
Connect wants using label selectors in the `using` field:
- `role: source` - Connect to wants with `role: source` label
- `category: processor` - Connect to wants with `category: processor` label
- `stage: first` - Connect to wants with `stage: first` label

### Dynamic Want Addition

```go
// Add wants at runtime
builder.AddDynamicNode(Want{
    Metadata: Metadata{
        Name: "dynamic-processor",
        Type: "queue",
        Labels: map[string]string{"role": "processor"},
    },
    Spec: WantSpec{
        Params: map[string]interface{}{"service_time": 0.4},
        Using: []map[string]string{{"role": "source"}},
    },
})
```

## Memory Reconciliation

The system automatically manages persistent state:
- Want states and statistics survive restarts  
- Configuration recovery from memory dumps
- Dynamic topology preservation
- Memory files stored in `memory/` directory

## Typical Workflow

1. **Create/Choose Recipe**: Define reusable want patterns in `recipes/`
2. **Create Config File**: Reference recipe and set parameters in `config/config-*.yaml`
3. **Execute**: Run with `make` targets or `go run demo_*.go config/config-*.yaml`
4. **Parameterize**: Reuse recipes with different configs for different scenarios

## Architecture

- **Config Layer**: Top-level user interface for execution
- **Recipe Layer**: Reusable component templates with parameters
- **Want Layer**: Individual processing units (independent or dependent)
- **Independent Wants**: Execute in parallel, coordinated by a coordinator want
- **Dependent Wants**: Form processing pipelines with `using` connections
- **Labels**: Enable flexible connections without hardcoded want names
- **Memory System**: Persistent state across executions

## Dependencies

- Go 1.21+
- `gopkg.in/yaml.v3` for YAML configuration support

## Contributing

The codebase follows these patterns:
- Want types in `*_types.go` files
- Recipe files in `recipes/` directory
- Configuration files in `config/` directory follow `config-*-recipe.yaml` naming
- Demo programs follow `demo_*_recipe.go` naming