# MyWant: Want Type Definition System - Complete Analysis

## 1. CORE TYPE HIERARCHY

### 1.1 Metadata (Want Identification)
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/want.go:21`

```go
type Metadata struct {
    ID              string            `json:"id,omitempty" yaml:"id,omitempty"`
    Name            string            `json:"name" yaml:"name"`
    Type            string            `json:"type" yaml:"type"`           // Want type name
    Labels          map[string]string `json:"labels" yaml:"labels"`       // Label selectors
    OwnerReferences []OwnerReference  `json:"ownerReferences,omitempty" yaml:"ownerReferences,omitempty"`
}
```

**Purpose**: Identification and classification of wants
- **Name**: Unique identifier for the want instance
- **Type**: References a registered want type (e.g., "restaurant", "hotel", "fibonacci_numbers")
- **Labels**: Used for want-to-want connections via `using` selectors
- **OwnerReferences**: Parent-child relationships for lifecycle management

### 1.2 WantSpec (Desire Configuration)
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/want.go:30`

```go
type WantSpec struct {
    Params              map[string]interface{} `json:"params" yaml:"params"`
    Using               []map[string]string    `json:"using,omitempty" yaml:"using,omitempty"`
    StateSubscriptions  []StateSubscription    `json:"stateSubscriptions,omitempty" yaml:"stateSubscriptions,omitempty"`
    NotificationFilters []NotificationFilter   `json:"notificationFilters,omitempty" yaml:"notificationFilters,omitempty"`
    Requires            []string               `json:"requires,omitempty" yaml:"requires,omitempty"`
}
```

**Purpose**: Configuration for want execution
- **Params**: Key-value parameters passed to want during creation and execution
- **Using**: Array of label selector maps for connecting to input wants
- **Requires**: Agent capability requirements (e.g., ["flight_reservation"])

### 1.3 Want (Base Type)
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/want.go:70`

```go
type Want struct {
    Metadata Metadata               `json:"metadata" yaml:"metadata"`
    Spec     WantSpec               `json:"spec" yaml:"spec"`
    Status   WantStatus             `json:"status,omitempty" yaml:"status,omitempty"`
    State    map[string]interface{} `json:"state,omitempty" yaml:"state,omitempty"`
    History  WantHistory            `json:"history" yaml:"history"`
    
    // Internal fields for state management
    pendingStateChanges     map[string]interface{} `json:"-" yaml:"-"`
    pendingParameterChanges map[string]interface{} `json:"-" yaml:"-"`
    inExecCycle             bool                   `json:"-" yaml:"-"`
    execCycleCount          int                    `json:"-" yaml:"-"`
    
    // Agent system
    agentRegistry     *AgentRegistry                `json:"-" yaml:"-"`
    runningAgents     map[string]context.CancelFunc `json:"-" yaml:"-"`
    
    // Type-specific fields (set by constructors)
    WantType             string               `json:"-" yaml:"-"`
    ConnectivityMetadata ConnectivityMetadata `json:"-" yaml:"-"`
}
```

**Key Features**:
- **Embedded metadata + spec** for identity and configuration
- **State map** for persistent key-value storage across execution cycles
- **History** for tracking parameter and state changes
- **Batching mechanism** via pendingStateChanges for efficient state updates
- **Agent support** via agentRegistry for autonomous execution

---

## 2. WANT TYPE FACTORY PATTERN

### 2.1 WantFactory Interface
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/want.go:67`

```go
type WantFactory func(metadata Metadata, spec WantSpec) interface{}
```

**Signature Analysis**:
- **Input**: Metadata + WantSpec (complete configuration)
- **Output**: interface{} (must be cast to concrete want type)
- **Purpose**: Factory function for creating want instances from configuration

### 2.2 Type Assertion Pattern
All want constructors return `interface{}` which requires type assertion:

```go
restaurant := NewRestaurantWant(metadata, spec).(*RestaurantWant)
```

---

## 3. WANT TYPE DEFINITION PATTERNS

