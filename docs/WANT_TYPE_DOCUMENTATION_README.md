# MyWant Want Type Definition System - Documentation

This directory contains comprehensive analysis and reference documentation for the MyWant want type definition system.

## Documents Overview

### 1. WANT_TYPE_SYSTEM_ANALYSIS.md (854 lines, 24KB)
**Comprehensive Deep-Dive Analysis**

The main document covering:
- Complete type hierarchy (Metadata, WantSpec, Want)
- Factory pattern implementation
- Want type definition patterns with multiple examples
- Registration system and type registration process
- Parameter extraction patterns for all data types
- Execution interfaces (Executable, PacketHandler)
- Connectivity metadata system
- State management with thread safety
- YAML configuration structure
- All 5 common want type patterns
- Constructor signature standardization
- Key methods in Want base type
- 10 critical design principles
- Step-by-step extension guide
- All want type files in codebase

**Best for**: Understanding the complete architecture and implementing new want types

### 2. WANT_TYPE_QUICK_REFERENCE.md (357 lines, 8.4KB)
**Quick Reference and Templates**

Fast-lookup document containing:
- File location quick index
- Core type checklists
- Constructor template
- Registration template
- Exec implementation template
- Parameter extraction snippets (string, numeric, boolean, float)
- Connectivity patterns for all 4 architectural styles
- State management examples
- YAML configuration examples
- Common issues and solutions
- Implementation testing checklist
- Field naming conventions
- Key methods summary table

**Best for**: Implementing new want types, copy-paste templates, quick lookups

### 3. WANT_TYPE_ANALYSIS_SUMMARY.txt (318 lines, 11KB)
**Executive Summary**

High-level overview containing:
- Key findings organized by category
- Want type files listing by domain
- Key patterns identified
- Critical design principles
- YAML configuration formats
- Extension steps overview
- Recent commits and changes
- Document file descriptions

**Best for**: Quick overview, understanding context, reference summaries

## Quick Start

### For Understanding the System
1. Read WANT_TYPE_ANALYSIS_SUMMARY.txt first (5 minutes)
2. Review section 2 of WANT_TYPE_SYSTEM_ANALYSIS.md (10 minutes)
3. Read section 14 of WANT_TYPE_SYSTEM_ANALYSIS.md for extension guide (10 minutes)

### For Implementing a New Want Type
1. Use WANT_TYPE_QUICK_REFERENCE.md Constructor Template
2. Follow the Exec Implementation Template
3. Use Parameter Extraction code snippets
4. Reference Connectivity Pattern for your want type
5. Check Testing Checklist before finalizing

### For Understanding Specific Topics
- **Type Hierarchy**: Section 1 of WANT_TYPE_SYSTEM_ANALYSIS.md
- **State Management**: Section 8 of WANT_TYPE_SYSTEM_ANALYSIS.md
- **YAML Configuration**: Section 9 of WANT_TYPE_SYSTEM_ANALYSIS.md
- **Connectivity Patterns**: Section 10 of WANT_TYPE_SYSTEM_ANALYSIS.md
- **Parameters**: Section 5 of WANT_TYPE_SYSTEM_ANALYSIS.md

## Key Concepts at a Glance

### Core Types
```
Metadata        -> Want identification and classification
WantSpec        -> Want configuration (params, connections)
Want            -> Base type with state, history, agent support
```

### Factory Pattern
```
Type Registration: builder.RegisterWantType("type_name", NewWantConstructor)
Type Lookup:       factory = registry["type_name"]
Type Creation:     instance = factory(metadata, spec)
Type Assertion:    want := instance.(*WantType)
```

### Want Structure
```go
type MyWant struct {
    Want                           // Embedded base type
    DomainSpecificField string     // Custom fields
    // ...
}
```

### Constructor Signature
```go
func NewMyWant(metadata Metadata, spec WantSpec) interface{} {
    want := &MyWant{Want: Want{}}
    want.Init(metadata, spec)
    // Extract parameters
    // Set WantType and ConnectivityMetadata
    return want
}
```

### Five Architectural Patterns

1. **Generator** (source): RequiredInputs=0, RequiredOutputs=1
2. **Processor** (filter): RequiredInputs=1, RequiredOutputs=1
3. **Sink** (collector): RequiredInputs=1, RequiredOutputs=0
4. **Coordinator** (hub): RequiredInputs=3+, RequiredOutputs=0
5. **Independent** (parallel): No inputs, coordinated by hub

## Want Type Files by Domain

### Travel Domain
- RestaurantWant, HotelWant, BuffetWant, FlightWant, TravelCoordinatorWant
- File: `/engine/cmd/types/travel_types.go`

### Queue/Network Domain
- Numbers (generator), Queue (processor), Combiner, Sink
- File: `/engine/cmd/types/qnet_types.go`

