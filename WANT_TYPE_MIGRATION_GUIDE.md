# Want Type Migration Guide - Converting Go to YAML

## Overview

This guide provides step-by-step instructions for migrating want types from hardcoded Go implementations to declarative YAML definitions. The migration is gradual and non-breaking, allowing both systems to coexist.

---

## Phase 1: Create YAML Definition (Non-Breaking)

### Step 1.1: Extract Want Type Metadata

For each want type in a `*_types.go` file, create a corresponding YAML file in `want_types/`.

**Example: Convert `NumbersWant` from `qnet_types.go`**

Go source:
```go
// qnet_types.go
type NumbersWant struct {
    Want
    start int
    count int
}

func NewNumbersWant(metadata Metadata, spec WantSpec) interface{} {
    w := &NumbersWant{Want: NewWant(metadata, spec)}
    w.ConnectivityMetadata = ConnectivityMetadata{
        InputLabel:  []string{},
        OutputLabel: []string{"numbers"},
    }
    start := spec.Params["start"].(int)
    count := spec.Params["count"].(int)
    w.start = start
    w.count = count
    return w
}
```

Create `want_types/generators/numbers.yaml`:
```yaml
wantType:
  name: "numbers"
  title: "Number Generator"
  description: "Generates a sequence of integers from start to count"
  version: "1.0"
  category: "math"
  pattern: "generator"

  parameters:
    - name: "start"
      type: "int"
      default: 0
      required: false
      validation:
        min: 0

    - name: "count"
      type: "int"
      default: 10
      required: false
      validation:
        min: 1

  # ... rest of definition
```

### Step 1.2: Document Parameter Definitions

Extract all parameters used in the want type and document them:

1. **Identify all `spec.Params` accesses** in the constructor and Exec() method
2. **Record parameter names and types** (infer from type assertions)
3. **Determine if required** (check for panics on missing params)
4. **Add default values** (check for defaults in constructor logic)
5. **Document validation rules** (min/max/enum constraints)

Example from `QueueWant`:
```go
// Appears in Exec() method
serviceTime := w.Spec.Params["service_time"].(float64)
maxSize := w.Spec.Params["max_queue_size"].(int)
```

Becomes in YAML:
```yaml
parameters:
  - name: "service_time"
    type: "float64"
    validation:
      min: 0.01
      max: 3600
  - name: "max_queue_size"
    type: "int"
    validation:
      min: -1
```

### Step 1.3: Document State Keys

Extract all state keys used via `StoreState()` and `GetState()`:

1. **Search for `StoreState()` calls** in want type implementation
2. **Search for `GetState()` calls** in external code
3. **Record the purpose** of each state key
4. **Determine persistence** (should state survive restarts?)

Example from `RestaurantWant`:
```go
// In Exec() or agent handling:
w.StoreState("restaurant_name", result.Name)
w.StoreState("reservation_id", result.ID)
w.StoreState("booking_time", result.Time)

// Later retrieval:
if name, exists := w.GetState("restaurant_name"); exists {
    // use name
}
```

Becomes in YAML:
```yaml
state:
  - name: "restaurant_name"
    type: "string"
    persistent: true
    description: "Name of the reserved restaurant"

  - name: "reservation_id"
    type: "string"
    persistent: true
    description: "Unique reservation identifier"

  - name: "booking_time"
    type: "string"
    persistent: true
    description: "Confirmed booking time"
```

### Step 1.4: Document Connectivity

From the want type, extract input/output channels:

1. **Check ConnectivityMetadata** in constructor:
   ```go
   w.ConnectivityMetadata = ConnectivityMetadata{
       InputLabel:  []string{"input"},
       OutputLabel: []string{"output"},
   }
   ```

2. **Determine pattern** from connectivity:
   - No inputs, has outputs → **generator**
   - Has inputs, has outputs → **processor**
   - Has inputs, no outputs → **sink**
   - Multiple independent inputs → **coordinator**
   - No inputs, no outputs → **independent**

