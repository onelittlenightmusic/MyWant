# Want Type Server Loading - Implementation Summary

## Problem

You have **24 want types** across 7 Go files, but only **5 YAML definitions** created. The backend server needs to load all want type YAMLs at startup (similar to how recipes are loaded) and provide them via API for frontend discovery.

---

## Current State

### Want Types (24 Total)

| Domain | File | Count | Pattern | YAML Status |
|--------|------|-------|---------|-------------|
| **Travel** | travel_types.go | 5 | independent, coordinator | 2/5 ✅✅❌❌❌ |
| **Queue/Net** | qnet_types.go | 5 | generator, processor, sink | 3/5 ✅✅❌✅❌ |
| **Fibonacci** | fibonacci_types.go | 3 | generator, processor | 0/3 ❌❌❌ |
| **Fibonacci Loop** | fibonacci_loop_types.go | 3 | generator, processor | 0/3 ❌❌❌ |
| **Prime** | prime_types.go | 3 | generator, processor, sink | 0/3 ❌❌❌ |
| **Approval** | approval_types.go | 2 | independent | 0/2 ❌❌ |
| **Flight** | flight_types.go | 1 | independent | 0/1 ❌ |
| **System** | owner_types.go + monitor_types.go | 2 | independent, system | 0/2 ❌❌ |

**Total YAML Coverage: 5 / 24 = 21%**

---

## Solution Architecture

### Pattern: Recipe Loader Model

The system already has a proven pattern: `GenericRecipeLoader` loads all recipes from YAML at startup.

We apply the same pattern for want types:

```
Server Startup
    ↓
Load WantTypeDefinitions from want_types/
    ↓
Index by:
  - Category (travel, queue, math, approval, system)
  - Pattern (generator, processor, sink, coordinator, independent)
    ↓
Make available via API
    ↓
Used by:
  - Config loader (validation)
  - Frontend (form generation)
  - API (introspection)
```

---

## Implementation Phases

### Phase 1: Want Type Loader (1-2 days)
Create `engine/src/want_type_loader.go` with:
- Load YAML from `want_types/{pattern}/{name}.yaml`
- Parse and validate definitions
- Index by category and pattern
- Provide access methods (Get, List, etc.)

### Phase 2: Server Integration (1 day)
Modify `engine/cmd/server/main.go`:
- Initialize WantTypeLoader in NewServer()
- Load all definitions at startup
- Log warnings if any type lacks definition
- Store in Server struct

### Phase 3: API Endpoints (1 day)
Add REST endpoints:
- GET /api/v1/want-types
- GET /api/v1/want-types/{name}
- GET /api/v1/want-types/{name}/examples
- Filtering by category/pattern

### Phase 4: Missing YAML Files (3-5 days)
Extract from Go code and create:
- hotel.yaml, buffet.yaml, flight.yaml
- fibonacci_*.yaml, prime_*.yaml
- approval types, system types
- All with complete definitions

---

## Key Design Decisions

### 1. **Location: `want_types/` Directory**
```
want_types/
├── generators/        (no inputs)
├── processors/        (input → output)
├── sinks/             (input only)
├── coordinators/      (orchestrates)
├── independent/       (standalone)
├── system/            (special)
└── templates/         (for copying)
```

### 2. **Naming: By Want Type Name**
- `numbers.yaml` → Numbers want type
- `restaurant.yaml` → RestaurantWant
- `hotel.yaml` → HotelWant

### 3. **Structure: Metadata-First**
```yaml
wantType:
  metadata:           # Identity
    name: ...
    pattern: ...
  parameters: [...]   # Configuration
  state: [...]        # State keys
  connectivity: ...   # Input/output
  agents: [...]       # Agents
  examples: [...]     # Usage examples
```

### 4. **Loading: At Startup**
```go
// In main.go NewServer()
loader := want.NewWantTypeLoader("want_types")
err := loader.LoadAllWantTypes()
if err != nil {
    log.Fatal(err)  // Fail fast if YAML invalid
}
server.wantTypeLoader = loader
```