### 3.1 Travel Domain Example: RestaurantWant
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/travel_types.go:35`

**Structure**:
```go
type RestaurantWant struct {
    Want                      // Embedded base type
    RestaurantType string     // Domain-specific field
    Duration       time.Duration // Domain-specific field
    paths          Paths      // Connection info
}
```

**Constructor Pattern**:
```go
func NewRestaurantWant(metadata Metadata, spec WantSpec) interface{} {
    restaurant := &RestaurantWant{
        Want:           Want{},
        RestaurantType: "casual",
        Duration:       2 * time.Hour,
    }
    
    // Initialize base Want fields
    restaurant.Init(metadata, spec)
    
    // Extract parameters from spec and populate domain-specific fields
    if rt, ok := spec.Params["restaurant_type"]; ok {
        if rts, ok := rt.(string); ok {
            restaurant.RestaurantType = rts
        }
    }
    
    // Set connectivity metadata (required by ChainBuilder)
    restaurant.WantType = "restaurant"
    restaurant.ConnectivityMetadata = ConnectivityMetadata{
        RequiredInputs:  0,
        RequiredOutputs: 1,
        MaxInputs:       1,
        MaxOutputs:      1,
        WantType:        "restaurant",
        Description:     "Restaurant reservation scheduling want",
    }
    
    return restaurant
}
```

**Key Patterns**:
1. Create instance with base Want embedded
2. Call `Init(metadata, spec)` to initialize base fields
3. Extract parameters from `spec.Params` map with type assertions
4. Set `WantType` and `ConnectivityMetadata` for the type system
5. Return as `interface{}`

### 3.2 Number Processing Example: Numbers (QNet)
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/qnet_types.go:62`

**Highlights Different Pattern**:
```go
type Numbers struct {
    mywant.Want
    Rate                float64
    Count               int
    batchUpdateInterval int     // Batch mechanism
    cycleCount          int     // State tracking
    currentTime         float64 // Simulation state
    currentCount        int     // Simulation state
}

func PacketNumbers(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
    gen := &Numbers{
        Want:                mywant.Want{},
        Rate:                1.0,
        Count:               100,
        batchUpdateInterval: 100,
        cycleCount:          0,
    }
    
    // Similar initialization...
    gen.Init(metadata, spec)
    
    // Handle numeric parameters with fallback to float64
    if c, ok := spec.Params["count"]; ok {
        if ci, ok := c.(int); ok {
            gen.Count = ci
        } else if cf, ok := c.(float64); ok {
            gen.Count = int(cf)
        }
    }
    
    return gen
}
```

**Key Pattern Difference**:
- Handles both `int` and `float64` from YAML (YAML parsers default to float64)
- Uses domain-specific persistent fields for simulation state
- Implements batching mechanism for state history optimization

### 3.3 Fibonacci Example: FibonacciNumbers
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/fibonacci_types.go:14`

**Simple Pattern**:
```go
type FibonacciNumbers struct {
    Want
    Count int
    paths Paths
}

func NewFibonacciNumbers(metadata Metadata, spec WantSpec) interface{} {
    gen := &FibonacciNumbers{
        Want:  Want{},
        Count: 20,
    }
    
    gen.Init(metadata, spec)
    
    if c, ok := spec.Params["count"]; ok {
        if ci, ok := c.(int); ok {
            gen.Count = ci
        } else if cf, ok := c.(float64); ok {
            gen.Count = int(cf)
        }
    }
    
    return gen
}
```

---

## 4. WANT TYPE REGISTRATION SYSTEM

### 4.1 ChainBuilder Registry
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/chain_builder.go:36-133`

```go
type ChainBuilder struct {
    registry map[string]WantFactory    // Want type factories
    // ... other fields
}

func (cb *ChainBuilder) RegisterWantType(wantType string, factory WantFactory) {
    cb.registry[wantType] = factory
}
```

**Registration Process**:
1. Create ChainBuilder instance
2. Call `RegisterWantType(typeName, factory)` for each type
3. Type system uses string key to look up factory function
4. Factory called during want instantiation

