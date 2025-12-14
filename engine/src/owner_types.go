package mywant

import (
	"context"
	"fmt"
	"mywant/engine/src/chain"
	"reflect"
	"strings"
	"sync"
	"time"
)

// extractIntParam extracts an integer parameter with type conversion and default fallback
func extractIntParam(params map[string]interface{}, key string, defaultValue int) int {
	if value, ok := params[key]; ok {
		if intVal, ok := value.(int); ok {
			return intVal
		} else if floatVal, ok := value.(float64); ok {
			return int(floatVal)
		}
	}
	return defaultValue
}

// Target represents a parent want that creates and manages child wants
type Target struct {
	Want
	MaxDisplay             int
	Description            string                 // Human-readable description of this target
	RecipePath             string                 // Path to the recipe file to use for child creation
	RecipeParams           map[string]interface{} // Parameters to pass to recipe (derived from spec.params)
	parameterSubscriptions map[string][]string    // Map of parameter names to child want names that subscribe to them
	childWants             []*Want
	completedChildren      map[string]bool      // Track which children have completed
	childCompletionMutex   sync.Mutex           // Protect completedChildren map
	builder                *ChainBuilder        // Reference to builder for dynamic want creation
	recipeLoader           *GenericRecipeLoader // Reference to generic recipe loader
	stateMutex             sync.RWMutex         // Mutex to protect concurrent state updates
	childrenDone           chan bool            // Signal when all children complete
	childrenCreated        bool                 // Track if child wants have been created
	childCount             int                  // Count of child wants
}

// NewTarget creates a new target want
func NewTarget(metadata Metadata, spec WantSpec) *Target {
	target := &Target{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		MaxDisplay:             1000,
		RecipePath:             "../recipes/queue-system.yaml", // Default recipe path
		RecipeParams:           make(map[string]interface{}),
		parameterSubscriptions: make(map[string][]string),
		childWants:             make([]*Want, 0),
		completedChildren:      make(map[string]bool),
		childrenDone:           make(chan bool, 1), // Signal channel for subscription system
	}

	// Extract target-specific configuration from params
	target.MaxDisplay = extractIntParam(spec.Params, "max_display", target.MaxDisplay)

	// Recipe path will be set by custom target type registration

	// Separate recipe parameters from target-specific parameters
	targetSpecificParams := map[string]bool{
		"max_display": true,
	}

	// Collect recipe parameters (excluding target-specific ones)
	target.RecipeParams = make(map[string]interface{})
	for key, value := range spec.Params {
		if !targetSpecificParams[key] {
			target.RecipeParams[key] = value
		}
	}

	// Register with want system
	RegisterWant(&target.Want)

	// Subscribe to OwnerCompletionEvents
	target.subscribeToChildCompletion()

	return target
}

// subscribeToChildCompletion subscribes the target to child completion events
func (t *Target) subscribeToChildCompletion() {
	subscription := &TargetCompletionSubscription{
		target: t,
	}
	t.GetSubscriptionSystem().Subscribe(EventTypeOwnerCompletion, subscription)
}

// TargetCompletionSubscription handles child completion events for a target
type TargetCompletionSubscription struct {
	target *Target
}
func (tcs *TargetCompletionSubscription) GetSubscriberName() string {
	return tcs.target.Metadata.Name + "-completion-handler"
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

	// Track child completion
	tcs.target.childCompletionMutex.Lock()
	tcs.target.completedChildren[completionEvent.ChildName] = true
	allComplete := tcs.target.checkAllChildrenComplete()
	tcs.target.childCompletionMutex.Unlock()

	// If all children are complete, signal the target via channel (only for old-style blocking) Note: New implementation uses polling in Exec(), not channel-based signaling
	if allComplete {
		select {
		case tcs.target.childrenDone <- true:
			// Signal sent (legacy support)
		default:
			// Channel already has signal, ignore
		}
	}

	return EventResponse{
		Handled:          true,
		ExecutionControl: ExecutionContinue,
	}
}
func (t *Target) checkAllChildrenComplete() bool {
	// If we have no children yet AND no completions have arrived, we can't be complete (children are still being added asynchronously)
	if len(t.childWants) == 0 && len(t.completedChildren) == 0 {
		return false
	}

	// If we have completed children but no childWants yet, they're still being added asynchronously
	if len(t.childWants) == 0 && len(t.completedChildren) > 0 {
		return false
	}

	for _, child := range t.childWants {
		if !t.completedChildren[child.Metadata.Name] {
			return false
		}
	}
	return true
}
func (t *Target) SetBuilder(builder *ChainBuilder) {
	t.builder = builder
}
func (t *Target) SetRecipeLoader(loader *GenericRecipeLoader) {
	t.recipeLoader = loader
	// Resolve recipe parameters using recipe defaults when loader is available
	t.resolveRecipeParameters()
}

