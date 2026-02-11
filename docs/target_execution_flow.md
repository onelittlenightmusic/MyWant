# Target Want Execution Flow and OwnerReferences Setup

## Summary

Target wants are parent wants that create and manage child wants from recipe files. The child wants are created dynamically during Target execution and are properly set up with OwnerReferences pointing to the parent Target.

---

## 1. Target Execution Entry Point

**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/core/owner_types.go`

**Method:** `Target.Exec()` (Lines 268-342)

```go
func (t *Target) Exec(inputs []chain.Chan, outputs []chain.Chan) bool {
	InfoLog("[TARGET] 🎯 Target %s: Managing child nodes with owner references\n", t.Metadata.Name)

	// Dynamically create child wants
	if t.builder != nil {
		InfoLog("[TARGET] 🎯 Target %s: Creating child wants dynamically...\n", t.Metadata.Name)
		childWants := t.CreateChildWants()  // <-- LINE 274: Creates child wants

		// Add child wants to the builder's configuration
		for _, childWant := range childWants {
			InfoLog("[TARGET] 🔧 Adding child want: %s (type: %s)\n", childWant.Metadata.Name, childWant.Metadata.Type)
		}

		// Send child wants to reconcile loop asynchronously
		if err := t.builder.AddWantsAsync(childWants); err != nil {
			InfoLog("[TARGET] ⚠️  Warning: Failed to send child wants: %v\n", err)
		}
		...
	}

	// Target waits for signal that all children have finished
	<-t.childrenDone  // <-- LINE 307: Waits for all children to complete
	...
	return true
}
```

---

## 2. Child Want Creation with OwnerReferences

**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/core/owner_types.go`

**Method:** `Target.CreateChildWants()` (Lines 227-265)

**KEY FINDING:** OwnerReferences are set here!

```go
func (t *Target) CreateChildWants() []*Want {
	// Recipe loader is required for target wants
	if t.recipeLoader == nil {
		InfoLog("[TARGET] ❌ Target %s: No recipe loader available\n", t.Metadata.Name)
		return []*Want{}
	}

	// Load child wants from recipe (LINE 235)
	config, err := t.recipeLoader.LoadConfigFromRecipe(t.RecipePath, t.RecipeParams)
	if err != nil {
		InfoLog("[TARGET] ❌ Target %s: Failed to load recipe %s: %v\n", t.Metadata.Name, t.RecipePath, err)
		return []*Want{}
	}

	InfoLog("[TARGET] ✅ Target %s: Successfully loaded recipe %s with %d child wants\n",
		t.Metadata.Name, t.RecipePath, len(config.Wants))

	// ======== CRITICAL SECTION: SET OWNER REFERENCES ========
	// ADD OWNER REFERENCES TO ALL CHILD WANTS (LINES 244-261)
	for i := range config.Wants {
		config.Wants[i].Metadata.OwnerReferences = []OwnerReference{
			{
				APIVersion:         "MyWant/v1",
				Kind:               "Want",
				Name:               t.Metadata.Name,        // Parent Target name
				ID:                 t.Metadata.ID,          // Parent Target ID
				Controller:         true,
				BlockOwnerDeletion: true,
			},
		}
		// Add owner label for easier identification
		if config.Wants[i].Metadata.Labels == nil {
			config.Wants[i].Metadata.Labels = make(map[string]string)
		}
		config.Wants[i].Metadata.Labels["owner"] = "child"
	}

	t.childWants = config.Wants
	return t.childWants
}
```

**Key Points:**
- Lines 244-255: Each child want gets exactly ONE OwnerReference pointing to the parent Target
- The OwnerReference includes:
  - `Name`: Parent Target's name
  - `ID`: Parent Target's unique ID
  - `Controller: true`: Marks the Target as the controlling owner
  - `BlockOwnerDeletion: true`: Cascade deletion flag
- Line 260: Adds `owner: "child"` label for identification

---

