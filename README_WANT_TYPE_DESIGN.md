# Want Type YAML Definition System - Complete Design

## 🎯 What Is This?

A complete design specification for converting want types from hardcoded Go implementations to declarative YAML definitions. This makes want types:
- **First-class citizens** like recipes
- **Self-documenting** with built-in examples
- **Type-safe** with pre-validation before creation
- **API-discoverable** so frontend can query definitions
- **Dynamic** with hot-reload capability

---

## 📦 What You Get

### Documentation Files (9 Files, ~142 KB)

| File | Size | Purpose |
|------|------|---------|
| **WANT_TYPE_DEFINITION.md** | 22 KB | Complete YAML schema specification with examples |
| **WANT_TYPE_DESIGN_SUMMARY.md** | 12 KB | Executive summary and design decisions |
| **WANT_TYPE_MIGRATION_GUIDE.md** | 16 KB | Step-by-step conversion instructions |
| **WANT_TYPE_DESIGN_INDEX.md** | 17 KB | Navigation index and reading paths |
| **WANT_TYPE_VISUAL_GUIDE.md** | 23 KB | ASCII diagrams and visual explanations |
| **WANT_TYPE_SYSTEM_ANALYSIS.md** | 24 KB | Deep analysis of current want type system |
| START_HERE.md | 6.4 KB | Quick entry point with FAQ |
| WANT_TYPE_QUICK_REFERENCE.md | 8.4 KB | Code templates and snippets |
| WANT_TYPE_DOCUMENTATION_README.md | 8.8 KB | Navigation guide |

**All documentation is in the root directory of your project**

### Example YAML Files (5 Files, ~25 KB) - Production Ready

| File | Pattern | Size | Use Case |
|------|---------|------|----------|
| **want_types/generators/numbers.yaml** | Generator | 2.2 KB | Creates data (no inputs) |
| **want_types/processors/queue.yaml** | Processor | 3.6 KB | Transforms data (inputs→outputs) |
| **want_types/independent/restaurant.yaml** | Independent | 6.3 KB | Standalone execution with agents |
| **want_types/coordinators/travel_coordinator.yaml** | Coordinator | 6.5 KB | Orchestrates independent wants |
| **want_types/sinks/sink.yaml** | Sink | 6.4 KB | Terminal processing node |

**All YAML files are in the want_types/ directory**

### Template File (1 File, 10 KB)

**want_types/templates/WANT_TYPE_TEMPLATE.yaml**
- Heavily commented template
- Copy this to create new want types
- Every field explained with guidelines

---

## 🚀 Quick Start (Choose Your Path)

### Path 1: "Just Show Me What This Is" (5 min)
1. Read: [WANT_TYPE_DESIGN_SUMMARY.md](./WANT_TYPE_DESIGN_SUMMARY.md)
2. Look at: [want_types/independent/restaurant.yaml](./want_types/independent/restaurant.yaml)

### Path 2: "I Need To Convert My Want Types" (1 hour)
1. Read: [WANT_TYPE_MIGRATION_GUIDE.md](./WANT_TYPE_MIGRATION_GUIDE.md)
2. Copy: [want_types/templates/WANT_TYPE_TEMPLATE.yaml](./want_types/templates/WANT_TYPE_TEMPLATE.yaml)
3. Review: All examples in want_types/ directory

### Path 3: "I Need The Complete Specification" (2 hours)
1. Read: [WANT_TYPE_DEFINITION.md](./WANT_TYPE_DEFINITION.md)
2. Review: All examples in want_types/ directory
3. Skim: [WANT_TYPE_VISUAL_GUIDE.md](./WANT_TYPE_VISUAL_GUIDE.md)

### Path 4: "I'm Implementing This System" (3 hours)
1. Read: [WANT_TYPE_DEFINITION.md](./WANT_TYPE_DEFINITION.md) (full)
2. Read: [WANT_TYPE_MIGRATION_GUIDE.md](./WANT_TYPE_MIGRATION_GUIDE.md) (implementation sections)
3. Review: [WANT_TYPE_VISUAL_GUIDE.md](./WANT_TYPE_VISUAL_GUIDE.md) (validation flow section)

---

## 🏗️ System Overview

### 5 Want Type Patterns (All Covered)

```
GENERATOR          PROCESSOR         SINK              COORDINATOR      INDEPENDENT
┌─────────┐       ┌───────┐         ┌──────┐         ┌─────┐┌─────┐   ┌──────────┐
│  Data   │──→    │Process│──→      │  End │        │Want1││Want2│──→ │Standalone│
│Creation │       │  Flow │         │Point │        └─────┘└─────┘    │   Want   │
└─────────┘       └───────┘         └──────┘             │             └──────────┘
                                                   Orchestrates → Output
No inputs        Transform data     Terminal node    Independent      No connections
```

### Validation System (Type-Safe Before Creation)

