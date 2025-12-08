# Exec() Method Call Flow - Search Results Index

## Overview

Complete search analysis of how the `Exec()` method is called for wants and how the "using" input channels are populated from both local `using` and global `usingGlobal` selectors.

## Quick Answers

### Question 1: Where is Exec() called?
**Answer**: `chain_builder.go:1630` in `startWant()` function, called repeatedly in a loop within a goroutine.

### Question 2: How is "using" parameter populated?
**Answer**: Both local `using` selectors AND `usingGlobal` selectors are converted to channels and added to the same `usingChans []chain.Chan` array. Order:
1. First all local using connections
2. Then all global using connections

### Question 3: How are paths converted to channels?
**Answer**: Three-step process:
1. `generatePathsFromConnections()` creates PathInfo objects for each connection
2. Channels created in `cb.channels` map indexed by path name
3. `startWant()` looks up channels from map and appends to usingChans

### Question 4: Does Level 2 coordinator receive usingGlobal channels?
**Answer**: **YES** - Level 2 coordinators receive both local and global connections in the same `using` parameter array with no distinction or special handling.

## Documentation Files

### 1. SEARCH_FINDINGS.md (7.8 KB)
**Best for**: Comprehensive technical explanation with code examples

Contents:
- Executive summary
- Detailed answers to all 4 questions
- Technical implementation details
- Configuration examples
- Execution cycle flow diagrams
- Data flow examples

**Start here if**: You want complete technical documentation with all code context

### 2. EXEC_FLOW_ANALYSIS.md (3.8 KB)
**Best for**: Quick reference and executive overview

Contents:
- Brief overview of architecture
- Key findings summary
- Code reference table mapping concepts to file locations
- Summary of findings

**Start here if**: You want a quick summary and reference table

### 3. EXEC_FLOW_VISUAL_GUIDE.txt (18 KB)
**Best for**: Understanding the complete flow with visual diagrams

Contents:
- 6 detailed phases with ASCII diagrams
- Phase 1: Configuration specification
- Phase 2: Path generation with line-by-line walkthrough
- Phase 3: Channel creation
- Phase 4: Want execution with goroutine setup
- Phase 5: Execution inside Exec() method
- Phase 6: State batching
- Data flow timeline
- Key architectural insights
- Final Q&A summary

**Start here if**: You prefer visual flowcharts and step-by-step walkthroughs

## Key Code Locations

| Concept | File | Lines | Details |
|---------|------|-------|---------|
| Executable Interface | declarative.go | 117-120 | Method signature for Exec() |
| PathInfo/Paths Types | declarative.go | 149-159 | Data structures for paths |
| generatePathsFromConnections() | chain_builder.go | 156-288 | Main path generation function |
| Local using processing | chain_builder.go | 172-230 | Process want.spec.Using |
| Global using processing | chain_builder.go | 232-283 | Process want.spec.UsingGlobal |
| Channel creation | chain_builder.go | 663-686 | Create actual channel objects |
| startWant() function | chain_builder.go | 1569-1660 | Want execution goroutine |
| Exec() call site | chain_builder.go | 1630 | Actual Exec() call location |
| Execution cycle wrapping | chain_builder.go | 1626-1637 | BeginExecCycle/EndExecCycle |
| Level 2 coordinator Exec | approval_types.go | 464-540 | How coordinator processes inputs |
| State batching | want.go | 166-206, 354-422 | BeginExecCycle, StoreState, EndExecCycle |

## Data Structures

### PathInfo
```go
type PathInfo struct {
    Channel chan interface{}    // Actual channel object
    Name    string              // "provider_to_consumer" or "provider_global_to_consumer"
    Active  bool                // Include in execution
}
```

### Paths
```go
type Paths struct {
    In  []PathInfo   // Input connections (both local and global mixed)
    Out []PathInfo   // Output connections
}
```

### Executable Interface
```go
type ChainWant interface {
    Exec(using []chain.Chan, outputs []chain.Chan) bool
    GetWant() *Want
}
```

## Execution Flow Overview

