# Want Type Loader Implementation Plan

## Current Status

### Want Type Implementations Found (7 Files, 24+ Types)

**File: travel_types.go**
1. ✅ RestaurantWant → want_types/independent/restaurant.yaml
2. ✅ HotelWant → (MISSING)
3. ✅ BuffetWant → (MISSING)
4. ✅ FlightWant → (MISSING)
5. ✅ TravelCoordinatorWant → want_types/coordinators/travel_coordinator.yaml

**File: qnet_types.go**
6. ✅ Numbers → want_types/generators/numbers.yaml
7. ✅ Queue → want_types/processors/queue.yaml
8. ✅ Combiner → (MISSING)
9. ✅ Sink → want_types/sinks/sink.yaml
10. ✅ Collector → (MISSING)

**File: fibonacci_types.go**
11. ✅ FibonacciNumbers → (MISSING)
12. ✅ FibonacciSequence → (MISSING)
13. ✅ FibonacciAdder → (MISSING)

**File: fibonacci_loop_types.go**
14. ✅ FibonacciLoopWant → (MISSING)
15. ✅ FibonacciSourceLoopWant → (MISSING)
16. ✅ FibonacciAdderLoopWant → (MISSING)

**File: prime_types.go**
17. ✅ PrimeNumbers → (MISSING)
18. ✅ PrimeSieve → (MISSING)
19. ✅ PrimeSequence → (MISSING)

**File: approval_types.go**
20. ✅ EvidenceWant → (MISSING)
21. ✅ DescriptionWant → (MISSING)

**File: flight_types.go**
22. ✅ FlightWant → (MISSING) [Note: Different from travel_types.go FlightWant?]

**System Types:**
23. ✅ OwnerWant (owner_types.go) → (MISSING)
24. ✅ CustomTargetWant → (MISSING)

**Status Summary:**
- YAML Files Created: 5
- YAML Files Missing: 19
- Total Want Types: 24

---

## Architecture: Recipe Loader Pattern

### Current Recipe Loading (Proven Pattern)

**File:** `recipe_loader_generic.go`

```go
type GenericRecipeLoader struct {
    recipesDirectory string
    recipes          map[string]*GenericRecipeConfig
    customTypes      map[string]*metadata.CustomType
}

func (g *GenericRecipeLoader) LoadRecipe(name string) (*GenericRecipeConfig, error) {
    // Load YAML file
    // Validate against OpenAPI spec
    // Substitute parameters
    // Return config
}

func (g *GenericRecipeLoader) ListRecipes() map[string]*metadata.CustomType {
    // Return all available recipes
}
```

**At Startup (main.go, lines 150-160):**
```go
// Create loader
loader := recipe.NewGenericRecipeLoader("recipes")

// Load all recipes
loader.LoadRecipeFilesIntoRegistry()

// Register custom types
loader.ScanAndRegisterCustomTypes()

// Store in global state
server.recipeRegistry = loader
```

---

## Proposed: Want Type Loader Pattern

### Implementation Strategy

Mimic recipe loader pattern for want types:

```go
// want_type_loader.go
type WantTypeLoader struct {
    wantTypesDirectory string
    definitions        map[string]*WantTypeDefinition
    categories         map[string][]string  // category -> [type names]
    patterns           map[string][]string  // pattern -> [type names]
}

func (w *WantTypeLoader) LoadWantType(name string) (*WantTypeDefinition, error) {
    // Load YAML file from want_types/{pattern}/{name}.yaml
    // Parse and validate
    // Return definition
}

func (w *WantTypeLoader) LoadAllWantTypes() error {
    // Scan want_types/ directory
    // Load all *.yaml files
    // Build index by category and pattern
    // Return error if any file fails
}

func (w *WantTypeLoader) ListByCategory(category string) []*WantTypeDefinition {
    // Return all want types in category
}

func (w *WantTypeLoader) ListByPattern(pattern string) []*WantTypeDefinition {
    // Return all want types with pattern
}

func (w *WantTypeLoader) GetDefinition(name string) *WantTypeDefinition {
    // Get definition by name
}
```

---

## File Organization Plan

### Current Structure
```
want_types/
├── generators/
│   └── numbers.yaml ✅
├── processors/
│   └── queue.yaml ✅
├── independent/
│   └── restaurant.yaml ✅
├── coordinators/
│   └── travel_coordinator.yaml ✅
├── sinks/
│   └── sink.yaml ✅
└── templates/
    └── WANT_TYPE_TEMPLATE.yaml ✅
```

### Required Structure (After Implementation)

