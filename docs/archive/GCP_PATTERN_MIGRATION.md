# Migration Guide: Agent State Labeling

This document outlines the implementation of the "State Labeling" pattern, which provides semantic meaning to state fields while maximizing concurrent access performance.

## 1. Core Concept: Labeled State Fields

This pattern separates a want's state into two distinct maps:
1.  **`State` (`sync.Map`)**: Stores the actual key-value data. It uses a `sync.Map` for highly concurrent, lock-free reads and writes of individual fields.
2.  **`StateLabels` (`map[string]StateLabel`)**: A static map that defines the semantic "label" for each key in the `State`. This map is typically populated during want initialization and is read-only thereafter.

This design avoids complex `StateEntry` structs and costly string manipulations, while ensuring high performance for dynamic state updates.

### StateLabel Enum

A new enum defines the possible semantic labels for a state field.

```go
// engine/core/state_types.go
package mywant

type StateLabel int

const (
    LabelNone StateLabel = iota
    LabelGoal
    LabelCurrent
    LabelPlan
    LabelPredefined // For system/UI reserved words
    LabelInternal   // For agent-internal flags
)
```

## 2. Helper Method Implementation

Helper methods will be implemented to provide a clean, semantic interface for agents, abstracting away the two-map system.

```go
// Example: engine/core/want.go

// Want struct change
type Want struct {
    // ...
    State       sync.Map                `json:"-" yaml:"-"`
    StateLabels map[string]StateLabel `json:"state_labels,omitempty" yaml:"state_labels,omitempty"`
    // ...
}

// SetCurrent updates the 'State' sync.Map atomically.
// It assumes the label for 'key' is already defined in 'StateLabels'.
func (n *Want) SetCurrent(key string, value any) {
    if label, ok := n.StateLabels[key]; !ok || label != LabelCurrent {
        // Log a warning: this field is not defined as a 'Current' value
        return
    }
    n.State.Store(key, value)
    // ... (pending changes and history logic)
}

// GetCurrent checks the label and then loads from the 'State' sync.Map.
func (n *Want) GetCurrent(key string) (any, bool) {
    if label, ok := n.StateLabels[key]; ok && label == LabelCurrent {
        return n.State.Load(key)
    }
    return nil, false
}
```

## 3. Agent Implementation Changes

Agent logic becomes much cleaner. There are no prefixes, and agents simply call the method corresponding to the semantic action they are performing.

### Think Agent Example
```go
// The 'reservation_status' field will be defined with the 'LabelGoal'
// in the 'StateLabels' map when the want is initialized.
want.SetGoal("reservation_status", "confirmed")
```

### Monitor Agent Example
```go
// The 'flight_status' field will have the 'LabelCurrent' in StateLabels.
want.SetCurrent("flight_status", "delayed")
```

### System Metadata Example
```go
// The 'achieving_percentage' field will have the 'LabelPredefined'.
// The SetPredefined method can handle dual-writing to a legacy top-level
// field if needed for UI compatibility, hiding that detail from the agent.
want.SetPredefined("achieving_percentage", 100)
```