// resolveRecipeParameters uses the recipe system to resolve parameters with proper defaults
func (t *Target) resolveRecipeParameters() {
	if t.recipeLoader == nil {
		return
	}
	recipeParams, err := t.recipeLoader.GetRecipeParameters(t.RecipePath)
	if err != nil {
		return
	}
	resolvedParams := make(map[string]interface{})
	for key, value := range recipeParams {
		resolvedParams[key] = value
	}

	// Override with provided recipe parameters
	for key, value := range t.RecipeParams {
		resolvedParams[key] = value
	}
	resolvedParams["targetName"] = t.Metadata.Name
	if _, hasCount := resolvedParams["count"]; !hasCount {
		resolvedParams["count"] = t.MaxDisplay
	}

	// CRITICAL: Override prefix with target name to prevent label cross-contamination Each target must have a unique prefix to namespace its child wants' labels
	resolvedParams["prefix"] = t.Metadata.Name

	// Update recipe parameters with resolved values
	t.RecipeParams = resolvedParams
}
func (t *Target) CreateChildWants() []*Want {
	// Recipe loader is required for target wants
	if t.recipeLoader == nil {
		return []*Want{}
	}

	// Load child wants from recipe
	config, err := t.recipeLoader.LoadConfigFromRecipe(t.RecipePath, t.RecipeParams)
	if err != nil {
		return []*Want{}
	}

	// VALIDATION: Prevent want type name conflicts between parent and children This prevents infinite loops where a want type references a recipe that contains a want of the same type, which would cause recursive instantiation
	parentType := t.Metadata.Type
	for _, childWant := range config.Wants {
		if childWant.Metadata.Type == parentType {
			t.StoreLog(fmt.Sprintf("[TARGET] ❌ ERROR: Target %s (type=%s) cannot have child wants of the same type from recipe %s\n",
				t.Metadata.Name, parentType, t.RecipePath))
			t.StoreLog(fmt.Sprintf("[TARGET] 💡 HINT: Child want type '%s' must be different from parent type '%s' to prevent recursive instantiation\n",
				childWant.Metadata.Type, parentType))
			return []*Want{}
		}
	}
	for i := range config.Wants {
		config.Wants[i].Metadata.OwnerReferences = []OwnerReference{
			{
				APIVersion:         "mywant/v1",
				Kind:               "Want",
				Name:               t.Metadata.Name,
				ID:                 t.Metadata.ID,
				Controller:         true,
				BlockOwnerDeletion: true,
			},
		}
		if config.Wants[i].Metadata.Labels == nil {
			config.Wants[i].Metadata.Labels = make(map[string]string)
		}
		config.Wants[i].Metadata.Labels["owner"] = "child"
	}

	t.childWants = config.Wants
	return t.childWants
}

// IsAchieved checks if target is complete (all children created and completed)
func (t *Target) IsAchieved() bool {
	if !t.childrenCreated {
		return false
	}

	t.childCompletionMutex.Lock()
	allComplete := t.checkAllChildrenComplete()
	t.childCompletionMutex.Unlock()

	return allComplete
}