```
1. Configuration Phase
   └─ Want defines Using[] and UsingGlobal[] selectors

2. Path Generation Phase (generatePathsFromConnections)
   ├─ Process local using selectors → create PathInfo objects
   └─ Process global using selectors → add to SAME paths.In[]

3. Channel Creation Phase
   ├─ Store PathInfo in pathMap
   └─ Create actual channel objects in cb.channels map

4. Want Execution Phase (startWant)
   ├─ Retrieve paths from pathMap
   ├─ Build usingChans by looking up channels in cb.channels
   └─ Launch goroutine with Exec() loop

5. Execution Phase
   ├─ BeginExecCycle() - initialize state batching
   ├─ Exec(usingChans, outputChans) - main logic
   ├─ EndExecCycle() - commit batched state changes
   └─ Repeat until finished

6. Inside Exec()
   ├─ Read from all usingChans (no distinction)
   ├─ Process data from any source
   ├─ Call StoreState() to batch changes
   └─ Return finished flag
```

## Key Architectural Insights

### Unified Channel Handling
The system treats local and global connections identically:

```
Want Specification
    ├─ Using[]        (local selectors)
    └─ UsingGlobal[]  (global selectors)
              ↓
    generatePathsFromConnections()
              ↓
    paths.In[] (SAME ARRAY for both)
              ↓
    cb.channels map (SAME MAP for both)
              ↓
    usingChans (SAME PARAMETER for both)
              ↓
    Exec() has no distinction
```

This unified approach enables:
- Flexible cross-recipe connectivity
- Simple, uniform execution logic
- Transparent data aggregation from multiple sources
- No special coordinator handling needed

### Channel Naming Convention
- Local paths: `"{provider}_to_{consumer}"`
- Global paths: `"{provider}_global_to_{consumer}"`

The "_global" suffix is purely informational - the execution treats them identically.

### State Batching Optimization
During `Exec()`, state changes are batched for efficiency:

1. **BeginExecCycle()**: Initialize `pendingStateChanges` map, set `inExecCycle = true`
2. **During Exec()**: `StoreState()` calls append to `pendingStateChanges` without locks
3. **EndExecCycle()**: Apply batched changes to `State`, create history entries

This minimizes lock contention during execution.

## Common Use Cases

### Scenario 1: Single Source
Want receives from one provider:
```yaml
Using:
  - role: "provider"
```
Result: `usingChans = [provider_channel]`

### Scenario 2: Local Multi-Source
Want receives from multiple providers in same recipe:
```yaml
Using:
  - role: "evidence-provider"
  - role: "description-provider"
```
Result: `usingChans = [evidence_channel, description_channel]`

### Scenario 3: Cross-Recipe Connections
Want receives from providers in other recipes:
```yaml
UsingGlobal:
  - approval_id: "A123"
```
Result: `usingChans = [provider1_global_channel, provider2_global_channel, ...]`

### Scenario 4: Mixed Connections (Level 2 Coordinator)
Want receives from both local and global sources:
```yaml
Using:
  - role: "evidence-provider"
  - role: "description-provider"
UsingGlobal:
  - approval_id: "A123"
```
Result: `usingChans = [evidence, description, provider1_global, provider2_global, ...]`

## FAQ

**Q: Can we distinguish between local and global channels in Exec()?**
A: No. Both are passed in the same `using []chain.Chan` array. The want has no way to tell which channel came from where. This is by design - it allows flexible topology changes without modifying want code.

**Q: What order are channels in?**
A: Local using connections first, then global using connections, in the order they appear in the specification.

**Q: Do we need separate handling for Level 2 coordinators?**
A: No. Level 2 coordinators are identified by type but processed identically to other wants. The same Exec() interface is used.

**Q: Can a want have both Using and UsingGlobal from the same source?**
A: Yes, but it would create duplicate channels. The system doesn't deduplicate - both would appear in usingChans.

**Q: What if a Using selector matches multiple wants?**
A: Each matched want creates a separate PathInfo and channel. The consumer's usingChans array will have multiple channels.

## See Also

- **CLAUDE.md**: Project overview and architecture guide
- **AUTOCONNECT_CODE_REFERENCE.md**: Auto-connection logic for coordinators
- Approval system configuration files:
  - `config/config-hierarchical-approval.yaml`
  - `recipes/approval-level-1.yaml`

## Document Generation Info

These documents were generated by analyzing:
1. `chain_builder.go` - Main execution engine
2. `declarative.go` - Type definitions and interfaces
3. `approval_types.go` - Coordinator implementations
4. `want.go` - State management and execution cycles

Total lines of code analyzed: ~3000+ lines
Total documentation pages: 30+ pages
Total diagrams: 15+ visual flowcharts

---

**Last Updated**: November 8, 2025
**Status**: Complete - All questions answered and documented
