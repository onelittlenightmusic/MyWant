# CLAUDE.md - Quick Reference

## Project Overview

MyWant is a Go library for functional chain programming with channels. Uses **recipe-based configuration** where config YAML files reference reusable recipe templates.

## Core Architecture

**Main Components:**
- `declarative.go` - Core Config/Want/Recipe types
- `recipe_loader_generic.go` - Generic recipe loading
- `*_types.go` - Want type implementations
- `engine/src/chain_builder.go` - Chain execution engine

**Key Types:**
- `Want` - Processing unit with Metadata, Spec, State, Status
- `WantSpec` - Config with Params, Using selectors, Recipe reference
- `ChainBuilder` - Builds and executes chains

**Want Categories:**
- **Independent** - Execute in parallel (travel planning: restaurant, hotel, buffet)
- **Dependent** - Pipeline via `using` selectors (queue: generator→queue→sink)
- **Coordinator** - Orchestrates other wants via input channels

## Essential Commands

**Server Management:**
```sh
make restart-all             # Start servers: MyWant (8080) + Mock Flight (8081)
make test-concurrent-deploy  # Test concurrent deployments with race detection
curl -s http://localhost:8080/api/v1/wants | jq '.wants | length'  # Check status
```

**Run Examples:**
- `make run-travel-recipe` - Independent wants (restaurant, hotel, buffet)
- `make run-queue-system-recipe` - Pipeline (generator→queue→sink)
- `make run-qnet-recipe` - Complex multi-stream system
- `make run-sample-owner` - Dynamic want creation

**Important**: Never use `make build-server` directly; `make restart-all` includes building.

## Key Patterns

**Recipe Structure:**
- **Independent wants** (no `using`) - Execute in parallel, need Coordinator
- **Dependent wants** (with `using`) - Form pipelines via label matching
- **Parameters** - Referenced by name, no Go templating

**State Management:**
- Use `StoreState(key, value)` to update state (batched during execution)
- Use `GetState(key)` to retrieve values
- State auto-persists across executions via memory reconciliation

**Dynamic Wants:**
```go
builder.AddDynamicNode(Want{...})  // Add single want
builder.AddDynamicNodes([]Want{})  // Add multiple
// Auto-connected via label selectors
```

## File Organization

- `config/` - Config YAML files (user interface)
- `recipes/` - Recipe templates (reusable components)
- `engine/src/` - Core system: declarative.go, chain_builder.go
- `engine/cmd/types/` - Want implementations: *_types.go files
- `engine/cmd/server/` - HTTP server and API endpoints

## Coding Rules

1. **Sleep timings**: Max 7 seconds in build/test
2. **Never** call `make build-server` directly - use `make restart-all`
3. Use `StoreState()` for all state updates (enables proper batching)
4. Use `GetState()` for all state reads (returns value, exists)
5. Initialize `StateHistory` before appending:
   ```go
   if want.History.StateHistory == nil {
       want.History.StateHistory = make([]StateHistoryEntry, 0)
   }
   ```

## Want Execution Lifecycle

1. **BeginProgressCycle()** - Start batching state changes
2. **Exec()** - Main execution (channel I/O)
3. **EndProgressCycle()** - Commit state changes and history

**Connectivity Requirements (`require` field):**
- `none` - No connections needed (default)
- `users` - Needs output connections (generators, independent wants)
- `providers` - Needs input connections (sinks, collectors)
- `providers_and_users` - Needs both (pipelines: queue, processors)

## Pending Improvements

- Frontend recipe cards need deploy button for direct recipe launch
- Replace direct `State` field access with `GetState()` everywhere
- Consider async rebooking in dynamic travel changes
- Logs location: `logs/mywant-backend.log`

## RAG Database (Code Search)

**Quick Usage:**
```bash
# Interactive search
python3 tools/codebase_rag.py

# Python API
from tools.codebase_rag import CodebaseRAG
rag = CodebaseRAG('codebase_rag.db')
results = rag.search("ChainBuilder", entity_types=['struct'])
rag.close()
```

**Auto-sync**: RAG database auto-updates via post-commit hook. Just commit normally.

**Resources**: See `tools/README_RAG.md` and `QUICKSTART_RAG.md` for details.

## Completed Tasks (2025-12-13)

### UI Improvements
✅ **Fixed LabelSelectorAutocomplete keyboard navigation** (Commit 54e8484)
- Added arrow key (Up/Down) navigation through dropdown options
- Tab key now confirms selection and moves to next field
- Enter key confirms selection from dropdown
- Visual highlight (blue background) for selected option
- Auto-focus management for smooth workflow
- Full keyboard accessibility for dependency selector

### Coordinator System Refactoring
✅ **Verified Coordinator Unification**
- Confirmed all recipes use unified `type: coordinator`
- Generic `CoordinatorWant` handles all variations through handler interfaces
- Approval and Travel coordinators fully unified in codebase

✅ **Removed Legacy Backward-Compatibility Code** (Commit 3db770d)
- Removed checks for old type names: `level1_coordinator`, `level2_coordinator`, `buffet_coordinator`
- Simplified `getCoordinatorConfig()` logic
- Current implementation relies on parameters: `coordinator_type`, `coordinator_level`, `is_buffet`

## Pending Tasks
None - all major refactoring complete!