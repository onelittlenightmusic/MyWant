package mywant

import (
	"context"
	"encoding/json"
	"fmt"
	"mywant/engine/core/chain"
	"mywant/engine/planner"
	"reflect"
	"strings"
	"sync"
	"time"

	ws "github.com/onelittlenightmusic/want-spec"
)

// plannerRoleToChildRole converts a Planner step role to a child-role label value
// used by the GUI balloon to display the correct category.
//
//	monitor     → "monitor"   (Satisfied? / Monitor カード)
//	intermediate → "thinker"  (Thinker カード)
//	terminal     → "doer"     (Doer カード)
func plannerRoleToChildRole(role string) string {
	switch role {
	case "monitor":
		return "monitor"
	case "intermediate":
		return "thinker"
	case "terminal":
		return "doer"
	default:
		return role
	}
}

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
	completedChildren      sync.Map             // Track which children have completed (key: string name, value: bool)
	builder                *ChainBuilder        // Reference to builder for dynamic want creation
	recipeLoader           *GenericRecipeLoader // Reference to generic recipe loader
	childrenDone           chan bool            // Signal when all children complete
	childrenCreated        bool                 // Track if child wants have been created
	childCount             int                  // Count of child wants
	stateNotify            chan struct{}        // Notified when MergeParentState writes pending state

	// Declarative planning fields — populated from recipe at Initialize time.
	// When RecipeAchieve is non-empty, the Planner derives child wants automatically.
	RecipeAchieve     []PlanTarget       // achieve section from recipe
	RecipeIsSatisfied *RecipeIsSatisfied // isSatisfied section — short-circuits achieve chain if condition true
	RecipeHints       []PlanHint         // hints section for Planner guidance

	// isSatisfied gate — evaluated in Progress() once check want completes.
	// Achieve chain wants use the Target itself as provider via _isSatisfied_gate label.
	isSatisfiedCheckWant           *Want // reference to the pre-check want for condition evaluation
	isSatisfiedGateFired           bool  // true once Target has evaluated and provided/achieved
	isSatisfiedRecheckAfterAchieve bool  // re-run check after achieve chain completes
}