### 4.2 Travel Registration Example
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/travel_types.go:989`

```go
func RegisterTravelWantTypes(builder *ChainBuilder) {
    builder.RegisterWantType("flight", NewFlightWant)
    builder.RegisterWantType("restaurant", NewRestaurantWant)
    builder.RegisterWantType("hotel", NewHotelWant)
    builder.RegisterWantType("buffet", NewBuffetWant)
    builder.RegisterWantType("travel_coordinator", NewTravelCoordinatorWant)
}

// Variant with Agent Support
func RegisterTravelWantTypesWithAgents(builder *ChainBuilder, agentRegistry *AgentRegistry) {
    builder.RegisterWantType("restaurant", func(metadata Metadata, spec WantSpec) interface{} {
        restaurant := NewRestaurantWant(metadata, spec).(*RestaurantWant)
        restaurant.SetAgentRegistry(agentRegistry)  // Enable agent execution
        return restaurant
    })
    // ... other types with agent support
}
```

**Key Points**:
- Register string type name to factory function
- Can wrap factories to add agent support
- Each want type gets its own registration call

### 4.3 QNet Registration
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/qnet_types.go:526`

```go
func RegisterQNetWantTypes(builder *mywant.ChainBuilder) {
    builder.RegisterWantType("qnet numbers", PacketNumbers)
    builder.RegisterWantType("qnet queue", NewQueue)
    builder.RegisterWantType("qnet combiner", NewCombiner)
}
```

---

## 5. PARAMETER EXTRACTION PATTERN

### 5.1 Standard Parameter Types

**Boolean Parameters**:
```go
if det, ok := spec.Params["deterministic"]; ok {
    if detBool, ok := det.(bool); ok {
        useDeterministic = detBool
    } else if detStr, ok := det.(string); ok {
        useDeterministic = (detStr == "true")
    }
}
```

**Numeric Parameters**:
```go
// Handle both int and float64 from YAML
if c, ok := spec.Params["count"]; ok {
    if ci, ok := c.(int); ok {
        paramCount = ci
    } else if cf, ok := c.(float64); ok {
        paramCount = int(cf)
    }
}
```

**String Parameters**:
```go
if rt, ok := spec.Params["restaurant_type"]; ok {
    if rts, ok := rt.(string); ok {
        restaurant.RestaurantType = rts
    }
}
```

**Float Parameters**:
```go
if r, ok := spec.Params["rate"]; ok {
    if rf, ok := r.(float64); ok {
        gen.Rate = rf
    }
}
```

### 5.2 Two-Phase Parameter Reading

**Phase 1: Initialization** (in constructor):
```go
func NewRestaurantWant(metadata Metadata, spec WantSpec) interface{} {
    restaurant := &RestaurantWant{Want: Want{}, RestaurantType: "casual"}
    restaurant.Init(metadata, spec)
    
    // Read parameters during construction for initialization
    if rt, ok := spec.Params["restaurant_type"]; ok {
        if rts, ok := rt.(string); ok {
            restaurant.RestaurantType = rts
        }
    }
    return restaurant
}
```

**Phase 2: Execution** (in Exec method):
```go
func (r *RestaurantWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Read parameters FRESH each cycle - enables dynamic changes!
    restaurantType := "casual"
    if rt, ok := r.Spec.Params["restaurant_type"]; ok {
        if rts, ok := rt.(string); ok {
            restaurantType = rts
        }
    }
    // Use restaurantType in execution...
}
```

**Benefit**: Parameters can be updated dynamically between execution cycles

---

## 6. EXECUTION INTERFACES

### 6.1 Progressable Interface
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/declarative.go:116`

```go
type Progressable interface {
    Exec(using []chain.Chan, outputs []chain.Chan) bool
    GetWant() *Want
}
```

**Implementation Pattern**:
```go
func (r *RestaurantWant) GetWant() *Want {
    return &r.Want
}

func (r *RestaurantWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Main execution logic
}
```

### 6.2 Packet Handler Interface
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/declarative.go:101`

