# Target OwnerReferences - Quick Reference Checklist

## Where OwnerReferences are SET

**Location:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/src/owner_types.go`
**Method:** `Target.CreateChildWants()` 
**Lines:** 244-261

```go
for i := range config.Wants {
    config.Wants[i].Metadata.OwnerReferences = []OwnerReference{
        {
            APIVersion:         "MyWant/v1",
            Kind:               "Want",
            Name:               t.Metadata.Name,        // Parent's name
            ID:                 t.Metadata.ID,          // Parent's ID
            Controller:         true,
            BlockOwnerDeletion: true,
        },
    }
    if config.Wants[i].Metadata.Labels == nil {
        config.Wants[i].Metadata.Labels = make(map[string]string)
    }
    config.Wants[i].Metadata.Labels["owner"] = "child"
}
```

## Execution Sequence

1. **Target.Exec() called** (line 268)
   - Condition: `if t.builder != nil` (line 272)

2. **CreateChildWants() called** (line 274)
   - Load recipe via `t.recipeLoader.LoadConfigFromRecipe()` (line 235)
   - Recipe loader returns Config WITHOUT OwnerReferences

3. **OwnerReferences ADDED** (lines 244-255)
   - Loop through each child want
   - Set OwnerReferences array with parent metadata
   - Set `owner: "child"` label (line 260)

4. **AddWantsAsync() called** (line 284)
   - Sends child wants to reconcile loop

5. **Reconcile loop processes** (chain_builder.go lines 422-435)
   - Calls `cb.addWant()` for each child
   - Which calls `createWantFunction()` (line 1212)
   - Which checks for OwnerReferences (chain_builder.go line 268)
   - Which wraps with `OwnerAwareWant` if OwnerReferences exist

6. **Child wants execute**
   - On completion, `OwnerAwareWant.Exec()` fires (line 603)
   - Emits `OwnerCompletionEvent` (line 611)

7. **Parent Target receives event**
   - Via `TargetCompletionSubscription.OnEvent()` (line 106)
   - Tracks completion in `completedChildren` map (line 122)
   - When all done, signals `childrenDone` channel (line 132)

8. **Target.Exec() unblocks**
   - Returns from `<-t.childrenDone` (line 307)
   - Computes result (line 311)
   - Marks as completed (line 314)

## Data Structures

### OwnerReference (in Metadata)
```go
type OwnerReference struct {
    APIVersion         string  // "MyWant/v1"
    Kind              string  // "Want" for child Target wants
    Name              string  // Parent's name
    ID                string  // Parent's unique ID
    Controller        bool    // true for controlling owner
    BlockOwnerDeletion bool   // true for cascade deletion
}
```

### Target struct key fields
```go
type Target struct {
    Want                      // Embedded
    builder     *ChainBuilder // For AddWantsAsync
    recipeLoader *GenericRecipeLoader
    childWants  []*Want       // Cached child wants
    completedChildren map[string]bool // Track completion
    childrenDone chan bool    // Wait channel
}
```

### OwnerAwareWant
```go
type OwnerAwareWant struct {
    BaseWant   interface{}     // Original want
    TargetName string          // Parent Target name
    WantName   string          // This want's name
}
```

## Critical Paths

### Path 1: OwnerReferences are Set
- owner_types.go: 244-261 (CreateChildWants)
- Sets Name and ID from parent Target

### Path 2: OwnerReferences Cause Wrapping
- chain_builder.go: 268-270 (createWantFunction)
- Creates OwnerAwareWant if OwnerReferences exist

### Path 3: Child Notifies Parent
- owner_types.go: 611 (OwnerAwareWant.emitOwnerCompletionEvent)
- Emits event with parent Target name
- owner_types.go: 106-145 (TargetCompletionSubscription.OnEvent)
- Receives and handles event

### Path 4: Parent Waits for All Children
- owner_types.go: 307 (Target.Exec)
- Blocks on `<-t.childrenDone`
- owner_types.go: 129-136 (checkAllChildrenComplete)
- Signals channel when all children done

## Verification Checklist

- [x] OwnerReferences ARE set (owner_types.go:244-261)
- [x] Set on ALL child wants in loop (line 245: `for i := range config.Wants`)
- [x] Include parent Name (line 250: `Name: t.Metadata.Name`)
- [x] Include parent ID (line 251: `ID: t.Metadata.ID`)
- [x] Controller set to true (line 252: `Controller: true`)
- [x] BlockOwnerDeletion set to true (line 253)
- [x] Owner label added (line 260: `"owner": "child"`)
- [x] Child wants wrapped with OwnerAwareWant when has OwnerReferences (chain_builder.go:268-270)
- [x] Parent subscribes to completion events (owner_types.go:87-93)
- [x] Parent receives events via TargetCompletionSubscription (owner_types.go:105-145)
- [x] Parent waits on childrenDone channel (owner_types.go:307)

