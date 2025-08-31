# GoChain

A Go library implementing functional chain programming patterns with channels, supporting both imperative chain building and declarative configuration-based approaches for flexible stream processing and pipeline architectures.

## Features

- **Recipe-based Configuration**: Define complex processing topologies using YAML recipe files
- **Independent & Dependent Wants**: Support both parallel processing and sequential pipelines
- **Dynamic Want Addition**: Add wants to running chains at runtime
- **Memory Reconciliation**: Persistent state management across chain executions
- **Label-based Connectivity**: Flexible want connections using label selectors
- **Multi-stream Processing**: Support for parallel processing streams with combiners

## Core Concepts

### Config YAML - Top-Level User Interface
Config YAML files are the main interface for running GoChain programs. They specify what to execute and how:

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
- **Dependent**: Connected in processing pipelines using `using` selectors (e.g., data processing chains)

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

**Step 2: Create the config file** (top-level user interface in `config-travel-recipe.yaml`):
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
make run-travel-recipe  # Uses config-travel-recipe.yaml
```

### Example 2: Dependent Wants (Queue System Pipeline)

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

**Step 2: Create the config file** (top-level user interface in `config-queue-system-recipe.yaml`):
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
make run-queue-system-recipe  # Uses config-queue-system-recipe.yaml
```

## Available Make Targets

```sh
# Basic demos
make run-qnet          # QNet simulation
make run-prime         # Prime number sieve
make run-fibonacci     # Fibonacci sequence
make run-fibonacci-loop # Advanced fibonacci with feedback loop
make run-travel        # Travel planning (direct config)

# Recipe-based demos
make run-travel-recipe        # Travel planning from recipe
make run-queue-system-recipe  # Queue system from recipe
make run-qnet-recipe         # QNet with recipe parameters
make run-qnet-using-recipe   # QNet with YAML-defined connections

# Owner-based dynamic wants
make run-sample-owner    # Dynamic want creation with recipes
```

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
2. **Create Config File**: Reference recipe and set parameters in `config-*.yaml`
3. **Execute**: Run with `make` targets or `go run demo_*.go config-*.yaml`
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
- Configuration files follow `config-*-recipe.yaml` naming
- Demo programs follow `demo_*_recipe.go` naming