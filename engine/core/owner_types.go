package mywant

import (
	"context"
	"encoding/json"
	"fmt"
	"mywant/engine/core/chain"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

// extractIntParam extracts an integer parameter with type conversion and default fallback
func extractIntParam(params map[string]any, key string, defaultValue int) int {
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
	Description            string              // Human-readable description of this target
	RecipePath             string              // Path to the recipe file to use for child creation
	RecipeParams           map[string]any      // Parameters to pass to recipe (derived from spec.params)
	parameterSubscriptions map[string][]string // Map of parameter names to child want names that subscribe to them
	childWants             []*Want
	completedChildren      map[string]bool      // Track which children have completed
	childCompletionMutex   sync.Mutex           // Protect completedChildren map
	builder                *ChainBuilder        // Reference to builder for dynamic want creation
	recipeLoader           *GenericRecipeLoader // Reference to generic recipe loader
	stateMutex             sync.RWMutex         // Mutex to protect concurrent state updates
	childrenDone           chan bool            // Signal when all children complete
	childrenCreated        bool                 // Track if child wants have been created
	childCount             int                  // Count of child wants
	stateNotify            chan struct{}         // Notified when MergeParentState writes pending state
}

// NewTarget creates a new target want
func NewTarget(metadata Metadata, spec WantSpec) *Target {
	target := &Target{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Status:   WantStatusIdle,
			State:    make(map[string]any),
		},
		MaxDisplay:   1000,
		RecipePath:   filepath.Join(RecipesDir, "empty.yaml"), // Relative to project root
		RecipeParams: make(map[string]any), parameterSubscriptions: make(map[string][]string),
		childWants:        make([]*Want, 0),
		completedChildren: make(map[string]bool),
		childrenDone: make(chan bool, 1), // Signal channel for subscription system
		stateNotify:  make(chan struct{}, 64),
	}

	// Hook: whenever Want.MergeState is called on the embedded Want, signal stateNotify.
	// This works even when the caller holds a *Want pointer (not *Target) due to embedding.
	target.Want.onMergeState = func() {
		select {
		case target.stateNotify <- struct{}{}:
		default:
		}
	}

	// Extract target-specific configuration from params
	target.MaxDisplay = extractIntParam(spec.Params, "max_display", target.MaxDisplay)

	// Recipe path will be set by custom target type registration

	// Separate recipe parameters from target-specific parameters
	targetSpecificParams := map[string]bool{
		"max_display": true,
	}

	// Collect recipe parameters (excluding target-specific ones)
	target.RecipeParams = make(map[string]any)
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

	// GUARD: If target is already achieved, no need to process further completion events
	if tcs.target.GetStatus() == WantStatusAchieved {
		return EventResponse{Handled: true}
	}

	// Only handle events targeted at this target
	if completionEvent.TargetName != tcs.target.Metadata.Name {
		return EventResponse{Handled: false}
	}

	// Track child completion
	tcs.target.childCompletionMutex.Lock()
	tcs.target.completedChildren[completionEvent.ChildName] = true
	tcs.target.childCompletionMutex.Unlock()

	allComplete := tcs.target.checkAllChildrenComplete()

	// If all children are complete, wake up Target.Progress() so it can finalize
	if allComplete {
		// Signal stateNotify so Progress() unblocks and checks completion
		select {
		case tcs.target.stateNotify <- struct{}{}:
		default:
		}

		// Also signal the legacy channel (kept for compatibility)
		select {
		case tcs.target.childrenDone <- true:
		default:
		}
	}

	return EventResponse{
		Handled:          true,
		ExecutionControl: ExecutionContinue,
	}
}
func (t *Target) checkAllChildrenComplete() bool {
	// If we have no children, target is immediately complete (nothing to do)
	if len(t.childWants) == 0 {
		return true
	}

	// For targets with children (recipe or dynamic), check both map AND actual Status
	// This matches the logic in Progress() and ensures consistency
	for _, child := range t.childWants {
		// A child is complete if either:
		// 1. It's marked in completedChildren map, OR
		// 2. Its actual Status is ACHIEVED
		completed := t.completedChildren[child.Metadata.Name] || child.Status == WantStatusAchieved
		if !completed {
			return false
		}
	}

	// All children are complete
	return true
}

