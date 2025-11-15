# Want Type YAML Definition System - Executive Summary

## Vision

Want types become **first-class citizens** in MyWant, defined declaratively in YAML files rather than hardcoded in Go. This mirrors the recipe system and enables:

- 🎯 **Configuration-driven architecture** throughout the stack
- 📝 **Dynamic registration/unregistration** at runtime
- 🔍 **Self-documenting API** with built-in help and validation
- 🚀 **Faster prototyping** - add new types without writing Go code
- 🔧 **Backward compatible** - gradual migration path

---

## What Has Been Designed

### 1. **WANT_TYPE_DEFINITION.md** (Core Schema)

Complete YAML schema for want type definitions including:

**Core Sections**:
- **Identity**: name, title, description, version, category
- **Parameters**: Full type definitions with validation (min/max/enum/pattern)
- **State**: Keys, types, and persistence flags
- **Connectivity**: Input/output patterns (generator/processor/sink/coordinator/independent)
- **Agents**: Agent support and requirements
- **Constraints**: Business logic validation rules
- **Examples**: Realistic usage scenarios with parameters
- **Relations**: Links to related types and documentation

**Architectural Patterns Defined**:
- **Generator**: No inputs, produces output (Numbers, FibonacciNumbers, PrimeNumbers)
- **Processor**: Consumes input, transforms, produces output (Queue, Combiner, Fibonacci Sequence)
- **Sink**: Terminal node, consumes input, no output (Sink, PrimeSink)
- **Coordinator**: Orchestrates independent wants (TravelCoordinator)
- **Independent**: Standalone execution (Restaurant, Hotel, Buffet, Flight)

### 2. **Example YAML Files** (4 Representative Types)

Created complete YAML definitions for:

**generators/numbers.yaml** - Simple generator pattern
- Parameters: start, count
- State: current, generated_count, completion_time
- Pattern: generator (no inputs)

**processors/queue.yaml** - Complex processor pattern
- Parameters: service_time, max_queue_size, priority_mode
- State: queued_items, processed_count, discarded_count, avg_wait_time
- Demonstrates state tracking and metrics

**independent/restaurant.yaml** - Comprehensive independent pattern
- Parameters: restaurant_type, party_size, preferred_time, cuisine, budget
- State: 9 state keys for complete booking lifecycle
- Agents: MonitorRestaurant, AgentRestaurant
- 4 realistic usage examples

**coordinators/travel_coordinator.yaml** - Coordinator orchestration
- Parameters: display_name, output_format, timezone, include_costs
- State: Complete itinerary, status, costs, summaries
- Demonstrates multi-want coordination

**sinks/sink.yaml** - Terminal processing
- Parameters: operation (count/sum/average/min/max/collect)
- State: Aggregation results, throughput metrics
- 5 operation modes with different behaviors

### 3. **System Architecture** (Registry & API)

**WantTypeRegistry**:
```go
type WantTypeRegistry struct {
    types      map[string]*WantTypeDefinition  // name -> definition
    factories  map[string]WantFactory          // name -> constructor
    mu         sync.RWMutex
}
```

**API Endpoints Designed**:
```
GET    /api/v1/want-types                    # List all (with filters)
GET    /api/v1/want-types/{name}             # Get definition
GET    /api/v1/want-types/{name}/examples   # Get examples
POST   /api/v1/want-types                    # Register (dynamic)
PUT    /api/v1/want-types/{name}             # Update
DELETE /api/v1/want-types/{name}             # Unregister
```

**Directory Structure**:
```
want_types/
├── generators/        (Numbers, Fibonacci, Primes)
├── processors/        (Queue, Combiner, Fibonacci/Prime Sequence)
├── sinks/             (Sink, PrimeSink)
├── coordinators/      (TravelCoordinator)
└── independent/       (Restaurant, Hotel, Buffet, Flight)
```

### 4. **Integration Points** (Parameter Validation & Binding)