### 5. **Validation: Strict Schema**
- All metadata fields required
- Pattern must be valid (5 values)
- Parameters must have type and description
- State keys must have type

---

## File Mapping: 24 Want Types

### Travel Domain (5 types)
```
RestaurantWant         → restaurant.yaml ✅
HotelWant             → hotel.yaml (TODO)
BuffetWant            → buffet.yaml (TODO)
FlightWant            → flight.yaml (TODO)
TravelCoordinatorWant → travel_coordinator.yaml ✅
```

### Queue/Network Domain (5 types)
```
Numbers               → numbers.yaml ✅
Queue                → queue.yaml ✅
Combiner             → combiner.yaml (TODO)
Sink                 → sink.yaml ✅
Collector            → collector.yaml (TODO)
```

### Fibonacci Domain (3 types)
```
FibonacciNumbers      → fibonacci_numbers.yaml (TODO)
FibonacciSequence     → fibonacci_sequence.yaml (TODO)
FibonacciAdder        → fibonacci_adder.yaml (TODO)
```

### Fibonacci Loop Domain (3 types)
```
FibonacciLoopWant     → fibonacci_loop.yaml (TODO)
FibonacciSourceLoop   → fibonacci_source_loop.yaml (TODO)
FibonacciAdderLoop    → fibonacci_adder_loop.yaml (TODO)
```

### Prime Domain (3 types)
```
PrimeNumbers          → prime_numbers.yaml (TODO)
PrimeSieve            → prime_sieve.yaml (TODO)
PrimeSequence         → prime_sequence.yaml (TODO)
```

### Approval Domain (2 types)
```
EvidenceWant          → evidence.yaml (TODO)
DescriptionWant       → description.yaml (TODO)
```

### System Domain (2 types)
```
OwnerWant             → owner.yaml (TODO)
CustomTargetWant      → custom_target.yaml (TODO)
```

**Total Coverage Needed: 19 more files to reach 100%**

---

## Go Code Structure (To Implement)

### WantTypeLoader
```go
type WantTypeLoader struct {
    directory      string
    definitions    map[string]*WantTypeDefinition
    byCategory     map[string][]*WantTypeDefinition
    byPattern      map[string][]*WantTypeDefinition
    mu             sync.RWMutex
}

// Load all YAML files from directory
func (w *WantTypeLoader) LoadAllWantTypes() error

// Get definition by name
func (w *WantTypeLoader) GetDefinition(name string) *WantTypeDefinition

// List by category
func (w *WantTypeLoader) ListByCategory(cat string) []*WantTypeDefinition

// List by pattern
func (w *WantTypeLoader) ListByPattern(pat string) []*WantTypeDefinition

// Validate definition
func (w *WantTypeLoader) ValidateDefinition(def *WantTypeDefinition) error
```

### WantTypeDefinition
```go
type WantTypeDefinition struct {
    Metadata      WantTypeMetadata
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

---

## Server Integration Points

### 1. **Startup (main.go)**
```go
// After recipe loader
loader := want.NewWantTypeLoader("want_types")
err := loader.LoadAllWantTypes()
if err != nil {
    log.Fatalf("Failed to load want types: %v", err)
}
server.wantTypeLoader = loader
```

### 2. **Config Loading (chain_builder.go)**
```go
// Before creating want
def := server.wantTypeLoader.GetDefinition(metadata.Type)
if def == nil {
    return fmt.Errorf("unknown want type: %s", metadata.Type)
}

// Validate parameters
err := validateAgainstDef(spec.Params, def.Parameters)
if err != nil {
    return fmt.Errorf("invalid params for %s: %v", metadata.Type, err)
}
```

### 3. **API Endpoints (main.go)**
```go
// GET /api/v1/want-types
func (s *Server) ListWantTypes(w http.ResponseWriter, r *http.Request) {
    defs := s.wantTypeLoader.GetAll()  // or filter by category/pattern
    json.NewEncoder(w).Encode(defs)
}