// BeginProgressCycle overrides Want.BeginProgressCycle to NOT wipe pendingStateChanges.
// Target accumulates MergeParentState writes across iterations; wiping would lose async data.
func (t *Target) BeginProgressCycle() {
	t.inExecCycle = true
	t.execCycleCount++
	// Do NOT wipe pendingStateChanges — preserve accumulated MergeParentState data
	// pendingLogs and pendingParameterChanges are still reset each cycle
	t.pendingParameterChanges = make(map[string]any)
	t.pendingLogs = make([]string, 0)
}

// EndProgressCycle calls parent implementation
func (t *Target) EndProgressCycle() {
	t.Want.EndProgressCycle()
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
	resolvedParams := make(map[string]any)
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
		t.StoreLog("[TARGET] ❌ ERROR: Recipe loader is nil for target %s", t.Metadata.Name)
		return []*Want{}
	}

	// Create a copy of RecipeParams and add prefix for proper namespace isolation
	paramsWithPrefix := make(map[string]any)
	for k, v := range t.RecipeParams {
		paramsWithPrefix[k] = v
	}
	// Use target name as prefix to ensure child wants are properly namespaced
	// This ensures that each target instance's children have unique namespace prefixes
	paramsWithPrefix["prefix"] = t.Metadata.Name

	// Load child wants from recipe
	config, err := t.recipeLoader.LoadConfigFromRecipe(t.RecipePath, paramsWithPrefix)
	if err != nil {
		t.StoreLog("[TARGET] ❌ ERROR: Failed to load config from recipe %s: %v", t.RecipePath, err)
		return []*Want{}
	}

	// VALIDATION: Prevent want type name conflicts between parent and children This prevents infinite loops where a want type references a recipe that contains a want of the same type, which would cause recursive instantiation
	parentType := t.Metadata.Type
	for _, childWant := range config.Wants {
		if childWant.Metadata.Type == parentType {
			t.StoreLog("[TARGET] ❌ ERROR: Target %s (type=%s) cannot have child wants of the same type from recipe %s",
				t.Metadata.Name, parentType, t.RecipePath)
			return []*Want{}
		}
	}
	for i := range config.Wants {
		// Ensure ID is generated if not present
		if config.Wants[i].Metadata.ID == "" {
			config.Wants[i].Metadata.ID = generateUUID()
		}

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
		config.Wants[i].metadataMutex.Lock()
		if config.Wants[i].Metadata.Labels == nil {
			config.Wants[i].Metadata.Labels = make(map[string]string)
		}
		config.Wants[i].Metadata.Labels["owner"] = "child"
		// Inject affinity label to namespace children of this target
		// Use both name and ID to ensure uniqueness across redeployments
		instanceID := t.Metadata.Name
		if t.Metadata.ID != "" {
			instanceID = fmt.Sprintf("%s-%s", t.Metadata.Name, t.Metadata.ID)
		}
		config.Wants[i].Metadata.Labels["owner-name"] = instanceID
		config.Wants[i].metadataMutex.Unlock()

		// Inject the same affinity label into all 'using' selectors of the child
		// This ensures sibling wants within the same target connect to each other
		for j := range config.Wants[i].Spec.Using {
			if config.Wants[i].Spec.Using[j] == nil {
				config.Wants[i].Spec.Using[j] = make(map[string]string)
			}
			config.Wants[i].Spec.Using[j]["owner-name"] = instanceID
		}
	}

	t.childWants = config.Wants
	t.StoreLog("📦 Target %s initialized: %d child wants created from recipe", t.Metadata.Name, len(t.childWants))
	return t.childWants
}

// Initialize resets state before execution begins
func (t *Target) Initialize() {
	// No state reset needed for target wants
}

// IsAchieved reports true only after Progress() has set Status to Achieved.
func (t *Target) IsAchieved() bool {
	return t.Status == WantStatusAchieved
}