```go
type PacketHandler interface {
    OnEnded(packet Packet) error
}
```

**Example Implementation**:
```go
func (q *Queue) OnEnded(packet mywant.Packet) error {
    // Store final state
    q.StoreState("average_wait_time", avgWaitTime)
    q.StoreState("total_processed", q.processedCount)
    return nil
}
```

---

## 7. CONNECTIVITY METADATA SYSTEM

### 7.1 ConnectivityMetadata Type
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/declarative.go:193`

```go
type ConnectivityMetadata struct {
    RequiredInputs  int    // Minimum inputs required
    RequiredOutputs int    // Minimum outputs required
    MaxInputs       int    // -1 for unlimited
    MaxOutputs      int    // -1 for unlimited
    WantType        string // Type name for logging
    Description     string // Human-readable description
}
```

### 7.2 Implementation Examples

**Independent Want (Restaurant)**:
```go
restaurant.ConnectivityMetadata = ConnectivityMetadata{
    RequiredInputs:  0,
    RequiredOutputs: 1,
    MaxInputs:       1,
    MaxOutputs:      1,
    WantType:        "restaurant",
    Description:     "Restaurant reservation scheduling want",
}
```

**Pipeline Want (Queue)**:
```go
queue.ConnectivityMetadata = mywant.ConnectivityMetadata{
    RequiredInputs:  1,          // Must have input
    RequiredOutputs: 1,          // Must have output
    MaxInputs:       1,          // Only one input allowed
    MaxOutputs:      -1,         // Unlimited outputs
    WantType:        "queue",
    Description:     "Queue processing want",
}
```

**Hub Want (Travel Coordinator)**:
```go
coordinator.ConnectivityMetadata = ConnectivityMetadata{
    RequiredInputs:  3,          // Needs 3 inputs
    RequiredOutputs: 0,          // No outputs needed
    MaxInputs:       3,          // Exactly 3 inputs
    MaxOutputs:      0,          // No outputs
    WantType:        "travel_coordinator",
    Description:     "Travel itinerary coordinator want",
}
```

---

## 8. STATE MANAGEMENT PATTERN

### 8.1 State Storage
```go
func (r *RestaurantWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Store state using thread-safe method
    r.StoreState("attempted", true)
    r.StoreState("total_processed", 1)
    r.StoreState("reservation_type", restaurantType)
    r.StoreState("reservation_start_time", newEvent.Start.Format("15:04"))
    // ... more state storage
    return true
}
```

### 8.2 State Retrieval
```go
attemptedVal, _ := r.GetState("attempted")
attempted, _ := attemptedVal.(bool)

if attempted {
    return true
}
```

### 8.3 Batch State Changes (Exec Cycle)
```go
func (t *TravelCoordinatorWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Begin batching state changes
    {
        t.BeginProgressCycle()
        t.StoreState("schedules", schedules)
        t.EndProgressCycle()
    }
    // ... more logic ...
}
```

---

## 9. YAML CONFIGURATION STRUCTURE

### 9.1 Config File Format
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/yaml/config/config-travel-recipe.yaml`

```yaml
wants:
  - metadata:
      name: travel planner
      type: travel itinerary planner          # References registered type
      labels:
        role: travel-planner
    spec:
      params:
        prefix: vacation
        display_name: "One Day Travel Itinerary"
        restaurant_type: "fine dining"
        hotel_type: "luxury"
        buffet_type: "international"
        dinner_duration: 2.0
```

### 9.2 Recipe File Format
**File**: `/Users/hiroyukiosaki/work/golang/MyWant/yaml/recipes/travel-itinerary.yaml`

```yaml
recipe:
  metadata:
    name: "Travel Itinerary"
    description: "Travel planning system with coordination"
    custom_type: "travel itinerary planner"
    version: "1.0.0"
    
  parameters:
    prefix: "travel"
    display_name: "One Day Travel Itinerary"
    restaurant_type: "fine dining"
    # ... other parameters
  
  wants:
    - metadata:
        type: restaurant               # References want type
        labels:
          role: scheduler
      spec:
        params:
          restaurant_type: restaurant_type  # Parameter reference
          duration_hours: dinner_duration
    
    - metadata:
        type: travel_coordinator
      spec:
        params:
          display_name: display_name
        using:                         # Label-based connection
          - role: scheduler
```

