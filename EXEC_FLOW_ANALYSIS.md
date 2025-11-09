# Exec() Method Call Flow Analysis

## Overview
This document traces how the `Exec()` method is called for wants and how the "using" input channels are populated. The flow shows both local `using` selectors and `usingGlobal` connections are included in the same path construction phase.

---

## 1. ChainWant Interface Definition

**Location**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/declarative.go:117-120`

```go
type ChainWant interface {
	Exec(using []chain.Chan, outputs []chain.Chan) bool
	GetWant() *Want
}
```

The `Exec()` method takes:
- `using []chain.Chan` - Input channels for the want to read from
- `outputs []chain.Chan` - Output channels for the want to write to
- Returns `bool` - true if execution is finished, false if should continue

---

## 2. Path Construction Phase

### 2.1 Function: `generatePathsFromConnections()`

**Location**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/chain_builder.go:156-288`

This function creates `Paths` objects for all wants by processing both local `using` and `usingGlobal` connections.

**Key Data Structures**:

```go
// From declarative.go:149-159
type PathInfo struct {
	Channel chan interface{}
	Name    string
	Active  bool
}

type Paths struct {
	In  []PathInfo      // Input connections
	Out []PathInfo      // Output connections
}
```

### 2.2 Processing Order

For each want, the function processes:

1. **Local Using Selectors** (Lines 172-230)
   - Process `want.spec.Using` array
   - Match wants with matching labels using `matchesSelector()`
   - Create one `PathInfo` per matched want
   - Add to `paths.In`

2. **Global Using Selectors** (Lines 232-283)
   - Process `want.spec.UsingGlobal` array
   - Resolve wants using `ResolveWantsUsingGlobalIndex()`
   - Create one `PathInfo` per matched want
   - Add to `paths.In`

**Critical Finding**: Both local and global connections are added to the **same** `paths.In` array. They are not separated by type.

---

## 3. Summary of Findings

### Question 1: Where Exec() is called
**Answer**: In `startWant()` function at line 1630 of `chain_builder.go`, called in a loop within a goroutine for each execution cycle.

### Question 2: How "using" parameter is populated
**Answer**: Both local `using` selectors AND `usingGlobal` selectors are converted to channels and added to the same `usingChans` array. The order is:
1. First all local using connections (from `want.spec.Using`)
2. Then all global using connections (from `want.spec.UsingGlobal`)

### Question 3: How paths are converted to channels
**Answer**:
1. `generatePathsFromConnections()` creates `PathInfo` objects for each connection
2. Each `PathInfo` has a unique name like `"provider_to_consumer"` or `"provider_global_to_consumer"`
3. `cb.channels` map stores actual channel objects indexed by path name
4. In `startWant()`, we iterate `paths.In` and look up channels from `cb.channels` map using the path name

### Question 4: Level 2 coordinators and usingGlobal
**Answer**: **YES**, Level 2 coordinators receive input channels from both local and global connections via the `using` parameter. There is no special separation - they both appear in the same `usingChans []chain.Chan` array passed to `Exec()`.

---

## Code References

| Item | File | Lines |
|------|------|-------|
| ChainWant Interface | declarative.go | 117-120 |
| PathInfo/Paths Types | declarative.go | 149-159 |
| generatePathsFromConnections() | chain_builder.go | 156-288 |
| Local using processing | chain_builder.go | 172-230 |
| Global using processing | chain_builder.go | 232-283 |
| Channel creation | chain_builder.go | 663-686 |
| startWant() | chain_builder.go | 1569-1660 |
| Exec() call site | chain_builder.go | 1630 |
| Execution cycle wrapping | chain_builder.go | 1626-1637 |
| Level 2 coordinator Exec | approval_types.go | 464-540 |
| StoreState/batching | want.go | 166-206, 354-422 |