// DisownChild removes a want from this target's tracking
func (t *Target) DisownChild(wantID string) {
	t.childCompletionMutex.Lock()
	defer t.childCompletionMutex.Unlock()

	newChildWants := make([]*Want, 0)
	var removedName string
	for _, child := range t.childWants {
		if child.Metadata.ID == wantID {
			removedName = child.Metadata.Name
			continue
		}
		newChildWants = append(newChildWants, child)
	}

	if removedName != "" {
		t.childWants = newChildWants
		delete(t.completedChildren, removedName)
		t.StoreLog("[TARGET] Disowned child: %s\n", removedName)

		// Update stats and check if status needs to change back from achieved
		t.childCount = len(t.childWants)
		t.StoreState("child_count", t.childCount)

		if t.Status == WantStatusAchieved {
			// If we were achieved but lost a child (or now have none), re-evaluate or reset
			// For now, simple reset to reaching to allow re-evaluation in next Progress()
			t.SetStatus(WantStatusReaching)
		}
	}
}

// AdoptChild adds a single want as a child of this target if it's not already tracked
func (t *Target) AdoptChild(want *Want) {
	if !t.isChildWant(want) {
		return
	}

	// Ensure this want is in our childWants tracking list
	exists := false
	for _, existingChild := range t.childWants {
		if existingChild.Metadata.ID == want.Metadata.ID {
			exists = true
			break
		}
	}

	if !exists {
		t.childWants = append(t.childWants, want)
		t.StoreLog("[TARGET] Adopted dynamic child: %s (%s)\n", want.Metadata.Name, want.Metadata.Type)

		// If the newly adopted child is already achieved, mark it as completed
		if want.Status == WantStatusAchieved {
			t.childCompletionMutex.Lock()
			t.completedChildren[want.Metadata.Name] = true
			t.childCompletionMutex.Unlock()
		}

		// Update stats
		t.childCount = len(t.childWants)
		t.StoreState("child_count", t.childCount)
	}
}