## 3. Recipe Loading Process

**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/core/recipe_loader_generic.go`

**Method:** `GenericRecipeLoader.LoadConfigFromRecipe()` (Lines 212-219)

```go
func (grl *GenericRecipeLoader) LoadConfigFromRecipe(recipePath string, params map[string]interface{}) (Config, error) {
	recipeConfig, err := grl.LoadRecipe(recipePath, params)
	if err != nil {
		return Config{}, err
	}

	return recipeConfig.Config, nil
}
```

This calls `LoadRecipe()` which:
- Reads the recipe YAML file
- Validates it against OpenAPI spec
- Performs parameter substitution
- Generates names and IDs if missing (Lines 177-189)
- Returns a Config with Want array

**IMPORTANT:** At this point, the child wants DO NOT have OwnerReferences set yet!
The OwnerReferences are added by `Target.CreateChildWants()` AFTER loading.

---

## 4. Want Function Creation with OwnerAware Wrapping

**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/core/chain_builder.go`

**Method:** `ChainBuilder.createWantFunction()` (Lines 233-273)

```go
func (cb *ChainBuilder) createWantFunction(want *Want) (interface{}, error) {
	wantType := want.Metadata.Type

	// Check if it's a custom target type first
	if cb.customRegistry.IsCustomType(wantType) {
		return cb.createCustomTargetWant(want)
	}

	// Fall back to standard type registration
	factory, exists := cb.registry[wantType]
	if !exists {
		...
		return nil, fmt.Errorf("Unknown want type: '%s'...", wantType)
	}

	wantInstance := factory(want.Metadata, want.Spec)

	// Set agent registry if available
	if cb.agentRegistry != nil {
		...
	}

	// ========== AUTOMATIC OWNAWARE WRAPPING ==========
	// Automatically wrap with OwnerAwareWant if the want has owner references
	// This enables parent-child coordination via subscription events
	if len(want.Metadata.OwnerReferences) > 0 {
		wantInstance = NewOwnerAwareWant(wantInstance, want.Metadata)  // LINE 269
	}

	return wantInstance, nil
}
```

**Key Point:** At LINE 269, any want with OwnerReferences is automatically wrapped with `OwnerAwareWant` to enable parent-child coordination.

---

## 5. Custom Target Type Creation

**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/core/chain_builder.go`

**Method:** `ChainBuilder.createCustomTargetWant()` (Lines 281-310)

```go
func (cb *ChainBuilder) createCustomTargetWant(want *Want) (interface{}, error) {
	config, exists := cb.customRegistry.Get(want.Metadata.Type)
	if !exists {
		return nil, fmt.Errorf("custom type '%s' not found in registry", want.Metadata.Type)
	}

	InfoLog("🎯 Creating custom target type: '%s' - %s\n", config.Name, config.Description)

	// Merge custom type defaults with user-provided spec
	mergedSpec := cb.mergeWithCustomDefaults(want.Spec, config)

	// Create the custom target using the registered function (LINE 293)
	target := config.CreateTargetFunc(want.Metadata, mergedSpec)

	// Set up target with builder and recipe loader (LINES 296-300)
	target.SetBuilder(cb)
	recipeLoader := NewGenericRecipeLoader("recipes")
	target.SetRecipeLoader(recipeLoader)

	// ========== AUTOMATIC OWNAWARE WRAPPING FOR NESTED TARGETS ==========
	// Automatically wrap with OwnerAwareWant if the custom target has owner references
	// This enables parent-child coordination via subscription events (critical for nested targets)
	var wantInstance interface{} = target
	if len(want.Metadata.OwnerReferences) > 0 {
		wantInstance = NewOwnerAwareWant(wantInstance, want.Metadata)  // LINE 306
	}

	return wantInstance, nil
}
```

**Key Points:**
- LINE 293: Creates the custom Target instance
- LINES 296-300: Sets the builder and recipe loader (needed for child creation)
- LINE 306: Wraps with OwnerAwareWant if this Target is itself a child of another Target (nested targets)

---

## 6. OwnerAwareWant Wrapper

**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/core/owner_types.go`

