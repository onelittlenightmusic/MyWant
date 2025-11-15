# Want Type Definition System - YAML Schema Design

## Overview

This document defines a YAML-based system for defining want types declaratively, similar to how recipes define reusable want templates. This allows want type definitions (metadata, parameters, agents, examples) to be stored as YAML files instead of hardcoded in Go code.

**Motivation**:
- Want types become first-class citizens like recipes
- Dynamic registration/unregistration of want types at runtime
- Reduced boilerplate in Go code (only constructors needed)
- Configuration-driven architecture throughout the stack

---

## 1. WANT TYPE YAML SCHEMA

### 1.1 High-Level Structure

```yaml
wantType:
  # Metadata Layer - Identity and Classification
  metadata:
    name: string                  # Unique want type identifier (e.g., "restaurant")
    title: string                 # Human-readable title (e.g., "Restaurant Reservation")
    description: string           # What this want does
    version: string               # Version of this want type (e.g., "1.0")
    category: string              # Category for grouping (e.g., "travel", "queue", "math")
    pattern: string               # Type of want: "generator" | "processor" | "sink" | "coordinator" | "independent"

  # Parameter Definition
  parameters:
    - name: string                # Parameter name (used in want configs)
      description: string         # What this parameter does
      type: string                # Go type: "int", "float64", "string", "bool", "[]string", "map[string]interface{}"
      default: <varies>           # Default value if not provided
      required: boolean            # Whether parameter is mandatory
      validation:
        min: <number>             # For numeric types
        max: <number>
        pattern: string           # Regex pattern for string validation
        enum: [list]              # Allowed values
      example: <varies>           # Example value

  # State Definition
  state:
    - name: string                # State key name
      description: string         # Purpose of this state
      type: string                # Go type (same as parameters)
      persistent: boolean         # Whether state survives execution cycles
      example: <varies>

  # Input/Output Connectivity
  connectivity:
    inputs:
      - name: string              # Input channel name
        type: string              # "want" | "agent" | "external"
        description: string
        required: boolean
    outputs:
      - name: string              # Output channel name
        type: string              # "want" | "agent" | "state" | "event"
        description: string

  # Agent Requirements
  agents:
    - name: string                # Agent type this want requires/supports
      role: string                # "monitor" | "action" | "validator" | "transformer"
      description: string
      example: string             # Example agent reference (e.g., "AgentRestaurant")

  # Validation and Constraints
  constraints:
    - description: string         # Human-readable constraint description
      validation: string          # Validation rule in pseudo-code or expression language

  # Examples and Use Cases
  examples:
    - name: string                # Example name (e.g., "Basic Restaurant Reservation")
      description: string
      params:                      # Example parameter values
        <param_name>: <value>
      expectedBehavior: string     # What happens when want executes
      connectedTo: [string]        # Example: ["hotel", "buffet"] if part of travel coordinator

  # Related Types
  relatedTypes: [string]          # Other want types this works with
  seeAlso: [string]               # Documentation links or related recipes
```

### 1.2 Pattern Classifications

Want types fall into 5 architectural patterns:

#### Generator Pattern
Produces data on output channels, no input dependencies:
```yaml
pattern: "generator"
connectivity:
  inputs: []
  outputs:
    - name: "items"
      type: "want"
      description: "Produces items for downstream processing"
```
**Examples**: Numbers, Sequence, FibonacciNumbers, PrimeNumbers

#### Processor Pattern
Consumes input, performs transformation, produces output:
```yaml
pattern: "processor"
connectivity:
  inputs:
    - name: "input"
      type: "want"
      required: true
  outputs:
    - name: "output"
      type: "want"
```
**Examples**: Queue, Combiner, FibonacciSequence, PrimeSequence

#### Sink Pattern
Terminal processing node, no output to wants:
```yaml
pattern: "sink"
connectivity:
  inputs:
    - name: "input"
      type: "want"
      required: true
  outputs: []
```
**Examples**: Sink, PrimeSink

