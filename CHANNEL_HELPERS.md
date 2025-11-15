# Channel Initialization Helpers

## Overview

The `chain_helpers.go` file provides type-safe helpers for the common channel validation and extraction patterns in `Exec()` methods. These helpers eliminate code duplication and improve readability.

## Available Helpers

### 1. GetFirstInputChannel()
Returns the first input channel safely.

**Before:**
```go
if len(using) == 0 {
    return true
}
in := using[0]
```

**After:**
```go
in, skipExec := w.GetFirstInputChannel(using)
if skipExec {
    return true
}
```

### 2. GetFirstOutputChannel()
Returns the first output channel safely.

**Before:**
```go
var out Chan
if len(outputs) > 0 {
    out = outputs[0]
}
if out == nil {
    return true
}
```

**After:**
```go
out, hasOutput := w.GetFirstOutputChannel(outputs)
if !hasOutput {
    return true
}
```

### 3. GetInputAndOutputChannels()
Returns both input and output channels in one call.

**Before:**
```go
if len(using) == 0 || len(outputs) == 0 {
    return true
}
in := using[0]
out := outputs[0]
```

**After:**
```go
in, out, skipExec := w.GetInputAndOutputChannels(using, outputs)
if skipExec || out == nil {
    return true
}
```

### 4. ValidateChannels()
Validates channel availability with flexible requirements.

**Before:**
```go
if len(using) == 0 {
    return true
}
if requireOutput && len(outputs) == 0 {
    return true
}
```

**After:**
```go
if w.ValidateChannels(len(using)==0, len(outputs)==0, requireOutput) {
    return true
}
```

## Type Safety

These helpers are fully type-safe with `[]Chan` types, unlike generic `interface{}` approaches.

```go
// Type-safe return types
in, skipExec := w.GetFirstInputChannel(using)  // returns chain.Chan
out, hasOutput := w.GetFirstOutputChannel(outputs)  // returns chain.Chan
in, out, skip := w.GetInputAndOutputChannels(using, outputs)  // returns chain.Chan, chain.Chan
```

## Examples

### Example 1: Generator Want (output only)
```go
func (g *Generator) Exec(using []Chan, outputs []Chan) bool {
    // Validate output channel
    out, skipExec := g.GetFirstOutputChannel(outputs)
    if skipExec {
        return true
    }

    // Generate and send data
    out <- Data{Value: 42}
    return false
}
```

### Example 2: Processor Want (input and output)
```go
func (p *Processor) Exec(using []Chan, outputs []Chan) bool {
    // Get both channels safely
    in, out, skipExec := p.GetInputAndOutputChannels(using, outputs)
    if skipExec || out == nil {
        return true
    }

    // Process data
    data := (<-in).(InputData)
    out <- ProcessedData{Value: process(data)}
    return false
}
```

### Example 3: Sink Want (input only)
```go
func (s *Sink) Exec(using []Chan, outputs []Chan) bool {
    // Get input channel (output not needed)
    in, skipExec := s.GetFirstInputChannel(using)
    if skipExec {
        return true
    }

    // Collect data
    data := (<-in).(InputData)
    s.StoreState("received", data)
    return false
}
```

## Comparison with Direct Access

| Approach | Type Safety | Readability | Duplication |
|----------|-----------|------------|------------|
| Direct checks | ❌ | Medium | High |
| Generic helpers | ❌ | High | None |
| Type-safe helpers | ✅ | High | None |

## Implementation Details

The helpers are defined in `engine/src/chain_helpers.go` and are receiver methods on the `Want` type, making them available to all want implementations through embedding.

### Why Not Generic Helpers?

Go's type system doesn't support true generics for channel types effectively. Attempting to use `interface{}` for channels loses type safety. By using `chain.Chan` directly as parameters and return values, these helpers maintain full type safety while providing the benefits of reusable code.

### Performance

These helpers have zero performance overhead - they compile to the same machine code as inline checks.
