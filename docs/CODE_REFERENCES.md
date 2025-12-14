# Target OwnerReferences - Complete Code References

## 1. Target.Exec() - Entry Point

**File:** engine/src/owner_types.go
**Lines:** 268-342

```go
func (t *Target) Exec(inputs []chain.Chan, outputs []chain.Chan) bool {
	InfoLog("[TARGET] 🎯 Target %s: Managing child nodes with owner references\n", t.Metadata.Name)

	// Dynamically create child wants
	if t.builder != nil {
		InfoLog("[TARGET] 🎯 Target %s: Creating child wants dynamically...\n", t.Metadata.Name)
		childWants := t.CreateChildWants()  // LINE 274: Creates with OwnerReferences

		// Add child wants to the builder's configuration
		for _, childWant := range childWants {
			InfoLog("[TARGET] 🔧 Adding child want: %s (type: %s)\n", childWant.Metadata.Name, childWant.Metadata.Type)
		}

		// Send child wants to reconcile loop asynchronously
		// This avoids deadlock by not trying to acquire locks already held by parent execution
		InfoLog("[TARGET] 🔧 Sending child wants to reconcile loop for async addition...\n")
		if err := t.builder.AddWantsAsync(childWants); err != nil {
			InfoLog("[TARGET] ⚠️  Warning: Failed to send child wants: %v\n", err)
		} else {
			InfoLog("[TARGET] 🔧 Child wants sent to reconcile loop\n")
		}

		// Re-check completion status: children may have already completed before being added
		t.childCompletionMutex.Lock()
		if t.checkAllChildrenComplete() {
			InfoLog("[TARGET] ✅ All children already completed! Signaling completion...\n")
			select {
			case t.childrenDone <- true:
				InfoLog("[TARGET] ✅ Sent completion signal to channel\n")
			default:
				InfoLog("[TARGET] ℹ️  Completion signal already sent\n")
			}
		}
		t.childCompletionMutex.Unlock()
	}

	// Target waits for signal that all children have finished
	InfoLog("[TARGET] 🎯 Target %s: Waiting for all child wants to complete...\n", t.Metadata.Name)
	<-t.childrenDone  // LINE 307: Blocks until all children done
	InfoLog("[TARGET] 🎯 Target %s: All child wants completed, computing result...\n", t.Metadata.Name)

	// Compute and store recipe result
	t.computeTemplateResult()

	// Mark the target as completed
	t.SetStatus(WantStatusCompleted)
	InfoLog("[TARGET] 🎯 Target %s: Result computed, target finishing\n", t.Metadata.Name)

	// Publish completion event to parent target (if this target is a child of another target)
	if len(t.Metadata.OwnerReferences) > 0 {
		for _, ownerRef := range t.Metadata.OwnerReferences {
			if ownerRef.Kind == "Target" {
				InfoLog("[TARGET] 📢 Publishing completion event from %s to parent %s\n",
					t.Metadata.Name, ownerRef.Name)
				completionEvent := &OwnerCompletionEvent{
					BaseEvent: BaseEvent{
						EventType:  EventTypeOwnerCompletion,
						SourceName: t.Metadata.Name,
						TargetName: ownerRef.Name,
						Timestamp:  time.Now(),
						Priority:   1,
					},
					ChildName: t.Metadata.Name,
				}
				globalSys := GetGlobalSubscriptionSystem()
				globalSys.Emit(context.Background(), completionEvent)
				InfoLog("[TARGET] ✅ Completion event emitted from %s to parent %s\n",
					t.Metadata.Name, ownerRef.Name)
			}
		}
	}

	return true
}
```

---

## 2. Target.CreateChildWants() - WHERE OwnerReferences ARE SET

**File:** engine/src/owner_types.go
**Lines:** 227-265

