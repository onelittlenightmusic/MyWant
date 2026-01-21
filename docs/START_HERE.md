# MyWant Want Type System - Documentation Index

Start here for navigation through the want type definition system analysis.

## Quick Navigation

### I just want a quick overview (5 minutes)
Read: **WANT_TYPE_ANALYSIS_SUMMARY.txt**

### I want to implement a new want type (30 minutes)
1. Read: **WANT_TYPE_QUICK_REFERENCE.md** (templates section)
2. Follow the Constructor Template
3. Use parameter extraction snippets
4. Reference connectivity patterns

### I want to understand the complete architecture (60 minutes)
Read: **WANT_TYPE_SYSTEM_ANALYSIS.md** (all sections)

### I need help with a specific topic
See: **WANT_TYPE_DOCUMENTATION_README.md** (Questions section)

## Document Directory

| Document | Purpose | Time | Best For |
|----------|---------|------|----------|
| **WANT_TYPE_DOCUMENTATION_README.md** | Navigation & guide | 10 min | Getting oriented |
| **WANT_TYPE_SYSTEM_ANALYSIS.md** | Deep technical dive | 60 min | Understanding architecture |
| **WANT_TYPE_QUICK_REFERENCE.md** | Templates & snippets | 10 min | Implementing new types |
| **WANT_TYPE_ANALYSIS_SUMMARY.txt** | Executive summary | 5 min | Quick overview |

## Key Concepts (1 minute)

The want type system consists of:

1. **Core Types**: Metadata (ID), WantSpec (config), Want (base)
2. **Factory Pattern**: Register string type -> factory function
3. **Constructor**: (Metadata, WantSpec) -> interface{}
4. **State**: Thread-safe storage with StoreState/GetState
5. **Connectivity**: Label-based want-to-want connections
6. **YAML Configuration**: Declarative syntax for want definitions

## Common Tasks

**Task: Create a new want type**
- See: WANT_TYPE_QUICK_REFERENCE.md "Constructor Template"
- Time: 30 minutes
- Steps: 6

**Task: Understand how want types work**
- See: WANT_TYPE_SYSTEM_ANALYSIS.md "Section 3"
- Time: 20 minutes
- Examples: 3

**Task: Register a want type**
- See: WANT_TYPE_SYSTEM_ANALYSIS.md "Section 4"
- Time: 10 minutes
- Examples: 3

**Task: Handle parameters correctly**
- See: WANT_TYPE_QUICK_REFERENCE.md "Parameter Type Patterns"
- Time: 5 minutes
- Examples: 4

**Task: Manage want state**
- See: WANT_TYPE_QUICK_REFERENCE.md "State Management"
- Time: 10 minutes
- Examples: 3

**Task: Connect wants together**
- See: WANT_TYPE_QUICK_REFERENCE.md "Connectivity Patterns"
- Time: 10 minutes
- Examples: 4

## Architecture Overview

```
Want Type System
├── Core Types
│   ├── Metadata (identification)
│   ├── WantSpec (configuration)
│   └── Want (base type with state, history)
├── Factory Pattern
│   ├── Registration (type name -> factory function)
│   ├── Creation (factory returns interface{})
│   └── Assertion (cast interface{} to concrete type)
├── Want Types (16+)
│   ├── Travel domain (5 types)
│   ├── Queue/Network domain (4 types)
│   ├── Math domain (5 types)
│   └── System domain (2 types)
├── State Management
│   ├── StoreState/GetState (thread-safe)
│   ├── Batching (BeginProgressCycle/EndProgressCycle)
│   └── History (differential tracking)
├── Connectivity
│   ├── Label-based selectors
│   ├── ConnectivityMetadata
│   └── 5 architectural patterns
└── YAML Configuration
    ├── Config files (instance-level)
    └── Recipe files (template-level)
```

## 5-Step Implementation Guide

1. **Define Type**
   ```go
   type MyWant struct { Want; CustomField string }
   ```

2. **Create Constructor**
   ```go
   func NewMyWant(metadata Metadata, spec WantSpec) interface{}
   ```

3. **Implement Exec**
   ```go
   func (w *MyWant) Exec(using []Chan, outputs []Chan) bool
   ```

4. **Register Type**
   ```go
   builder.RegisterWantType("my_type", NewMyWant)
   ```

5. **Use in YAML**
   ```yaml
   wants:
     - metadata:
         type: my_type
   ```

See WANT_TYPE_QUICK_REFERENCE.md for complete templates.

## File Structure

```
MyWant/
├── START_HERE.md (this file)
├── WANT_TYPE_DOCUMENTATION_README.md (navigation guide)
├── WANT_TYPE_SYSTEM_ANALYSIS.md (complete reference)
├── WANT_TYPE_QUICK_REFERENCE.md (templates & snippets)
├── WANT_TYPE_ANALYSIS_SUMMARY.txt (executive summary)
├── engine/
│   ├── src/
│   │   ├── want.go (core Want type)
│   │   └── chain_builder.go (factory system)
│   └── cmd/types/
│       ├── travel_types.go (5 want types)
│       ├── qnet_types.go (4 want types)
│       ├── fibonacci_types.go (2 want types)
│       └── prime_types.go (3 want types)
├── yaml/config/ (YAML config files)
└── yaml/recipes/ (YAML recipe files)
```

## Recent Changes

- **Nov 2024**: Standardized all want constructors to (Metadata, WantSpec) pattern
- **Recent commits**: Added Init() method, type assertions, interface{} returns

See WANT_TYPE_SYSTEM_ANALYSIS.md Section 11 for details.

## Support for Future Features

These documents provide foundation for:
- YAML-based want type definitions
- Code generation from type definitions
- Runtime want type creation
- Custom type registration from config
- Plugin-based want type system

All patterns and signatures are documented.

## Frequently Asked Questions

**Q: How do I create a new want type?**
A: See WANT_TYPE_QUICK_REFERENCE.md "Constructor Template" (5 steps)

**Q: How do parameters work?**
A: See WANT_TYPE_SYSTEM_ANALYSIS.md Section 5 (two-phase reading)

**Q: How is state managed?**
A: See WANT_TYPE_SYSTEM_ANALYSIS.md Section 8 (thread-safe storage)

**Q: How do wants connect together?**
A: See WANT_TYPE_SYSTEM_ANALYSIS.md Section 7 (label-based selectors)

**Q: What are the 5 want patterns?**
A: Generator, Processor, Sink, Coordinator, Independent (see Section 10)

**Q: How do I register a type?**
A: See WANT_TYPE_SYSTEM_ANALYSIS.md Section 4 (RegisterWantType)

**Q: Where are all the want types?**
A: See WANT_TYPE_ANALYSIS_SUMMARY.txt Section 3 (files by domain)

**Q: How does YAML configuration work?**
A: See WANT_TYPE_SYSTEM_ANALYSIS.md Section 9 (config & recipe files)

## Next Steps

1. **5-minute read**: Read WANT_TYPE_ANALYSIS_SUMMARY.txt
2. **10-minute reference**: Bookmark WANT_TYPE_QUICK_REFERENCE.md
3. **30-minute implementation**: Follow templates and checklist
4. **60-minute deep-dive**: Read WANT_TYPE_SYSTEM_ANALYSIS.md

Choose your level and start reading!

---

Generated: November 12, 2025
Documentation Version: 1.0
Total Pages: 4 documents, 1,900+ lines, 52 KB
