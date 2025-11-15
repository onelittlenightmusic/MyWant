# Want Type System - Visual Guide

## System Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         MyWant Application                       │
└─────────────────────────────────────────────────────────────────┘

                    ┌──────────────────────────┐
                    │   Config YAML Files      │
                    │  (config-*.yaml)         │
                    └──────────┬───────────────┘
                               │
                               ▼
                    ┌──────────────────────────┐
                    │   Config Loader          │
                    │  (declarative.go)        │
                    └──────────┬───────────────┘
                               │
                ┌──────────────┴──────────────┐
                ▼                             ▼
    ┌──────────────────────┐    ┌──────────────────────┐
    │ Want Type Registry   │    │  Recipe Loader       │
    │ (NEW COMPONENT)      │    │  (recipe_loader.go)  │
    │                      │    │                      │
    │ - Loads YAML defs    │    │ - Loads recipes      │
    │ - Validates params   │    │ - Expands parameters │
    │ - Provides API       │    │                      │
    └──────────┬───────────┘    └──────────┬───────────┘
               │                            │
               └────────────┬───────────────┘
                            ▼
                ┌──────────────────────────┐
                │   ChainBuilder           │
                │  (chain_builder.go)      │
                │                          │
                │ - Create wants           │
                │ - Validate parameters    │
                │ - Lookup factories       │
                │ - Connect wants          │
                └──────────┬───────────────┘
                           │
                ┌──────────┴──────────┐
                ▼                     ▼
    ┌─────────────────────┐  ┌─────────────────────┐
    │  Want Factories     │  │  WantTypeRegistry   │
    │  (registered in     │  │  Definitions        │
    │   Go code)          │  │  (YAML files)       │
    │                     │  │                     │
    │ - NewNumbersWant    │  │ - numbers.yaml      │
    │ - NewQueueWant      │  │ - queue.yaml        │
    │ - NewRestaurant     │  │ - restaurant.yaml   │
    │ - etc...            │  │ - etc...            │
    └──────────┬──────────┘  └─────────────────────┘
               │
               ▼
    ┌──────────────────────────┐
    │   Executing Wants        │
    │  (want.go - Exec method) │
    │                          │
    │ - StoreState()           │
    │ - GetState()             │
    │ - Agent integration      │
    └──────────────────────────┘
```

---

## Want Type YAML Structure (Simplified)

```yaml
wantType:
  # IDENTITY - What is this?
  name: "restaurant"
  title: "Restaurant Reservation"
  pattern: "independent"

  # PARAMETERS - What configuration?
  parameters:
    - name: "restaurant_type"
      type: "string"
      validation:
        enum: ["fine dining", "casual", "buffet"]

  # STATE - What data storage?
  state:
    - name: "reservation_id"
      type: "string"
      persistent: true

  # CONNECTIVITY - How connects?
  connectivity:
    inputs: []          # No inputs
    outputs: []         # No outputs
    pattern: "independent"

  # AGENTS - Who executes?
  agents:
    - name: "MonitorRestaurant"
      role: "monitor"

  # EXAMPLES - How use?
  examples:
    - name: "Fine dining"
      params:
        restaurant_type: "fine dining"
      expectedBehavior: "Book fine dining restaurant"
```

---

## 5 Architectural Patterns

```
1. GENERATOR (Creates Data)
   ┌──────────────────────────────┐
   │     Generator Want           │
   │  (Numbers, Fibonacci, Prime)  │
   └─────────────────────┬────────┘
                         │
                   ┌─────▼──────┐
                   │   Output   │ ──→ To other wants
                   └────────────┘

   Characteristics:
   • No inputs
   • Produces output stream
   • Initiates pipeline

   Examples: Numbers, FibonacciNumbers, PrimeNumbers

═══════════════════════════════════════════════════════════════════

2. PROCESSOR (Transforms Data)

   ┌───────────┐         ┌──────────────────┐         ┌────────────┐
   │  Input    │  ──→   │  Processor Want   │  ──→   │   Output   │
   │Generator  │        │  (Queue, Combiner)│        │ To next    │
   └───────────┘        └──────────────────┘        │ stage      │
                                                     └────────────┘

   Characteristics:
   • Has inputs (required)
   • Has outputs
   • Middle stage in pipeline

   Examples: Queue, Combiner, FibonacciSequence, PrimeSequence