```go
func (t *Target) CreateChildWants() []*Want {
	// Recipe loader is required for target wants
	if t.recipeLoader == nil {
		InfoLog("[TARGET] ❌ Target %s: No recipe loader available - target wants require recipes\n", t.Metadata.Name)
		return []*Want{}
	}

	// Load child wants from recipe (no OwnerReferences yet)
	config, err := t.recipeLoader.LoadConfigFromRecipe(t.RecipePath, t.RecipeParams)
	if err != nil {
		InfoLog("[TARGET] ❌ Target %s: Failed to load recipe %s: %v\n", t.Metadata.Name, t.RecipePath, err)
		return []*Want{}
	}

	InfoLog("[TARGET] ✅ Target %s: Successfully loaded recipe %s with %d child wants\n",
		t.Metadata.Name, t.RecipePath, len(config.Wants))

	// ======== CRITICAL SECTION: ADD OWNER REFERENCES ========
	// Add owner references to all child wants (LINES 244-261)
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

**Key Lines:**
- Line 235: Load recipe (returns Config WITHOUT OwnerReferences)
- Line 245: Loop through each child want
- Lines 246-254: Create OwnerReference with parent metadata
- Line 260: Add "owner: child" label
- Line 265: Return with OwnerReferences properly set

---

## 3. ChainBuilder.createWantFunction() - Automatic OwnerAwareWant Wrapping

**File:** engine/src/chain_builder.go
**Lines:** 233-273

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
		// List available types for better error message
		availableTypes := make([]string, 0, len(cb.registry))
		for typeName := range cb.registry {
			availableTypes = append(availableTypes, typeName)
		}
		customTypes := cb.customRegistry.ListTypes()

		return nil, fmt.Errorf("Unknown want type: '%s'. Available standard types: %v. Available custom types: %v",
			wantType, availableTypes, customTypes)
	}

	wantInstance := factory(want.Metadata, want.Spec)

	// Set agent registry if available and the want instance supports it
	if cb.agentRegistry != nil {
		if wantWithGetWant, ok := wantInstance.(interface{ GetWant() *Want }); ok {
			wantWithGetWant.GetWant().SetAgentRegistry(cb.agentRegistry)
		} else if w, ok := wantInstance.(*Want); ok {
			w.SetAgentRegistry(cb.agentRegistry)
		}
	}

	// ========== AUTOMATIC OWNAWARE WRAPPING (LINES 266-270) ==========
	// Automatically wrap with OwnerAwareWant if the want has owner references
	// This enables parent-child coordination via subscription events
	if len(want.Metadata.OwnerReferences) > 0 {
		wantInstance = NewOwnerAwareWant(wantInstance, want.Metadata)
	}

	return wantInstance, nil
}
```

**Key Section:** Lines 266-270
- Checks if want has OwnerReferences
- If yes, wraps with OwnerAwareWant
- This enables parent notification on completion

---

## 4. OwnerAwareWant.Exec() - Child Completion Notification

**File:** engine/src/owner_types.go
**Lines:** 603-620

```go
// Exec wraps the base want's execution to add completion notification
func (oaw *OwnerAwareWant) Exec(inputs []chain.Chan, outputs []chain.Chan) bool {
	// Call the original Exec method directly
	if chainWant, ok := oaw.BaseWant.(Progressable); ok {
		result := progressable.Exec(inputs, outputs)

		// If want completed successfully and we have a target, notify it
		if result && oaw.TargetName != "" {
			// Emit OwnerCompletionEvent through unified subscription system
			oaw.emitOwnerCompletionEvent()  // LINE 611: Notify parent
		}

		return result
	} else {
		// Fallback for non-Progressable types
		InfoLog("[TARGET] ⚠️  Want %s: No Exec method available\n", oaw.WantName)
		return true
	}
}
```

**Key Logic:**
- Line 606: Execute the wrapped want
- Line 611: If successful, emit completion event
- Parent Target receives this event via subscription system

---

## 5. TargetCompletionSubscription.OnEvent() - Parent Receives Completion

**File:** engine/src/owner_types.go
**Lines:** 105-145