```
Config YAML
    ↓
Check parameter types (int, string, etc)
    ↓
Check ranges (min/max)
    ↓
Check enums/patterns
    ↓
Apply defaults
    ↓
✅ Create want with validated parameters
```

### API Design (8 Endpoints)

```
GET    /api/v1/want-types                    # List all
GET    /api/v1/want-types/{name}             # Get definition
GET    /api/v1/want-types/{name}/examples   # Get examples
GET    /api/v1/want-types?category=X        # Filter by category
GET    /api/v1/want-types?pattern=X         # Filter by pattern
POST   /api/v1/want-types                    # Register new (optional)
PUT    /api/v1/want-types/{name}             # Update (optional)
DELETE /api/v1/want-types/{name}             # Delete (optional)
```

---

## 📚 Key Design Features

### 1. Complete YAML Schema
Every want type includes:
- **Identity**: name, title, description, version, category
- **Parameters**: types, validation, defaults, examples
- **State**: keys, types, persistence flags
- **Connectivity**: input/output patterns
- **Agents**: integration points
- **Constraints**: business logic validation
- **Examples**: 2-4 realistic usage scenarios
- **Relations**: links to related types

### 2. 5 Architectural Patterns
All want types fit into one of:
- **Generator**: Creates data (Numbers, FibonacciNumbers, PrimeNumbers)
- **Processor**: Transforms data (Queue, Combiner, Fibonacci/Prime Sequence)
- **Sink**: Terminal node (Sink, PrimeSink)
- **Coordinator**: Orchestrates (TravelCoordinator)
- **Independent**: Standalone (Restaurant, Hotel, Buffet, Flight)

### 3. Parameter Validation
Before want creation, validate:
- Type checking (int vs string vs bool)
- Range validation (min/max)
- Enum validation (allowed values)
- Regex patterns (format validation)
- Default value injection

### 4. Self-Documenting API
Want type definitions are queryable:
- Frontend can fetch definitions
- Auto-generate parameter forms
- Show examples and help text
- Validate client-side before submit

### 5. Gradual Migration Path
Phases can be implemented independently:
- Phase 1: Load YAML (no changes to Go)
- Phase 2: Validate parameters
- Phase 3: Create API endpoints
- Phase 4: Update frontend
- Phase 5: Convert all want types

---

## 📋 Implementation Roadmap

### Phase 1: Foundation (2-3 days)
Create `WantTypeRegistry` that loads YAML files
- Result: Can parse and load want type definitions

### Phase 2: Validation (2-3 days)
Validate parameters before want creation
- Result: Type-safe parameter checking

### Phase 3: API (1-2 days)
Create REST endpoints for want type browsing
- Result: Frontend can query definitions

### Phase 4: Frontend (2-3 days)
Update UI to show want type information
- Result: Users see parameter help and examples

### Phase 5: Convert Types (3-5 days)
Create YAML for all 16+ existing want types
- Result: Complete system with all types defined

### Phase 6: Polish (2-3 days)
Testing, performance, documentation
- Result: Production-ready system

**Total: 2-3 weeks (can run phases in parallel)**

---

## ✨ Key Highlights

### What's Different From Recipes?

| Aspect | Recipe | Want Type |
|--------|--------|-----------|
| **Purpose** | Want combinations | Individual want contract |
| **Scope** | Multiple wants | Single want |
| **Parameters** | Instance values | Parameter definitions |
| **Validation** | Loose | Strict (types, ranges) |
| **Examples** | Use cases | Parameter examples |

### Before vs After

**Before (Go Only)**:
- Type assertions can panic
- No validation before creation
- Parameters scattered in comments
- State keys buried in code
- Hard to discover capabilities

**After (YAML Definitions)**:
- Type-safe validation
- Pre-validated before creation
- Self-documenting parameters
- Explicit state definitions
- API queryable

---

## 📖 Document Navigation

### Start Here
- **New to the design?** → [WANT_TYPE_DESIGN_SUMMARY.md](./WANT_TYPE_DESIGN_SUMMARY.md) (5 min)
- **Presenting to team?** → [WANT_TYPE_VISUAL_GUIDE.md](./WANT_TYPE_VISUAL_GUIDE.md) (show diagrams)
- **Need quick overview?** → [WANT_TYPE_DESIGN_INDEX.md](./WANT_TYPE_DESIGN_INDEX.md)

### Implementation
- **Full specification** → [WANT_TYPE_DEFINITION.md](./WANT_TYPE_DEFINITION.md)
- **How to convert types** → [WANT_TYPE_MIGRATION_GUIDE.md](./WANT_TYPE_MIGRATION_GUIDE.md)
- **Step-by-step guide** → [WANT_TYPE_DESIGN_INDEX.md](./WANT_TYPE_DESIGN_INDEX.md) (roadmap section)