---

## 10. COMMON WANT TYPE PATTERNS

### Pattern 1: Generator/Source
**Characteristics**:
- No inputs (`RequiredInputs: 0`)
- Produces output (`RequiredOutputs: 1`)
- Generates data into channels
- Examples: Numbers, PrimeNumbers, FibonacciNumbers

### Pattern 2: Processor/Filter
**Characteristics**:
- Requires input (`RequiredInputs: 1`)
- Produces output (`RequiredOutputs: 1`)
- Processes and transforms data
- Examples: Queue, PrimeSequence, FibonacciSequence

### Pattern 3: Sink/Collector
**Characteristics**:
- Requires input (`RequiredInputs: 1`)
- No outputs (`RequiredOutputs: 0`)
- Terminal node
- Examples: Collector

### Pattern 4: Coordinator/Hub
**Characteristics**:
- Multiple inputs (`RequiredInputs: 3+`)
- No outputs (`RequiredOutputs: 0`)
- Aggregates multiple sources
- Examples: TravelCoordinator

### Pattern 5: Independent/Parallel
**Characteristics**:
- No inputs (`RequiredInputs: 0`)
- Produces output (`RequiredOutputs: 1`)
- Executes in parallel with others
- Coordinated by hub want
- Examples: RestaurantWant, HotelWant, BuffetWant

---

## 11. WANT CONSTRUCTOR SIGNATURE STANDARDIZATION

### Recent Standardization (Commit 5df1758)
All want constructors now use consistent signature:

```go
func NewWantType(metadata Metadata, spec WantSpec) interface{} {
    want := &WantType{Want: Want{}}
    want.Init(metadata, spec)
    // Extract parameters
    // Set WantType and ConnectivityMetadata
    return want
}
```

**Before**: Mixed signatures with different parameter orders and return types
**After**: Unified (Metadata, WantSpec) -> interface{} signature

---

## 12. KEY METHODS IN WANT BASE TYPE

### State Management
- `StoreState(key string, value interface{})` - Thread-safe state storage
- `GetState(key string) (interface{}, bool)` - Thread-safe state retrieval
- `GetAllState() map[string]interface{}` - Get entire state map

### Execution Lifecycle
- `BeginProgressCycle()` - Start batching state changes
- `EndProgressCycle()` - Commit batched changes
- `AggregateChanges()` - Merge pending changes into state

### Initialization
- `Init(metadata Metadata, spec WantSpec)` - Initialize base fields

### Parameter Management
- `GetParameter(paramName string) (interface{}, bool)` - Get parameter value
- `UpdateParameter(paramName string, paramValue interface{})` - Update parameter

### Status Management
- `SetStatus(status WantStatus)` - Update status
- `GetStatus() WantStatus` - Get current status

---

## 13. CRITICAL DESIGN PRINCIPLES

### 1. Embedding Pattern
All want types embed the `Want` base type:
```go
type RestaurantWant struct {
    Want  // Embedded base - inherits all methods
    // ... specific fields
}
```

### 2. Initialization Order
```go
func NewRestaurantWant(metadata Metadata, spec WantSpec) interface{} {
    restaurant := &RestaurantWant{Want: Want{}}  // Create with empty base
    restaurant.Init(metadata, spec)               // Initialize base
    // Extract and set specific fields
    restaurant.WantType = "restaurant"            // Set type
    restaurant.ConnectivityMetadata = ...         // Set connectivity
    return restaurant
}
```

### 3. Parameter Reading Flexibility
- Extract parameters in constructor for initialization defaults
- Re-read parameters in Exec() for dynamic updates
- Allows runtime parameter changes without requiring want restart

### 4. Type Safety via Assertions
```go
// Factory returns interface{}
want := factory(metadata, spec)

// Cast to concrete type
restaurant := want.(*RestaurantWant)

// Access type-specific fields
restaurant.RestaurantType = "fine dining"
```