```
want_types/
├── generators/
│   ├── numbers.yaml ✅
│   ├── fibonacci_numbers.yaml
│   └── prime_numbers.yaml
│
├── processors/
│   ├── queue.yaml ✅
│   ├── combiner.yaml
│   ├── fibonacci_sequence.yaml
│   └── prime_sequence.yaml
│
├── sinks/
│   ├── sink.yaml ✅
│   └── prime_sieve.yaml
│
├── coordinators/
│   └── travel_coordinator.yaml ✅
│
├── independent/
│   ├── restaurant.yaml ✅
│   ├── hotel.yaml
│   ├── buffet.yaml
│   ├── flight.yaml
│   ├── evidence.yaml (approval domain)
│   └── description.yaml (approval domain)
│
├── system/
│   ├── owner.yaml
│   ├── custom_target.yaml
│   └── fibonacci_loop.yaml
│
├── templates/
│   ├── WANT_TYPE_TEMPLATE.yaml ✅
│   ├── generator_template.yaml
│   ├── processor_template.yaml
│   └── sink_template.yaml
│
└── README.md (index of all types)
```

---

## Implementation Phases

### Phase 1: WantTypeLoader Implementation (1-2 days)

**File to Create:** `engine/src/want_type_loader.go`

```go
type WantTypeDefinition struct {
    Metadata       WantTypeMetadata
    Parameters     []ParameterDef
    State          []StateDef
    Connectivity   ConnectivityDef
    Agents         []AgentDef
    Constraints    []ConstraintDef
    Examples       []ExampleDef
    RelatedTypes   []string
    SeeAlso        []string
}

type WantTypeMetadata struct {
    Name        string
    Title       string
    Description string
    Version     string
    Category    string
    Pattern     string
}

// Implement:
// - LoadWantType(name string) error
// - LoadAllWantTypes() error
// - GetDefinition(name string) *WantTypeDefinition
// - ListByCategory(cat string) []*WantTypeDefinition
// - ListByPattern(pat string) []*WantTypeDefinition
// - ValidateDefinition(def *WantTypeDefinition) error
```

**Tests to Write:**
- Load single YAML file
- Load all files from directory
- Validate definition structure
- Index by category and pattern
- Handle missing/invalid files

---

### Phase 2: Integration into Server Startup (1 day)

**File to Modify:** `engine/cmd/server/main.go`

**In NewServer() method (after recipe loader):**
```go
// Load want type definitions
wantTypeLoader := want.NewWantTypeLoader("want_types")
err := wantTypeLoader.LoadAllWantTypes()
if err != nil {
    log.Fatalf("Failed to load want type definitions: %v", err)
}

server.wantTypeLoader = wantTypeLoader
```

**In Start() method (after registering want types):**
```go
// Validate all registered want types have definitions
for _, wantTypeName := range getRegisteredWantTypeNames() {
    def := server.wantTypeLoader.GetDefinition(wantTypeName)
    if def == nil {
        log.Warnf("No definition found for want type: %s", wantTypeName)
    }
}
```

---

### Phase 3: API Endpoints (1 day)

**File to Modify:** `engine/cmd/server/main.go` (handlers section)

Add endpoints:
```go
// GET /api/v1/want-types
func (s *Server) ListWantTypes(w http.ResponseWriter, r *http.Request) {
    category := r.URL.Query().Get("category")
    pattern := r.URL.Query().Get("pattern")

    var defs []*WantTypeDefinition
    if category != "" {
        defs = s.wantTypeLoader.ListByCategory(category)
    } else if pattern != "" {
        defs = s.wantTypeLoader.ListByPattern(pattern)
    } else {
        defs = s.wantTypeLoader.GetAll()
    }

    json.NewEncoder(w).Encode(defs)
}

// GET /api/v1/want-types/{name}
func (s *Server) GetWantType(w http.ResponseWriter, r *http.Request) {
    name := chi.URLParam(r, "name")
    def := s.wantTypeLoader.GetDefinition(name)
    if def == nil {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    json.NewEncoder(w).Encode(def)
}

// GET /api/v1/want-types/{name}/examples
func (s *Server) GetWantTypeExamples(w http.ResponseWriter, r *http.Request) {
    name := chi.URLParam(r, "name")
    def := s.wantTypeLoader.GetDefinition(name)
    if def == nil {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    json.NewEncoder(w).Encode(map[string]interface{}{
        "name": name,
        "examples": def.Examples,
    })
}
```

---

### Phase 4: Create Missing YAML Files (3-5 days)

**Priority Order:**

**High Priority (Used in demos):**
1. hotel.yaml - travel domain
2. buffet.yaml - travel domain
3. fibonacci_numbers.yaml - math domain
4. fibonacci_sequence.yaml - math domain
5. prime_numbers.yaml - math domain