3. **Document in YAML**:
   ```yaml
   pattern: "processor"
   connectivity:
     inputs:
       - name: "input"
         type: "want"
         required: true
     outputs:
       - name: "output"
         type: "want"
   ```

### Step 1.5: Identify Agent Requirements

Check if want type requires agents:

1. **Search for AgentRegistry usage** in want type
2. **Look for agent execution** in Exec() method
3. **Document agent types** that this want supports

Example from `RestaurantWant`:
```go
// In travel_types.go
if monitorAgent, exists := w.agentRegistry.GetAgent("MonitorRestaurant"); exists {
    monitorAgent.Exec(ctx, w)
}
if actionAgent, exists := w.agentRegistry.GetAgent("AgentRestaurant"); exists {
    actionAgent.Exec(ctx, w)
}
```

Becomes in YAML:
```yaml
agents:
  - name: "MonitorRestaurant"
    role: "monitor"
    description: "Loads existing restaurant data"

  - name: "AgentRestaurant"
    role: "action"
    description: "Books new reservation if needed"
```

### Step 1.6: Create Examples

Document expected usage patterns:

1. **List common parameter combinations**
2. **Describe typical connectivity** (what other wants connect to this)
3. **Explain expected behavior**

```yaml
examples:
  - name: "Basic usage"
    description: "..."
    params:
      service_time: 0.1
    expectedBehavior: "Items are processed..."
    connectedTo: ["queue"]
```

---

## Phase 2: Integrate YAML with Validation

### Step 2.1: Load Want Types at Startup

Create `WantTypeRegistry` that loads all YAML definitions:

```go
// In server initialization
wantTypeRegistry := NewWantTypeRegistry()
err := wantTypeRegistry.LoadFromDirectory("./want_types")
if err != nil {
    log.Fatal("Failed to load want types:", err)
}

// Store in application context
app.WantTypeRegistry = wantTypeRegistry
```

### Step 2.2: Validate Parameters During Config Load

Before creating wants, validate parameters:

```go
func (builder *ChainBuilder) CreateWant(metadata Metadata, spec WantSpec) (Want, error) {
    // Look up want type definition
    wantTypeDef, exists := builder.wantTypeRegistry.Get(metadata.Type)
    if !exists {
        return nil, fmt.Errorf("unknown want type: %s", metadata.Type)
    }

    // Validate parameters against definition
    err := validateParameters(spec.Params, wantTypeDef.Parameters)
    if err != nil {
        return nil, fmt.Errorf("invalid parameters for %s: %v", metadata.Type, err)
    }

    // Apply default values for missing parameters
    applyDefaults(spec.Params, wantTypeDef.Parameters)

    // Create want using registered factory
    factory := builder.WantFactories[metadata.Type]
    if factory == nil {
        return nil, fmt.Errorf("no factory for want type: %s", metadata.Type)
    }

    return factory(metadata, spec), nil
}
```

### Step 2.3: Generate Validation Errors with Context

Include want type definition in error messages:

```go
func validateParameters(params map[string]interface{}, paramDefs []ParameterDef) error {
    for _, paramDef := range paramDefs {
        if paramDef.Required && params[paramDef.Name] == nil {
            return fmt.Errorf(
                "required parameter '%s' not provided\n"+
                "Description: %s\n"+
                "Type: %s\n"+
                "Example: %v",
                paramDef.Name, paramDef.Description, paramDef.Type, paramDef.Example,
            )
        }

        // Type validation
        value := params[paramDef.Name]
        if !isValidType(value, paramDef.Type) {
            return fmt.Errorf(
                "parameter '%s' has invalid type\n"+
                "Expected: %s, Got: %T",
                paramDef.Name, paramDef.Type, value,
            )
        }

        // Range validation
        if paramDef.Validation.Min != nil {
            if !isGreaterThan(value, paramDef.Validation.Min) {
                return fmt.Errorf(
                    "parameter '%s' must be >= %v (got %v)",
                    paramDef.Name, paramDef.Validation.Min, value,
                )
            }
        }
    }
    return nil
}
```