// Progress implements the Progressable interface for Target with direct execution
func (t *Target) Progress() {
	// Phase 1: Create child wants (only once)
	if !t.childrenCreated && t.builder != nil {
		childWants := t.CreateChildWants()
		if err := t.builder.AddWantsAsync(childWants); err != nil {
			t.StoreLog(fmt.Sprintf("[TARGET] ⚠️  Warning: Failed to send child wants: %v\n", err))
			return
		}

		// Mark that we've created children
		t.childrenCreated = true
		return // Not complete yet, waiting for children
	}

	// Phase 2: Check if all children have completed
	if t.childrenCreated {
		t.childCompletionMutex.Lock()
		allComplete := t.checkAllChildrenComplete()
		t.childCompletionMutex.Unlock()

		if allComplete {
			// Only compute result once - check if already completed
			if t.Status != WantStatusAchieved {
				// Compute and store recipe result
				t.computeTemplateResult()

				// Mark the target as completed
				t.SetStatus(WantStatusAchieved)
			}
			return
		}

		// Children not all complete yet
		return
	}
}

// UpdateParameter updates a parameter and pushes it to child wants
func (t *Target) UpdateParameter(paramName string, paramValue interface{}) {
	// Update our own parameter first
	t.Want.UpdateParameter(paramName, paramValue)

	// Also update the RecipeParams if this is a recipe parameter
	if t.RecipeParams != nil {
		t.RecipeParams[paramName] = paramValue
	}

	// Push parameter change to child wants
	t.PushParameterToChildren(paramName, paramValue)

	t.StoreLog(fmt.Sprintf("[TARGET] 🎯 Target %s: Parameter %s updated to %v and pushed to children\n",
		t.Metadata.Name, paramName, paramValue))
}

// ChangeParameter provides a convenient API to change target parameters at runtime
func (t *Target) ChangeParameter(paramName string, paramValue interface{}) {
	t.StoreLog(fmt.Sprintf("[TARGET] 🔄 Target %s: Changing parameter %s from %v to %v\n",
		t.Metadata.Name, paramName, t.Spec.Params[paramName], paramValue))
	t.UpdateParameter(paramName, paramValue)
}
func (t *Target) GetParameterValue(paramName string) interface{} {
	if value, ok := t.Spec.Params[paramName]; ok {
		return value
	}
	return nil
}

// PushParameterToChildren propagates parameter changes to all child wants
func (t *Target) PushParameterToChildren(paramName string, paramValue interface{}) {
	if t.builder == nil {
		return
	}
	for wantName, runtimeWant := range t.builder.wants {
		if t.isChildWant(runtimeWant.want) {
			// Map target parameters to child parameters based on naming patterns
			childParamName := t.mapParameterNameForChild(paramName, runtimeWant.want.Metadata.Type)
			if childParamName != "" {
				// Update the child's parameter
				runtimeWant.want.UpdateParameter(childParamName, paramValue)

				t.StoreLog(fmt.Sprintf("[TARGET] 🔄 Target %s → Child %s: %s=%v (mapped to %s)\n",
					t.Metadata.Name, wantName, paramName, paramValue, childParamName))
			}
		}
	}
}

// isChildWant checks if a want is a child of this target
func (t *Target) isChildWant(want *Want) bool {
	for _, ownerRef := range want.Metadata.OwnerReferences {
		if ownerRef.Controller && ownerRef.Kind == "Want" && ownerRef.Name == t.Metadata.Name {
			return true
		}
	}
	return false
}

// mapParameterNameForChild maps target parameters to child-specific parameter names
func (t *Target) mapParameterNameForChild(targetParamName string, childWantType string) string {
	// Define parameter mapping rules for different child types
	parameterMappings := map[string]map[string]string{
		"queue": {
			"primary_service_time":   "service_time",
			"secondary_service_time": "service_time",
			"final_service_time":     "service_time",
			"service_time":           "service_time",
		},
		"numbers": {
			"primary_count":   "count",
			"secondary_count": "count",
			"primary_rate":    "rate",
			"secondary_rate":  "rate",
			"count":           "count",
			"rate":            "rate",
		},
		"sink": {
			"display_format": "display_format",
		},
	}
	if typeMapping, exists := parameterMappings[childWantType]; exists {
		if childParam, exists := typeMapping[targetParamName]; exists {
			return childParam
		}
	}

	// If no specific mapping, try direct mapping
	return targetParamName
}