**Medium Priority (Used in examples):**
6. combiner.yaml - queue domain
7. prime_sieve.yaml - sink pattern
8. evidence.yaml - approval domain
9. description.yaml - approval domain

**Lower Priority (System/special):**
10. owner.yaml - system type
11. custom_target.yaml - system type
12. fibonacci_loop.yaml - special variant
13. fibonacci_source_loop.yaml - special variant
14. fibonacci_adder_loop.yaml - special variant

**For Each Type, Extract From Code:**
- Parameters used (from spec.Params access in Exec())
- State keys (from StoreState/GetState calls)
- Agent requirements (from agentRegistry access)
- Input/output connectivity
- Default values
- Example configurations

---

## Validation Strategy

### At Load Time
```go
// validate.go
func ValidateWantTypeDefinition(def *WantTypeDefinition) error {
    // Check metadata fields non-empty
    if def.Metadata.Name == "" {
        return fmt.Errorf("metadata.name is required")
    }

    // Check pattern is valid
    validPatterns := []string{"generator", "processor", "sink", "coordinator", "independent"}
    if !contains(validPatterns, def.Metadata.Pattern) {
        return fmt.Errorf("invalid pattern: %s", def.Metadata.Pattern)
    }

    // Check parameter definitions are valid
    for _, param := range def.Parameters {
        if param.Name == "" {
            return fmt.Errorf("parameter missing name")
        }
        if param.Type == "" {
            return fmt.Errorf("parameter %s missing type", param.Name)
        }
    }

    // Check state definitions
    for _, st := range def.State {
        if st.Name == "" {
            return fmt.Errorf("state key missing name")
        }
    }

    return nil
}
```

### At Deploy Time (When Creating Want)
```go
// In ChainBuilder.CreateWant()
def := server.wantTypeLoader.GetDefinition(metadata.Type)
if def == nil {
    return fmt.Errorf("unknown want type: %s", metadata.Type)
}

// Validate parameters
err := validateParameters(spec.Params, def.Parameters)
if err != nil {
    return fmt.Errorf("invalid parameters for %s: %v", metadata.Type, err)
}

// Apply defaults
applyDefaults(spec.Params, def.Parameters)

// Create want
factory := builder.GetFactory(metadata.Type)
return factory(metadata, spec)
```

---

## Integration Points

### 1. Server Initialization
- Load WantTypeLoader in NewServer()
- Store in Server struct
- Validate all types have definitions

### 2. API Routes
- Add routes for want type browsing
- Implement handlers
- Return definitions in responses

### 3. Want Creation Flow
- Check definition before creating
- Validate parameters
- Apply defaults
- Better error messages

### 4. Frontend Integration
- Fetch want types on load
- Show definitions in UI
- Validate parameters client-side
- Show examples

### 5. Config Loading
- Check definition when loading config
- Validate want types exist
- Validate parameters
- Better error reporting

---

## Data Structures

### Go Structs (to implement)

```go
// engine/src/want_type_loader.go

type WantTypeLoader struct {
    directory   string
    definitions map[string]*WantTypeDefinition
    byCategory  map[string][]*WantTypeDefinition
    byPattern   map[string][]*WantTypeDefinition
    mu          sync.RWMutex
}

type WantTypeDefinition struct {
    Metadata    WantTypeMetadata
    Parameters  []ParameterDef
    State       []StateDef
    Connectivity ConnectivityDef
    Agents      []AgentDef
    Constraints []ConstraintDef
    Examples    []ExampleDef
    RelatedTypes []string
    SeeAlso     []string
}

type WantTypeMetadata struct {
    Name        string
    Title       string
    Description string
    Version     string
    Category    string
    Pattern     string
}

type ParameterDef struct {
    Name        string
    Description string
    Type        string
    Default     interface{}
    Required    bool
    Validation  ValidationRules
    Example     interface{}
}

type ValidationRules struct {
    Min     *float64
    Max     *float64
    Pattern string
    Enum    []interface{}
}

type StateDef struct {
    Name        string
    Description string
    Type        string
    Persistent  bool
    Example     interface{}
}

type ConnectivityDef struct {
    Inputs  []ChannelDef
    Outputs []ChannelDef
}

type ChannelDef struct {
    Name        string
    Type        string // "want" | "agent" | "state" | "event"
    Description string
    Required    bool
    Multiple    bool
}

type AgentDef struct {
    Name        string
    Role        string // "monitor" | "action" | "validator" | "transformer"
    Description string
    Example     string
}

type ConstraintDef struct {
    Description string
    Validation  string
}

type ExampleDef struct {
    Name              string
    Description       string
    Params            map[string]interface{}
    ExpectedBehavior  string
    ConnectedTo       []string
}
```