#### Coordinator Pattern
Independent wants with orchestration (special processor):
```yaml
pattern: "coordinator"
connectivity:
  inputs:
    - name: "reservations"
      type: "want"
      required: false
      multiple: true
  outputs:
    - name: "itinerary"
      type: "state"
```
**Examples**: TravelCoordinator

#### Independent Pattern
Standalone execution, no input/output connectivity:
```yaml
pattern: "independent"
connectivity:
  inputs: []
  outputs: []
```
**Examples**: RestaurantWant, HotelWant, FlightWant (when used individually)

---

## 2. WANT TYPE DEFINITION EXAMPLES

### 2.1 Generator Example: Numbers Want

```yaml
wantType:
  metadata:
    name: "numbers"
    title: "Number Generator"
    description: "Generates a sequence of integers from start to count"
    version: "1.0"
    category: "math"
    pattern: "generator"

  parameters:
    - name: "start"
      description: "Starting number"
      type: "int"
      default: 0
      required: false
      example: 0

    - name: "count"
      description: "How many numbers to generate"
      type: "int"
      default: 10
      required: false
      example: 100
      validation:
        min: 1

  state:
    - name: "current"
      description: "Current number being generated"
      type: "int"
      persistent: true
      example: 42

    - name: "generated_count"
      description: "Total numbers generated so far"
      type: "int"
      persistent: true
      example: 50

  connectivity:
    inputs: []
    outputs:
      - name: "numbers"
        type: "want"
        description: "Stream of generated numbers"

  agents: []

  constraints:
    - description: "Count must be positive"
      validation: "count > 0"

  examples:
    - name: "Generate first 100 numbers"
      description: "Creates a generator for numbers 0-99"
      params:
        start: 0
        count: 100
      expectedBehavior: "Outputs integers 0 through 99 on the numbers output"
      connectedTo: ["queue"]

  relatedTypes: ["fibonacci_numbers", "prime_numbers"]
  seeAlso: ["queue-system recipe"]
```

### 2.2 Processor Example: Queue Want

```yaml
wantType:
  metadata:
    name: "queue"
    title: "Queue Processor"
    description: "FIFO queue with configurable service time"
    version: "1.0"
    category: "queue"
    pattern: "processor"

  parameters:
    - name: "service_time"
      description: "Time to process each item (seconds)"
      type: "float64"
      default: 0.1
      required: false
      validation:
        min: 0.01
        max: 3600
      example: 0.1

    - name: "max_queue_size"
      description: "Maximum items in queue (-1 = unlimited)"
      type: "int"
      default: -1
      required: false
      example: 1000

  state:
    - name: "queued_items"
      description: "Current number of items waiting"
      type: "int"
      persistent: true

    - name: "processed_count"
      description: "Total items processed"
      type: "int"
      persistent: true

    - name: "current_item"
      description: "Item currently being processed"
      type: "interface{}"
      persistent: false

  connectivity:
    inputs:
      - name: "input"
        type: "want"
        description: "Items to queue"
        required: true
    outputs:
      - name: "output"
        type: "want"
        description: "Processed items"

  agents: []

  constraints:
    - description: "Service time must be positive"
      validation: "service_time > 0"

  examples:
    - name: "Basic queue processing"
      description: "Queue with 0.1s service time"
      params:
        service_time: 0.1
      expectedBehavior: "Items flow through queue, each taking 0.1s to process"
      connectedTo: ["numbers"]

  relatedTypes: ["numbers", "combiner", "sink"]
  seeAlso: ["queue-system recipe"]
```

### 2.3 Independent Pattern Example: Restaurant Want