// computeTemplateResult computes the result from child wants using recipe-defined result specs
func (t *Target) computeTemplateResult() {
	// Use mutex to prevent concurrent map access with state updates
	t.stateMutex.Lock()
	defer t.stateMutex.Unlock()

	if t.recipeLoader == nil {
		t.StoreLog(fmt.Sprintf("[TARGET] ⚠️  Target %s: No recipe loader available for result computation\n", t.Metadata.Name))
		t.computeFallbackResultUnsafe() // Use unsafe version since we already have the mutex
		return
	}
	recipeResult, err := t.recipeLoader.GetRecipeResult(t.RecipePath)
	if err != nil {
		t.StoreLog(fmt.Sprintf("[TARGET] ⚠️  Target %s: Failed to load recipe result definition: %v\n", t.Metadata.Name, err))
		t.computeFallbackResultUnsafe() // Use unsafe version since we already have the mutex
		return
	}

	if recipeResult == nil {
		t.StoreLog(fmt.Sprintf("[TARGET] ⚠️  Target %s: No result definition in recipe, using fallback\n", t.Metadata.Name))
		t.computeFallbackResultUnsafe() // Use unsafe version since we already have the mutex
		return
	}
	allWantStates := t.builder.GetAllWantStates()
	childWantsByName := make(map[string]*Want)
	for _, want := range allWantStates {
		for _, ownerRef := range want.Metadata.OwnerReferences {
			if ownerRef.Controller && ownerRef.Kind == "Want" && ownerRef.Name == t.Metadata.Name {
				childWantsByName[want.Metadata.Type] = want

				// Also extract and store by short type name For "qnet queue" -> also store as "queue"
				typeParts := strings.Fields(want.Metadata.Type)
				if len(typeParts) > 0 {
					lastPart := typeParts[len(typeParts)-1]
					childWantsByName[lastPart] = want
				}

				// Also store by exact want name for recipes that specify exact names
				if want.Metadata.Name != "" {
					childWantsByName[want.Metadata.Name] = want
				}
				break
			}
		}
	}

	// Log child want names for debugging result extraction
	childNames := make([]string, 0, len(childWantsByName))
	for name := range childWantsByName {
		childNames = append(childNames, name)
	}
	t.StoreLog(fmt.Sprintf("🧮 Target %s: Found %d child wants for recipe-defined result computation: %v\n", t.Metadata.Name, len(childWantsByName), childNames))

	// Stats are now stored in State - no separate initialization needed
	var primaryResult interface{}
	metrics := make(map[string]interface{})

	for i, resultSpec := range *recipeResult {
		resultValue := t.getResultFromSpec(resultSpec, childWantsByName)
		statName := strings.TrimPrefix(resultSpec.StatName, ".")
		if statName == "" {
			statName = "all_metrics"
		}
		metricKey := resultSpec.WantName + "_" + statName
		metrics[metricKey] = resultValue

		// Use first result as primary result for backward compatibility
		if i == 0 {
			primaryResult = resultValue
			t.StoreLog(fmt.Sprintf("[TARGET] ✅ Target %s: Primary result (%s from %s): %v\n", t.Metadata.Name, resultSpec.StatName, resultSpec.WantName, primaryResult))
		} else {
			t.StoreLog(fmt.Sprintf("📊 Target %s: Metric %s (%s from %s): %v\n", t.Metadata.Name, resultSpec.Description, resultSpec.StatName, resultSpec.WantName, resultValue))
		}
	}
	for i, resultSpec := range *recipeResult {
		statName := strings.TrimPrefix(resultSpec.StatName, ".")
		if statName == "" {
			statName = "all_metrics"
		}
		metricKey := resultSpec.WantName + "_" + statName
		t.StoreState(metricKey, metrics[metricKey])
		if i == 0 {
			statLabel := strings.TrimPrefix(resultSpec.StatName, ".")
			if statLabel == "" {
				statLabel = resultSpec.Description
			}
			t.StoreState("result", fmt.Sprintf("%s: %v", statLabel, primaryResult))
		}
	}
	t.childCount = len(childWantsByName)

	t.StoreLog(fmt.Sprintf("[TARGET] ✅ Target %s: Recipe-defined result computation completed\n", t.Metadata.Name))
}
func (t *Target) addChildWantsToMemory() error {
	// This is a placeholder - in a real implementation, this would interact with the ChainBuilder to add wants to the memory file For now, we'll assume the reconcile loop will pick up the wants
	t.StoreLog(fmt.Sprintf("[TARGET] 🔧 Adding %d child wants to memory configuration\n", len(t.childWants)))
	return nil
}