**At Config Load Time**:
1. Look up want type definition
2. Validate parameters (types, ranges, enums)
3. Apply defaults for missing parameters
4. Provide helpful error messages with examples

**At Runtime**:
1. Framework-guaranteed valid parameters
2. Type assertion safe (validation pre-done)
3. Default values auto-applied
4. Constraints enforced

**State Access**:
```go
want.GetState("restaurant_name")  // Type from definition
want.GetState("reservation_id")   // Persistent flag checked
want.GetState("avg_wait_time")    // Type-safe access
```

### 5. **Migration Path** (5-Phase Approach)

**Phase 1: YAML Definitions** (Non-breaking)
- Create YAML for each want type
- No changes to Go code required
- Both systems coexist

**Phase 2: Integrate Validation**
- WantTypeRegistry loads YAML at startup
- Parameters validated against definitions
- Error messages reference want type docs

**Phase 3: Create API Endpoints**
- Browse want types via API
- Get definitions and examples
- Dynamic registration support

**Phase 4: Update Frontend**
- Fetch want types on page load
- Show parameter help and validation
- Client-side validation before submit

**Phase 5: Full Conversion** (Optional)
- Reduce Go boilerplate
- Framework handles connectivity
- Generic want type support

---

## Key Design Decisions

### 1. **Declarative YAML Over Go Code**
- ✅ Configuration-driven architecture
- ✅ Easier for non-Go developers to understand
- ✅ Enables dynamic registration
- ✅ Mirrors recipe system design

### 2. **Pattern-Based Classification**
Rather than separate type hierarchies, all wants fall into 5 patterns:
- Enables flexible connectivity rules
- Allows tool generation from patterns
- Clearer semantics for users

### 3. **Comprehensive Parameter Definitions**
Each parameter includes:
- Type, default, validation constraints
- Example values for testing
- Human-readable descriptions

Enables:
- Client-side validation
- Automatic form generation
- IDE autocomplete hints

### 4. **State as First-Class Definition**
State keys explicitly defined with:
- Type (matches parameters)
- Persistence flag
- Purpose/description

Benefits:
- Self-documenting state interface
- Type-safe access
- Clear persistence guarantees

### 5. **Examples as Documentation**
Each want type includes 2+ examples showing:
- Real parameter values
- Expected behavior
- Connectivity (what connects to what)

Enables:
- Copy-paste starting points
- Runnable documentation
- Regression test cases

---

## Comparison: Before vs After

### Before (Current State)

```go
// Numbers want type definition scattered across:
// 1. qnet_types.go - Go struct + constructor
// 2. chain_builder.go - Factory registration
// 3. Documentation - Markdown files (if they exist)
// 4. Tests - Buried in test files
// 5. Examples - Hard to find

type NumbersWant struct {
    Want
    start int
    count int
}

// What parameters does Numbers use? Look at spec.Params
// What state does it create? Search GetState/StoreState
// What are the validation rules? Hope they're in comments
```

### After (Proposed System)

```yaml
# want_types/generators/numbers.yaml - SINGLE SOURCE OF TRUTH
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
```

**Benefits**:
- ✅ Single source of truth
- ✅ Machine-readable definitions
- ✅ Enables validation
- ✅ Self-documenting
- ✅ API can serve definitions
- ✅ Frontend can auto-generate forms

---

## Implementation Roadmap

### Immediate (Foundation)
- [ ] Create WantTypeRegistry struct and loader
- [ ] Load all YAML files at startup
- [ ] Implement parameter validation logic
- [ ] Create helper functions for type checking

### Short-term (Validation)
- [ ] Integrate validation into chain builder
- [ ] Add error messages referencing want type docs
- [ ] Update config loader to validate parameters
- [ ] Add default value application

### Medium-term (API)
- [ ] Implement GET /api/v1/want-types endpoints
- [ ] Create endpoint for examples
- [ ] Add filtering by category/pattern
- [ ] Document API in OpenAPI/Swagger

