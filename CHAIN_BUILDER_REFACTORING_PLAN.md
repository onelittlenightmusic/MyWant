# chain_builder.go Refactoring Plan

## Executive Summary
Refactor the monolithic `chain_builder.go` (2,939 lines, 101 functions) into 10 focused files following the Single Responsibility Principle while maintaining full backward compatibility with the public API.

## Current State Analysis
- **File Size**: 2,939 lines
- **Function Count**: 101 functions
- **Responsibilities**: 9 distinct functional domains mixed in one file
- **Cognitive Load**: Very high due to size and complexity
- **Maintenance Cost**: High - difficult to navigate, understand, and modify

## Proposed File Structure

### 1. chain_builder_core.go (~400 lines)
**Responsibility**: Core ChainBuilder lifecycle and public API

**Components**:
- `type ChainBuilder` struct definition
- `type runtimeWant` struct + `GetSpec()`, `GetMetadata()`
- Constructor: `NewChainBuilder()`, `NewChainBuilderWithPaths()`
- Configuration setters: `RegisterWantType()`, `SetAgentRegistry()`, `SetCustomTargetRegistry()`, `SetConfigInternal()`, `SetServerMode()`
- Public query methods: `FindWantByID()`, `GetAllWantStates()`, `IsRunning()`, `IsSuspended()`

**Purpose**: Entry point and primary public interface. Keeps initialization and core configuration in one place.

---

### 2. chain_builder_execution.go (~500 lines)
**Responsibility**: Want execution orchestration and lifecycle

**Components**:
- `Execute()`, `ExecuteWithMode()` - main execution
- `startWant()`, `startPhase()` - execution phases
- `writeStatsToMemory()`, `dumpWantMemoryToYAML()` - persistence
- `registerWantForNotifications()` - notification setup
- `SuspendWant()`, `ResumeWant()`, `StopWant()`, `RestartWant()` - want control
- `Suspend()`, `Resume()`, `Stop()`, `Start()` - global control
- `SendControlCommand()`, `TriggerReconcile()` - signaling

**Purpose**: Manage want execution lifecycle including startup, control, and monitoring.

---

### 3. chain_builder_reconciliation.go (~700 lines)
**Responsibility**: Configuration reconciliation and change detection

**Components**:
- Reconciliation loop: `reconcileLoop()`, `reconcileWants()`
- Phases: `compilePhase()`, `connectPhase()`
- Change handling: `detectConfigChanges()`, `applyWantChanges()`, `shouldRestartCompletedWant()`
- Comparison: `wantsEqual()`, `mapsEqual()`
- Utilities: `deepCopyConfig()`, `copyStringMap()`, `copyInterfaceMap()`, `copyUsing()`, `copyStringSlice()`, `copyStateSubscriptions()`, `copyNotificationFilters()`

**Purpose**: Handle configuration changes, reconciliation loop, and state synchronization.

---

### 4. chain_builder_topology.go (~400 lines)
**Responsibility**: Path generation, connectivity, and want topology

**Components**:
- Path generation: `generatePathsFromConnections()`
- Validation: `validateConnections()`, `isConnectivitySatisfied()`, `matchesSelector()`
- Auto-connection: `processAutoConnections()`, `autoConnectWant()`, `hasRecipeAgent()`
- Utilities: `addConnectionLabel()`, `generateConnectionKey()`
- Target support: `buildTargetParameterSubscriptions()`

**Purpose**: Manage want connectivity, path generation, and topology validation.

---

### 5. chain_builder_want_creation.go (~300 lines)
**Responsibility**: Want instantiation and factory functions

**Components**:
- Want creation: `createWantFunction()`, `TestCreateWantFunction()`
- Custom targets: `createCustomTargetWant()`, `mergeWithCustomDefaults()`
- Want management: `addWant()`, `addDynamicWantUnsafe()`, `deleteWant()`, `DeleteWantByID()`, `UpdateWant()`
- Ordering: `sortChangesByDependency()`, `calculateDependencyLevels()`, `calculateDependencyLevel()`

**Purpose**: Factory functions for creating, updating, and managing want instances.

---

### 6. chain_builder_config_io.go (~350 lines)
**Responsibility**: Configuration file I/O and validation

**Components**:
- File operations: `copyConfigToMemory()`, `loadMemoryConfig()`
- File hashing: `calculateFileHash()`, `hasMemoryFileChanged()`, `hasConfigFileChanged()`
- Config loading: `LoadConfigFromYAML()`, `LoadConfigFromYAMLBytes()`, `loadConfigFromYAML()`, `loadConfigFromYAMLBytes()`
- Validation: `validateConfigWithSpec()`, `validateWantsStructure()`
- ID management: `generateUUID()`, `assignWantIDs()`

**Purpose**: Handle YAML loading, validation, and file I/O operations. Foundation layer with no dependencies.

---

### 7. chain_builder_retrigger.go (~200 lines)
**Responsibility**: Completed want retrigger detection and handling

**Components**:
- Main logic: `checkAndRetriggerCompletedWants()`, `findUsersOfCompletedWant()`, `RetriggerReceiverWant()`
- Tracking: `MarkWantCompleted()`, `UpdateCompletedFlag()`, `IsCompleted()`, `TriggerCompletedWantRetriggerCheck()`

**Purpose**: Manage completed want retrigger detection and execution for cascading updates.

---

### 8. chain_builder_control.go (~150 lines)
**Responsibility**: Control loop and suspension management