**Struct:** `OwnerAwareWant` (Lines 577-664)

```go
type OwnerAwareWant struct {
	BaseWant   interface{} // The original want (Generator, Queue, Sink, Target, etc.)
	TargetName string
	WantName   string
}

// Exec wraps the base want's execution to add completion notification
func (oaw *OwnerAwareWant) Exec(inputs []chain.Chan, outputs []chain.Chan) bool {
	// Call the original Exec method directly
	if progressable, ok := oaw.BaseWant.(Progressable); ok {
		result := progressable.Exec(inputs, outputs)

		// If want completed successfully and we have a target, notify it
		if result && oaw.TargetName != "" {
			// Emit OwnerCompletionEvent through unified subscription system
			oaw.emitOwnerCompletionEvent()  // LINE 611
		}

		return result
	}
	...
}

// emitOwnerCompletionEvent emits an owner completion event through the unified subscription system
func (oaw *OwnerAwareWant) emitOwnerCompletionEvent() {
	// Get the child want
	want := oaw.GetWant()
	if want == nil {
		return
	}

	// Create OwnerCompletionEvent
	event := &OwnerCompletionEvent{
		BaseEvent: BaseEvent{
			EventType:  EventTypeOwnerCompletion,
			SourceName: oaw.WantName,
			TargetName: oaw.TargetName,      // Parent Target name
			Timestamp:  time.Now(),
			Priority:   10,
		},
		ChildName: oaw.WantName,
	}

	// Emit through subscription system (blocking mode)
	want.GetSubscriptionSystem().Emit(context.Background(), event)
}
```

**Key Points:**
- Wraps any want type (child wants or child Targets)
- On completion, emits `OwnerCompletionEvent` to notify the parent Target
- Parent Target is identified by `TargetName` and responds via subscription system

---

## 7. Parent-Child Completion Event Flow

**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/core/owner_types.go`

**Type:** `TargetCompletionSubscription` (Lines 95-145)

The Target subscribes to completion events:

```go
// subscribeToChildCompletion subscribes the target to child completion events
func (t *Target) subscribeToChildCompletion() {
	subscription := &TargetCompletionSubscription{
		target: t,
	}
	t.GetSubscriptionSystem().Subscribe(EventTypeOwnerCompletion, subscription)  // LINE 92
}

// OnEvent handles the OwnerCompletionEvent
func (tcs *TargetCompletionSubscription) OnEvent(ctx context.Context, event WantEvent) EventResponse {
	completionEvent, ok := event.(*OwnerCompletionEvent)
	if !ok {
		return EventResponse{
			Handled: false,
			Error:   fmt.Errorf("expected OwnerCompletionEvent, got %T", event),
		}
	}

	// Only handle events targeted at this target
	if completionEvent.TargetName != tcs.target.Metadata.Name {
		return EventResponse{Handled: false}
	}

	// Track child completion (LINES 121-126)
	tcs.target.childCompletionMutex.Lock()
	tcs.target.completedChildren[completionEvent.ChildName] = true
	InfoLog("[TARGET:COMPLETION] 📍 Child '%s' completed for target '%s'\n", 
		completionEvent.ChildName, tcs.target.Metadata.Name)
	...
	tcs.target.childCompletionMutex.Unlock()

	// If all children are complete, signal the target via channel (LINES 129-139)
	if allComplete {
		InfoLog("[TARGET:COMPLETION] ✅ All children complete! Signaling target '%s'\n", tcs.target.Metadata.Name)
		select {
		case tcs.target.childrenDone <- true:
			// Signal sent successfully
		default:
			// Channel already has signal, ignore
		}
	}
	...
}
```

---

## 8. Async Want Addition via Reconcile Loop

**File:** `/Users/hiroyukiosaki/work/golang/MyWant/engine/core/chain_builder.go`

**Method:** `ChainBuilder.reconcileLoop()` (Lines 404-462)

When Target calls `AddWantsAsync()`:

```go
case newWants := <-cb.addWantsChan:
	InfoLog("[RECONCILE] Received %d wants to add asynchronously\n", len(newWants))
	// Add wants to config and runtime
	cb.reconcileMutex.Lock()
	for _, want := range newWants {
		if _, exists := cb.wants[want.Metadata.Name]; !exists {
			cb.config.Wants = append(cb.config.Wants, want)
			cb.addWant(want)  // <-- Creates runtime want with function
			InfoLog("[RECONCILE] Added want: %s\n", want.Metadata.Name)
		}
	}
	cb.reconcileMutex.Unlock()
	// Trigger reconciliation to connect and start new wants
	cb.reconcileWants()
