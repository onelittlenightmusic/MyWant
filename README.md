# MyWant

## Key Differentiation: Declarative Desire Expression

MyWant transforms system design from describing **operations** to expressing **human desires and business outcomes**. Instead of telling the system "how to do something," you declare "what you want to achieve" - like `customer-satisfaction: "delighted"` or `order-fulfillment: "next-day"`. The system automatically determines the operational steps needed to achieve your desired states.

---

A Go library implementing functional chain programming patterns with channels. The project uses a **recipe-based configuration system** where config YAML files serve as the top-level user interface and recipes provide reusable component templates. MyWant introduces a declarative programming paradigm that eliminates the need for prior knowledge beyond YAML configuration files, removing prerequisites for understanding individual components or their internal implementations.

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
- **Recipe-Based Configuration**: Use reusable component templates with parameter substitution
- **Direct Configuration**: Define wants with explicit parameters and connections
- **Label-based Connections**: Flexible want topology using label selectors
- **Dynamic Composition**: Add or modify components at runtime through configuration

### **Maintenance Freedom**
- **Configuration-Driven Changes**: Modify system behavior without code changes
- **Version Control Friendly**: Track system evolution through YAML file changes
- **Environment Adaptation**: Different parameters for dev/staging/production

## Features

- **Recipe-Based Configuration**: Reusable component templates with parameter substitution
- **Independent & Dependent Wants**: Support both parallel processing and sequential pipelines
- **Dynamic Want Addition**: Add wants to running systems at runtime
- **Memory Reconciliation**: Persistent state management across system executions
- **Label-based Connectivity**: Flexible want connections using label selectors
- **Multi-flow Processing**: Support for parallel processing flows with combiners
- **Notification System**: Built-in monitoring and alerting capabilities
- **Parameter History**: Track parameter changes and execution cycles

## Core Concepts

### Wants
A "want" is a processing unit that performs a specific task. Wants can be:
- **Independent**: Execute in parallel without dependencies (e.g., travel bookings)
- **Dependent**: Connected in processing pipelines using `using` selectors (e.g., data processing flows)

Wants are defined in configuration files with explicit parameters and connections.

## Quick Start

### Configuration Approaches

MyWant supports two configuration approaches:
1. **Recipe-Based**: Use reusable component templates (recommended)
2. **Direct Configuration**: Define wants directly in config files

### Example 1: Recipe-Based Configuration (Travel Planning)

Independent wants execute in parallel without dependencies - perfect for orchestrated tasks.

**Create config file** (`config/config-travel-recipe.yaml`):
```yaml
recipe_path: "recipes/travel-itinerary.yaml"
parameters:
  prefix: "travel"
  restaurant_type: "fine dining"
  hotel_type: "luxury"
  display_name: "One Day Travel Itinerary"
```

**Create recipe file** (`recipes/travel-itinerary.yaml`):
```yaml
recipe:
  parameters:
    prefix: "travel"
    restaurant_type: "fine dining"
    hotel_type: "luxury"
    display_name: "Travel Itinerary"

  wants:
    # Restaurant booking (independent)
    - type: restaurant
      labels:
        role: scheduler
        category: dining
      params:
        restaurant_type: restaurant_type
        duration_hours: 2.0

    # Hotel booking (independent)
    - type: hotel
      labels:
        role: scheduler
        category: accommodation
      params:
        hotel_type: hotel_type

    # Coordinator (collects all bookings)
  coordinator:
    type: travel_coordinator
    params:
      display_name: display_name
```

**Run the example:**
```sh
make run-travel-recipe  # Uses config/config-travel-recipe.yaml → recipes/travel-itinerary.yaml
```

### Example 2: Hierarchical Approval System (RecipeAgent Auto-Connection)

This example demonstrates advanced auto-connection capabilities where RecipeAgent wants automatically discover and connect to compatible wants based on shared identifiers.

**Create config file** (`config/config-hierarchical-approval.yaml`):
```yaml
wants:
  # Evidence want - shared by all approval levels
  - metadata:
      name: evidence
      type: evidence
      labels:
        role: evidence-provider
        category: approval-data
        approval_id: "approval-001"
    spec:
      params:
        evidence_type: "document"
        approval_id: "approval-001"

  # Description want - shared by all approval levels
  - metadata:
      name: description
      type: description
      labels:
        role: description-provider
        category: approval-data
        approval_id: "approval-001"
    spec:
      params:
        description_format: "Request for approval: %s"
        approval_id: "approval-001"

  # Level 1 Approval Target - uses "level 1 approval" custom type
  - metadata:
      name: level1_approval
      type: "level 1 approval"
      labels:
        role: approval-target
        approval_level: "1"
    spec:
      params:
        approval_id: "approval-001"
        coordinator_type: "level1"

  # Level 2 Approval Target - uses "level 2 approval" custom type
  - metadata:
      name: level2_approval
      type: "level 2 approval"
      labels:
        role: approval-target
        approval_level: "2"
    spec:
      params:
        approval_id: "approval-001"
        coordinator_type: "level2"
        level2_authority: "senior_manager"
```