### Mathematical Domain
- FibonacciNumbers, FibonacciSequence
- File: `/engine/cmd/types/fibonacci_types.go`
- PrimeNumbers, PrimeSequence, PrimeSink
- File: `/engine/cmd/types/prime_types.go`

### System Domain
- OwnerWant (dynamic want creation)
- File: `/engine/src/owner_types.go`
- CustomTargetWant (recipe-based types)
- File: `/engine/src/custom_target_types.go`

## Common Implementation Checklist

- [ ] Define struct with Want embedded
- [ ] Implement constructor: (Metadata, WantSpec) interface{}
- [ ] Call Init(metadata, spec) in constructor
- [ ] Extract parameters from spec.Params
- [ ] Set WantType and ConnectivityMetadata
- [ ] Return interface{}
- [ ] Implement Exec(using []Chan, outputs []Chan) bool
- [ ] Implement GetWant() *Want
- [ ] Use StoreState/GetState for state management
- [ ] Read parameters fresh in Exec()
- [ ] Create registration function
- [ ] Register types with builder
- [ ] Define YAML configuration

## Key Design Principles

1. **Embedding**: All types embed Want base type
2. **Initialization**: Create -> Init -> Extract -> Configure -> Return
3. **Parameters**: Extract in constructor, re-read in Exec
4. **State**: Only via StoreState/GetState (never direct access)
5. **Connectivity**: Label-based want-to-want connections
6. **Thread Safety**: Mutex protection on state access
7. **Persistence**: State survives across execution cycles
8. **History**: Automatic differential tracking of changes
9. **Flexibility**: Support for dynamic parameter changes
10. **Type Safety**: Factory pattern with type assertions

## State Management Summary

- **Storage**: `want.StoreState(key, value)` (thread-safe)
- **Retrieval**: `value, exists := want.GetState(key)` (thread-safe)
- **Batching**: `BeginExecCycle()` / `EndExecCycle()`
- **Persistent**: State survives across cycles
- **Thread-Safe**: Mutex-protected internally

## Recent Changes

- **Commit 97a0952**: Init() method for base initialization
- **Commit 540356f**: Constructors return interface{}
- **Commit 5df1758**: Standardize (Metadata, WantSpec) signature
- **Commit 6210e24**: Type assertion for returns
- **Commit ec9a9d0**: Standardize all 19 constructors

See WANT_TYPE_SYSTEM_ANALYSIS.md Section 11 for details.

## Document Statistics

| Document | Lines | Size | Focus |
|----------|-------|------|-------|
| WANT_TYPE_SYSTEM_ANALYSIS.md | 854 | 24KB | Comprehensive analysis |
| WANT_TYPE_QUICK_REFERENCE.md | 357 | 8.4KB | Templates and quick lookups |
| WANT_TYPE_ANALYSIS_SUMMARY.txt | 318 | 11KB | Executive summary |
| **Total** | **1,529** | **43.4KB** | Complete documentation |

## Document Usage Statistics

- **Comprehensive Read**: ~60 minutes (all documents in order)
- **Quick Reference Only**: ~10 minutes
- **Summary Only**: ~5 minutes
- **Template Copy-Paste**: ~20 minutes
- **Topic Search**: Variable (use Contents sections)

## Additional Resources

For implementation, also refer to:
- `/engine/src/want.go` - Core Want type definition
- `/engine/src/chain_builder.go` - ChainBuilder and registration
- `/engine/cmd/types/travel_types.go` - Reference implementations (5 types)
- `/engine/cmd/types/qnet_types.go` - Reference implementations (4 types)
- `/config/config-*.yaml` - Configuration examples
- `/recipes/*.yaml` - Recipe examples

## Support for YAML-Based Want Type Definition System

These documents are prepared as foundational analysis for implementing a YAML-based want type definition system. The current Go implementation patterns are fully documented to enable:

1. Code generation from YAML type definitions
2. Custom type registration from configuration
3. Runtime want type creation
4. Dynamic topology composition

All necessary patterns, signatures, and design principles are documented for this future enhancement.

## Questions or Issues?

Refer to the appropriate section in the documentation:
- **"How do I create a new want type?"** -> WANT_TYPE_QUICK_REFERENCE.md Constructor Template
- **"What are the connectivity requirements?"** -> WANT_TYPE_SYSTEM_ANALYSIS.md Section 7 & 10
- **"How does state management work?"** -> WANT_TYPE_SYSTEM_ANALYSIS.md Section 8
- **"What are all the want types?"** -> WANT_TYPE_SYSTEM_ANALYSIS.md Section 15
- **"How do parameters work?"** -> WANT_TYPE_SYSTEM_ANALYSIS.md Section 5
- **"What's the factory pattern?"** -> WANT_TYPE_SYSTEM_ANALYSIS.md Section 2
- **"How do I register a type?"** -> WANT_TYPE_SYSTEM_ANALYSIS.md Section 4

---

Generated: November 12, 2025
Analysis Version: 1.0