**Components**:
- `controlLoop()` - suspension/resume control loop
- `startControlLoop()` - loop startup
- `distributeControlCommand()` - command distribution

**Purpose**: Manage suspension, resumption, and control signaling.

---

### 9. chain_builder_async.go (~100 lines)
**Responsibility**: Asynchronous want management API

**Components**:
- Async addition: `AddWantsAsync()`, `AddWantsAsyncWithTracking()`, `AreWantsAdded()`
- Async deletion: `DeleteWantsAsync()`, `DeleteWantsAsyncWithTracking()`, `AreWantsDeleted()`

**Purpose**: Non-blocking async API for want addition/deletion from executing wants.

---

### 10. chain_builder_global.go (~50 lines)
**Responsibility**: Global builder instance management

**Components**:
- `SetGlobalChainBuilder()`, `GetGlobalChainBuilder()`
- `var globalChainBuilder *ChainBuilder`

**Purpose**: Singleton global builder instance for server mode access.

---

## Dependency Graph

```
┌─────────────────────────────────────────┐
│  chain_builder_config_io.go             │ (foundation)
└──────────────────┬──────────────────────┘
                   │
    ┌──────────────┴──────────────┐
    │                             │
┌───▼────────────────┐    ┌──────▼─────────────┐
│ chain_builder_core │    │ chain_builder_     │
│                    │    │ topology.go        │
└────┬───────────────┘    └────────────────────┘
     │                              │
     ├──────────────┬───────────────┘
     │              │
┌────▼──────────────▼───────────────────────────┐
│ chain_builder_want_creation.go                │
└────┬─────────────────────────────────────────┘
     │
┌────▼──────────────────────────────────────────┐
│ chain_builder_reconciliation.go               │
└────┬─────────────────────────────────────────┘
     │
     ├──────────────┬──────────────┬────────────┐
     │              │              │            │
┌────▼──┐    ┌─────▼─┐     ┌──────▼──┐   ┌────▼──┐
│ exec  │    │ async │     │ control │   │retrig-│
│ .go   │    │ .go   │     │ .go     │   │ger.go │
└───────┘    └───────┘     └─────────┘   └───────┘

global.go (isolated)
```

## Benefits of Refactoring

1. **Reduced Cognitive Load**
   - From 2,939 lines in one file → max 700 lines per file
   - Each file has a single, clear responsibility
   - Easier to understand and reason about code

2. **Improved Maintainability**
   - Related functions grouped logically
   - Changes to one concern don't scatter across a huge file
   - Faster to locate and modify code

3. **Better Testing**
   - Each file can have focused unit tests
   - Easier to mock and test individual concerns
   - Clearer test organization

4. **Simplified Navigation**
   - IDE navigation tools work better with smaller files
   - Easier code review with focused changes
   - Reduced file diff noise

5. **No Breaking Changes**
   - All public methods preserved
   - API remains identical
   - Existing code continues to work unchanged

## Implementation Phases

### Phase 1: Foundation (1-2 days)
1. Create `chain_builder_config_io.go` - extract config I/O functions
2. Create `chain_builder_global.go` - extract global singleton
3. Verify imports work, no circular dependencies

### Phase 2: Core Layer (1-2 days)
4. Create `chain_builder_core.go` - extract types and constructors
5. Create `chain_builder_topology.go` - extract path/connectivity
6. Add type definitions and basic method receivers

### Phase 3: Main Logic (2-3 days)
7. Create `chain_builder_want_creation.go` - extract factory functions
8. Create `chain_builder_reconciliation.go` - extract reconciliation
9. Update cross-file imports

### Phase 4: Execution & Control (2-3 days)
10. Create `chain_builder_execution.go` - extract execution
11. Create `chain_builder_retrigger.go` - extract retrigger
12. Create `chain_builder_control.go` - extract control
13. Create `chain_builder_async.go` - extract async API

### Phase 5: Testing & Integration (1-2 days)
14. Run full test suite
15. Verify no regressions
16. Update documentation

## Backward Compatibility

**No breaking changes to public API**:
- All exported methods remain accessible
- Constructor functions unchanged
- All types accessible at same visibility level
- Existing imports continue to work

**Example - usage remains identical**:
```go
// Old code (still works)
import "mywant/engine/src"
builder := mywant.NewChainBuilder(config)
builder.RegisterWantType("custom", factory)
builder.Execute()
```

## Critical Success Factors

1. **No Circular Dependencies**
   - Foundation layer (config_io) has no dependencies
   - Clear one-directional dependency flow
   - Each layer only depends on lower layers

2. **Preserve Method Receivers**
   - All `(cb *ChainBuilder)` receivers stay with their functions
   - Methods remain on ChainBuilder type

3. **Complete Test Coverage**
   - Each file gets focused unit tests
   - Integration tests verify cross-file interactions
   - No regressions from refactoring

4. **Documentation**
   - Add file-level comments explaining responsibility
   - Update package documentation
   - Cross-reference related functions

## Success Metrics

**Before**:
- Single file: 2,939 lines
- Functions: 101
- Avg function distance: Very high

**After**:
- 10 files, 290 lines average
- Max file: 700 lines (reconciliation)
- Clear responsibility boundaries
- 100% test coverage preserved

## Status

This plan is ready for implementation. Start with Phase 1 (Foundation) to establish the base structure, then proceed through phases sequentially.
