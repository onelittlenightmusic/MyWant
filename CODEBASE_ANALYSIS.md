# MyWant Codebase Analysis - Complete Summary

## Quick Reference

**Analysis Date**: November 12, 2025
**Repository**: /Users/hiroyukiosaki/work/golang/MyWant
**Focus**: Recipe loader, want type implementations, and server initialization

---

## 1. Recipe Loader System

### Main File
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/recipe_loader_generic.go` (706 lines)

### Key Class: `GenericRecipeLoader`

**Public Methods:**
- `LoadRecipe(recipePath string, params map[string]interface{})` - Load and process recipe
- `ValidateRecipe(recipePath string)` - Validate YAML structure
- `ListRecipes()` - Enumerate recipe files in directory
- `GetRecipeMetadata(recipePath string)` - Extract recipe metadata
- `GetRecipeParameters(recipePath string)` - Extract parameter definitions
- `GetRecipeResult(recipePath string)` - Extract result specification
- `ScanAndRegisterCustomTypes(recipeDir string, registry *CustomTargetTypeRegistry)` - Register custom types

### Processing Flow

```
Config YAML → LoadRecipe() → GenericRecipeConfig → ChainBuilder.Execute()
```

**Processing Steps (in LoadRecipe):**
1. Read recipe YAML file
2. Validate against OpenAPI spec (`spec/recipe-spec.yaml`)
3. Merge parameters (recipe defaults + provided overrides)
4. Substitute parameter references (simple string matching: `restaurant_type: restaurant_type`)
5. Generate missing want names (`prefix-type-index`)
6. Namespace labels and using selectors
7. Return GenericRecipeConfig with fully processed Want objects

### Data Structures

```go
// Input recipe file structure
GenericRecipe {
  Recipe: RecipeContent {
    Metadata: {name, description, version, type, custom_type}
    Parameters: map[string]interface{}  // Recipe parameter defaults
    Wants: []RecipeWant  // Array of want templates
    Result: *RecipeResult  // How to compute results
    Example: *RecipeExample  // Example deployments
  }
}

// Individual want in recipe
RecipeWant {
  Metadata: Metadata{Name, Type, Labels}
  Spec: WantSpec{Params, Using, Requires}
  // Also supports legacy flattened fields
}

// Output configuration
GenericRecipeConfig {
  Config: Config  // Ready to execute
  Parameters: map[string]interface{}  // Merged parameters
  Metadata: GenericRecipeMetadata
  Result: *RecipeResult
}
```

---

## 2. Want Type Implementations

### All 24 Want Types (Across 8 Files)

#### Travel Domain (5 types)
**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/travel_types.go`
- `NewFlightWant()` → "flight"
- `NewRestaurantWant()` → "restaurant"
- `NewHotelWant()` → "hotel"
- `NewBuffetWant()` → "buffet"
- `NewTravelCoordinatorWant()` → "travel_coordinator"
- **Registration:** `RegisterTravelWantTypes(builder *ChainBuilder)`

#### Queue Network Domain (5 types)
**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/qnet_types.go`
- `PacketNumbers()` → "qnet numbers"
- `NewQueue()` → "qnet queue"
- `NewCombiner()` → "qnet combiner"
- `Goal()` → "qnet sink" / "qnet collector"
- **Registration:** `RegisterQNetWantTypes(builder *ChainBuilder)`

#### Fibonacci Domain (6 types)
**Files:**
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/fibonacci_types.go`
  - `NewFibonacciWant()` → "fibonacci"
  - `NewFibonacciSourceWant()` → "fibonacci_source"
  - `NewFibonacciAdderWant()` → "fibonacci_adder"
  - **Registration:** `RegisterFibonacciWantTypes(builder)`

- `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/fibonacci_loop_types.go`
  - `NewFibonacciLoopWant()` → "fibonacci_loop"
  - `NewFibonacciSourceLoopWant()` → "fibonacci_source_loop"
  - `NewFibonacciAdderLoopWant()` → "fibonacci_adder_loop"
  - **Registration:** `RegisterFibonacciLoopWantTypes(builder)`

#### Prime Domain (3 types)
**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/prime_types.go`
- `NewPrimeWant()` → "prime"
- `NewSieveWant()` → "sieve"
- `NewPrimeSourceWant()` → "prime_source"
- **Registration:** `RegisterPrimeWantTypes(builder)`

#### Approval Domain (3 types)
**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/approval_types.go`
- `NewApprovalWant()` → "approval"
- `NewApprovalDecisionWant()` → "approval_decision"
- `NewApprovalEscalationWant()` → "approval_escalation"
- **Registration:** `RegisterApprovalWantTypes(builder)`

#### System Domain (2 types)
**Files:**
- `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/owner_types.go`
  - `NewOwnerWant()` → "owner"
  - **Registration:** `RegisterOwnerWantTypes(builder)`