### Step 2.4: Add Want Type Info to Config Response

Update API responses to include want type information:

```go
type WantResponse struct {
    Metadata    Metadata
    Spec        WantSpec
    Status      WantStatus
    State       map[string]interface{}

    // New: Include want type definition
    TypeDef     *WantTypeDefinition `json:"typeDef,omitempty"`
}
```

---

## Phase 3: Create API Endpoints

### Step 3.1: List Want Types

```go
// GET /api/v1/want-types
func (h *Handler) ListWantTypes(w http.ResponseWriter, r *http.Request) {
    category := r.URL.Query().Get("category")
    pattern := r.URL.Query().Get("pattern")

    var types []*WantTypeDefinition

    if category != "" {
        types = h.registry.FilterByCategory(category)
    } else if pattern != "" {
        types = h.registry.FilterByPattern(pattern)
    } else {
        types = h.registry.GetAll()
    }

    json.NewEncoder(w).Encode(types)
}
```

### Step 3.2: Get Want Type Definition

```go
// GET /api/v1/want-types/{name}
func (h *Handler) GetWantType(w http.ResponseWriter, r *http.Request) {
    name := chi.URLParam(r, "name")

    typeDef, exists := h.registry.Get(name)
    if !exists {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }

    json.NewEncoder(w).Encode(typeDef)
}
```

### Step 3.3: Get Want Type Examples

```go
// GET /api/v1/want-types/{name}/examples
func (h *Handler) GetWantTypeExamples(w http.ResponseWriter, r *http.Request) {
    name := chi.URLParam(r, "name")

    typeDef, exists := h.registry.Get(name)
    if !exists {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }

    json.NewEncoder(w).Encode(map[string]interface{}{
        "name": name,
        "examples": typeDef.Examples,
    })
}
```

### Step 3.4: Register New Want Type (Optional)

For dynamic registration:

```go
// POST /api/v1/want-types
// Body: YAML want type definition
func (h *Handler) RegisterWantType(w http.ResponseWriter, r *http.Request) {
    var typeDef WantTypeDefinition

    // Parse YAML from request
    decoder := yaml.NewDecoder(r.Body)
    err := decoder.Decode(&typeDef)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Validate definition
    err = h.registry.Validate(&typeDef)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Register
    h.registry.Register(&typeDef)

    w.Header().Set("Location", fmt.Sprintf("/api/v1/want-types/%s", typeDef.Name))
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(typeDef)
}
```

---

## Phase 4: Update Frontend

### Step 4.1: Fetch Want Types on Page Load

```typescript
// In recipe creation form
useEffect(() => {
    fetch('/api/v1/want-types')
        .then(r => r.json())
        .then(types => {
            setAvailableWantTypes(types);
            // Populate dropdown
        });
}, []);
```

### Step 4.2: Show Want Type Help

```typescript
// When want type is selected, show:
const showWantTypeHelp = (wantTypeName: string) => {
    fetch(`/api/v1/want-types/${wantTypeName}`)
        .then(r => r.json())
        .then(typeDef => {
            // Show description, parameters, examples
            setTypeDescription(typeDef.description);
            setTypeParameters(typeDef.parameters);
            setTypeExamples(typeDef.examples);
        });
};
```

### Step 4.3: Validate Parameters Client-Side

```typescript
// Before submitting want config
const validateWantParams = (wantType: string, params: Record<string, any>) => {
    const typeDef = wantTypeRegistry[wantType];

    for (const paramDef of typeDef.parameters) {
        // Check required
        if (paramDef.required && params[paramDef.name] === undefined) {
            showError(`Required parameter: ${paramDef.name}`);
            return false;
        }

        // Check type
        if (params[paramDef.name] !== undefined) {
            if (!isValidType(params[paramDef.name], paramDef.type)) {
                showError(`Parameter ${paramDef.name}: expected ${paramDef.type}`);
                return false;
            }
        }

        // Check validation
        if (paramDef.validation?.min && params[paramDef.name] < paramDef.validation.min) {
            showError(`Parameter ${paramDef.name}: must be >= ${paramDef.validation.min}`);
            return false;
        }
    }

    return true;
};
```

