# Search Results: Exec() Method Call Flow

## Executive Summary

This search analyzed how the `Exec()` method is called for wants and how input channels are populated from both local `using` and global `usingGlobal` selectors.

**Key Finding**: Both local and global connections are processed identically and combined into a single `usingChans []chain.Chan` array passed to `Exec()`. There is no distinction between them in the execution method.

---

## Questions Answered

### 1. Where is Exec() called?

**Location**: `chain_builder.go:1630`

The `Exec()` method is called in the `startWant()` function at line 1630:

```go
func (cb *ChainBuilder) startWant(wantName string, want *runtimeWant) {
    // ... setup code ...
    
    go func() {
        for {
            runtimeWant.want.BeginExecCycle()
            finished := chainWant.Exec(usingChans, outputChans)  // LINE 1630
            runtimeWant.want.EndExecCycle()
            if finished { break }
        }
    }()
}
```

The call is in a loop within a goroutine, executed repeatedly until `finished` is true.

---

### 2. How is the "using" parameter populated?

**Answer**: By combining BOTH local and global connections in a single array.

**Process**:

1. **Path Generation** (`generatePathsFromConnections()`)
   - Local using selectors create paths (lines 172-230)
   - Global using selectors create paths (lines 232-283)
   - Both add to the **same** `paths.In[]` array

2. **Channel Preparation** (`startWant()`)
   - All paths in `paths.In` are iterated (line 1580)
   - For each active path, the channel is looked up in `cb.channels` map
   - All channels added to `usingChans []chain.Chan`

**Order of channels**:
- First: All local using connections
- Then: All global using connections

**Example**:
```yaml
Using:
  - role: "evidence-provider"    # Creates path at In[0]
  - role: "description-provider" # Creates path at In[1]

UsingGlobal:
  - approval_id: "A123"          # Creates path at In[2]
```

Results in: `usingChans = [ch0, ch1, ch2]` (no distinction)

---

### 3. How are paths converted to channels?

**Three-step process**:

**Step 1: Path Creation** (chain_builder.go:199-203, 254-258)
```go
inPath := PathInfo{
    Channel: make(chan interface{}, 10),
    Name:    fmt.Sprintf("%s_to_%s", provider, consumer),  // Local
    // OR
    Name:    fmt.Sprintf("%s_global_to_%s", provider, consumer),  // Global
    Active:  true,
}
paths.In = append(paths.In, inPath)
```

**Step 2: Channel Creation** (chain_builder.go:670-683)
```go
cb.channels = make(map[string]chain.Chan)
for _, paths := range cb.pathMap {
    for _, outputPath := range paths.Out {
        if outputPath.Active {
            cb.channels[outputPath.Name] = make(chan interface{}, 10)
        }
    }
}
```

**Step 3: Channel Lookup** (chain_builder.go:1578-1589)
```go
for _, usingPath := range paths.In {
    if usingPath.Active {
        channelKey := usingPath.Name
        ch := cb.channels[channelKey]  // Map lookup
        usingChans = append(usingChans, ch)
    }
}
```

**Conversion flow**:
```
PathInfo.Name (string) → cb.channels[Name] lookup → channel object → append to usingChans
```

---

### 4. Does Level 2 coordinator receive usingGlobal channels?

**YES** - Level 2 coordinators receive input channels from both sources.

**Evidence**:

**Location**: `approval_types.go:464-540` (Level2CoordinatorWant.Exec)

```go
func (l *Level2CoordinatorWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Process ALL input channels identically
    for _, input := range using {  // Both local and global mixed here
        select {
        case data := <-input:
            // ... process data from any source ...
        default:
        }
    }
    // No distinction between local and global sources
}
```

**No special handling**: The coordinator simply iterates all channels in the `using` array with no knowledge of whether they came from local or global connections.

**Configuration example**:
```yaml
level2_coordinator:
  spec:
    Using:           # Local connections
      - role: "evidence-provider"
      - role: "description-provider"
    UsingGlobal:     # Global connections
      - approval_id: "A123"  # Cross-recipe
```

Results in: `using []chain.Chan` with 3+ channels, all processed identically.

---

## Technical Details

### Path Processing in generatePathsFromConnections()

**Lines 172-230: Local using selectors**
```go
for _, usingSelector := range want.spec.Using {
    var matchedWants []*runtimeWant
    for _, w := range cb.wants {
        if cb.matchesSelector(w.metadata.Labels, usingSelector) {
            matchedWants = append(matchedWants, w)
        }
    }
    for _, matchedWant := range matchedWants {
        inPath := PathInfo{
            Channel: make(chan interface{}, 10),
            Name:    fmt.Sprintf("%s_to_%s", matchedName, wantName),
            Active:  true,
        }
        paths.In = append(paths.In, inPath)  // Added to same array
    }
}
```

**Lines 232-283: Global using selectors**
```go
for _, globalSelector := range want.spec.UsingGlobal {
    matchedWants := ResolveWantsUsingGlobalIndex(cb, globalSelector)
    for _, matchedWant := range matchedWants {
        inPath := PathInfo{
            Channel: make(chan interface{}, 10),
            Name:    fmt.Sprintf("%s_global_to_%s", matchedName, wantName),
            Active:  true,
        }
        paths.In = append(paths.In, inPath)  // SAME ARRAY!
    }
}
```

**Key Observation**: The only difference is the path naming convention. Both are added to `paths.In[]` in the same loop structure.

---

## Execution Cycle Flow

Each execution cycle wraps `Exec()` with state batching:

```
[1] BeginExecCycle()
    ├─ Set inExecCycle = true
    └─ Initialize pendingStateChanges = {}

[2] Exec(usingChans, outputChans)
    ├─ Read from usingChans (all sources)
    ├─ Process data
    └─ Call StoreState() → batches changes

[3] EndExecCycle()
    ├─ Apply pendingStateChanges to State
    ├─ Create state history entries
    └─ Set inExecCycle = false

[4] if finished { break } else { goto [1] }
```

State changes made via `StoreState()` during `Exec()` are batched and not committed until `EndExecCycle()`.

---

## Data Flow Example

**Scenario**: Level 2 Coordinator receiving evidence and description

```
Time T₁:
  Evidence.Exec([...], [evidence_to_coordinator])
    └─ SendPacket(data, [evidence_to_coordinator])
        └─ Write to cb.channels["evidence_to_coordinator"]

Time T₂:
  Description.Exec([...], [description_global_to_coordinator])
    └─ SendPacket(data, [description_global_to_coordinator])
        └─ Write to cb.channels["description_global_to_coordinator"]

Time T₃:
  Level2Coordinator.Exec(
    [cb.channels["evidence_to_coordinator"],
     cb.channels["description_global_to_coordinator"]],
    outputChans
  )
    └─ Read from both channels
    └─ Process ApprovalData from either source
```

The coordinator has no way to distinguish which channel came from where.

---

## Files Modified During Search

No files were modified. This was a read-only analysis.

## Files Referenced

| File | Purpose |
|------|---------|
| chain_builder.go | Main execution logic, path generation, channel creation |
| declarative.go | Interface definitions, type definitions |
| approval_types.go | Level 2 coordinator implementation |
| want.go | State batching during execution cycle |

---

## Conclusion

The system treats local `using` and global `usingGlobal` connections identically in execution. Both are:
1. Processed in `generatePathsFromConnections()` into the same `Paths.In[]` array
2. Converted to channels and stored in the same `cb.channels` map
3. Collected into the same `usingChans []chain.Chan` array
4. Passed to `Exec()` with no distinction

This unified approach allows flexible topology building while keeping the execution logic simple and consistent.