// OwnerAwareWant wraps any want type to add parent notification capability
type OwnerAwareWant struct {
	BaseWant   interface{} // The original want (Generator, Queue, Sink, etc.)
	Want       *Want       // Direct reference to Want (extracted at creation time)
	TargetName string
	WantName   string
}

// NewOwnerAwareWant creates a wrapper that adds parent notification to any want wantPtr is the Want pointer extracted from baseWant (can be nil for some types)
func NewOwnerAwareWant(baseWant interface{}, metadata Metadata, wantPtr *Want) *OwnerAwareWant {
	targetName := ""
	for _, ownerRef := range metadata.OwnerReferences {
		if ownerRef.Controller && ownerRef.Kind == "Want" {
			targetName = ownerRef.Name
			break
		}
	}

	// If wantPtr is nil, try to extract it using reflection for custom types with embedded Want
	if wantPtr == nil {
		wantPtr = extractWantViaReflection(baseWant)
	}

	return &OwnerAwareWant{
		BaseWant:   baseWant,
		Want:       wantPtr,
		TargetName: targetName,
		WantName:   metadata.Name,
	}
}

// extractWantViaReflection extracts Want pointer from custom types with embedded Want field
func extractWantViaReflection(baseWant interface{}) *Want {
	if baseWant == nil {
		return nil
	}

	// Use reflection to inspect the value
	v := reflect.ValueOf(baseWant)
	if v.Kind() == reflect.Ptr {
		elem := v.Elem()

		// Try to find a Want field in the struct
		if elem.Kind() == reflect.Struct {
			wantField := elem.FieldByName("Want")

			if wantField.IsValid() {
				// Case 1: Embedded Want struct (e.g., RestaurantWant.Want)
				if wantField.Kind() == reflect.Struct {
					if wantField.CanAddr() {
						wantAddr := wantField.Addr()
						// Type assert to *Want
						if want, ok := wantAddr.Interface().(*Want); ok {
							return want
						}
					}
				} else if wantField.Kind() == reflect.Ptr {
					// Case 2: Want pointer field (e.g., OwnerAwareWant.Want is *Want)
					if !wantField.IsNil() {
						if want, ok := wantField.Interface().(*Want); ok {
							return want
						}
					}
				}
			}
		}
	}

	return nil
}

// IsAchieved checks if the wrapped want is complete
func (oaw *OwnerAwareWant) IsAchieved() bool {
	if progressable, ok := oaw.BaseWant.(Progressable); ok {
		return progressable.IsAchieved()
	}
	// Fallback for non-Progressable types
	return true
}

// Progress wraps the base want's execution to add completion notification
func (oaw *OwnerAwareWant) Progress() {
	// Call the original Progress method directly
	if progressable, ok := oaw.BaseWant.(Progressable); ok {
		progressable.Progress()

		// If want is now achieved and we have a target, notify it
		if progressable.IsAchieved() && oaw.TargetName != "" {
			// Emit OwnerCompletionEvent through unified subscription system
			oaw.emitOwnerCompletionEvent()
		}
	}
}

// BeginExecCycle delegates to the stored Want to start batching state changes
func (oaw *OwnerAwareWant) BeginExecCycle() {
	if oaw.Want != nil {
		oaw.Want.BeginExecCycle()
	}
}

// EndExecCycle delegates to the stored Want to commit batched state changes
func (oaw *OwnerAwareWant) EndExecCycle() {
	if oaw.Want != nil {
		oaw.Want.EndExecCycle()
	}
}
func (oaw *OwnerAwareWant) SetPaths(inPaths []PathInfo, outPaths []PathInfo) {
	if oaw.Want != nil {
		oaw.Want.SetPaths(inPaths, outPaths)
	}
}