---

## File Mapping (24 Want Types → YAML Files)

| Want Type | File | Category | Pattern | YAML | Status |
|-----------|------|----------|---------|------|--------|
| RestaurantWant | travel_types.go | travel | independent | restaurant.yaml | ✅ |
| HotelWant | travel_types.go | travel | independent | hotel.yaml | ❌ |
| BuffetWant | travel_types.go | travel | independent | buffet.yaml | ❌ |
| FlightWant | travel_types.go | travel | independent | flight.yaml | ❌ |
| TravelCoordinatorWant | travel_types.go | travel | coordinator | travel_coordinator.yaml | ✅ |
| Numbers | qnet_types.go | queue | generator | numbers.yaml | ✅ |
| Queue | qnet_types.go | queue | processor | queue.yaml | ✅ |
| Combiner | qnet_types.go | queue | processor | combiner.yaml | ❌ |
| Sink | qnet_types.go | queue | sink | sink.yaml | ✅ |
| Collector | qnet_types.go | queue | sink | collector.yaml | ❌ |
| FibonacciNumbers | fibonacci_types.go | math | generator | fibonacci_numbers.yaml | ❌ |
| FibonacciSequence | fibonacci_types.go | math | processor | fibonacci_sequence.yaml | ❌ |
| FibonacciAdder | fibonacci_types.go | math | processor | fibonacci_adder.yaml | ❌ |
| FibonacciLoopWant | fibonacci_loop_types.go | math | processor | fibonacci_loop.yaml | ❌ |
| FibonacciSourceLoopWant | fibonacci_loop_types.go | math | generator | fibonacci_source_loop.yaml | ❌ |
| FibonacciAdderLoopWant | fibonacci_loop_types.go | math | processor | fibonacci_adder_loop.yaml | ❌ |
| PrimeNumbers | prime_types.go | math | generator | prime_numbers.yaml | ❌ |
| PrimeSieve | prime_types.go | math | processor | prime_sieve.yaml | ❌ |
| PrimeSequence | prime_types.go | math | processor | prime_sequence.yaml | ❌ |
| EvidenceWant | approval_types.go | approval | independent | evidence.yaml | ❌ |
| DescriptionWant | approval_types.go | approval | independent | description.yaml | ❌ |
| FlightWant | flight_types.go | travel | independent | flight_alt.yaml | ❌ |
| OwnerWant | owner_types.go | system | independent | owner.yaml | ❌ |
| CustomTargetWant | - | system | independent | custom_target.yaml | ❌ |

---

## Success Criteria

### Phase 1 Complete When:
- ✅ WantTypeLoader implemented and tested
- ✅ All methods working (Load, List, Get, Validate)
- ✅ Handles all file types correctly
- ✅ Index building works

### Phase 2 Complete When:
- ✅ Server loads want type definitions at startup
- ✅ No errors on startup
- ✅ All 24 want types have definitions (or logged as warnings)
- ✅ Server field initialized correctly

### Phase 3 Complete When:
- ✅ API endpoints functional
- ✅ All endpoints return correct data
- ✅ Filtering works (by category, pattern)
- ✅ Error handling correct

### Phase 4 Complete When:
- ✅ All 19 missing YAML files created
- ✅ All files validate successfully
- ✅ Examples included in each
- ✅ Parameters match code implementation

---

## Timeline

**Total Estimated Time: 1-2 weeks**

- Phase 1 (Loader): 1-2 days
- Phase 2 (Integration): 1 day
- Phase 3 (API): 1 day
- Phase 4 (YAML files): 3-5 days
- Testing & Polish: 1-2 days

Can run Phase 1, 2, 3 in parallel (would reduce to 1 week)

---

## Risks & Mitigation

**Risk 1: Missing parameters in YAML**
- Mitigation: Extract from code first, review with team
- Mitigation: Add validation to catch mismatches

**Risk 2: YAML structure mismatch**
- Mitigation: Use strict schema validation
- Mitigation: Automated tests for each type

**Risk 3: Server startup delays**
- Mitigation: Load asynchronously if needed
- Mitigation: Cache parsed definitions

**Risk 4: Breaking existing code**
- Mitigation: YAML is additive, doesn't change Go code
- Mitigation: All existing tests continue to work

---

## Next Steps

1. **Review** this plan with team
2. **Approve** the approach and timeline
3. **Start Phase 1**: Implement WantTypeLoader
4. **Parallel**: Create missing YAML files (can start immediately)
5. **Phase 2**: Integrate into server startup
6. **Phase 3**: Add API endpoints
7. **Test**: Verify everything works