### Long-term (Frontend & Tools)
- [ ] Update recipe card to show want type info
- [ ] Add help panel showing want type definition
- [ ] Create CLI tool: `mywant want-type list`
- [ ] Create CLI tool: `mywant want-type show {name}`
- [ ] Generate forms from want type definitions

---

## Files Delivered

### 1. Documentation
- **WANT_TYPE_DEFINITION.md** (10 KB)
  - Complete YAML schema specification
  - 4 full example definitions
  - Comparison with recipes
  - Future enhancements

### 2. Example YAML Files
- **want_types/generators/numbers.yaml**
- **want_types/processors/queue.yaml**
- **want_types/independent/restaurant.yaml**
- **want_types/coordinators/travel_coordinator.yaml**
- **want_types/sinks/sink.yaml**

### 3. Migration Guide
- **WANT_TYPE_MIGRATION_GUIDE.md** (9 KB)
  - Step-by-step conversion for each want type
  - Phase-by-phase implementation approach
  - Code examples for validation
  - Frontend integration patterns
  - Migration checklist

### 4. Summary (This Document)
- **WANT_TYPE_DESIGN_SUMMARY.md**

---

## Next Steps

### 1. Review & Feedback
- [ ] Review YAML schema - Is it complete?
- [ ] Review example definitions - Are they accurate?
- [ ] Validate against existing want types (all 16+)
- [ ] Check if all patterns are covered

### 2. Convert Existing Want Types
- [ ] Create YAML for Numbers, Queue, Sink, etc.
- [ ] Extract examples from existing tests/code
- [ ] Validate YAML definitions against code

### 3. Implement Registry System
- [ ] Create WantTypeRegistry struct
- [ ] Implement YAML loader
- [ ] Add parameter validation
- [ ] Create helper functions

### 4. Add Validation to Chain Builder
- [ ] Integrate registry into config loading
- [ ] Validate parameters at config load time
- [ ] Apply default values
- [ ] Generate helpful error messages

### 5. Create API Endpoints
- [ ] GET /api/v1/want-types
- [ ] GET /api/v1/want-types/{name}
- [ ] GET /api/v1/want-types/{name}/examples
- [ ] POST/PUT/DELETE for dynamic registration

### 6. Update Frontend
- [ ] Fetch want types on load
- [ ] Show want type info in recipe builder
- [ ] Add parameter help tooltips
- [ ] Validate parameters client-side

---

## Success Criteria

✅ **Design Phase Complete When**:
1. YAML schema reviewed and approved
2. Examples validated against 3-5 real want types
3. Migration path is clear and documented
4. API design is finalized
5. Frontend integration points are identified

🎯 **Implementation Phase Complete When**:
1. WantTypeRegistry fully functional
2. All parameter validation working
3. API endpoints implemented and tested
4. Frontend shows want type definitions
5. Documentation generated from YAML

🚀 **Full Rollout Complete When**:
1. All 16+ want types have YAML definitions
2. New types can be defined in YAML only
3. Dynamic registration is working
4. CLI tools available for exploration
5. Backward compatibility maintained

---

## Q&A

**Q: Do we need to rewrite all Go code?**
A: No! Go constructors stay the same. YAML is an additional validation layer.

**Q: What if a want type doesn't fit the patterns?**
A: Patterns are flexible. You can define custom patterns (e.g., "aggregator", "splitter").

**Q: Can we still use dynamic want addition?**
A: Yes! Fully compatible. Add dynamic wants with validated parameters.

**Q: How does this affect existing recipes?**
A: Recipes remain unchanged. They'll benefit from better parameter validation.

**Q: When can we start using this?**
A: Immediately for YAML definitions. Registry integration can happen gradually.

---

## Related Documentation

- `WANT_TYPE_DEFINITION.md` - Full schema specification
- `WANT_TYPE_MIGRATION_GUIDE.md` - Step-by-step conversion guide
- `WANT_TYPE_SYSTEM_ANALYSIS.md` - Deep dive into current system (from Explore agent)
- `CLAUDE.md` - Project architecture overview