// Channel delegation methods for OwnerAwareWant These methods delegate to the stored Want pointer to provide access to paths.In and paths.Out
func (oaw *OwnerAwareWant) GetInputChannel(index int) (chain.Chan, bool) {
	if oaw.Want != nil {
		return oaw.Want.GetInputChannel(index)
	}
	return nil, true
}
func (oaw *OwnerAwareWant) GetOutputChannel(index int) (chain.Chan, bool) {
	if oaw.Want != nil {
		return oaw.Want.GetOutputChannel(index)
	}
	return nil, true
}
func (oaw *OwnerAwareWant) GetFirstInputChannel() (chain.Chan, bool) {
	if oaw.Want != nil {
		return oaw.Want.GetFirstInputChannel()
	}
	return nil, true
}
func (oaw *OwnerAwareWant) GetFirstOutputChannel() (chain.Chan, bool) {
	if oaw.Want != nil {
		return oaw.Want.GetFirstOutputChannel()
	}
	return nil, true
}
func (oaw *OwnerAwareWant) GetMetadata() *Metadata {
	if oaw.Want != nil {
		return oaw.Want.GetMetadata()
	}
	return nil
}
func (oaw *OwnerAwareWant) GetSpec() *WantSpec {
	if oaw.Want != nil {
		return oaw.Want.GetSpec()
	}
	return nil
}

// emitOwnerCompletionEvent emits an owner completion event through the unified subscription system
func (oaw *OwnerAwareWant) emitOwnerCompletionEvent() {
	// Use the Want pointer stored at creation time
	if oaw.Want == nil {
		// Only log warnings if we actually have a target (not a standalone want)
		if oaw.TargetName != "" {
			InfoLog("[TARGET] ⚠️  OwnerAwareWant %s: Want pointer is nil, cannot emit completion event\n", oaw.WantName)
		}
		return
	}
	event := &OwnerCompletionEvent{
		BaseEvent: BaseEvent{
			EventType:  EventTypeOwnerCompletion,
			SourceName: oaw.WantName,
			TargetName: oaw.TargetName,
			Timestamp:  time.Now(),
			Priority:   10, // High priority for completion events
		},
		ChildName: oaw.WantName,
	}

	// Emit through subscription system (blocking mode)
	oaw.Want.GetSubscriptionSystem().Emit(context.Background(), event)
}
func (oaw *OwnerAwareWant) setupStateNotifications(want *Want) {
	if oaw.Want == nil {
		oaw.Want = want
	}
	// For now, we'll rely on the child wants to call our notification method directly This is a placeholder for a more sophisticated hooking mechanism
}

// RegisterOwnerWantTypes registers the owner-based want types with a ChainBuilder
func RegisterOwnerWantTypes(builder *ChainBuilder) {
	recipeLoader := NewGenericRecipeLoader("recipes")

	// Register target type with recipe support
	builder.RegisterWantType("target", func(metadata Metadata, spec WantSpec) Progressable {
		target := NewTarget(metadata, spec)
		target.SetBuilder(builder)           // Set builder reference for dynamic want creation
		target.SetRecipeLoader(recipeLoader) // Set recipe loader for external recipes
		return target
	})

	// Note: OwnerAware wrapping is now automatic in ChainBuilder.createWantFunction() All wants with OwnerReferences are automatically wrapped at creation time, eliminating the need for registration-time wrapping and registration order dependencies. 
	// This means: 1. Domain types can be registered in any order (QNet, Travel, etc.) 2. No need for separate "NoOwner" builder variants 3. Wrapping happens at runtime based on actual metadata, not factory registration
}
func (t *Target) getResultFromSpec(spec RecipeResultSpec, childWants map[string]*Want) interface{} {
	want, exists := childWants[spec.WantName]
	if !exists {
		// Log available want names to help with debugging
		availableNames := make([]string, 0, len(childWants))
		for name := range childWants {
			availableNames = append(availableNames, name)
		}
		t.StoreLog(fmt.Sprintf("[TARGET] ⚠️  Target %s: Want '%s' not found for result computation (available: %v)\n", t.Metadata.Name, spec.WantName, availableNames))
		return 0
	}
	statName := spec.StatName
	if strings.HasPrefix(statName, ".") {
		return t.extractValueByPath(want.State, statName)
	}

	// Try to get the specified stat from the want's dynamic Stats map
	if want.State != nil {
		// Try exact stat name first
		if value, ok := want.State[statName]; ok {
			return value
		}
		// Try lowercase version
		if value, ok := want.State[strings.ToLower(statName)]; ok {
			return value
		}
		// Try common variations
		if spec.StatName == "TotalProcessed" {
			if value, ok := want.State["total_processed"]; ok {
				return value
			}
			if value, ok := want.State["totalprocessed"]; ok {
				return value
			}
		}
	}

	// Fallback: try to get from State map
	if value, ok := want.State[spec.StatName]; ok {
		return value
	}
	if value, ok := want.State[strings.ToLower(spec.StatName)]; ok {
		return value
	}

	t.StoreLog(fmt.Sprintf("[TARGET] ⚠️  Target %s: Stat '%s' not found in want '%s' (available stats: %v)\n", t.Metadata.Name, spec.StatName, spec.WantName, want.State))
	return 0
}

