# Want Type YAML Definition System - Complete Design Index

## 📋 Document Overview

This is your index to understanding and implementing the new want type YAML definition system. Everything has been designed and documented.

---

## 🚀 Quick Start (5 Minutes)

**New to the design?** Start here:

1. **Read**: [WANT_TYPE_DESIGN_SUMMARY.md](./WANT_TYPE_DESIGN_SUMMARY.md) (5 min)
   - What's changing and why
   - Key design decisions
   - Benefits and success criteria

2. **Look at**: `want_types/independent/restaurant.yaml` (3 min)
   - Real-world example
   - Shows all features in practice

3. **Decide**: Ready to implement?
   - Go to [Implementation Roadmap](#implementation-roadmap)

---

## 📚 Core Documentation

### 1. WANT_TYPE_DEFINITION.md
**Complete YAML Schema Specification** (10 KB, 500+ lines)

Contains:
- ✅ Full YAML schema with all fields explained
- ✅ 4 complete example definitions (Numbers, Queue, Restaurant, TravelCoordinator)
- ✅ 5 architectural patterns (generator, processor, sink, coordinator, independent)
- ✅ Parameter definition system with validation
- ✅ State management and persistence
- ✅ Connectivity model for want-to-want connections
- ✅ Agent integration patterns
- ✅ API endpoint designs
- ✅ Comparison with recipes
- ✅ Future enhancements and extensibility

**When to read**: When you need the complete specification or are creating new want types

---

### 2. WANT_TYPE_DESIGN_SUMMARY.md
**Executive Summary & Design Decisions** (8 KB, 400+ lines)

Contains:
- ✅ Vision and goals for the system
- ✅ What has been designed (4 sections)
- ✅ Key design decisions and rationale
- ✅ Before/after comparison
- ✅ Implementation roadmap
- ✅ Files delivered
- ✅ Next steps and success criteria
- ✅ FAQ

**When to read**: When presenting the design to stakeholders or needing a high-level overview

---

### 3. WANT_TYPE_MIGRATION_GUIDE.md
**Step-by-Step Conversion Instructions** (9 KB, 450+ lines)

Contains:
- ✅ Phase-by-phase migration approach (5 phases)
- ✅ How to convert existing Go want types to YAML
- ✅ Parameter extraction from Go code
- ✅ State key documentation
- ✅ Connectivity pattern identification
- ✅ Parameter validation implementation
- ✅ API endpoint code examples
- ✅ Frontend integration patterns
- ✅ Complete migration checklist
- ✅ File organization after migration

**When to read**: When implementing the registry system or converting want types

---

## 📁 Example YAML Files (Complete, Production-Ready)

All examples are in `want_types/` directory:

### Generators
**want_types/generators/numbers.yaml** (70 lines)
- Simple generator pattern
- Parameters: start, count
- State: current, generated_count, completion_time
- 3 realistic examples

### Processors
**want_types/processors/queue.yaml** (140 lines)
- Complex processor with metrics
- Parameters: service_time, max_queue_size, priority_mode
- State: 6 keys tracking queue metrics and throughput
- 4 usage examples (normal, slow, high-throughput, size-limited)

### Independent
**want_types/independent/restaurant.yaml** (200 lines)
- Full real-world example
- Parameters: 5 dining preferences
- State: 9 keys for complete reservation lifecycle
- Agents: MonitorRestaurant, AgentRestaurant
- 4 usage examples with different scenarios

### Coordinators
**want_types/coordinators/travel_coordinator.yaml** (180 lines)
- Orchestrator pattern
- Parameters: display, format, timezone, costs
- State: Complete itinerary, costs, status
- 4 complex scenarios (full trip, weekend, single restaurant, multi-city)

### Sinks
**want_types/sinks/sink.yaml** (150 lines)
- Terminal aggregation node
- Parameters: operation (count/sum/average/min/max)
- State: Statistics and metrics
- 5 operation modes with examples

### Templates
**want_types/templates/WANT_TYPE_TEMPLATE.yaml** (350 lines)
- Heavily commented template
- Every field explained
- Guidelines for each section
- Real example snippets

---

## 🏗️ Architecture

### Directory Structure (After Implementation)

```
project/
├── want_types/                          # Want type definitions (NEW)
│   ├── generators/
│   │   ├── numbers.yaml                 ✅ Created
│   │   ├── fibonacci_numbers.yaml       (to create)
│   │   └── prime_numbers.yaml           (to create)
│   ├── processors/
│   │   ├── queue.yaml                   ✅ Created
│   │   ├── combiner.yaml                (to create)
│   │   ├── fibonacci_sequence.yaml      (to create)
│   │   └── prime_sequence.yaml          (to create)
│   ├── sinks/
│   │   ├── sink.yaml                    ✅ Created
│   │   └── prime_sink.yaml              (to create)
│   ├── coordinators/
│   │   └── travel_coordinator.yaml      ✅ Created
│   ├── independent/
│   │   ├── restaurant.yaml              ✅ Created
│   │   ├── hotel.yaml                   (to create)
│   │   ├── buffet.yaml                  (to create)
│   │   └── flight.yaml                  (to create)
│   └── templates/
│       └── WANT_TYPE_TEMPLATE.yaml      ✅ Created
│
├── engine/src/                          # Go code
│   ├── want_type_registry.go            (to implement)
│   ├── *_types.go                       (existing, with YAML defs)
│   ├── chain_builder.go                 (update validation)
│   └── api_handlers.go                  (new endpoints)
│
├── web/                                 # Frontend
│   ├── src/components/                  (update)
│   └── src/pages/                       (update)
│
└── Documentation/
    ├── WANT_TYPE_DEFINITION.md          ✅ Created
    ├── WANT_TYPE_DESIGN_SUMMARY.md      ✅ Created
    ├── WANT_TYPE_MIGRATION_GUIDE.md     ✅ Created
    └── WANT_TYPE_DESIGN_INDEX.md        ✅ This file
```

---

## 🎯 Implementation Roadmap

### Phase 1: Foundation (2-3 days)
**Objective**: Load and parse want type YAML files

- [ ] Create `WantTypeRegistry` struct in `engine/src/want_type_registry.go`
- [ ] Implement YAML loader from `want_types/` directory
- [ ] Create `WantTypeDefinition` struct matching schema
- [ ] Add startup code to load registry at server start
- [ ] Write unit tests for registry

**Deliverables**:
- Registry struct (300-400 lines)
- YAML loader (200-300 lines)
- Tests (200+ lines)

**Code Location**: `engine/src/want_type_registry.go`

---

### Phase 2: Validation (2-3 days)
**Objective**: Validate parameters before want creation

- [ ] Implement parameter validator in registry
- [ ] Add type checking (int, float64, string, bool, etc)
- [ ] Add range validation (min/max)
- [ ] Add enum validation
- [ ] Add regex pattern validation
- [ ] Implement default value injection
- [ ] Create validation error builder with helpful messages

**Deliverables**:
- Validator functions (300-400 lines)
- Error messages with examples
- Tests (300+ lines)

**Code Location**: `engine/src/want_type_registry.go` (Validator section)

**Integration Points**:
- Update `chain_builder.go` to call validator
- Update `declarative.go` config loading

---

### Phase 3: API Endpoints (1-2 days)
**Objective**: Expose want type definitions via REST API

Endpoints to implement:
```go
GET    /api/v1/want-types               # List all want types
GET    /api/v1/want-types/{name}        # Get specific definition
GET    /api/v1/want-types/{name}/examples  # Get examples
GET    /api/v1/want-types?category=X   # Filter by category
GET    /api/v1/want-types?pattern=X    # Filter by pattern
POST   /api/v1/want-types               # Register new type (optional)
PUT    /api/v1/want-types/{name}        # Update definition (optional)
DELETE /api/v1/want-types/{name}        # Delete type (optional)
```

**Deliverables**:
- Handler functions (200-300 lines)
- Route registrations (50+ lines)
- Tests (200+ lines)

**Code Location**: `engine/src/api_handlers.go` (new file)

---

### Phase 4: Frontend Integration (2-3 days)
**Objective**: Show want type definitions in UI

Updates needed:
- [ ] Fetch want types on component load
- [ ] Show parameter definitions in recipe builder
- [ ] Display parameter help/tooltips
- [ ] Client-side parameter validation
- [ ] Show examples in modal/drawer
- [ ] Update RecipeCard with want type info

**Deliverables**:
- Components (300-400 lines TypeScript)
- Hooks for want type fetching
- Validation logic
- Tests (200+ lines)

**Code Location**:
- `web/src/hooks/useWantTypes.ts` (new)
- `web/src/components/WantTypeSelector.tsx` (new)
- `web/src/components/ParameterForm.tsx` (update)

---

### Phase 5: Convert Existing Types (3-5 days)
**Objective**: Create YAML definitions for all 16+ want types

Types to convert:
- [ ] Numbers (generator)
- [ ] FibonacciNumbers (generator)
- [ ] PrimeNumbers (generator)
- [ ] Queue (processor)
- [ ] Combiner (processor)
- [ ] FibonacciSequence (processor)
- [ ] PrimeSequence (processor)
- [ ] Sink (sink)
- [ ] PrimeSink (sink)
- [ ] RestaurantWant (independent)
- [ ] HotelWant (independent)
- [ ] BuffetWant (independent)
- [ ] FlightWant (independent)
- [ ] TravelCoordinator (coordinator)
- [ ] OwnerWant (special)
- [ ] CustomTargetWant (special)

**Deliverables**:
- 16+ YAML files (70-200 lines each)
- Examples for each type (2-4 examples)
- Validation of definitions against code

**Code Location**: `want_types/` directory

---

### Phase 6: Testing & Refinement (2-3 days)
**Objective**: Ensure system works end-to-end

Tests to write:
- [ ] Registry loading and parsing
- [ ] Parameter validation (positive and negative cases)
- [ ] Type conversion and coercion
- [ ] Enum and pattern validation
- [ ] Default value application
- [ ] API endpoint functionality
- [ ] Frontend integration
- [ ] Backward compatibility with existing configs

**Deliverables**:
- Unit tests (500+ lines)
- Integration tests (300+ lines)
- E2E tests (200+ lines)
- Bug fixes and refinements

---

## 📊 Design Principles

### 1. Configuration-Driven
Want types are defined in YAML, not hardcoded. This enables:
- Non-developers can understand want types
- Dynamic registration at runtime
- API can serve definitions
- Frontend can generate forms

### 2. Single Source of Truth
Each want type has ONE definition file:
- Parameters, state, connectivity, agents
- No scattered documentation
- Easy to update
- Version controlled

### 3. Self-Documenting
YAML includes:
- Parameter descriptions and examples
- State key purposes
- Agent integration points
- Usage examples
- Related types

Frontend can use this to:
- Show help text
- Generate forms
- Suggest related types
- Validate inputs

### 4. Type-Safe Validation
Before want creation:
- Parameter types are checked
- Ranges are validated
- Enums are enforced
- Default values applied

This prevents:
- Runtime panics from type assertions
- Invalid configurations
- Confusing error messages

### 5. Progressive Enhancement
System works at multiple levels:
- **Minimal**: YAML definitions exist but aren't used
- **Basic**: Validation enabled, error messages improved
- **Full**: API, frontend integration, dynamic registration
- **Advanced**: Auto-generated forms, smart suggestions

Can implement gradually without breaking existing system.

---

## 🔍 File Reference Quick Lookup

| Need | File | Location |
|------|------|----------|
| Full specification | WANT_TYPE_DEFINITION.md | Root |
| Overview & decisions | WANT_TYPE_DESIGN_SUMMARY.md | Root |
| Migration steps | WANT_TYPE_MIGRATION_GUIDE.md | Root |
| This index | WANT_TYPE_DESIGN_INDEX.md | Root |
| Numbers want example | numbers.yaml | want_types/generators/ |
| Queue want example | queue.yaml | want_types/processors/ |
| Restaurant example | restaurant.yaml | want_types/independent/ |
| TravelCoordinator example | travel_coordinator.yaml | want_types/coordinators/ |
| Sink example | sink.yaml | want_types/sinks/ |
| Template to copy | WANT_TYPE_TEMPLATE.yaml | want_types/templates/ |

---

## 💡 Key Insights

### Pattern-Based Thinking
Instead of separate type hierarchies, all wants fit into 5 patterns:
- **Generator**: Creates data
- **Processor**: Transforms data
- **Sink**: Consumes data
- **Coordinator**: Orchestrates independent wants
- **Independent**: Standalone execution

This enables:
- Clear semantics
- Flexible connectivity rules
- Pattern-specific tools
- Easier onboarding

### Parameter Definitions as API Contract
By explicitly defining parameters, we get:
- Type safety (pre-validated before creation)
- Self-documenting API (definitions are queryable)
- Client-side validation (forms generated from defs)
- Better error messages (reference examples)

### State as Contract
By explicitly defining state keys, we get:
- Type-safe access (know what types to expect)
- Persistence guarantees (know what survives)
- API discoverability (what state is available)
- Documentation (what each key means)

---

## ❓ Common Questions

**Q: Do we need to update all Go code?**
A: No. YAML is additive. Go constructors work as-is.

**Q: What about want types that don't fit patterns?**
A: Define custom patterns. Or use "generic" pattern for special cases.

**Q: Can we migrate gradually?**
A: Yes! YAML can coexist with Go definitions. Migrate at own pace.

**Q: What about backward compatibility?**
A: Fully compatible. Existing configs work unchanged.

**Q: How do we handle want type versioning?**
A: Include `version: "1.0"` in YAML. Support multiple versions if needed.

**Q: Can want types be registered dynamically?**
A: Yes! POST /api/v1/want-types with YAML. Hot-loads without restart.

---

## 🎓 Reading Paths

### Path 1: "I want to understand the big picture" (15 min)
1. This index (overview)
2. WANT_TYPE_DESIGN_SUMMARY.md (design decisions)
3. Look at one YAML example (restaurant.yaml)

### Path 2: "I need to convert existing want types" (1 hour)
1. WANT_TYPE_MIGRATION_GUIDE.md (process)
2. Review want_types/independent/restaurant.yaml (example)
3. Review WANT_TYPE_DEFINITION.md (schema reference)
4. Copy WANT_TYPE_TEMPLATE.yaml and customize

### Path 3: "I need to implement the registry system" (2 hours)
1. WANT_TYPE_DEFINITION.md (understand schema)
2. WANT_TYPE_MIGRATION_GUIDE.md (phase 2 validation section)
3. Review all YAML examples (understand patterns)
4. Look at template (understand all possible fields)

### Path 4: "I need to implement the API" (1 hour)
1. WANT_TYPE_DEFINITION.md (API endpoint designs)
2. WANT_TYPE_MIGRATION_GUIDE.md (phase 3 code examples)

### Path 5: "I need to update the frontend" (1.5 hours)
1. WANT_TYPE_MIGRATION_GUIDE.md (phase 4 frontend section)
2. Review restaurant.yaml (understand data structure)

---

## ✅ Validation Checklist

Before implementing, verify:

- [ ] Have you read WANT_TYPE_DESIGN_SUMMARY.md?
- [ ] Do the 5 patterns match your want types?
- [ ] Are the example YAML files realistic?
- [ ] Does the API design cover your needs?
- [ ] Is the migration path clear?
- [ ] Do you understand the validation strategy?
- [ ] Is the file structure what you expected?

Before deploying, verify:

- [ ] All parameter validations working
- [ ] Error messages helpful
- [ ] API endpoints returning correct data
- [ ] Frontend shows want type info
- [ ] Backward compatibility confirmed
- [ ] All 16+ types have YAML definitions
- [ ] Tests passing

---

## 📞 Next Steps

1. **Review** this design with team (30 min)
2. **Validate** YAML schema against want types (1 hour)
3. **Approve** architecture and approach
4. **Assign** implementation phases to team members
5. **Start** Phase 1: Foundation (WantTypeRegistry)

---

## 📝 Files Delivered Summary

| File | Type | Size | Purpose |
|------|------|------|---------|
| WANT_TYPE_DEFINITION.md | Spec | 10 KB | Complete YAML schema |
| WANT_TYPE_DESIGN_SUMMARY.md | Design | 8 KB | Executive summary |
| WANT_TYPE_MIGRATION_GUIDE.md | Guide | 9 KB | Step-by-step conversion |
| WANT_TYPE_DESIGN_INDEX.md | Index | 8 KB | This file |
| numbers.yaml | Example | 70 lines | Generator pattern |
| queue.yaml | Example | 140 lines | Processor pattern |
| restaurant.yaml | Example | 200 lines | Independent pattern |
| travel_coordinator.yaml | Example | 180 lines | Coordinator pattern |
| sink.yaml | Example | 150 lines | Sink pattern |
| WANT_TYPE_TEMPLATE.yaml | Template | 350 lines | Copy for new types |

**Total**: 6 documentation files + 5 examples + 1 template = 12 files
**Total Size**: ~60 KB of specification and examples

---

**Document Version**: 1.0
**Last Updated**: 2024
**Status**: ✅ Design Complete - Ready for Implementation