**Create recipe files** (`recipes/approval-level-1.yaml` and `recipes/approval-level-2.yaml`):
```yaml
# recipes/approval-level-1.yaml
recipe:
  metadata:
    name: "Level 1 Approval"
    description: "Level 1 approval workflow coordinator"
    custom_type: "level 1 approval"
    version: "1.0.0"

  parameters:
    approval_id: "approval-001"
    coordinator_type: "level1"

  wants:
    # Level 1 Coordinator with RecipeAgent auto-connection
    - type: level1_coordinator
      labels:
        role: coordinator
        category: approval-coordinator
        component: level1-coordinator
        approval_level: "1"
      params:
        approval_id: approval_id
        coordinator_type: coordinator_type
      recipeAgent: true  # Enables automatic connection to evidence + description
```

**Key Features:**
- **RecipeAgent Auto-Connection**: Coordinators with `recipeAgent: true` automatically connect to wants with matching `approval_id`
- **Shared Wants**: Evidence and description wants are reused by both approval levels
- **Custom Target Types**: Approval targets are registered as custom types from recipe scanning
- **Hierarchical Structure**: Level 1 approval can dynamically create Level 2 approval
- **Dynamic Want Creation**: Target wants create coordinator children at runtime
- **Memory Persistence**: All connections and states are preserved in memory dumps

**Run the example:**
```sh
make run-hierarchical-approval  # Uses config/config-hierarchical-approval.yaml
```

**View the connections in memory dump:**
After execution, check `memory/memory-0000-latest.yaml` to see the auto-generated `using` selectors:
```yaml
- metadata:
    name: want-level1_coordinator-1
    type: level1_coordinator
  spec:
    params:
      approval_id: approval-001
      coordinator_type: level1
    using:
      - role: evidence-provider
      - role: description-provider
```

### Example 3: Direct Configuration (Queue System)

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

## Execution Modes

MyWant supports multiple execution modes to fit different deployment scenarios:

### Server Mode
Run MyWant as a persistent server with HTTP API endpoints:

```bash
# Start server mode
make server

# Test server API
make test-server-api    # Comprehensive API testing with JSON output
make test-server-simple # Basic API testing without jq
```

The server mode enables:
- **Remote Management**: Control MyWant instances via HTTP API
- **Live Configuration**: Update configurations without restart
- **State Inspection**: Query want states and statistics via HTTP endpoints
- **Multi-Client Support**: Handle multiple concurrent requests

#### Server API Documentation

The server provides a complete REST API documented in OpenAPI 3.0.3 format. View the full specification:
- **OpenAPI Spec**: [`openapi.yaml`](openapi.yaml) - Complete API specification
- **Local Server**: http://localhost:8080/health (when server is running)

#### Key Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Server health check |
| POST | `/api/v1/wants` | Create and execute wants (auto-starts execution) |
| GET | `/api/v1/wants` | List all want executions |
| GET | `/api/v1/wants/{id}` | Get current runtime state (consistent with memory dumps) |
| PUT | `/api/v1/wants/{id}` | Update want configuration (if not running) |
| DELETE | `/api/v1/wants/{id}` | Delete want execution (if not running) |
| GET | `/api/v1/wants/{id}/status` | Get execution status |
| GET | `/api/v1/wants/{id}/results` | Get execution results (after completion) |

#### API Usage Examples

**Create and execute wants:**
```bash
# Upload YAML configuration (auto-starts execution)
curl -X POST http://localhost:8080/api/v1/wants \
  -H "Content-Type: application/yaml" \
  --data-binary @config/config-qnet-target.yaml
```

**Monitor runtime state:**
```bash
# Get live want states (same format as memory dumps)
curl -s http://localhost:8080/api/v1/wants/{id} | jq .
```

**Check execution status:**
```bash
curl -s http://localhost:8080/api/v1/wants/{id}/status
```

The API returns structured JSON responses with proper error handling and supports both YAML and JSON content types for configuration uploads.

### Offline Mode
Execute configurations in standalone batch mode:

```bash
# Run in offline mode (default for demos)
make run-travel-recipe    # Executes and exits
make run-qnet            # Processes data and terminates
```

Offline mode is ideal for:
- **Batch Processing**: One-time data processing tasks
- **Testing**: Development and validation scenarios
- **CI/CD Integration**: Automated pipeline execution
- **Resource Management**: Controlled execution lifecycle

### Choosing the Right Mode

| Use Case | Server Mode | Offline Mode |
|----------|-------------|--------------|
| Web Applications | ✓ | |
| Interactive Systems | ✓ | |
| Batch Processing | | ✓ |
| Development/Testing | | ✓ |
| Production Services | ✓ | |
| CI/CD Pipelines | | ✓ |

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

MyWant provides various examples demonstrating different processing patterns:

```sh
# Independent wants - parallel execution
make run-travel        # Travel planning with coordination

# Dependent wants - pipeline processing
make run-qnet          # Queue system (generator → queue → sink)
make run-qnet-using-recipe # Advanced QNet with multiple streams

# RecipeAgent auto-connection system
make run-hierarchical-approval # Hierarchical approval with auto-connection

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

### Configuration Principles
- **Explicit**: All want parameters are clearly defined
- **Traceable**: Easy to understand what each want does
- **Flexible**: Support for both direct wants and custom target types

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

1. **Create Config File**: Define wants in `config/config-*.yaml`
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