```yaml
wantType:
  metadata:
    name: "restaurant"
    title: "Restaurant Reservation"
    description: "Finds and books a restaurant reservation"
    version: "1.0"
    category: "travel"
    pattern: "independent"

  parameters:
    - name: "restaurant_type"
      description: "Type of restaurant (fine dining, casual, buffet)"
      type: "string"
      default: "fine dining"
      required: false
      validation:
        enum: ["fine dining", "casual", "buffet", "fast food"]
      example: "fine dining"

    - name: "party_size"
      description: "Number of people"
      type: "int"
      default: 2
      required: false
      validation:
        min: 1
        max: 20

    - name: "preferred_time"
      description: "Preferred booking time (HH:MM format)"
      type: "string"
      default: "19:00"
      required: false
      validation:
        pattern: "^([0-1][0-9]|2[0-3]):[0-5][0-9]$"

  state:
    - name: "reservation_status"
      description: "Current reservation status"
      type: "string"
      persistent: true
      example: "confirmed"

    - name: "restaurant_name"
      description: "Name of reserved restaurant"
      type: "string"
      persistent: true
      example: "The French Laundry"

    - name: "booking_time"
      description: "Confirmed booking time"
      type: "string"
      persistent: true

    - name: "reservation_id"
      description: "Unique reservation identifier"
      type: "string"
      persistent: true

  connectivity:
    inputs: []
    outputs:
      - name: "schedule"
        type: "state"
        description: "Restaurant schedule and availability"

  agents:
    - name: "MonitorRestaurant"
      role: "monitor"
      description: "Loads existing restaurant data from YAML files"
      example: "agent_result field contains RestaurantSchedule"

    - name: "AgentRestaurant"
      role: "action"
      description: "Finds and books restaurant if no existing reservation"
      example: "Called after MonitorRestaurant"

  constraints:
    - description: "Party size within restaurant capacity"
      validation: "party_size >= 1 and party_size <= 20"

    - description: "Preferred time within service hours"
      validation: "preferred_time between 06:00 and 23:00"

  examples:
    - name: "Fine dining reservation"
      description: "Books upscale restaurant for 4 people"
      params:
        restaurant_type: "fine dining"
        party_size: 4
        preferred_time: "19:30"
      expectedBehavior: "Checks existing data, if none found books restaurant via agent"
      connectedTo: ["travel_coordinator"]

    - name: "Casual family dinner"
      description: "Books casual restaurant for 6 people"
      params:
        restaurant_type: "casual"
        party_size: 6
      expectedBehavior: "Quick booking for accessible restaurant"
      connectedTo: ["hotel", "buffet"]

  relatedTypes: ["hotel", "buffet", "flight"]
  seeAlso: ["travel-itinerary recipe", "travel domain agents"]
```

### 2.4 Coordinator Example: Travel Coordinator

```yaml
wantType:
  metadata:
    name: "travel_coordinator"
    title: "Travel Itinerary Coordinator"
    description: "Orchestrates multiple independent travel wants (restaurant, hotel, buffet)"
    version: "1.0"
    category: "travel"
    pattern: "coordinator"

  parameters:
    - name: "display_name"
      description: "Name of the travel itinerary"
      type: "string"
      default: "My Trip"
      required: false
      example: "European Vacation 2024"

    - name: "include_summary"
      description: "Whether to show summary statistics"
      type: "bool"
      default: true
      required: false

  state:
    - name: "itinerary"
      description: "Complete travel itinerary"
      type: "map[string]interface{}"
      persistent: true

    - name: "coordination_status"
      description: "Overall status of all reservations"
      type: "string"
      persistent: true
      example: "all_confirmed"

    - name: "completion_time"
      description: "When all reservations were completed"
      type: "string"
      persistent: true

  connectivity:
    inputs:
      - name: "reservations"
        type: "want"
        description: "Individual reservations (restaurant, hotel, buffet)"
        required: false
        multiple: true
    outputs:
      - name: "itinerary"
        type: "state"
        description: "Complete coordinated itinerary"

  agents: []

  constraints:
    - description: "At least one input want should be connected"
      validation: "len(using) > 0"

  examples:
    - name: "Full European itinerary"
      description: "Coordinates restaurant, hotel, and buffet reservations"
      params:
        display_name: "Paris Trip"
        include_summary: true
      expectedBehavior: "Collects all reservation data and presents unified itinerary"
      connectedTo: ["restaurant", "hotel", "buffet", "flight"]

  relatedTypes: ["restaurant", "hotel", "buffet", "flight"]
  seeAlso: ["travel-itinerary recipe"]
```

---

## 3. WANT TYPE REGISTRY AND MANAGEMENT

### 3.1 Registry Structure

