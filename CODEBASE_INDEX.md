# MyWant Codebase Analysis - File Index

## Files Analyzed

### 1. Recipe Loader Files
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/src/recipe_loader_generic.go`**
  - 706 lines
  - GenericRecipeLoader class with recipe loading, validation, and custom type registration
  - Key methods: LoadRecipe(), ValidateRecipe(), ListRecipes(), ScanAndRegisterCustomTypes()

- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/src/recipe_loader.go`**
  - 490 lines
  - Legacy RecipeLoader for owner-based recipe instantiation
  - Key methods: LoadRecipes(), InstantiateRecipe(), GetRecipe()

### 2. Want Type Implementation Files
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/travel_types.go`**
  - 5 want types: flight, restaurant, hotel, buffet, travel_coordinator
  - Registration: RegisterTravelWantTypes()

- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/qnet_types.go`**
  - 5 want types: qnet numbers, qnet queue, qnet combiner, qnet sink, qnet collector
  - Registration: RegisterQNetWantTypes()

- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/fibonacci_types.go`**
  - 3 want types: fibonacci, fibonacci_source, fibonacci_adder
  - Registration: RegisterFibonacciWantTypes()

- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/fibonacci_loop_types.go`**
  - 3 want types: fibonacci_loop, fibonacci_source_loop, fibonacci_adder_loop
  - Registration: RegisterFibonacciLoopWantTypes()

- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/prime_types.go`**
  - 3 want types: prime, sieve, prime_source
  - Registration: RegisterPrimeWantTypes()

- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/approval_types.go`**
  - 3 want types: approval, approval_decision, approval_escalation
  - Registration: RegisterApprovalWantTypes()

### 3. System Want Type Files
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/src/owner_types.go`**
  - 1 want type: owner
  - Registration: RegisterOwnerWantTypes()

- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/src/monitor_types.go`**
  - 1 want type: monitor
  - Registration: RegisterMonitorWantTypes()

### 4. Core Type Definition File
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/src/declarative.go`**
  - Core types: Config, Want, WantSpec, Metadata
  - Recipe types: GenericRecipe, RecipeWant, RecipeContent

### 5. Server Initialization File
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/server/main.go`**
  - 1989 lines
  - NewServer() - Phase 1 initialization (lines 134-182)
  - Start() - Phase 2 startup (lines 1530-1546)
  - loadRecipeFilesIntoRegistry() - Recipe loading (lines 1864-1919)

### 6. Recipe Files (Deployable Templates)
Located in `/Users/hiroyukiosaki/work/golang/MyWant/recipes/`:
- travel-itinerary.yaml
- travel-agent.yaml
- travel-itinerary-agent.yaml
- queue-system.yaml
- qnet-pipeline.yaml
- fibonacci-sequence.yaml
- fibonacci-pipeline.yaml
- prime-sieve.yaml
- approval-level-1.yaml
- approval-level-2.yaml
- dynamic-travel-change.yaml

### 7. Config Files (User Interface)
Located in `/Users/hiroyukiosaki/work/golang/MyWant/config/`:
- config-travel-recipe.yaml
- config-qnet-recipe.yaml
- config-fibonacci-recipe.yaml
- config-prime-recipe.yaml
- config-travel-agent-full.yaml
- config-flight.yaml
- config-hierarchical-approval.yaml
- And 6+ more config files

---

## Summary Documents Created

1. **SUMMARY.md** - Complete comprehensive analysis with absolute paths
2. **FINDINGS.txt** - Detailed findings organized by topic
3. **INDEX.md** - This file, quick reference to all analyzed files

---

## Key Statistics

- **Total Want Types**: 24 (across 8 files)
- **Recipe Files**: 12 (loaded at startup)
- **Config Files**: 13 (user interface)
- **Main Loader File**: 706 lines (recipe_loader_generic.go)
- **Server Startup File**: 1989 lines (main.go)
- **Demo Programs**: 18+ files

---

## Architecture Summary

### Three-Tier System

```
Go Code (Static)
  ↓
Register*WantTypes() [8 files, 24 types]
  ↓
ChainBuilder (initialized at startup)

YAML Recipes (Dynamic)
  ↓
GenericRecipeLoader.LoadRecipe() [706 lines]
  ↓
GenericRecipeConfig [fully processed]
  ↓
ChainBuilder.Execute()

YAML Configs (User Interface)
  ↓
LoadRecipeWithConfig() [references recipes]
  ↓
Override parameters and deploy
```

### Server Startup Flow

Phase 1 (NewServer - lines 134-182):
- Create registries (agents, recipes)
- Load agents and capabilities
- ScanAndRegisterCustomTypes() from recipes/
- loadRecipeFilesIntoRegistry() from recipes/

Phase 2 (Start - lines 1530-1546):
- setupRoutes()
- Register all 24 want types
- ExecuteWithMode(true) - start reconcile loop
- ListenAndServe() - start HTTP server

---

## Quick Navigation

To understand:

1. **How recipes are loaded**
   → Read: `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/recipe_loader_generic.go`
   → Focus: LoadRecipe() method (lines ~131-215)

2. **All want types available**
   → Read: Summary documents Section 2
   → Inventory: 24 types across 8 files

3. **Server initialization**
   → Read: `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/server/main.go`
   → Focus: NewServer() (lines 134-182) + Start() (lines 1530-1546)

4. **How recipes are discovered**
   → Read: ScanAndRegisterCustomTypes() in recipe_loader_generic.go (lines 656-706)
   → Also: loadRecipeFilesIntoRegistry() in main.go (lines 1864-1919)

5. **Want type registration pattern**
   → Read: `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/travel_types.go`
   → Example: RegisterTravelWantTypes() + NewRestaurantWant()

---

## Key Insights

1. **Want Types are Go Functions**: Registered via RegisterTravelWantTypes() etc.
2. **Recipes are YAML Templates**: Loaded dynamically from recipes/ directory
3. **Configs Reference Recipes**: User interface that overrides recipe parameters
4. **Parameter Substitution**: Simple string matching (not Go templating)
5. **Custom Types**: Recipes with metadata.custom_type become deployable units
6. **Server Mode**: Global reconcile loop processes wants concurrently

---

Generated: November 12, 2025
Repository: /Users/hiroyukiosaki/work/golang/MyWant