---

## Phase 5: Full Conversion (Optional)

Once YAML definitions are complete and validated, optionally reduce Go boilerplate:

### Before (Full Go implementation):
```go
type NumbersWant struct {
    Want
    start int
    count int
}

func NewNumbersWant(metadata Metadata, spec WantSpec) interface{} {
    w := &NumbersWant{Want: NewWant(metadata, spec)}
    w.ConnectivityMetadata = ConnectivityMetadata{...}
    w.start = spec.Params["start"].(int)
    w.count = spec.Params["count"].(int)
    return w
}
```

### After (Simplified with YAML):
```go
// Framework-provided generic constructor
func NewNumbersWant(metadata Metadata, spec WantSpec) interface{} {
    w := NewGenericWant(metadata, spec)
    // Load connectivity from YAML
    w.ConnectivityMetadata = loadConnectivityFromYAML("want_types/generators/numbers.yaml")
    // Parameters already validated and typed by framework
    w.start = int(spec.Params["start"].(float64))
    w.count = int(spec.Params["count"].(float64))
    return w
}
```

---

## Migration Checklist

For each want type being migrated:

- [ ] Created YAML file in `want_types/{pattern}/{name}.yaml`
- [ ] Documented all parameters with:
  - [ ] Name and description
  - [ ] Type (int, float64, string, bool, etc.)
  - [ ] Default value (if applicable)
  - [ ] Validation rules (min, max, enum, pattern)
  - [ ] Example values
- [ ] Documented all state keys with:
  - [ ] Name and description
  - [ ] Type
  - [ ] Persistence flag
- [ ] Documented connectivity:
  - [ ] Pattern classification
  - [ ] Input channels
  - [ ] Output channels
- [ ] Documented agents:
  - [ ] Agent names
  - [ ] Role (monitor, action, validator, transformer)
- [ ] Created examples:
  - [ ] At least 2-3 realistic usage examples
  - [ ] Parameter values for each example
  - [ ] Expected behavior description
- [ ] Listed related types
- [ ] Listed helpful documentation links

---

## File Organization After Migration

```
want_types/
├── generators/
│   ├── numbers.yaml
│   ├── fibonacci_numbers.yaml
│   ├── prime_numbers.yaml
│   └── sequence.yaml
├── processors/
│   ├── queue.yaml
│   ├── combiner.yaml
│   ├── fibonacci_sequence.yaml
│   ├── prime_sequence.yaml
│   └── custom_processor.yaml
├── sinks/
│   ├── sink.yaml
│   ├── prime_sink.yaml
│   └── stats_collector.yaml
├── coordinators/
│   ├── travel_coordinator.yaml
│   └── custom_coordinator.yaml
├── independent/
│   ├── restaurant.yaml
│   ├── hotel.yaml
│   ├── buffet.yaml
│   ├── flight.yaml
│   └── custom_service.yaml
└── templates/
    ├── generator_template.yaml
    ├── processor_template.yaml
    ├── sink_template.yaml
    ├── coordinator_template.yaml
    └── independent_template.yaml
```

---

## Success Criteria

After migration is complete:

1. ✅ All 16+ want types have corresponding YAML definitions
2. ✅ Parameters are validated against YAML definitions before want creation
3. ✅ Frontend shows parameter help/validation based on YAML
4. ✅ API endpoints available for browsing want types
5. ✅ Error messages reference want type definitions
6. ✅ No breaking changes to existing configs
7. ✅ New want types can be defined in YAML only (no Go code needed)
