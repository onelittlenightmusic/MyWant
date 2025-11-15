# Want Type Definition System - Quick Reference Guide

## File Locations

| Type Files | Location |
|------------|----------|
| **Core Types** | `/engine/src/want.go` |
| **Factory System** | `/engine/src/chain_builder.go` |
| **Travel Wants** | `/engine/cmd/types/travel_types.go` |
| **QNet Wants** | `/engine/cmd/types/qnet_types.go` |
| **Fibonacci Wants** | `/engine/cmd/types/fibonacci_types.go` |
| **Prime Wants** | `/engine/cmd/types/prime_types.go` |

## Core Type Checklist

### Metadata (Identification)
```go
Metadata{
    Name:   "unique_name",           // Instance identifier
    Type:   "registered_type_name",  // Type reference
    Labels: map[string]string{...},  // For want-to-want connections
}
```

### WantSpec (Configuration)
```go
WantSpec{
    Params: map[string]interface{}{  // Configuration parameters
        "param1": value,
    },
    Using: []map[string]string{      // Input connections
        {"role": "source"},
    },
    Requires: []string{...},         // Agent requirements
}
```

### Want (Base Type)
```go
Want{
    Metadata: Metadata{...},
    Spec:     WantSpec{...},
    State:    map[string]interface{},  // Persistent state
    Status:   WantStatusIdle,          // Current status
    History:  WantHistory{...},        // Change tracking
}
```

## Constructor Template

```go
func New<WantType>Want(metadata Metadata, spec WantSpec) interface{} {
    want := &<WantType>Want{
        Want: Want{},
        // domain-specific defaults
    }
    
    want.Init(metadata, spec)  // Initialize base
    
    // Extract parameters
    if param, ok := spec.Params["param_name"]; ok {
        if paramTyped, ok := param.(ExpectedType); ok {
            want.Field = paramTyped
        }
    }
    
    want.WantType = "type_name"
    want.ConnectivityMetadata = ConnectivityMetadata{
        RequiredInputs:  X,
        RequiredOutputs: Y,
        MaxInputs:       X, // -1 for unlimited
        MaxOutputs:      Y, // -1 for unlimited
        WantType:        "type_name",
        Description:     "Human description",
    }
    
    return want
}
```

## Registration Template

```go
func Register<Domain>WantTypes(builder *ChainBuilder) {
    builder.RegisterWantType("type_name_1", NewWantType1)
    builder.RegisterWantType("type_name_2", NewWantType2)
    // ...
}
```

## Exec Implementation Template

```go
func (w *<WantType>Want) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Read parameters fresh each cycle
    paramValue := defaultValue
    if param, ok := w.Spec.Params["param_name"]; ok {
        if paramTyped, ok := param.(ExpectedType); ok {
            paramValue = paramTyped
        }
    }
    
    // Early returns for edge cases
    if len(outputs) == 0 {
        return true
    }
    
    // Check persistent state
    stateVal, stateExists := w.GetState("state_key")
    
    // Main logic
    w.StoreState("output_key", outputValue)
    
    // Send output to channels
    if len(outputs) > 0 {
        outputs[0] <- data
    }
    
    return true/false  // Continue or stop?
}
```

## GetWant Implementation

```go
func (w *<WantType>Want) GetWant() *Want {
    return &w.Want
}
```

## Parameter Type Patterns

### String Parameter
```go
if param, ok := spec.Params["param_name"]; ok {
    if paramStr, ok := param.(string); ok {
        want.Field = paramStr
    }
}
```

### Numeric Parameter (Handles Float64 from YAML)
```go
if param, ok := spec.Params["param_name"]; ok {
    if paramInt, ok := param.(int); ok {
        want.Field = paramInt
    } else if paramFloat, ok := param.(float64); ok {
        want.Field = int(paramFloat)
    }
}
```

### Boolean Parameter
```go
if param, ok := spec.Params["param_name"]; ok {
    if paramBool, ok := param.(bool); ok {
        want.Field = paramBool
    } else if paramStr, ok := param.(string); ok {
        want.Field = (paramStr == "true")
    }
}
```

### Float Parameter
```go
if param, ok := spec.Params["param_name"]; ok {
    if paramFloat, ok := param.(float64); ok {
        want.Field = paramFloat
    }
}
```

## Connectivity Patterns

### Generator (Source)
```go
ConnectivityMetadata{
    RequiredInputs:  0,
    RequiredOutputs: 1,
    MaxInputs:       0,
    MaxOutputs:      -1,
}
```