// extractValueByPath extracts values using JSON path-like syntax
func (t *Target) extractValueByPath(data map[string]interface{}, path string) interface{} {
	if path == "." {
		return data
	}
	if strings.HasPrefix(path, ".") {
		fieldName := strings.TrimPrefix(path, ".")

		// Simple field access
		if value, ok := data[fieldName]; ok {
			return value
		}

		// Try common field name variations
		if value, ok := data[strings.ToLower(fieldName)]; ok {
			return value
		}
		if strings.Contains(fieldName, "_") {
			// Try camelCase version
			camelCase := t.toCamelCase(fieldName)
			if value, ok := data[camelCase]; ok {
				return value
			}
		} else {
			// Try snake_case version
			snakeCase := t.toSnakeCase(fieldName)
			if value, ok := data[snakeCase]; ok {
				return value
			}
		}
	}

	return nil
}

// toCamelCase converts snake_case to camelCase
func (t *Target) toCamelCase(str string) string {
	parts := strings.Split(str, "_")
	if len(parts) <= 1 {
		return str
	}

	result := parts[0]
	for _, part := range parts[1:] {
		if len(part) > 0 {
			result += strings.ToUpper(string(part[0])) + strings.ToLower(part[1:])
		}
	}
	return result
}

// toSnakeCase converts camelCase to snake_case
func (t *Target) toSnakeCase(str string) string {
	var result []rune
	for i, r := range str {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// computeFallbackResultUnsafe provides simple aggregation without mutex protection (caller must hold mutex)
func (t *Target) computeFallbackResultUnsafe() {
	allWantStates := t.builder.GetAllWantStates()
	var childWants []*Want

	// Filter to only wants owned by this target
	for _, want := range allWantStates {
		for _, ownerRef := range want.Metadata.OwnerReferences {
			if ownerRef.Controller && ownerRef.Kind == "Want" && ownerRef.Name == t.Metadata.Name {
				childWants = append(childWants, want)
				break
			}
		}
	}

	t.StoreLog(fmt.Sprintf("🧮 Target %s: Using fallback result computation for %d child wants\n", t.Metadata.Name, len(childWants)))

	// Simple aggregate result from child wants using dynamic stats
	totalProcessed := 0
	for _, child := range childWants {
		if child.State != nil {
			if processed, ok := child.State["total_processed"]; ok {
				if processedInt, ok := processed.(int); ok {
					totalProcessed += processedInt
				}
			} else if processed, ok := child.State["TotalProcessed"]; ok {
				if processedInt, ok := processed.(int); ok {
					totalProcessed += processedInt
				}
			}
		}
	}
	if t.State == nil {
		t.State = make(map[string]interface{})
	}
	t.StoreState("result", fmt.Sprintf("processed: %d", totalProcessed))
	t.childCount = len(childWants)
	t.StoreLog(fmt.Sprintf("[TARGET] ✅ Target %s: Fallback result computed - processed %d items from %d child wants\n", t.Metadata.Name, totalProcessed, len(childWants)))
}