// Progress implements the Progressable interface for Target.
//
// Lifecycle:
//  1. Phase 1 (first call): create child wants.
//  2. Phase 2 (subsequent calls): block on stateNotify until MergeParentState data
//     arrives (or a child-completion signal) with a 100 ms piggyback window.
//     EndProgressCycle() flushes pendingStateChanges → State after each return.
//  3. When all children are achieved, finalize and set Status = Achieved.
func (t *Target) Progress() {
	// Guard: already achieved
	if t.Status == WantStatusAchieved {
		t.StoreState("achieving_percentage", 100.0)
		return
	}

	// Phase 1: Create child wants (only once)
	if !t.childrenCreated && t.builder != nil {
		childWants := t.CreateChildWants()
		if err := t.builder.AddWantsAsync(childWants); err != nil {
			t.StoreLog("[TARGET] ⚠️  Warning: Failed to send child wants: %v", err)
			return
		}
		t.childrenCreated = true

		// Register recipe-defined state fields into ProvidedStateFields so they
		// appear as regular state (not hidden_state) in the frontend.
		if t.recipeLoader != nil {
			if stateDefs, err := t.recipeLoader.GetRecipeState(t.RecipePath); err == nil {
				for _, def := range stateDefs {
					if !Contains(t.ProvidedStateFields, def.Name) {
						t.ProvidedStateFields = append(t.ProvidedStateFields, def.Name)
					}
				}
			}
		}
		return
	}

	if !t.childrenCreated {
		return
	}

	// Phase 2: Wait for MergeParentState data or child-completion signal
	// Block until at least one notification arrives, then piggyback 100 ms.
	select {
	case <-t.stateNotify:
		// Piggyback: collect additional writes that arrive within 100 ms
		timer := time.NewTimer(100 * time.Millisecond)
	piggyback:
		for {
			select {
			case <-t.stateNotify:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(100 * time.Millisecond)
			case <-timer.C:
				break piggyback
			}
		}
	case <-time.After(500 * time.Millisecond):
		// Timeout: fall through to check children completion
	}

	// Drain any remaining notifications without blocking
	for {
		select {
		case <-t.stateNotify:
		default:
			goto drained
		}
	}
drained:

	// Update achieving_percentage
	t.childCompletionMutex.Lock()
	allComplete := t.checkAllChildrenComplete()
	completedSnapshot := make(map[string]bool)
	for k, v := range t.completedChildren {
		completedSnapshot[k] = v
	}
	t.childCompletionMutex.Unlock()

	if len(t.childWants) > 0 {
		achievedCount := 0
		for _, child := range t.childWants {
			completed := completedSnapshot[child.Metadata.Name] || child.Status == WantStatusAchieved
			if completed {
				achievedCount++
				if !completedSnapshot[child.Metadata.Name] {
					t.childCompletionMutex.Lock()
					t.completedChildren[child.Metadata.Name] = true
					t.childCompletionMutex.Unlock()
				}
			}
		}
		achievingPercentage := float64(achievedCount*100) / float64(len(t.childWants))
		t.StoreState("achieving_percentage", achievingPercentage)
	} else {
		t.StoreState("achieving_percentage", 100.0)
		allComplete = true
	}

	if allComplete {
		t.SetStatus(WantStatusAchieved)

		// Send completion packet to parent/upstream wants (only once)
		alreadySent, _ := t.GetStateBool("completion_packet_sent", false)
		if !alreadySent {
			approvalID := t.GetStringParam("approval_id", t.Metadata.ID)
			approvalStatus := "approved"
			if t.Status == WantStatusFailed {
				approvalStatus = "failed"
			}
			var finalResultDescription string
			if res, ok := t.State["result"].(string); ok {
				finalResultDescription = res
			} else {
				finalResultDescription = fmt.Sprintf("Approval %s completed", approvalID)
			}
			var evidenceMap map[string]any
			if ev, ok := t.State["final_itinerary"]; ok {
				if bytes, err := json.Marshal(ev); err == nil {
					json.Unmarshal(bytes, &evidenceMap)
				}
			}
			if evidenceMap == nil {
				evidenceMap = map[string]any{"status": approvalStatus}
			}
			approvalData := &ApprovalData{
				ApprovalID:  approvalID,
				Evidence:    evidenceMap,
				Description: finalResultDescription,
				Timestamp:   time.Now(),
			}
			t.Provide(approvalData)
			t.ProvideDone()
			time.Sleep(10 * time.Millisecond)
			t.StoreState("completion_packet_sent", true)
			t.StoreLog("✅ Target %s completed and sent results to parent", t.Metadata.Name)
		}

		t.computeTemplateResult()
	}
}

// UpdateParameter updates a parameter and pushes it to child wants
func (t *Target) UpdateParameter(paramName string, paramValue any) {
	// Update our own parameter first
	t.Want.UpdateParameter(paramName, paramValue)

	// Also update the RecipeParams if this is a recipe parameter
	if t.RecipeParams != nil {
		t.RecipeParams[paramName] = paramValue
	}

	// Push parameter change to child wants
	t.PushParameterToChildren(paramName, paramValue)

	t.StoreLog("[TARGET] 🎯 Target %s: Parameter %s updated to %v and pushed to children\n",
		t.Metadata.Name, paramName, paramValue)
}

// ChangeParameter provides a convenient API to change target parameters at runtime
func (t *Target) ChangeParameter(paramName string, paramValue any) {
	t.StoreLog("[TARGET] 🔄 Target %s: Changing parameter %s from %v to %v\n",
		t.Metadata.Name, paramName, t.Spec.Params[paramName], paramValue)
	t.UpdateParameter(paramName, paramValue)
}
func (t *Target) GetParameterValue(paramName string) any {
	if value, ok := t.Spec.Params[paramName]; ok {
		return value
	}
	return nil
}

// PushParameterToChildren propagates parameter changes to all child wants
func (t *Target) PushParameterToChildren(paramName string, paramValue any) {
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

				t.StoreLog("[TARGET] 🔄 Target %s → Child %s: %s=%v (mapped to %s)\n",
					t.Metadata.Name, wantName, paramName, paramValue, childParamName)
			}
		}
	}
}