### 5. Connectivity Validation
ConnectivityMetadata enables:
- Validation of want connections at construction time
- Clear documentation of want interface requirements
- Automatic path generation based on labels

### 6. State Isolation
- Private state field in Want struct
- Access via StoreState/GetState methods
- Mutex protection for concurrent access
- Enables safe parallel execution

---

## 14. EXTENDING THE SYSTEM: ADDING NEW WANT TYPES

### Step 1: Define Type
```go
type MyCustomWant struct {
    Want
    CustomField1 string
    CustomField2 int
    paths        Paths
}
```

### Step 2: Create Constructor
```go
func NewMyCustomWant(metadata Metadata, spec WantSpec) interface{} {
    custom := &MyCustomWant{
        Want:         Want{},
        CustomField1: "default",
        CustomField2: 0,
    }
    
    custom.Init(metadata, spec)
    
    // Extract parameters
    if field1, ok := spec.Params["custom_field_1"]; ok {
        if f1Str, ok := field1.(string); ok {
            custom.CustomField1 = f1Str
        }
    }
    
    // Set metadata
    custom.WantType = "my_custom_type"
    custom.ConnectivityMetadata = ConnectivityMetadata{
        RequiredInputs:  1,
        RequiredOutputs: 1,
        MaxInputs:       1,
        MaxOutputs:      -1,
        WantType:        "my_custom_type",
        Description:     "My custom want type",
    }
    
    return custom
}
```

### Step 3: Implement Exec
```go
func (m *MyCustomWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Read parameters fresh
    field1 := "default"
    if f1, ok := m.Spec.Params["custom_field_1"]; ok {
        if f1Str, ok := f1.(string); ok {
            field1 = f1Str
        }
    }
    
    // Main logic
    // Store state
    m.StoreState("key", value)
    
    return true/false
}
```

### Step 4: Implement GetWant
```go
func (m *MyCustomWant) GetWant() *Want {
    return &m.Want
}
```

### Step 5: Register Type
```go
func RegisterMyCustomTypes(builder *ChainBuilder) {
    builder.RegisterWantType("my_custom_type", NewMyCustomWant)
}
```

### Step 6: Use in Configuration
```yaml
wants:
  - metadata:
      name: my_instance
      type: my_custom_type
    spec:
      params:
        custom_field_1: "value"
```

---

## 15. WANT TYPE FILES IN CODEBASE

**Travel Domain**:
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/travel_types.go`
  - RestaurantWant, HotelWant, BuffetWant, FlightWant, TravelCoordinatorWant

**Queue/Network Domain**:
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/qnet_types.go`
  - Numbers (generator), Queue (processor), Combiner

**Mathematical Domain**:
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/fibonacci_types.go`
  - FibonacciNumbers, FibonacciSequence
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/prime_types.go`
  - PrimeNumbers, PrimeSequence

**System Domain**:
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/owner_types.go`
  - OwnerWant (dynamic want creation from recipes)
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/custom_target_types.go`
  - CustomTargetWant (dynamic recipe-based types)

---

## SUMMARY

The want type definition system is built on:

1. **Core Types**: Metadata, WantSpec, Want (base class)
2. **Factory Pattern**: String → Factory function → interface{} instance
3. **Constructor Convention**: (Metadata, WantSpec) -> interface{}
4. **Embedding**: All types embed Want base type
5. **Registration**: Types registered with ChainBuilder via RegisterWantType()
6. **Configuration**: YAML files specify wants with type, params, labels, using
7. **Connectivity**: Label-based want-to-want connections via using selectors
8. **State**: Thread-safe state storage with differential tracking and history
9. **Execution**: Exec() method receives input/output channels
10. **Parameters**: Extracted once in constructor, re-read in Exec for dynamism

This design enables:
- Easy extension with new want types
- Type-safe execution without losing flexibility
- Runtime parameter changes
- Complex multi-want architectures
- Declarative YAML-based configuration