### Processor (Filter/Transform)
```go
ConnectivityMetadata{
    RequiredInputs:  1,
    RequiredOutputs: 1,
    MaxInputs:       1,
    MaxOutputs:      -1,
}
```

### Sink (Collector)
```go
ConnectivityMetadata{
    RequiredInputs:  1,
    RequiredOutputs: 0,
    MaxInputs:       -1,
    MaxOutputs:      0,
}
```

### Coordinator (Hub)
```go
ConnectivityMetadata{
    RequiredInputs:  3,      // or more
    RequiredOutputs: 0,
    MaxInputs:       3,      // or -1 for unlimited
    MaxOutputs:      0,
}
```

## State Management

### Store State
```go
want.StoreState("key", value)
```

### Retrieve State
```go
value, exists := want.GetState("key")
if exists {
    typedValue := value.(ExpectedType)
}
```

### Batch State Changes
```go
want.BeginExecCycle()
want.StoreState("key1", value1)
want.StoreState("key2", value2)
want.EndExecCycle()
```

## YAML Configuration

### Want in Config File
```yaml
wants:
  - metadata:
      name: instance_name
      type: registered_type_name    # Must match RegisterWantType() call
      labels:
        key1: value1
        key2: value2
    spec:
      params:
        param1: value1
        param2: 42
      using:
        - key: value                 # Label selector for inputs
```

### Want in Recipe
```yaml
recipe:
  parameters:
    param1: default_value

  wants:
    - metadata:
        type: registered_type_name
        labels:
          role: processor
      spec:
        params:
          param1: param1              # References recipe parameter
        using:
          - role: source              # References labels
```

## Common Issues and Solutions

### Issue: Parameter Not Being Extracted
**Solution**: Ensure type assertion matches YAML type
- Booleans: Check for both `bool` and `string` types
- Numbers: Handle both `int` and `float64`
- Strings: Just check `string` type

### Issue: State Lost Between Cycles
**Solution**: Use StoreState/GetState (not local fields)
```go
// WRONG:
value := want.localField

// RIGHT:
value, _ := want.GetState("key")
```

### Issue: Dynamic Parameter Changes Not Working
**Solution**: Re-read parameters in Exec() method
```go
// In constructor: extract for defaults
// In Exec(): re-extract fresh parameters each cycle
```

### Issue: Concurrent Access Panics
**Solution**: Always use StoreState/GetState methods
- Never access State field directly
- StoreState() handles mutex protection
- State is protected with stateMutex internally

## Testing Checklist

When implementing a new want type:

- [ ] Struct embeds `Want` type
- [ ] Constructor signature: `(Metadata, WantSpec) interface{}`
- [ ] Constructor calls `Init(metadata, spec)`
- [ ] Constructor sets `WantType` and `ConnectivityMetadata`
- [ ] Constructor returns `interface{}`
- [ ] Implements `Exec(using []Chan, outputs []Chan) bool`
- [ ] Implements `GetWant() *Want`
- [ ] Uses `StoreState()`/`GetState()` for state management
- [ ] Reads parameters in Exec() method
- [ ] Registration function created: `Register<Type>WantTypes()`
- [ ] Type registered in registration function
- [ ] YAML configuration uses registered type name

## Field Naming Conventions

| Field | Convention | Example |
|-------|-----------|---------|
| Constructor | `New<Type>` or `New<Type>Want` | `NewRestaurantWant` |
| Struct | `<Type>Want` | `RestaurantWant` |
| Registered Name | `snake_case` | `restaurant` |
| Parameters | `snake_case` | `restaurant_type` |
| State Keys | `snake_case` | `total_processed` |
| Label Keys | `snake_case` | `role` |

## Key Methods Summary

| Method | Purpose | Thread-Safe |
|--------|---------|------------|
| `Init()` | Initialize base Want fields | Yes |
| `StoreState()` | Save state key-value pair | Yes |
| `GetState()` | Retrieve state value | Yes |
| `BeginExecCycle()` | Start batching state changes | Yes |
| `EndExecCycle()` | Commit batched changes | Yes |
| `GetParameter()` | Get parameter value | Yes |
| `UpdateParameter()` | Change parameter | Yes |
| `SetStatus()` | Update want status | Yes |
| `GetStatus()` | Get want status | Yes |

## Recent Changes (Latest Commits)

**Standardization (Commit 5df1758)**:
- All constructors now return `interface{}`
- All constructors use `(Metadata, WantSpec)` signature
- All constructors call `Init()` for base initialization

**Type Assertions (Commit 6210e24)**:
- Explicit type casting required after factory call
- Example: `want.(*RestaurantWant)`