// GET /api/v1/want-types/{name}
func (s *Server) GetWantType(w http.ResponseWriter, r *http.Request) {
    def := s.wantTypeLoader.GetDefinition(name)
    json.NewEncoder(w).Encode(def)
}
```

---

## API Design

### Endpoints (Same as Recipe Discovery)

```
GET /api/v1/want-types
  Response: [{name, title, pattern, category}, ...]
  Query params:
    ?category=travel
    ?pattern=generator
    ?search=restaurant

GET /api/v1/want-types/{name}
  Response: Complete WantTypeDefinition
  Usage: Get full definition for form generation

GET /api/v1/want-types/{name}/examples
  Response: {name, examples: [{...}, {...}]}
  Usage: Show usage examples in UI

POST /api/v1/want-types (Future)
  Request: YAML definition
  Usage: Dynamic type registration

PUT /api/v1/want-types/{name} (Future)
  Request: Updated definition
  Usage: Update type definition

DELETE /api/v1/want-types/{name} (Future)
  Usage: Remove type
```

---

## Frontend Integration

### What Changes
1. **Form Generation**: Frontend fetches want type def and generates param forms
2. **Parameter Hints**: Show descriptions, examples, validation rules
3. **Type Validation**: Client-side validation before submit
4. **Example Display**: Show example configurations
5. **Help Text**: Show purpose, related types, agent requirements

### Example Flow
```
User selects "restaurant" want type
    ↓
Frontend fetches GET /api/v1/want-types/restaurant
    ↓
Gets parameters definition:
  - restaurant_type (enum: fine dining, casual, buffet)
  - party_size (int, min: 1, max: 20)
  - preferred_time (string, pattern: HH:MM)
    ↓
Generates form with:
  - Enum dropdown for restaurant_type
  - Number input for party_size
  - Time input for preferred_time
  - Help text from definitions
    ↓
User fills form and submits
    ↓
Frontend validates against definition
    ↓
POST to create want with validated params
```

---

## Success Criteria

### When Implementation Complete

✅ **Phase 1**: WantTypeLoader loads all YAML files
✅ **Phase 2**: Server initializes loader at startup without errors
✅ **Phase 3**: API endpoints return want type definitions
✅ **Phase 4**: All 24 want types have YAML definitions
✅ **Testing**: All types load correctly, no validation errors
✅ **Frontend**: Can fetch and display want types
✅ **Documentation**: API documented, examples provided

---

## Timeline & Effort

| Phase | Task | Days | Effort |
|-------|------|------|--------|
| 1 | WantTypeLoader implementation | 1-2 | Medium |
| 2 | Server integration | 1 | Small |
| 3 | API endpoints | 1 | Small |
| 4 | Create YAML files | 3-5 | Large |
| - | Testing & polish | 1-2 | Medium |
| **TOTAL** | | **7-11 days** | **2-3 weeks** |

Can parallelize: Implement Phase 1 while creating Phase 4 files → **1 week total**

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Missing parameters in YAML | Extract from code first, code review |
| YAML structure mismatch | Strict schema validation, tests |
| Server startup delays | Load asynchronously if needed |
| Breaking existing code | YAML is additive, non-breaking |
| Missing definitions at deploy | Log warnings, provide fallback |

---

## Next Steps

1. **Review** this plan with team
2. **Approve** timeline and approach
3. **Start Phase 1** - Implement WantTypeLoader in Go
4. **Parallel start** - Begin creating missing YAML files
5. **Phase 2** - Integrate into server startup
6. **Phase 3** - Add API endpoints
7. **Validate** all files load and work correctly

---

## Related Documents

- **WANT_TYPE_LOADER_PLAN.md** - Detailed implementation plan with code examples
- **WANT_TYPE_DEFINITION.md** - YAML schema specification
- **SCHEMA_UPDATE_SUMMARY.md** - Layered metadata structure
- **README_WANT_TYPE_DESIGN.md** - Complete design overview