- `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/monitor_types.go`
  - `NewMonitorWant()` → "monitor"
  - **Registration:** `RegisterMonitorWantTypes(builder)`

### Constructor Pattern

All constructors follow this signature:
```go
func New*Want(metadata Metadata, spec WantSpec) interface{}
```

Example implementation:
```go
func NewRestaurantWant(metadata Metadata, spec WantSpec) interface{} {
    restaurant := &RestaurantWant{
        Want: Want{},
        RestaurantType: "casual",  // default
    }
    
    // Initialize base Want
    restaurant.Init(metadata, spec)
    
    // Extract parameters from spec.Params
    if rt, ok := spec.Params["restaurant_type"]; ok {
        if rts, ok := rt.(string); ok {
            restaurant.RestaurantType = rts
        }
    }
    
    // Set connectivity metadata
    restaurant.WantType = "restaurant"
    restaurant.ConnectivityMetadata = ConnectivityMetadata{
        RequiredInputs: 0,
        RequiredOutputs: 1,
        WantType: "restaurant",
        Description: "Restaurant reservation scheduling want",
    }
    
    return restaurant
}
```

---

## 3. Server Initialization Flow

### Main Server File
`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/server/main.go` (1989 lines)

### Phase 1: Server Creation (lines 134-182)
Function: `NewServer(config ServerConfig) *Server`

**Steps:**
1. Create `AgentRegistry`
2. Create `CustomTargetTypeRegistry`
3. Load agents from `agents/` directory
4. Load capabilities from `capabilities/` directory
5. **Scan and register custom types** from recipes
   - Function: `ScanAndRegisterCustomTypes("recipes", recipeRegistry)`
   - Scans all YAML files in `recipes/`
   - Extracts `metadata.custom_type` from each recipe
   - Registers as deployable custom types
6. **Load recipe files into registry**
   - Function: `loadRecipeFilesIntoRegistry("recipes", recipeRegistry)`
   - Parses each YAML file into GenericRecipe
   - Creates recipes in registry by name
   - Logs loaded recipes
7. Create global `ChainBuilder` with memory path
8. Set `AgentRegistry` on builder

### Phase 2: Server Start (lines 1530-1546)
Function: `(s *Server) Start() error`

**Steps:**
1. Call `setupRoutes()` - Configure HTTP endpoints
2. **Register all want types:**
   ```go
   types.RegisterQNetWantTypes(s.globalBuilder)
   types.RegisterFibonacciWantTypes(s.globalBuilder)
   types.RegisterPrimeWantTypes(s.globalBuilder)
   types.RegisterTravelWantTypes(s.globalBuilder)
   types.RegisterApprovalWantTypes(s.globalBuilder)
   mywant.RegisterMonitorWantTypes(s.globalBuilder)
   mywant.RegisterOwnerWantTypes(s.globalBuilder)
   ```
3. Start global reconcile loop
   ```go
   go s.globalBuilder.ExecuteWithMode(true)  // Server mode
   ```
4. Start HTTP server listening on configured port/host

### Recipe Loading Helper Functions

**`ScanAndRegisterCustomTypes()`** (lines 656-706 in recipe_loader_generic.go):
- Iterates all recipes in directory
- For each recipe with `metadata.custom_type`:
  - Extracts metadata (name, description, type)
  - Gets default parameters
  - Calls `RegisterCustomTargetType(registry, typename, description, path, params)`
- Logs: "Registered N custom types from recipes"

**`loadRecipeFilesIntoRegistry()`** (lines 1864-1919 in main.go):
- Lists all `.yaml` and `.yml` files in recipes directory
- For each file:
  - Reads file content
  - Unmarshals to GenericRecipe
  - Extracts recipe name from metadata.name or filename
  - Creates recipe in registry: `registry.CreateRecipe(recipeID, &recipe)`
  - Logs: "✅ Loaded recipe: {name}"
- Returns count of successfully loaded recipes

---

## 4. Directory Structure