═══════════════════════════════════════════════════════════════════

3. SINK (Consumes Data - Terminal Node)

   ┌───────────┐         ┌──────────────────┐
   │  Input    │  ──→   │  Sink Want       │
   │ Processor │        │  (Sink, etc)      │
   └───────────┘        └──────────────────┘
                        │
                        ├─→ Statistics
                        ├─→ Logs
                        └─→ Aggregation

   Characteristics:
   • Has inputs (required)
   • No outputs to other wants
   • Generates reports/aggregation
   • Terminal node

   Examples: Sink, PrimeSink

═══════════════════════════════════════════════════════════════════

4. COORDINATOR (Orchestrates Independent Wants)

   ┌──────────────┐
   │ Independent  │──┐
   │ Want 1       │  │
   └──────────────┘  │
                     ├──→ ┌──────────────────┐
   ┌──────────────┐  │    │ Coordinator Want │
   │ Independent  │──┤    │ (TravelCoord)    │
   │ Want 2       │  │    └────────┬─────────┘
   └──────────────┘  │             │
                     ├──→    ┌──────▼──────┐
   ┌──────────────┐  │       │  Itinerary  │
   │ Independent  │──┘       │ (Output)    │
   │ Want 3       │          └─────────────┘
   └──────────────┘

   Characteristics:
   • Multiple independent inputs
   • No sequential dependencies
   • Collects/merges results
   • Produces unified output

   Examples: TravelCoordinator

═══════════════════════════════════════════════════════════════════

5. INDEPENDENT (Standalone Execution)

   ┌──────────────────────────────┐
   │   Independent Want           │
   │  (Restaurant, Hotel, etc)    │
   │                              │
   │ • No input wants             │
   │ • No output to wants         │
   │ • Self-contained execution   │
   │ • May use agents             │
   └──────────────────────────────┘

   Characteristics:
   • No input/output connections
   • Self-contained execution
   • Often have agents
   • Store results in state

   Examples: Restaurant, Hotel, Buffet, Flight, CustomTarget
```

---

## Parameter Validation Flow

```
User Config (YAML)
    │
    ▼