Want types are stored in a dedicated directory:
```
want_types/
├── generators/
│   ├── numbers.yaml
│   ├── fibonacci_numbers.yaml
│   └── prime_numbers.yaml
├── processors/
│   ├── queue.yaml
│   ├── combiner.yaml
│   ├── fibonacci_sequence.yaml
│   └── prime_sequence.yaml
├── sinks/
│   ├── sink.yaml
│   └── prime_sink.yaml
├── coordinators/
│   └── travel_coordinator.yaml
└── independent/
    ├── restaurant.yaml
    ├── hotel.yaml
    ├── buffet.yaml
    └── flight.yaml
```

### 3.2 Want Type Registry Manager

```go
// In-memory registry loaded from YAML files
type WantTypeRegistry struct {
    types      map[string]*WantTypeDefinition  // name -> definition
    factories  map[string]WantFactory          // name -> constructor function
    mu         sync.RWMutex
}

type WantTypeDefinition struct {
    Name          string
    Title         string
    Description   string
    Version       string
    Category      string
    Pattern       string
    Parameters    []ParameterDef
    State         []StateDef
    Connectivity  ConnectivityDef
    Agents        []AgentDef
    Constraints   []ConstraintDef
    Examples      []ExampleDef
    RelatedTypes  []string
    SeeAlso       []string
}
```

### 3.3 API Endpoints for Want Type Management

```
GET    /api/v1/want-types                    # List all want types
GET    /api/v1/want-types/{name}             # Get want type definition
POST   /api/v1/want-types                    # Register new want type (YAML upload)
PUT    /api/v1/want-types/{name}             # Update want type definition
DELETE /api/v1/want-types/{name}             # Unregister want type
GET    /api/v1/want-types/{name}/examples   # Get want type examples
GET    /api/v1/want-types?category={cat}    # Filter by category
GET    /api/v1/want-types?pattern={pattern} # Filter by pattern
```

### 3.4 Dynamic Want Type Registration

At startup, all YAML files in `want_types/` are loaded and registered:

```go
func LoadWantTypesFromYAML(directory string) error {
    files, _ := filepath.Glob(filepath.Join(directory, "**/*.yaml"))
    for _, file := range files {
        definition, _ := parseWantTypeYAML(file)
        registry.Register(definition)
    }
    return nil
}
```

---

## 4. PARAMETER AND STATE BINDING

### 4.1 Parameter Resolution

In want config files, parameters reference want type definitions:

```yaml
# config-travel-recipe.yaml
config:
  wants:
    - type: "restaurant"              # References restaurant.yaml want type
      name: "restaurant_reservation"
      params:
        restaurant_type: "fine dining" # Must match parameter def in restaurant.yaml
        party_size: 4
        preferred_time: "19:30"
```

Validation during config loading:
1. Check that all required parameters are provided
2. Validate types match definitions
3. Apply default values from want type definitions
4. Validate ranges, enums, patterns

### 4.2 State Access Pattern

After want execution:

```go
want.GetState("restaurant_name")       // Get state from want type definition
want.GetState("reservation_id")        // Type: string (from state definition)
want.GetState("booking_time")          // Type: string
```

---

## 5. BACKWARD COMPATIBILITY

### 5.1 Gradual Migration Path

**Phase 1 (Current)**: Want types defined in Go code
```go
// qnet_types.go
func NewNumbersWant(metadata Metadata, spec WantSpec) interface{} { ... }
```

**Phase 2**: Parallel YAML definitions
```yaml
# want_types/generators/numbers.yaml
# Same definitions, referenced but not enforced
```

**Phase 3**: Pure YAML-driven (optional Go constructors)
```go
// Go code becomes thin wrappers around YAML
// Uses generic constructor pattern
```

**Phase 4**: Complete replacement
```yaml
# All want type info in YAML
# Go only provides base Want struct and generic execution
```

### 5.2 Compatibility Strategy

- Keep existing Go constructors functional
- New registrations prefer YAML definitions
- Validation uses both sources (union of constraints)
- Help CLI to assist migration from Go to YAML

---