```
/Users/hiroyukiosaki/work/golang/MyWant/
├── recipes/                              (12 recipe files)
│   ├── travel-itinerary.yaml            # Travel booking workflow
│   ├── travel-agent.yaml                # Travel with agents
│   ├── travel-itinerary-agent.yaml      # Travel + agents
│   ├── queue-system.yaml                # Queue processing
│   ├── qnet-pipeline.yaml               # Network queuing
│   ├── fibonacci-sequence.yaml          # Fibonacci computation
│   ├── fibonacci-pipeline.yaml          # Fibonacci pipeline
│   ├── prime-sieve.yaml                 # Prime number generation
│   ├── approval-level-1.yaml            # Multi-level approval
│   ├── approval-level-2.yaml            # Approval escalation
│   ├── dynamic-travel-change.yaml       # Dynamic parameter changes
│   └── README.md
│
├── config/                               (13 config files)
│   ├── config-travel-recipe.yaml        # Travel config (references recipe)
│   ├── config-qnet-recipe.yaml
│   ├── config-fibonacci-recipe.yaml
│   ├── config-prime-recipe.yaml
│   ├── config-travel-agent-full.yaml
│   ├── config-flight.yaml
│   ├── config-hierarchical-approval.yaml
│   └── ... (6 more config files)
│
├── engine/
│   ├── src/                              # Core library
│   │   ├── declarative.go               # Config/Want/Recipe types
│   │   ├── recipe_loader_generic.go     # GenericRecipeLoader (706 lines)
│   │   ├── recipe_loader.go             # RecipeLoader legacy (490 lines)
│   │   ├── owner_types.go               # Owner want type
│   │   ├── monitor_types.go             # Monitor want type
│   │   ├── agent_types.go
│   │   ├── chain_builder.go
│   │   └── ... (other core files)
│   │
│   └── cmd/
│       ├── types/                        # Want implementations (6 domain files + 1 agent file)
│       │   ├── travel_types.go
│       │   ├── qnet_types.go
│       │   ├── fibonacci_types.go
│       │   ├── fibonacci_loop_types.go
│       │   ├── prime_types.go
│       │   ├── approval_types.go
│       │   └── flight_types.go
│       │
│       ├── server/
│       │   ├── main.go                  # Server with startup logic (1989 lines)
│       │   └── ... (other handler files)
│       │
│       └── demos/                        # Demo programs (18+ files)
│           ├── demo_travel_recipe.go    # Recipe-based example
│           ├── demo_qnet_recipe.go
│           └── ... (other demo files)
```

---

## 5. Loading Want Type YAMLs - Analysis

### Question
Can we load want type definitions from YAML at startup?

### Current Approach
Want types are Go constructor functions, registered via `Register*WantTypes()` functions.

### Why NOT Load Want Type YAML

1. **Constructors are Go Functions**
   - Need to call `NewRestaurantWant(metadata, spec)`
   - YAML can't execute Go code
   - Would require reflection + dynamic lookup (adds complexity)

2. **Type Safety Lost**
   - Go compiler checks constructor signatures at compile-time
   - Parameter extraction code is type-safe
   - YAML approach would require runtime validation

3. **Current Pattern Already Works**
   - All types registered in server startup
   - Scalable to any number of types
   - Consistent with demo programs

### Better Alternative: Load Recipe YAML (Already Implemented)

The system DOES load YAML at startup:
- **12 recipe files** loaded from `recipes/` directory
- **Each recipe can define `metadata.custom_type`**
- **Custom types registered as deployable units**
- **Parameters parameterized for flexibility**

This is the RIGHT approach because:
- Recipes compose want types (high-level)
- Recipes are user-facing (configuration)
- Can be deployed multiple times with different parameters
- Pure YAML, no Go execution needed

### Architecture Separation

```
Go Code         → Want Types       → Register*WantTypes()
YAML Recipes    → Compose Wants    → ScanAndRegisterCustomTypes()
YAML Configs    → Deploy Recipes   → CreateWant() API
```

---

## 6. Key Files Reference

### Core Recipe System
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/src/recipe_loader_generic.go`** (706 lines)
  - GenericRecipeLoader class
  - LoadRecipe() algorithm
  - Parameter substitution logic
  - Custom type registration

### Server Startup
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/server/main.go`** (1989 lines)
  - NewServer() - Phase 1 initialization
  - Start() - Phase 2 startup
  - Want type registration calls
  - Recipe loading calls

### Core Type Definitions
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/src/declarative.go`**
  - Config struct
  - Want struct
  - WantSpec struct
  - Metadata struct
  - GenericRecipe/RecipeWant structs

### Example Want Type Implementation
- **`/Users/hiroyukiosaki/work/golang/MyWant/engine/cmd/types/travel_types.go`**
  - RegisterTravelWantTypes() pattern
  - NewRestaurantWant() constructor example
  - Parameter extraction from spec.Params

---

## 7. Complete Want Type Inventory

Total: **24 Want Types** across 8 implementation files

### By Domain
- Travel: 5 types
- Queue Network: 5 types
- Fibonacci: 3 types
- Fibonacci Loop: 3 types
- Prime: 3 types
- Approval: 3 types
- Owner (System): 1 type
- Monitor (System): 1 type

---

## Conclusion

MyWant implements a well-architected, three-tier configuration system:

1. **Static Want Type Registration (Go Code)**
   - Happens once at server startup
   - All 24 types registered via Register*WantTypes()
   - Provides type definitions for builders

2. **Dynamic Recipe Loading (YAML Files)**
   - Happens at startup from recipes/ directory
   - 12+ recipes available for deployment
   - Each can be deployed multiple times with different parameters
   - Custom types derived from recipes

3. **User-Facing Configurations (YAML Files)**
   - Reference recipes by path
   - Override recipe parameters
   - Deploy complete workflows

The pattern is clean, maintainable, and extensible. The current approach (hardcoded Go registrations) is optimal for type safety and maintainability.