### Examples
- **Simple generator** → [want_types/generators/numbers.yaml](./want_types/generators/numbers.yaml)
- **Complex processor** → [want_types/processors/queue.yaml](./want_types/processors/queue.yaml)
- **Real-world example** → [want_types/independent/restaurant.yaml](./want_types/independent/restaurant.yaml)
- **Orchestrator** → [want_types/coordinators/travel_coordinator.yaml](./want_types/coordinators/travel_coordinator.yaml)
- **Template to copy** → [want_types/templates/WANT_TYPE_TEMPLATE.yaml](./want_types/templates/WANT_TYPE_TEMPLATE.yaml)

### Visual Guides
- **Architecture diagrams** → [WANT_TYPE_VISUAL_GUIDE.md](./WANT_TYPE_VISUAL_GUIDE.md) (top section)
- **Validation flow** → [WANT_TYPE_VISUAL_GUIDE.md](./WANT_TYPE_VISUAL_GUIDE.md) (flow section)
- **Data flow** → [WANT_TYPE_VISUAL_GUIDE.md](./WANT_TYPE_VISUAL_GUIDE.md) (data flow section)

---

## 🎓 For Different Roles

### Team Leads (30 min)
1. [WANT_TYPE_DESIGN_SUMMARY.md](./WANT_TYPE_DESIGN_SUMMARY.md) - Know what's happening
2. [WANT_TYPE_VISUAL_GUIDE.md](./WANT_TYPE_VISUAL_GUIDE.md) - Patterns section
3. Assign phases to team

### Backend/Go Engineers (2 hours)
1. [WANT_TYPE_DEFINITION.md](./WANT_TYPE_DEFINITION.md) - Full spec
2. [WANT_TYPE_MIGRATION_GUIDE.md](./WANT_TYPE_MIGRATION_GUIDE.md) - Phase 2 & 3
3. Implement WantTypeRegistry

### Frontend Engineers (1.5 hours)
1. [WANT_TYPE_MIGRATION_GUIDE.md](./WANT_TYPE_MIGRATION_GUIDE.md) - Phase 4
2. [want_types/independent/restaurant.yaml](./want_types/independent/restaurant.yaml) - Example structure
3. Implement want type fetching and forms

### Architects (1.5 hours)
1. [WANT_TYPE_DESIGN_SUMMARY.md](./WANT_TYPE_DESIGN_SUMMARY.md) - Full overview
2. [WANT_TYPE_VISUAL_GUIDE.md](./WANT_TYPE_VISUAL_GUIDE.md) - Architecture diagram
3. Review and approve approach

---

## ✅ What's Ready Now

- ✅ **Complete YAML schema specification**
- ✅ **5 production-ready example YAML files**
- ✅ **Template for creating new want types**
- ✅ **8 API endpoints designed**
- ✅ **6-phase implementation roadmap**
- ✅ **Code examples for validation**
- ✅ **Frontend integration patterns**
- ✅ **Migration checklist for 16+ types**

## 🚀 Next Steps

1. **Review** - Team reviews the design (1 day)
2. **Validate** - Check YAML against your want types (1 day)
3. **Approve** - Green light for implementation (1 day)
4. **Implement** - Start Phase 1: WantTypeRegistry (2-3 weeks)

---

## 📞 Questions?

See [WANT_TYPE_DESIGN_INDEX.md](./WANT_TYPE_DESIGN_INDEX.md) for FAQ and common questions.

---

## 📊 Files at a Glance

```
Root Directory (Documentation):
├── WANT_TYPE_DEFINITION.md              (22 KB) - Complete spec
├── WANT_TYPE_DESIGN_SUMMARY.md          (12 KB) - Executive summary
├── WANT_TYPE_MIGRATION_GUIDE.md         (16 KB) - How to implement
├── WANT_TYPE_DESIGN_INDEX.md            (17 KB) - Navigation
├── WANT_TYPE_VISUAL_GUIDE.md            (23 KB) - Diagrams
├── WANT_TYPE_SYSTEM_ANALYSIS.md         (24 KB) - Current system analysis
├── START_HERE.md                        (6.4 KB) - Entry point
├── WANT_TYPE_QUICK_REFERENCE.md         (8.4 KB) - Code snippets
└── WANT_TYPE_DOCUMENTATION_README.md    (8.8 KB) - Nav guide

want_types/ Directory (Examples):
├── generators/
│   └── numbers.yaml                     (2.2 KB)
├── processors/
│   └── queue.yaml                       (3.6 KB)
├── independent/
│   └── restaurant.yaml                  (6.3 KB)
├── coordinators/
│   └── travel_coordinator.yaml          (6.5 KB)
├── sinks/
│   └── sink.yaml                        (6.4 KB)
└── templates/
    └── WANT_TYPE_TEMPLATE.yaml          (10 KB)

Total: 15 files, ~165 KB of spec and examples
```

---

**Status**: ✅ Design Complete - Ready for Implementation

**Version**: 1.0

**Next Phase**: Team Review & Implementation Planning
