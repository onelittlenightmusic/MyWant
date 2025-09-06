# MyWant

A Go library implementing functional programming patterns with channels, supporting declarative configuration-based approaches for flexible processing topologies and data flow architectures. MyWant introduces a declarative programming paradigm that eliminates the need for prior knowledge beyond YAML configuration files, removing prerequisites for understanding individual components or their internal implementations.

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
- **Direct Configuration**: Define wants with explicit parameters and connections
- **Label-based Connections**: Flexible want topology using label selectors
- **Dynamic Composition**: Add or modify components at runtime through configuration

### **Maintenance Freedom**
- **Configuration-Driven Changes**: Modify system behavior without code changes
- **Version Control Friendly**: Track system evolution through YAML file changes
- **Environment Adaptation**: Different parameters for dev/staging/production

## Features

- **Direct Want Configuration**: Define processing topologies using explicit YAML configurations
- **Independent & Dependent Wants**: Support both parallel processing and sequential pipelines
- **Dynamic Want Addition**: Add wants to running systems at runtime
- **Memory Reconciliation**: Persistent state management across system executions
- **Label-based Connectivity**: Flexible want connections using label selectors
- **Multi-flow Processing**: Support for parallel processing flows with combiners

## Core Concepts

### Config YAML - Direct Want Configuration
Config YAML files define wants directly without templates or recipes:

```yaml
# Direct want definitions only
wants:
  - metadata:
      name: processor
      type: queue
      labels:
        role: processor
    spec:
      params:
        service_time: 0.1
      using:
        - role: source
```

### Wants
A "want" is a processing unit that performs a specific task. Wants can be:
- **Independent**: Execute in parallel without dependencies (e.g., travel bookings)
- **Dependent**: Connected in processing pipelines using `using` selectors (e.g., data processing flows)

All wants are defined directly in configuration files with explicit parameters and connections.

## Quick Start

### Example 1: Independent Wants (Travel Planning)

Independent wants execute in parallel without dependencies - perfect for orchestrated tasks.

**Create your config file** (`config/config-travel.yaml`):
```yaml
wants:
  # Dinner restaurant reservation (independent)
  - metadata:
      name: dinner-reservation
      type: restaurant
      labels:
        role: scheduler
        category: dining
    spec:
      params:
        restaurant_type: "fine dining"
        duration_hours: 2.0

  # Hotel accommodation booking (independent)
  - metadata:
      name: hotel-booking
      type: hotel
      labels:
        role: scheduler
        category: accommodation
    spec:
      params:
        hotel_type: "luxury"

  # Morning breakfast buffet (independent)
  - metadata:
      name: breakfast-buffet
      type: buffet
      labels:
        role: scheduler
        category: dining
    spec:
      params:
        buffet_type: "international"

  # Travel coordinator (collects all bookings)
  - metadata:
      name: get-one-day-travel
      type: travel_coordinator
      labels:
        role: coordinator
    spec:
      params:
        display_name: "One Day Travel Itinerary"
      using:
        - role: scheduler
```

**Run the example:**
```sh
make run-travel  # Uses config/config-travel.yaml
```

### Example 2: Dependent Wants (Queue System)

Dependent wants form processing pipelines using `using` selectors to connect outputs to inputs.

**Create your config file** (`config/config-qnet.yaml`):
```yaml
wants:
  # Generator want (no dependencies)
  - metadata:
      name: gen-primary
      type: numbers
      labels:
        role: source
        stream: primary
    spec:
      params:
        count: 1000
        rate: 10.0

  # Queue want (depends on generator)
  - metadata:
      name: queue-main
      type: queue
      labels:
        role: processor
        stream: primary
    spec:
      params:
        service_time: 0.05
      using:
        - role: source  # Connect to generator

  # Sink want (depends on queue)
  - metadata:
      name: sink-main
      type: sink
      labels:
        role: collector
    spec:
      params: {}
      using:
        - role: processor  # Connect to queue
```

**Run the example:**
```sh
make run-qnet  # Uses config/config-qnet.yaml
```

## Usage

### Import MyWant Module

Add MyWant to your Go project:

```bash
go mod init your-project
go get github.com/onelittlenightmusic/MyWant
```

Import in your Go code:

```go
package main

import (
    "fmt"
    "mywant"
)

func main() {
    // Load configuration from YAML
    config, err := mywant.LoadConfigFromYAML("config.yaml")
    if err != nil {
        panic(err)
    }

    // Create and execute chain
    builder := mywant.NewChainBuilder(config)
    
    // Register your want types
    builder.RegisterWantType("your-type", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
        return NewYourWant(metadata, spec)
    })
    
    builder.Execute()
}
```

### Running Examples

MyWant uses direct want configuration defined in YAML files:

#### Direct Want Examples
All examples use direct want definitions with explicit parameters:

```sh
# Independent wants - parallel execution
make run-travel        # Travel planning with coordination

# Dependent wants - pipeline processing  
make run-qnet          # Queue system (generator → queue → sink)
make run-qnet-using-recipe # Advanced QNet with multiple streams

# Computational patterns
make run-fibonacci     # Fibonacci sequence generation
make run-fibonacci-loop # Fibonacci with feedback loop architecture
make run-prime         # Prime number generation

# Dynamic want creation examples
make run-sample-owner  # Target-based dynamic want creation
make run-sample-owner-dry # Queue system with wait time analysis
make run-travel-target # Travel planning using target wants
```

#### Build & Test
```sh
make build      # Build the MyWant library
make test-build # Test build with dependency check
```

#### Complete List of Available Examples

**Basic Processing Patterns:**
```sh
make run-qnet          # Simple queue system (generator → queue → sink)
make run-prime         # Prime number generation
make run-fibonacci-loop # Fibonacci with feedback loops
```

**Advanced Processing:**
```sh
make run-qnet-using-recipe  # Multi-stream QNet with combiners
make run-travel            # Independent travel planning wants
```

**Dynamic Want Creation (Target-Based):**
```sh
make run-sample-owner      # Basic target want demo
make run-sample-owner-dry  # Queue system with wait time analysis
make run-travel-target     # Dynamic travel planning
```

**Configuration Variants:**
```sh
make run-sample-owner-config           # Target with custom config
make run-sample-owner-high-throughput  # High-performance target
make run-sample-owner-input           # Target with input handling
make run-qnet-target                  # QNet using target architecture
```

### Usage Pattern Selection Guide

| Pattern | Best For | Configuration | Runtime Behavior |
|---------|----------|---------------|------------------|
| **Direct Config** | Explicit, clear configurations | Inline YAML definitions | Static want topology |
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

### Config Files - Direct Want Definition
Config files define wants directly with explicit parameters and connections:

```yaml
# Direct want definitions
wants:
  - metadata:
      name: my-generator
      type: numbers
      labels:
        role: source
    spec:
      params:
        count: 1000
        rate: 10.0

  - metadata:
      name: my-processor
      type: queue
      labels:
        role: processor
    spec:
      params:
        service_time: 0.05
      using:
        - role: source
```

### Configuration Principles
- **Explicit**: All want parameters are clearly defined
- **Direct**: No indirection through templates or recipes
- **Traceable**: Easy to understand what each want does

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

1. **Create Config File**: Define wants directly in `config/config-*.yaml`
2. **Execute**: Run with `make` targets or `go run demo_*.go`
3. **Customize**: Modify want parameters and connections as needed

## Architecture

- **Config Layer**: Top-level user interface for execution
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
- Configuration files in `config/` directory follow `config-*.yaml` naming
- Demo programs in `cmd/demos/` directory