// isChildWant checks if a want is a child of this target
func (t *Target) isChildWant(want *Want) bool {
	if want.Metadata.OwnerReferences == nil {
		return false
	}
	for _, ownerRef := range want.Metadata.OwnerReferences {
		match := ownerRef.Controller && ownerRef.Kind == "Want" && ownerRef.Name == t.Metadata.Name

		if match {
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
		t.computeFallbackResultUnsafe() // Use unsafe version since we already have the mutex
		return
	}
	recipeResult, err := t.recipeLoader.GetRecipeResult(t.RecipePath)
	if err != nil {
		t.computeFallbackResultUnsafe() // Use unsafe version since we already have the mutex
		return
	}

	if recipeResult == nil {
		t.computeFallbackResultUnsafe() // Use unsafe version since we already have the mutex
		return
	}

	// Check if builder is available (may be nil in test environments)
	if t.builder == nil {
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

	// Stats are now stored in State - no separate initialization needed
	var primaryResult any
	metrics := make(map[string]any)

	for i, resultSpec := range *recipeResult {
		resultValue := t.getResultFromSpec(resultSpec, childWantsByName)
		// Prefer state_field over stat_name as the metric key suffix
		statName := strings.TrimPrefix(resultSpec.StateField, ".")
		if statName == "" {
			statName = strings.TrimPrefix(resultSpec.StatName, ".")
		}
		if statName == "" {
			statName = "result"
		}
		metricKey := resultSpec.WantName + "_" + statName
		metrics[metricKey] = resultValue

		// Use first result as primary result for backward compatibility
		if i == 0 {
			primaryResult = resultValue
		}
	}
	for i, resultSpec := range *recipeResult {
		statName := strings.TrimPrefix(resultSpec.StateField, ".")
		if statName == "" {
			statName = strings.TrimPrefix(resultSpec.StatName, ".")
		}
		if statName == "" {
			statName = "result"
		}
		metricKey := resultSpec.WantName + "_" + statName
		t.StoreState(metricKey, metrics[metricKey])
		if i == 0 {
			t.StoreState("result", fmt.Sprintf("%v", primaryResult))
		}
	}
	t.childCount = len(childWantsByName)

	t.StoreLog("📊 Target %s: Results computed from %d child wants", t.Metadata.Name, t.childCount)
}

// ApprovalData represents shared evidence and description data
type ApprovalData struct {
	ApprovalID  string
	Evidence    any
	Description string
	Timestamp   time.Time
}

// OwnerAwareWant wraps any want type to add parent notification capability
type OwnerAwareWant struct {
	BaseWant   any   // The original want (Generator, Queue, Sink, etc.)
	Want       *Want // Direct reference to Want (extracted at creation time)
	TargetName string
	WantName   string
}

// NewOwnerAwareWant creates a wrapper that adds parent notification to any want wantPtr is the Want pointer extracted from baseWant (can be nil for some types)
func NewOwnerAwareWant(baseWant any, metadata Metadata, wantPtr *Want) *OwnerAwareWant {
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
func extractWantViaReflection(baseWant any) *Want {
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

// Initialize resets state before execution begins
func (oaw *OwnerAwareWant) Initialize() {
	if progressable, ok := oaw.BaseWant.(Progressable); ok {
		progressable.Initialize()
	}
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
		// OwnerCompletionEvent is now emitted automatically by SetStatus() in the base Want
		// when it reaches ACHIEVED status, so no need to emit here
	}
}

// OnDelete delegates to the wrapped want if it implements OnDeletable
func (oaw *OwnerAwareWant) OnDelete() {
	if deletable, ok := oaw.BaseWant.(OnDeletable); ok {
		deletable.OnDelete()
	}
}

// BeginProgressCycle delegates to the stored Want to start batching state changes
func (oaw *OwnerAwareWant) BeginProgressCycle() {
	if oaw.Want != nil {
		oaw.Want.BeginProgressCycle()
	}
}

// EndProgressCycle delegates to the stored Want to commit batched state changes
func (oaw *OwnerAwareWant) EndProgressCycle() {
	if oaw.Want != nil {
		oaw.Want.EndProgressCycle()
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
func (oaw *OwnerAwareWant) GetMetadata() Metadata {
	if oaw.Want != nil {
		return oaw.Want.GetMetadata()
	}
	return Metadata{}
}
func (oaw *OwnerAwareWant) GetSpec() *WantSpec {
	if oaw.Want != nil {
		return oaw.Want.GetSpec()
	}
	return nil
}

// RegisterOwnerWantTypes registers the owner-based want types with a ChainBuilder
func RegisterOwnerWantTypes(builder *ChainBuilder) {
	recipeLoader := NewGenericRecipeLoader(RecipesDir)

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
func (t *Target) getResultFromSpec(spec RecipeResultSpec, childWants map[string]*Want) any {
	want, exists := childWants[spec.WantName]
	if !exists {
		// Log available want names to help with debugging
		availableNames := make([]string, 0, len(childWants))
		for name := range childWants {
			availableNames = append(availableNames, name)
		}
		t.StoreLog("[TARGET] ⚠️  Target %s: Want '%s' not found for result computation (available: %v)\n", t.Metadata.Name, spec.WantName, availableNames)
		return 0
	}

	// Prefer state_field over stat_name
	fieldName := strings.TrimPrefix(spec.StateField, ".")
	if fieldName == "" {
		fieldName = spec.StatName
	}

	if strings.HasPrefix(fieldName, ".") {
		return t.extractValueByPath(want.State, fieldName)
	}

	// Try to get the specified stat from the want's State map
	if want.State != nil {
		if value, ok := want.State[fieldName]; ok {
			return value
		}
		if value, ok := want.State[strings.ToLower(fieldName)]; ok {
			return value
		}
		if fieldName == "TotalProcessed" {
			if value, ok := want.GetStateInt("total_processed", 0); ok {
				return value
			}
		}
	}

	t.StoreLog("[TARGET] ⚠️  Target %s: Field '%s' not found in want '%s' (available: %v)\n", t.Metadata.Name, fieldName, spec.WantName, want.State)
	return 0
}

// extractValueByPath extracts values using JSON path-like syntax
func (t *Target) extractValueByPath(data map[string]any, path string) any {
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
	// Check if builder is available (may be nil in test environments)
	if t.builder == nil {
		t.StoreLog("[TARGET] ⚠️  Target %s: No builder available for fallback result computation\n", t.Metadata.Name)
		return
	}

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

	t.StoreLog("🧮 Target %s: Using fallback result computation for %d child wants\n", t.Metadata.Name, len(childWants))

	// Simple aggregate result from child wants using dynamic stats
	totalProcessed := 0
	for _, child := range childWants {
		if child.State != nil {
			if processed, ok := child.GetStateInt("total_processed", 0); ok {
				totalProcessed += processed
			} else if processed, ok := child.GetStateInt("TotalProcessed", 0); ok {
				totalProcessed += processed
			}
		}
	}
	if t.State == nil {
		t.State = make(map[string]any)
	}
	t.StoreState("result", fmt.Sprintf("processed: %d", totalProcessed))
	t.childCount = len(childWants)
	t.StoreLog("[TARGET] ✅ Target %s: Fallback result computed - processed %d items from %d child wants\n", t.Metadata.Name, totalProcessed, len(childWants))
}

// === Test Helper Methods (for pkg/server tests) ===

// GetChildWants returns the list of child wants (for testing purposes)
func (t *Target) GetChildWants() []*Want {
	return t.childWants
}

// SetChildWants sets the list of child wants (for testing purposes)
func (t *Target) SetChildWants(wants []*Want) {
	t.childWants = wants
}

// GetCompletedChildren returns the completed children map (for testing purposes)
func (t *Target) GetCompletedChildren() map[string]bool {
	return t.completedChildren
}

// SetCompletedChildren sets the completed children map (for testing purposes)
func (t *Target) SetCompletedChildren(completed map[string]bool) {
	t.completedChildren = completed
}

// SetChildrenCreated sets the childrenCreated flag (for testing purposes)
func (t *Target) SetChildrenCreated(created bool) {
	t.childrenCreated = created
}