┌────────────────────────┐
│  Want Type Lookup      │
│  Get definition        │
│  (want_types/*.yaml)   │
└────────┬───────────────┘
         │
         ▼
┌────────────────────────┐
│  Type Checking         │
│  Verify param types    │
│  int vs string, etc    │
└────────┬───────────────┘
         │
         ├─ Type Error ──→ Return error with expected type
         │
         ▼
┌────────────────────────┐
│  Range Validation      │
│  Check min/max         │
│  Check enum values     │
└────────┬───────────────┘
         │
         ├─ Range Error ──→ Return error with valid range
         │
         ▼
┌────────────────────────┐
│  Pattern Validation    │
│  Check regex patterns  │
│  for string formats    │
└────────┬───────────────┘
         │
         ├─ Pattern Error ──→ Return error with pattern
         │
         ▼
┌────────────────────────┐
│  Apply Defaults        │
│  Fill missing params   │
│  with default values   │
└────────┬───────────────┘
         │
         ▼
┌────────────────────────┐
│  ✅ Create Want       │
│  Parameters valid      │
│  Ready for execution   │
└────────────────────────┘

Error Messages Include:
• What went wrong
• What was expected
• Example values
• Reference to want type definition
```

---

## Data Flow: Config to Execution

```
Step 1: Load Configuration
┌─────────────────────────────────┐
│ config-travel-recipe.yaml       │
│                                 │
│ wants:                          │
│ - type: restaurant              │ ◄── References want type
│   params:                       │
│     restaurant_type: "fine..."  │
│     party_size: 4               │
└─────────────────────────────────┘
         │
         ▼
Step 2: Lookup Want Type Definition
┌─────────────────────────────────┐
│ want_types/independent/          │
│ restaurant.yaml                 │
│                                 │
│ wantType:                       │
│   name: "restaurant"            │
│   parameters:                   │
│     - name: "restaurant_type"   │
│       type: "string"            │
│       validation:               │
│         enum: [...]             │
└─────────────────────────────────┘
         │
         ▼
Step 3: Validate Parameters
┌─────────────────────────────────┐
│ restaurant_type: "fine dining" │
│   ✓ Type: string (OK)          │
│   ✓ Enum check (OK)            │
│                                 │
│ party_size: 4                  │
│   ✓ Type: int (OK)             │
│   ✓ Range: 1-20 (OK)           │
└─────────────────────────────────┘
         │
         ▼
Step 4: Apply Defaults & Create Want
┌─────────────────────────────────┐
│ Metadata:                       │
│   type: "restaurant"            │
│ Spec:                           │
│   params:                       │
│     restaurant_type: "fine...". │
│     party_size: 4               │
│     preferred_time: "19:00"  ◄──┤ (default applied)
│     cuisine: "international" ◄──┤ (default applied)
│     budget: "moderate" ◄────────┤ (default applied)
└─────────────────────────────────┘
         │
         ▼
Step 5: Create and Connect Want
┌─────────────────────────────────┐
│ RestaurantWant Instance         │
│                                 │
│ StoreState("reservation_id", ...) │
│ GetState("restaurant_name")      │
│ ConnectTo(travel_coordinator)    │
└─────────────────────────────────┘
         │
         ▼
Step 6: Execute
┌─────────────────────────────────┐
│ for want.Exec() {               │
│   get params from spec          │
│   run agents if needed          │
│   store state                   │
│   send output                   │
│ }                               │
└─────────────────────────────────┘
```

---

## API Endpoints Overview

```
┌─────────────────────────────────────────────────────────────┐
│              Want Type API Endpoints                         │
└─────────────────────────────────────────────────────────────┘

GET /api/v1/want-types
├─ Description: List all want types
├─ Response: [{name, title, pattern, category}, ...]
└─ Optional filters:
   ├─ ?category=travel
   ├─ ?pattern=independent
   └─ ?search=restaurant

GET /api/v1/want-types/{name}
├─ Description: Get complete definition
├─ Path: /api/v1/want-types/restaurant
├─ Response:
│  ├─ name: "restaurant"
│  ├─ parameters: [...]
│  ├─ state: [...]
│  ├─ connectivity: {...}
│  ├─ agents: [...]
│  ├─ examples: [...]
│  └─ relatedTypes: [...]
└─ Use: Building forms, showing help

GET /api/v1/want-types/{name}/examples
├─ Description: Get usage examples
├─ Path: /api/v1/want-types/restaurant/examples
├─ Response:
│  ├─ examples[0]:
│  │  ├─ name: "Fine dining"
│  │  ├─ params: {...}
│  │  ├─ expectedBehavior: "..."
│  │  └─ connectedTo: ["travel_coordinator"]
│  └─ examples[1]: ...
└─ Use: Showing example configs

POST /api/v1/want-types (Future)
├─ Description: Register new want type
├─ Body: YAML want type definition
├─ Response: 201 Created
└─ Use: Dynamic registration

PUT /api/v1/want-types/{name} (Future)
├─ Description: Update definition
└─ Use: Version management

DELETE /api/v1/want-types/{name} (Future)
├─ Description: Unregister type
└─ Use: Cleanup
```

---

## Implementation Phases Timeline

```
Phase 1: Foundation (2-3 days)
├─ WantTypeRegistry struct
├─ YAML parser
└─ Startup loader
   │
   └─→ ✅ Can load YAML definitions

Phase 2: Validation (2-3 days)
├─ Parameter validator
├─ Type checker
├─ Range/enum/pattern validation
└─ Default value applicator
   │
   └─→ ✅ Can validate before creation

Phase 3: API (1-2 days)
├─ GET /api/v1/want-types
├─ GET /api/v1/want-types/{name}
├─ GET /api/v1/want-types/{name}/examples
└─ Tests and docs
   │
   └─→ ✅ API available

Phase 4: Frontend (2-3 days)
├─ Fetch want types
├─ Show definitions
├─ Parameter forms
└─ Client validation
   │
   └─→ ✅ UI shows want types

Phase 5: Convert Types (3-5 days)
├─ Create YAML for all 16+ types
├─ Extract examples from code
└─ Validate against implementations
   │
   └─→ ✅ All types have definitions

Phase 6: Polish (2-3 days)
├─ Integration testing
├─ Performance tuning
├─ Documentation
└─ Bug fixes
   │
   └─→ ✅ Production ready

Total: 2-3 weeks (can run phases in parallel)
```

---

## File Organization After Implementation

```
want_types/
│
├── generators/               (No inputs, produce output)
│   ├── numbers.yaml         ✅ Created
│   ├── fibonacci_numbers.yaml
│   ├── prime_numbers.yaml
│   └── README.md
│
├── processors/              (Have inputs and outputs)
│   ├── queue.yaml          ✅ Created
│   ├── combiner.yaml
│   ├── fibonacci_sequence.yaml
│   ├── prime_sequence.yaml
│   └── README.md
│
├── sinks/                   (Have inputs, no outputs)
│   ├── sink.yaml           ✅ Created
│   ├── prime_sink.yaml
│   └── README.md
│
├── coordinators/            (Orchestrate independent wants)
│   ├── travel_coordinator.yaml ✅ Created
│   └── README.md
│
├── independent/             (No connections)
│   ├── restaurant.yaml      ✅ Created
│   ├── hotel.yaml
│   ├── buffet.yaml
│   ├── flight.yaml
│   └── README.md
│
├── templates/               (Copy these to create new types)
│   ├── WANT_TYPE_TEMPLATE.yaml ✅ Created
│   ├── generator_template.yaml
│   ├── processor_template.yaml
│   ├── sink_template.yaml
│   ├── coordinator_template.yaml
│   └── independent_template.yaml
│
├── INDEX.md                 (Directory navigation)
└── README.md                (Want types overview)
```

---

## Key Differences: Go vs YAML Definition

```
┌──────────────────────────────────────────────────────────────┐
│                  BEFORE (Go Implementation)                    │
└──────────────────────────────────────────────────────────────┘

File: qnet_types.go (350+ lines)

type NumbersWant struct {
    Want
    start int
    count int
}

func NewNumbersWant(metadata, spec) interface{} {
    w := &NumbersWant{...}
    w.start = spec.Params["start"].(int)    // Unsafe type assertion
    w.count = spec.Params["count"].(int)    // Could panic
    return w
}

Issues:
• Type assertions can panic
• No validation before creation
• Parameters documented in comments (if at all)
• State keys scattered across Exec() method
• Hard to discover what parameters a want takes

┌──────────────────────────────────────────────────────────────┐
│                  AFTER (YAML Definition)                       │
└──────────────────────────────────────────────────────────────┘

File: want_types/generators/numbers.yaml (70 lines)

wantType:
  name: "numbers"
  parameters:
    - name: "start"
      type: "int"
      default: 0
      validation:
        min: 0
    - name: "count"
      type: "int"
      validation:
        min: 1

  state:
    - name: "current"
      type: "int"
      persistent: true

  examples:
    - name: "Generate first 100"
      params:
        start: 0
        count: 100

Benefits:
• Type-safe (pre-validated before creation)
• Self-documenting (readable YAML)
• API queryable (frontend can fetch definitions)
• Enables form generation
• Better error messages with examples
• Discoverable (what does numbers want need?)
```

---

## State Access Pattern

```
BEFORE (Direct type assertion - Unsafe)
│
├─ Code: val := want.GetState("current").(int)
├─ Problem: Don't know if key exists or type
├─ Risk: Runtime panic if wrong type
└─ Discovery: Have to read source code

AFTER (YAML Definition - Type Safe)
│
├─ Definition: state:
│             - name: "current"
│               type: "int"
│               persistent: true
│
├─ Code: val, exists := want.GetState("current")
├─ Framework: Guarantees type from definition
├─ Safety: Can't panic, type is known
└─ Discovery: Get /api/v1/want-types/numbers
             shows exactly what state keys exist
```

---

**Diagram Version**: 1.0
**Visual Guide Complete**: All patterns, flows, and architectures illustrated