// NewTarget creates a new target want
func NewTarget(metadata Metadata, spec WantSpec) *Target {
	target := &Target{
		Want: Want{
			Metadata:             metadata,
			Spec:                 spec,
			Status:               WantStatusIdle,
			PreservePendingState: true, // Target aggregates MergeParentState writes across iterations
		},
		MaxDisplay:   1000,
		RecipePath:   "", // Path will be set dynamically via target type registration
		RecipeParams: make(map[string]any), parameterSubscriptions: make(map[string][]string),
		childWants:   make([]*Want, 0),
		childrenDone: make(chan bool, 1), // Signal channel for subscription system
		stateNotify:  make(chan struct{}, 64),
	}

	// Hook: whenever Want.MergeState is called on the embedded Want, signal stateNotify.
	// This works even when the caller holds a *Want pointer (not *Target) due to embedding.
	// Skip the wakeup when already achieved — prevents feedback loops from currentStateExposeHandler.
	target.Want.onMergeState = func() {
		if IsAchievedStatus(target.GetStatus()) {
			return
		}
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
	for k, v := range spec.Params {
		if !targetSpecificParams[k] {
			target.RecipeParams[k] = v
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

	// GUARD: If target is already achieved, re-aggregate final_result so that
	// dynamically-added children (e.g. a hotel/reminder added after the target
	// first achieved) are included in the summary visible on the canvas.
	if IsAchievedStatus(tcs.target.GetStatus()) {
		tcs.target.completedChildren.Store(completionEvent.ChildName, true)
		tcs.target.computeFallbackResult()
		return EventResponse{Handled: true}
	}

	// Track child completion
	tcs.target.completedChildren.Store(completionEvent.ChildName, true)

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

	// For targets with children (recipe or dynamic), check both sync.Map AND actual Status
	for _, child := range t.childWants {
		val, inMap := t.completedChildren.Load(child.Metadata.Name)
		completed := (inMap && val.(bool)) || IsAchievedStatus(child.Status)
		if !completed {
			return false
		}
	}

	// All children are complete
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

	// ── Planner-derived children (recipe.achieve) ─────────────────────────────
	// Run the planner here inside Progress() Phase 1 so that:
	// (a) the blueprint is committed to state via EndProgressCycle(), and
	// (b) failures are observable and retriable within the Progress loop.
	if len(t.RecipeAchieve) > 0 && t.builder != nil {
		defs := t.builder.AllWantTypeDefinitions()
		wsDefs := make(map[string]*ws.WantTypeDefinition, len(defs))
		for k, v := range defs {
			wsDefs[k] = v
		}
		idx := planner.BuildExposableIndexFromDefs(wsDefs)
		p := planner.New(idx, wsDefs)
		plan := &ws.WantTypePlan{
			Achieve: t.RecipeAchieve,
			Hints:   t.RecipeHints,
		}
		if t.RecipeIsSatisfied != nil {
			mon := ws.PlanTarget{
				Type:        t.RecipeIsSatisfied.Type,
				Name:        t.RecipeIsSatisfied.Name,
				Description: "isSatisfied pre-check",
			}
			// Always set When — field defaults to "final_result" when omitted.
			cond := t.RecipeIsSatisfied.When
			mon.When = &cond
			plan.Monitor = []ws.PlanTarget{mon}
		}
		result := p.PlanFromWantType(t.Metadata.Type, "", plan, t.RecipeParams)

		// Store the blueprint in plan state so the GUI can display the reasoning
		// trace immediately after the first Progress cycle commits it.
		if t.StateLabels == nil {
			t.StateLabels = make(map[string]StateLabel)
		}
		t.StateLabels["planner_result"] = LabelPlan
		t.StoreState("planner_result", map[string]any{
			"confidence": result.Confidence,
			"steps":      result.Steps,
			"warnings":   result.Warnings,
		})

		if len(result.Warnings) > 0 {
			for _, w := range result.Warnings {
				t.StoreLog("[TARGET] Planner warning: %s", w)
			}
		}

		// Convert planner-derived RecipeWants to *Want and prepend to config.
		prefix := t.Metadata.Name
		planWants := make([]*Want, 0, len(result.Recipe.Wants))
		for i, rw := range result.Recipe.Wants {
			w := ConvertRecipeWantToWant(rw)
			// Apply the same prefix convention as LoadConfigFromRecipe uses.
			if w.Metadata.Name == "" {
				w.Metadata.Name = fmt.Sprintf("%s-%s-%d", prefix, w.Metadata.Type, i+1)
			} else {
				w.Metadata.Name = fmt.Sprintf("%s-%s", prefix, w.Metadata.Name)
			}
			if w.Metadata.ID == "" {
				w.Metadata.ID = GenerateUUID()
			}
			// Apply child-role label from planner step role so the GUI balloon
			// shows the correct category (Thinker / Doer / Monitor etc.).
			if i < len(result.Steps) {
				if w.Metadata.Labels == nil {
					w.Metadata.Labels = make(map[string]string)
				}
				w.Metadata.Labels["child-role"] = plannerRoleToChildRole(result.Steps[i].Role)
			}
			planWants = append(planWants, w)
		}

		// Inject label-level when: conditions from planner result into using: entries.
		if len(result.Recipe.LabelConditions) > 0 {
			for _, w := range planWants {
				injectLabelConditions(w, result.Recipe.LabelConditions)
			}
		}

		// Track the isSatisfied check want reference (first monitor want in planWants)
		// and enable AutoExpose so its exposable state fields are automatically
		// propagated to this coordinator want when they change.
		for _, w := range planWants {
			if w.Metadata.Labels["child-role"] == "monitor" && t.isSatisfiedCheckWant == nil {
				t.isSatisfiedCheckWant = w
				w.Spec.AutoExpose = true
			}
		}

		// Prepend planner wants; manually-listed recipe wants follow.
		config = append(planWants, config...)
		t.StoreLog("[TARGET] Planner derived %d child want(s) from recipe.achieve (confidence: %s)",
			len(planWants), result.Confidence)
	}

	// VALIDATION: Prevent want type name conflicts between parent and children This prevents infinite loops where a want type references a recipe that contains a want of the same type, which would cause recursive instantiation
	parentType := t.Metadata.Type
	for _, childWant := range config {
		if childWant.Metadata.Type == parentType {
			t.StoreLog("[TARGET] ❌ ERROR: Target %s (type=%s) cannot have child wants of the same type from recipe %s",
				t.Metadata.Name, parentType, t.RecipePath)
			return []*Want{}
		}
	}
	for i := range config {
		// Ensure ID is generated if not present
		if config[i].Metadata.ID == "" {
			config[i].Metadata.ID = GenerateUUID()
		}

		config[i].Metadata.OwnerReferences = []OwnerReference{
			{
				APIVersion:         "mywant/v1",
				Kind:               "Want",
				Name:               t.Metadata.Name,
				ID:                 t.Metadata.ID,
				Controller:         true,
				BlockOwnerDeletion: true,
			},
		}
		config[i].metadataMutex.Lock()
		if config[i].Metadata.Labels == nil {
			config[i].Metadata.Labels = make(map[string]string)
		}
		config[i].Metadata.Labels["owner"] = "child"
		// Inject affinity label to namespace children of this target
		// Use both name and ID to ensure uniqueness across redeployments
		instanceID := t.Metadata.Name
		if t.Metadata.ID != "" {
			instanceID = fmt.Sprintf("%s-%s", t.Metadata.Name, t.Metadata.ID)
		}
		config[i].Metadata.Labels["owner-name"] = instanceID
		config[i].metadataMutex.Unlock()

		// Inject the same affinity label into all 'using' selectors of the child
		// This ensures sibling wants within the same target connect to each other
		for j := range config[i].Spec.Using {
			if config[i].Spec.Using[j].Labels == nil {
				config[i].Spec.Using[j].Labels = make(map[string]string)
			}
			config[i].Spec.Using[j].Labels["owner-name"] = instanceID
		}
	}

	t.childWants = config
	t.StoreLog("📦 Target %s initialized: %d child wants created from recipe", t.Metadata.Name, len(t.childWants))
	return t.childWants
}

// Initialize resets state before execution begins
func (t *Target) Initialize() {
	// Copy declarative planning fields from the recipe (achieve/isSatisfied/hints).
	// These are used by CreateChildWants and Progress to drive Planner-based expansion.
	if t.recipeLoader != nil && t.RecipePath != "" {
		if cfg, err := t.recipeLoader.LoadRecipe(t.RecipePath, map[string]any{}); err == nil {
			if len(cfg.Achieve) > 0 && len(t.RecipeAchieve) == 0 {
				t.RecipeAchieve = cfg.Achieve
			}
			if cfg.IsSatisfied != nil && t.RecipeIsSatisfied == nil {
				t.RecipeIsSatisfied = cfg.IsSatisfied
				if cfg.IsSatisfiedRecheckAfterAchieve {
					t.isSatisfiedRecheckAfterAchieve = true
				}
			}
			if len(cfg.Hints) > 0 && len(t.RecipeHints) == 0 {
				t.RecipeHints = cfg.Hints
			}
		}
	}

	// Apply recipe-defined state labels early so SetGoal/SetCurrent/etc. work during Initialize.
	// This mirrors what Progress() does on first run, but must happen here so child wants and
	// the coordinator itself can call SetGoal/SetCurrent immediately.
	if t.recipeLoader != nil {
		if stateDefs, err := t.recipeLoader.GetRecipeState(t.RecipePath); err == nil {
			if t.StateLabels == nil {
				t.StateLabels = make(map[string]StateLabel)
			}
			for _, def := range stateDefs {
				if def.Label != "" {
					var label StateLabel
					switch def.Label {
					case "goal":
						label = LabelGoal
					case "current":
						label = LabelCurrent
					case "plan":
						label = LabelPlan
					case "internal":
						label = LabelInternal
					default:
						label = LabelNone
					}
					t.StateLabels[def.Name] = label
				}
				// Register in ProvidedStateFields so fields appear in explicit state (not hidden_state)
				if !Contains(t.ProvidedStateFields, def.Name) {
					t.ProvidedStateFields = append(t.ProvidedStateFields, def.Name)
				}
				// Initialize state with initial value if not already set
				if def.InitialValue != nil {
					if _, exists := t.getState(def.Name); !exists {
						t.storeState(def.Name, def.InitialValue)
					}
				}
			}
		}
	}

	// Ensure provider_state_map is accessible in Spec.Params for dispatch_thinker.
	// direction_map is now owned by the child planner want (itinerary/briefing),
	// not by the Target itself.
	if _, hasProviderStateMap := t.GetParameter("provider_state_map"); !hasProviderStateMap {
		if v, ok := t.RecipeParams["provider_state_map"]; ok && v != nil {
			t.UpdateParameter("provider_state_map", v)
		}
	}

	// Start DispatchExecutor to handle child want dispatch requests
	dispatchThinkerID := DispatchThinkerName + "-" + t.Metadata.ID
	if _, running := t.GetBackgroundAgent(dispatchThinkerID); !running {
		agent := NewDispatchThinker(dispatchThinkerID)
		if err := t.AddBackgroundAgent(agent); err != nil {
			t.StoreLog("ERROR: Failed to start DispatchThinkerAgent: %v", err)
		}
	}
}

// IsAchieved reports true after Progress() has set Status to Achieved (with or without warnings).
func (t *Target) IsAchieved() bool {
	return IsAchievedStatus(t.GetStatus())
}

// DisownChild removes a want from this target's tracking
func (t *Target) DisownChild(wantID string) {
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
		t.completedChildren.Delete(removedName)
		t.StoreLog("[TARGET] Disowned child: %s\n", removedName)

		// Update stats and check if status needs to change back from achieved
		t.childCount = len(t.childWants)
		t.storeState("child_count", t.childCount)

		if IsAchievedStatus(t.Status) {
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
		if IsAchievedStatus(want.Status) {
			t.completedChildren.Store(want.Metadata.Name, true)
		}

		// Update stats
		t.childCount = len(t.childWants)
		t.storeState("child_count", t.childCount)
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
	// Guard: already achieved (with or without warnings)
	if IsAchievedStatus(t.Status) {
		t.storeState("achieving_percentage", 100.0)
		return
	}

	// ── Phase 0: Compute planner blueprint and wait for user approval ──────────
	// Only applies to wants with an achieve: section (RecipeAchieve).
	// The blueprint is written once to state.plan and state.current.plan_status.
	// CreateChildWants (Phase 1) is blocked until plan_approved becomes true.
	if len(t.RecipeAchieve) > 0 {
		planStatus, _ := t.GetStateString("plan_status", "")

		// After a server restart, plan_status may be "" or "pending_approval" even
		// though children were already created and persisted. Detect this by checking
		// whether child wants with our OwnerReference already exist in the builder.
		// If they do, skip the planning phase and mark children as already created.
		if (planStatus == "" || planStatus == "pending_approval") && t.builder != nil {
			existingChildren := t.builder.FindChildWantsByOwnerID(t.Metadata.ID)
			if len(existingChildren) > 0 {
				t.childrenCreated = true
				t.StoreState("plan_status", "approved")
				t.StoreState("plan_approved", true)
				planStatus = "approved"
			}
		}

		if planStatus == "" {
			// First cycle: run planner, store blueprint in state.plan, mark pending approval.
			if t.StateLabels == nil {
				t.StateLabels = make(map[string]StateLabel)
			}
			t.StateLabels["plan_status"] = LabelCurrent
			t.StateLabels["plan_approved"] = LabelCurrent
			t.StateLabels["planner_result"] = LabelPlan

			// Run planner now so the blueprint is visible in the balloon before approval.
			if t.builder != nil {
				defs := t.builder.AllWantTypeDefinitions()
				wsDefs := make(map[string]*ws.WantTypeDefinition, len(defs))
				for k, v := range defs {
					wsDefs[k] = v
				}
				idx := planner.BuildExposableIndexFromDefs(wsDefs)
				p := planner.New(idx, wsDefs)
				plan := &ws.WantTypePlan{
					Achieve: t.RecipeAchieve,
					Hints:   t.RecipeHints,
				}
				if t.RecipeIsSatisfied != nil {
					cond := t.RecipeIsSatisfied.When
					plan.Monitor = []ws.PlanTarget{{
						Type: t.RecipeIsSatisfied.Type,
						Name: t.RecipeIsSatisfied.Name,
						When: &cond,
					}}
				}
				result := p.PlanFromWantType(t.Metadata.Type, "", plan, t.RecipeParams)
				t.StoreState("planner_result", map[string]any{
					"confidence": result.Confidence,
					"steps":      result.Steps,
					"warnings":   result.Warnings,
				})
			}

			t.StoreState("plan_status", "pending_approval")
			t.StoreState("plan_approved", false)
			t.StoreLog("[TARGET] Planner blueprint ready — waiting for user approval")
			return
		}
		if planStatus == "pending_approval" {
			approved, _ := t.GetStateBool("plan_approved", false)
			if !approved {
				return // still waiting
			}
			t.StoreState("plan_status", "approved")
			t.StoreLog("[TARGET] Plan approved — deploying child wants")
		}
	}

	// Phase 1: Create child wants (only once).
	// On restart, childrenCreated is false but children may already exist — check first.
	if !t.childrenCreated && t.builder != nil {
		if existing := t.builder.FindChildWantsByOwnerID(t.Metadata.ID); len(existing) > 0 {
			t.childrenCreated = true
		}
	}
	if !t.childrenCreated && t.builder != nil {
		childWants := t.CreateChildWants()
		if err := t.builder.AddWantsAsync(childWants); err != nil {
			t.StoreLog("[TARGET] ⚠️  Warning: Failed to send child wants: %v", err)
			return
		}
		t.childrenCreated = true

		// Register recipe-defined state fields into ProvidedStateFields so they
		// appear as regular state (not hidden_state) in the frontend.
		// Also apply any label definitions to StateLabels so SetCurrent/SetGoal/etc.
		// will accept these keys on the coordinator want.
		if t.recipeLoader != nil {
			if stateDefs, err := t.recipeLoader.GetRecipeState(t.RecipePath); err == nil {
				if t.StateLabels == nil {
					t.StateLabels = make(map[string]StateLabel)
				}
				for _, def := range stateDefs {
					if !Contains(t.ProvidedStateFields, def.Name) {
						t.ProvidedStateFields = append(t.ProvidedStateFields, def.Name)
					}
					if def.Label != "" {
						var label StateLabel
						switch def.Label {
						case "goal":
							label = LabelGoal
						case "current":
							label = LabelCurrent
						case "plan":
							label = LabelPlan
						case "internal":
							label = LabelInternal
						default:
							label = LabelNone
						}
						t.StateLabels[def.Name] = label
					}
				}
			}

			// Apply FinalResultField from recipe if not already set in spec
			if t.Spec.FinalResultField == "" {
				if field, err := t.recipeLoader.GetRecipeFinalResultField(t.RecipePath); err == nil && field != "" {
					t.Spec.FinalResultField = field
					t.StoreLog("[TARGET] Applied FinalResultField '%s' from recipe", field)
				}
			}
		}
		return
	}

	if !t.childrenCreated {
		return
	}

	// ── isSatisfied gate evaluation ────────────────────────────────────────────
	// Restore isSatisfiedGateFired from persisted state after restart.
	if !t.isSatisfiedGateFired {
		if fired, _ := t.GetStateBool("_isSatisfied_gate_fired", false); fired {
			t.isSatisfiedGateFired = true
		}
	}

	// Also try to wire up isSatisfiedCheckWant after restart if not yet set.
	if t.RecipeIsSatisfied != nil && t.isSatisfiedCheckWant == nil && t.builder != nil {
		for _, child := range t.builder.FindChildWantsByOwnerID(t.Metadata.ID) {
			if child.Metadata.Labels["child-role"] == "monitor" {
				t.isSatisfiedCheckWant = child
				break
			}
		}
	}

	// Once the pre-check want completes, evaluate the When condition.
	// Target-level responsibility: no achieve-chain want logic needs to know about this.
	if t.RecipeIsSatisfied != nil && !t.isSatisfiedGateFired && t.isSatisfiedCheckWant != nil {
		checkStatus := t.isSatisfiedCheckWant.GetStatus()
		if !IsAchievedStatus(checkStatus) && checkStatus != WantStatusFailed {
			return // still waiting for check want to complete
		}
		field := resolveConditionField(t.RecipeIsSatisfied.When.Field)
		actual, _ := t.isSatisfiedCheckWant.getState(field)
		if evaluateCondition(actual, t.RecipeIsSatisfied.When.Operator, t.RecipeIsSatisfied.When.Value) {
			// Condition met — goal already achieved, skip achieve chain
			t.StoreLog("[TARGET] isSatisfied condition met (%s %s %v) — marking achieved without running achieve chain",
				field, t.RecipeIsSatisfied.When.Operator, t.RecipeIsSatisfied.When.Value)
			// Close the achieve-chain gate so achieve-chain wants (which import
			// _achieve_gate_open) stay blocked even if they restart.
			t.StoreState("_achieve_gate_open", nil)
			t.SetStatus(WantStatusAchieved)
			t.isSatisfiedGateFired = true
			t.StoreState("_isSatisfied_gate_fired", true)
			return
		}
		// Condition not met — open the gate so achieve-chain wants can proceed.
		// _achieve_gate_open is imported by all achieve-chain wants; setting it
		// to true (non-nil) unblocks hasUnresolvedImports() on each of them.
		t.StoreLog("[TARGET] isSatisfied condition not met — opening achieve-chain gate")
		t.StoreState("_achieve_gate_open", true)
		t.isSatisfiedGateFired = true
		t.StoreState("_isSatisfied_gate_fired", true)
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
	// Ensure any dynamically-added children (added via API after target started) are adopted.
	// Without this, checkAllChildrenComplete() returns true for empty childWants and the
	// target achieves before children like route-search (slow Python script) have finished.
	if len(t.childWants) == 0 && t.builder != nil {
		for _, child := range t.builder.FindChildWantsByOwnerID(t.Metadata.ID) {
			t.AdoptChild(child)
		}
	}

	allComplete := t.checkAllChildrenComplete()

	if len(t.childWants) > 0 {
		achievedCount := 0
		for _, child := range t.childWants {
			val, inMap := t.completedChildren.Load(child.Metadata.Name)
			completed := (inMap && val.(bool)) || IsAchievedStatus(child.Status)
			if completed {
				achievedCount++
				if !inMap || !val.(bool) {
					t.completedChildren.Store(child.Metadata.Name, true)
				}
			}
		}
		achievingPercentage := float64(achievedCount*100) / float64(len(t.childWants))
		t.storeState("achieving_percentage", achievingPercentage)
	} else {
		// Even if no childWants exist yet, we might have directions being realized
		// by DispatchThinker. Stay reaching if there's evidence of ongoing work.
		directions, directionsFound := t.getState("directions")
		plannedCount, _ := t.GetStateInt("planned_count", 0)
		goalAchieved, _ := t.GetStateBool("goal_achieved", false)

		// If we have directions or a non-zero planned count, we are not done
		// unless goal_achieved is explicitly true.
		// For dynamic targets (direction_map present), we also wait until
		// directions state is at least initialized.
		_, hasDirectionMap := t.GetParameter("direction_map")

		if hasDirectionMap && !directionsFound && !goalAchieved {
			t.storeState("achieving_percentage", 10.0)
			allComplete = false
		} else if (directions != nil || plannedCount > 0) && !goalAchieved {
			t.storeState("achieving_percentage", 50.0)
			allComplete = false
		} else {
			t.storeState("achieving_percentage", 100.0)
			allComplete = true
		}
	}

	// Dynamic Dispatch Check: check if all actions dispatched by DispatchThinker are done
	if allComplete {
		if dispatched, ok := t.getState("_dispatched_directions"); ok {
			if m, ok := dispatched.(map[string]any); ok {
				for _, v := range m {
					if id, ok := v.(string); ok && id != "DONE" {
						allComplete = false
						t.storeState("achieving_percentage", 90.0)
						break
					}
				}
			}
		}
	}

	if allComplete {
		// recheckAfterAchieve: re-run the isSatisfied check want once the achieve chain
		// completes. This verifies the goal was actually reached (e.g. confirm a booking
		// was made) before marking the coordinator as achieved.
		if t.isSatisfiedRecheckAfterAchieve && t.RecipeIsSatisfied != nil && t.isSatisfiedCheckWant != nil {
			postRechecked, _ := t.GetStateBool("_post_achieve_rechecked", false)
			if !postRechecked {
				t.StoreLog("[TARGET] Achieve chain complete — re-running isSatisfied check (recheckAfterAchieve)")
				t.StoreState("_post_achieve_rechecked", true)
				t.isSatisfiedGateFired = false
				t.StoreState("_isSatisfied_gate_fired", false)
				// Reset the check want: clear the field that caused finalization so it runs again.
				cb := GetGlobalChainBuilder()
				if cb != nil {
					typeDef := cb.GetWantTypeDefinition(t.isSatisfiedCheckWant.Metadata.Type)
					if typeDef != nil && typeDef.FinalizeWhen != nil && typeDef.FinalizeWhen.Achieved != nil {
						t.isSatisfiedCheckWant.StoreState(typeDef.FinalizeWhen.Achieved.Field, nil)
					}
				}
				t.isSatisfiedCheckWant.StoreState("error", "")
				t.isSatisfiedCheckWant.SetStatus(WantStatusReaching)
				// Remove from completedChildren so allComplete recalculates correctly.
				t.completedChildren.Delete(t.isSatisfiedCheckWant.Metadata.Name)
				return
			}
		}

		// Propagate warning status: if any child achieved with warnings, so does this target.
		finalStatus := t.resolveAchievedStatus()
		t.SetStatus(finalStatus)

		// Send completion packet to parent/upstream wants (only once)
		alreadySent, _ := t.GetStateBool("completion_packet_sent", false)
		if !alreadySent {
			approvalID := t.GetStringParam("approval_id", t.Metadata.ID)
			approvalStatus := "approved"
			if t.Status == WantStatusFailed {
				approvalStatus = "failed"
			}
			var finalResultDescription string
			if res, ok := t.GetStateString("result", ""); ok && res != "" {
				finalResultDescription = res
			} else {
				finalResultDescription = fmt.Sprintf("Approval %s completed", approvalID)
			}
			var evidenceMap map[string]any
			if ev, ok := t.getState("final_itinerary"); ok {
				if bytes, err := json.Marshal(ev); err == nil {
					json.Unmarshal(bytes, &evidenceMap)
				}
			}
			if evidenceMap == nil {
				evidenceMap = map[string]any{"status": approvalStatus}
			}
			approvalData := NewDataObjectFrom("approval_result", map[string]any{
				"approval_id":  approvalID,
				"description":  finalResultDescription,
				"status":       approvalStatus,
				"evidence":     evidenceMap,
				"completed_at": time.Now().Format(time.RFC3339),
			})
			t.Provide(approvalData)
			t.ProvideDone()
			time.Sleep(10 * time.Millisecond)
			t.storeState("completion_packet_sent", true)
			t.StoreLog("✅ Target %s completed and sent results to parent", t.Metadata.Name)
		}

		t.computeTemplateResult()

		// Signal a status change to self to trigger immediate re-save and event emission
		// This ensures frontend and dependent wants see the latest aggregated results
		t.SetStatus(finalStatus)
	}
}

// resolveAchievedStatus returns WantStatusAchievedWithWarning if any child finished with
// warnings, otherwise WantStatusAchieved.  This propagates governance/label warnings upward.
func (t *Target) resolveAchievedStatus() WantStatus {
	for _, child := range t.childWants {
		if child.Status == WantStatusAchievedWithWarning || child.Status == WantStatusReachingWithWarning {
			return WantStatusAchievedWithWarning
		}
	}
	return WantStatusAchieved
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
	oldVal, _ := t.GetParameter(paramName)
	t.StoreLog("[TARGET] 🔄 Target %s: Changing parameter %s from %v to %v\n",
		t.Metadata.Name, paramName, oldVal, paramValue)
	t.UpdateParameter(paramName, paramValue)
}
func (t *Target) GetParameterValue(paramName string) any {
	if value, ok := t.GetParameter(paramName); ok {
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
	if t.recipeLoader == nil {
		t.computeFallbackResult()
		return
	}
	recipeResult, err := t.recipeLoader.GetRecipeResult(t.RecipePath)
	if err != nil {
		t.computeFallbackResult()
		return
	}

	if recipeResult == nil {
		t.computeFallbackResult()
		return
	}

	// Check if builder is available (may be nil in test environments)
	if t.builder == nil {
		t.computeFallbackResult()
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
					// Also store by short name (without target prefix) so recipe result lookups
					// still work after want names are prefixed with the target name
					targetPrefix := t.Metadata.Name + "-"
					if strings.HasPrefix(want.Metadata.Name, targetPrefix) {
						shortName := strings.TrimPrefix(want.Metadata.Name, targetPrefix)
						childWantsByName[shortName] = want
					}
				}
				break
			}
		}
	}

	// Stats are now stored in State - no separate initialization needed
	metrics := make(map[string]any)

	for _, resultSpec := range *recipeResult {
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
	}
	for _, resultSpec := range *recipeResult {
		statName := strings.TrimPrefix(resultSpec.StateField, ".")
		if statName == "" {
			statName = strings.TrimPrefix(resultSpec.StatName, ".")
		}
		if statName == "" {
			statName = "result"
		}
		metricKey := resultSpec.WantName + "_" + statName
		t.storeState(metricKey, metrics[metricKey])

		// Auto-register the computed key into ProvidedStateFields so it shows as explicit (not hidden)
		if !Contains(t.ProvidedStateFields, metricKey) {
			t.ProvidedStateFields = append(t.ProvidedStateFields, metricKey)
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

// IsFailed delegates to the wrapped want if it implements Failable.
func (oaw *OwnerAwareWant) IsFailed() bool {
	if failable, ok := oaw.BaseWant.(Failable); ok {
		return failable.IsFailed()
	}
	return false
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
	targetFactory := func(metadata Metadata, spec WantSpec) Progressable {
		target := NewTarget(metadata, spec)
		target.SetBuilder(builder)           // Set builder reference for dynamic want creation
		target.SetRecipeLoader(recipeLoader) // Set recipe loader for external recipes
		return target
	}
	builder.RegisterWantType("target", targetFactory)
	// custom_target has a YAML definition but must go through NewTarget() so that
	// subscribeToChildCompletion() is called and child completion events are handled.
	builder.RegisterWantType("custom_target", targetFactory)

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
		return t.extractValueByPath(want.GetAllState(), fieldName)
	}

	// Try to get the specified stat from the want's State map
	if value, ok := want.getState(fieldName); ok {
		return value
	}
	if value, ok := want.getState(strings.ToLower(fieldName)); ok {
		return value
	}
	if fieldName == "TotalProcessed" {
		if value, ok := want.GetStateInt("total_processed", 0); ok {
			return value
		}
	}

	t.StoreLog("[TARGET] ⚠️  Target %s: Field '%s' not found in want '%s'\n", t.Metadata.Name, fieldName, spec.WantName)
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

// computeFallbackResult provides simple aggregation of child want states.
func (t *Target) computeFallbackResult() {
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

	// For custom_target: aggregate children's final_result into parent's final_result.
	if t.Metadata.Type == "custom_target" {
		t.aggregateChildFinalResults(childWants)
		t.childCount = len(childWants)
		return
	}

	// Simple aggregate result from child wants using dynamic stats
	totalProcessed := 0
	for _, child := range childWants {
		if processed, ok := child.GetStateInt("total_processed", 0); ok {
			totalProcessed += processed
		} else if processed, ok := child.GetStateInt("TotalProcessed", 0); ok {
			totalProcessed += processed
		}
	}
	t.storeState("result", fmt.Sprintf("processed: %d", totalProcessed))
	t.childCount = len(childWants)
	t.StoreLog("[TARGET] ✅ Target %s: Fallback result computed - processed %d items from %d child wants\n", t.Metadata.Name, totalProcessed, len(childWants))
}

// aggregateChildFinalResults collects each child's final_result and writes a
// composed summary to the parent's final_result state field.
// Children are sorted by their canvas-y label (ascending) so the output order
// matches visual top-to-bottom reading order on the Want Canvas.
// Only children that have a non-empty final_result are included; if none do,
// the parent's final_result is left unchanged so a previous value is preserved.
func (t *Target) aggregateChildFinalResults(childWants []*Want) {
	type entry struct {
		name    string
		role    string
		result  any
		canvasY int
	}

	var entries []entry
	for _, child := range childWants {
		fr, ok := child.getState("final_result")
		if !ok || fr == nil {
			continue
		}
		// Skip empty strings
		if s, isStr := fr.(string); isStr && strings.TrimSpace(s) == "" {
			continue
		}
		name := child.Metadata.Name
		// strip parent-name prefix (e.g. "wife-target-weather-check" → "weather-check")
		if prefix := t.Metadata.Name + "-"; strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
		}
		role := child.Metadata.Labels["child-role"]

		y := 0
		if yStr, ok := child.Metadata.Labels["mywant.io/canvas-y"]; ok {
			fmt.Sscanf(yStr, "%d", &y)
		}
		entries = append(entries, entry{name: name, role: role, result: fr, canvasY: y})
	}

	if len(entries) == 0 {
		t.StoreLog("[TARGET] %s: no child final_results available yet", t.Metadata.Name)
		return
	}

	// sort by canvas-y (top to bottom)
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].canvasY < entries[j-1].canvasY; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}

	// Build JSON array: [{"name":"...","role":"...","result":...}, ...]
	type jsonEntry struct {
		Name   string `json:"name"`
		Role   string `json:"role,omitempty"`
		Result any    `json:"result"`
	}
	jsonEntries := make([]jsonEntry, len(entries))
	for i, e := range entries {
		jsonEntries[i] = jsonEntry{Name: e.name, Role: e.role, Result: e.result}
	}
	composed, err := json.Marshal(jsonEntries)
	if err != nil {
		t.StoreLog("[TARGET] ⚠️ %s: failed to marshal final_result to JSON: %v", t.Metadata.Name, err)
		return
	}

	// Use storeState (lowercase) to avoid emitting a StateChangeEvent that would
	// re-trigger currentStateExposeHandler → MergeParentState → onMergeState feedback loop.
	t.storeState("final_result", json.RawMessage(composed))
	t.StoreLog("[TARGET] ✅ %s: final_result aggregated from %d children:\n%s", t.Metadata.Name, len(entries), composed)
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

// GetCompletedChildren returns a snapshot of the completed children map (for testing purposes)
func (t *Target) GetCompletedChildren() map[string]bool {
	snapshot := make(map[string]bool)
	t.completedChildren.Range(func(k, v any) bool {
		snapshot[k.(string)] = v.(bool)
		return true
	})
	return snapshot
}

// SetCompletedChildren populates the completed children map (for testing purposes)
func (t *Target) SetCompletedChildren(completed map[string]bool) {
	// Clear existing entries
	t.completedChildren.Range(func(k, _ any) bool {
		t.completedChildren.Delete(k)
		return true
	})
	for k, v := range completed {
		t.completedChildren.Store(k, v)
	}
}

// SetChildrenCreated sets the childrenCreated flag (for testing purposes)
func (t *Target) SetChildrenCreated(created bool) {
	t.childrenCreated = created
}