```go
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

	// Track child completion
	tcs.target.childCompletionMutex.Lock()
	tcs.target.completedChildren[completionEvent.ChildName] = true
	InfoLog("[TARGET:COMPLETION] 📍 Child '%s' completed for target '%s'\n", completionEvent.ChildName, tcs.target.Metadata.Name)
	InfoLog("[TARGET:COMPLETION] 📊 Total children: %d, Completed: %d\n", len(tcs.target.childWants), len(tcs.target.completedChildren))
	allComplete := tcs.target.checkAllChildrenComplete()
	tcs.target.childCompletionMutex.Unlock()

	// If all children are complete, signal the target via channel
	if allComplete {
		InfoLog("[TARGET:COMPLETION] ✅ All children complete! Signaling target '%s'\n", tcs.target.Metadata.Name)
		select {
		case tcs.target.childrenDone <- true:
			// Signal sent successfully
		default:
			// Channel already has signal, ignore
		}
	} else {
		InfoLog("[TARGET:COMPLETION] ⏳ Not all children complete yet for target '%s'\n", tcs.target.Metadata.Name)
	}

	return EventResponse{
		Handled:          true,
		ExecutionControl: ExecutionContinue,
	}
}
```

**Key Logic:**
- Line 116: Check event is for this target
- Line 122: Track child completion
- Line 125: Check if all children done
- Line 132: Signal channel if all done

---

## 6. ChainBuilder.reconcileLoop() - Async Want Addition

**File:** engine/src/chain_builder.go
**Lines:** 422-435

```go
case newWants := <-cb.addWantsChan:
	InfoLog("[RECONCILE] Received %d wants to add asynchronously\n", len(newWants))
	// Add wants to config and runtime
	cb.reconcileMutex.Lock()
	for _, want := range newWants {
		if _, exists := cb.wants[want.Metadata.Name]; !exists {
			cb.config.Wants = append(cb.config.Wants, want)
			cb.addWant(want)  // <-- Calls addWant which creates runtime want
			InfoLog("[RECONCILE] Added want: %s\n", want.Metadata.Name)
		}
	}
	cb.reconcileMutex.Unlock()
	// Trigger reconciliation to connect and start new wants
	cb.reconcileWants()
```

**Key Flow:**
- Receives wants from Target.AddWantsAsync()
- Line 429: Calls addWant() for each want
- addWant() calls createWantFunction() which wraps if OwnerReferences exist
- Line 435: Triggers reconciliation to connect and start new wants

---

## 7. GenericRecipeLoader.LoadConfigFromRecipe() - Recipe Loading (No OwnerReferences)

**File:** engine/src/recipe_loader_generic.go
**Lines:** 212-219

```go
// LoadConfigFromRecipe loads configuration from any recipe type
func (grl *GenericRecipeLoader) LoadConfigFromRecipe(recipePath string, params map[string]interface{}) (Config, error) {
	recipeConfig, err := grl.LoadRecipe(recipePath, params)
	if err != nil {
		return Config{}, err
	}

	return recipeConfig.Config, nil
}
```

**Important Note:**
- This returns Config with Want array
- At this point, wants DO NOT have OwnerReferences
- OwnerReferences are added by Target.CreateChildWants() AFTER this call

---

## Summary of Code Locations

| Component | File | Lines | Code Path |
|-----------|------|-------|-----------|
| Target Entry | owner_types.go | 268-342 | Target.Exec() |
| Create Child Wants | owner_types.go | 227-265 | Target.CreateChildWants() |
| **SET OwnerReferences** | owner_types.go | 244-261 | **In CreateChildWants()** |
| Add Wants Async | owner_types.go | 284 | builder.AddWantsAsync() |
| Wrap with OwnerAware | chain_builder.go | 266-270 | createWantFunction() |
| Child Notify Parent | owner_types.go | 611 | OwnerAwareWant.emitOwnerCompletionEvent() |
| Parent Handle Completion | owner_types.go | 106-145 | TargetCompletionSubscription.OnEvent() |
| Reconcile Add Wants | chain_builder.go | 422-435 | reconcileLoop() |
| Load Recipe | recipe_loader_generic.go | 212-219 | LoadConfigFromRecipe() |