```

This flows to `addWant()` which calls `createWantFunction()` which wraps with `OwnerAwareWant` if OwnerReferences exist.

---

## 9. Complete Execution Path Summary

```
Target.Exec() START
    ↓
Target.CreateChildWants()
    ↓
GenericRecipeLoader.LoadConfigFromRecipe()
    ↓ (Returns Config with Want array, no OwnerReferences yet)
    ↓
FOR EACH child want:
    ↓
    ADD OwnerReferences pointing to parent Target (CRITICAL!)
    ↓
Target.builder.AddWantsAsync(childWants)
    ↓ (Asynchronous via reconcile loop)
    ↓
ChainBuilder.reconcileLoop() receives wants
    ↓
ChainBuilder.addWant() is called for each child
    ↓
ChainBuilder.createWantFunction(childWant)
    ↓
IF childWant has OwnerReferences:
    ↓
    Wrap with OwnerAwareWant (enables parent notification)
    ↓
Child want executes...
    ↓
On completion, OwnerAwareWant emits OwnerCompletionEvent
    ↓
Target's TargetCompletionSubscription receives event
    ↓
Target tracks child completion
    ↓
When all children done, signal childrenDone channel
    ↓
Target.Exec() wakes up from <-t.childrenDone
    ↓
Target computes result and finishes
    ↓
Target.Exec() END
```

---

## Key Files and Line Numbers

| Component | File | Lines | Purpose |
|-----------|------|-------|---------|
| Target.Exec() | owner_types.go | 268-342 | Entry point for Target execution |
| Target.CreateChildWants() | owner_types.go | 227-265 | **WHERE OwnerReferences ARE SET** |
| OwnerReferences Setting | owner_types.go | 244-261 | **CRITICAL: Sets parent references** |
| Target.subscribeToChildCompletion() | owner_types.go | 87-93 | Subscribe to child completion events |
| TargetCompletionSubscription.OnEvent() | owner_types.go | 105-145 | Handle completion events |
| ChainBuilder.createWantFunction() | chain_builder.go | 233-273 | Creates want function and wraps if needed |
| OwnerAwareWant wrapping | chain_builder.go | 266-270 | Standard want OwnerAware wrapping |
| Custom Target creation | chain_builder.go | 281-310 | Custom target creation with loader setup |
| GenericRecipeLoader.LoadConfigFromRecipe() | recipe_loader_generic.go | 212-219 | Loads recipe (no OwnerReferences) |
| ChainBuilder.reconcileLoop() | chain_builder.go | 404-462 | Async want addition handler |

---

## Verification Points

1. **Are OwnerReferences set?** YES - in `Target.CreateChildWants()` lines 244-261
2. **When are they set?** During Target.Exec() when child wants are created
3. **How are child wants identified as children?** By OwnerReferences with matching parent ID
4. **How do children notify parent?** Via OwnerAwareWant wrapper emitting OwnerCompletionEvent
5. **How does parent know all children are done?** Via TargetCompletionSubscription and childrenDone channel