## 6. IMPLEMENTATION ROADMAP

### Phase 1: Foundation
- [ ] Create YAML schema definition
- [ ] Implement WantTypeRegistry struct
- [ ] Parse YAML files into WantTypeDefinition
- [ ] Load want types at startup

### Phase 2: Validation
- [ ] Type checking for parameters during config load
- [ ] Constraint validation before want creation
- [ ] Default value injection
- [ ] Error messages with references to want type definitions

### Phase 3: API
- [ ] GET endpoints for browsing want types
- [ ] POST/PUT/DELETE for dynamic registration
- [ ] Integration with existing want creation flow

### Phase 4: Examples and Documentation
- [ ] Convert existing want types to YAML (16+ types)
- [ ] Create template files for each pattern
- [ ] Documentation generation from YAML

### Phase 5: CLI Tools
- [ ] `mywant want-type list` - List available want types
- [ ] `mywant want-type show {name}` - Display want type definition
- [ ] `mywant want-type validate {yaml}` - Validate YAML definition
- [ ] `mywant want-type generate {template}` - Generate YAML from template

---

## 7. COMPARISON WITH RECIPES

### Similarities
- Both are YAML-based definitions
- Both support parameters with defaults
- Both enable reusability without hardcoding

### Differences

| Aspect | Recipe | Want Type |
|--------|--------|-----------|
| **Purpose** | Define reusable want combinations | Define individual want type contract |
| **Scope** | Multiple wants working together | Single want type definition |
| **Parameters** | Instance parameters (count: 100) | Parameter definitions with types/validation |
| **State** | Not defined; emergent from wants | Explicitly defined state keys |
| **Connectivity** | Explicit `using` selectors | Pattern-based expectations |
| **Validation** | Loose (parameters passed as-is) | Strict (types, ranges, enums) |
| **Agents** | Optional per want | May be required per type |
| **Examples** | Use cases (what to do) | Parameter examples (how to use) |

---

## 8. EXAMPLE: CONVERTING GO WANT TYPE TO YAML

### Before (Go)
```go
// qnet_types.go
type NumbersWant struct {
    Want
    start int
    count int
}

func NewNumbersWant(metadata Metadata, spec WantSpec) interface{} {
    w := &NumbersWant{Want: NewWant(metadata, spec)}
    w.ConnectivityMetadata = ConnectivityMetadata{
        InputLabel:  []string{},
        OutputLabel: []string{"numbers"},
    }
    start := spec.Params["start"].(int)  // Type unsafe
    count := spec.Params["count"].(int)  // Could panic
    w.start = start
    w.count = count
    return w
}
```

### After (YAML)
```yaml
# want_types/generators/numbers.yaml
wantType:
  name: "numbers"
  title: "Number Generator"
  description: "Generates integers from start to count"
  version: "1.0"
  pattern: "generator"

  parameters:
    - name: "start"
      type: "int"
      default: 0
      validation:
        min: 0

    - name: "count"
      type: "int"
      required: true
      validation:
        min: 1

  connectivity:
    inputs: []
    outputs:
      - name: "numbers"
        type: "want"
```

Go code becomes just the constructor (framework handles validation):
```go
func NewNumbersWant(metadata Metadata, spec WantSpec) interface{} {
    // Framework validates parameters against want_types/generators/numbers.yaml
    // No manual type assertion needed
    w := &NumbersWant{Want: NewWant(metadata, spec)}
    w.start = int(spec.Params["start"].(float64))   // Framework guaranteed this is valid
    w.count = int(spec.Params["count"].(float64))
    return w
}
```

---

## 9. FUTURE ENHANCEMENTS

- **Dynamic Code Generation**: Generate Go constructors from YAML
- **Type Plugins**: Load want type libraries as plugins
- **Telemetry**: Auto-track parameter usage across want type instances
- **Versioning**: Support multiple versions of same want type (numbers v1.0, v2.0)
- **GraphQL Schema**: Auto-generate GraphQL schema from want type definitions
- **OpenAPI Integration**: Include want types in OpenAPI specification
- **Performance Profiles**: Define recommended parameter ranges per environment
