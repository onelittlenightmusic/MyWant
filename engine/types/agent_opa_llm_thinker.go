package types

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	. "mywant/engine/core"
)

const opaLLMThinkerAgentName = "opa_llm_thinker"

func init() {
	RegisterWantImplementation[OpaLLMPlannerWant, OpaLLMPlannerLocals]("opa_llm_planner")
	RegisterThinkAgentType(opaLLMThinkerAgentName, []Capability{
		{Name: "opa_llm_planning", Gives: []string{"opa_llm_planning"}, Description: "Plans actions using OPA policy engine and LLM reasoning"},
	}, opaLLMThinkerThink)
}

// OpaLLMPlannerLocals holds type-specific local state (no runtime locals needed).
type OpaLLMPlannerLocals struct{}

// OpaLLMPlannerWant is a passive coordinator whose planning logic is entirely
// handled by the OpaLLMThinker ThinkAgent. The want itself never self-completes.
type OpaLLMPlannerWant struct {
	Want
}

func (o *OpaLLMPlannerWant) GetLocals() *OpaLLMPlannerLocals {
	return CheckLocalsInitialized[OpaLLMPlannerLocals](&o.Want)
}

// Initialize copies goal/current from params to state.
// Always overwrites because initialValue: {} from YAML is a non-nil empty map.
func (o *OpaLLMPlannerWant) Initialize() {
	if goal, ok := o.Spec.Params["goal"]; ok && goal != nil {
		o.SetGoal("goal", goal)
	}
	if current, ok := o.Spec.Params["current"]; ok && current != nil {
		o.SetCurrent("current", current)
	}
}

// IsAchieved always returns false — the planner runs indefinitely.
func (o *OpaLLMPlannerWant) IsAchieved() bool { return false }

// Progress is a no-op; the ThinkAgent handles all logic each tick.
func (o *OpaLLMPlannerWant) Progress() {}

// opaLLMThinkerThink calls `opa-llm-planner plan` with the current goal/current state,
// and stores the resulting actions back to state. It uses a hash to skip execution
// when neither goal nor current have changed since the last run.
func opaLLMThinkerThink(ctx context.Context, want *Want) error {
	// Step 1: Collect all goal-labeled and current-labeled state fields.
	// The named "primary" blob (field named "goal"/"current") is expanded so OPA
	// sees its contents directly (e.g. input.goal.trip.X, input.current.hotel_cost).
	// All other labeled fields (costs, opa_input_hash, …) are kept as named keys
	// so OPA can still access them as input.current.costs etc.
	// Build goal: prefer parent's goal-labeled fields; fall back to own.
	goalAll := want.GetAllGoal()
	if parentGoal := want.GetParentAllGoal(); len(parentGoal) > 0 {
		goalAll = parentGoal
	}
	goalRaw := mergeOPAInput(goalAll, "goal")
	if len(goalRaw) == 0 {
		// Goal not yet available; wait for next tick
		return nil
	}

	// Build current: start from own current-labeled fields, then overlay parent's.
	// Parent (coordinator) carries costs written by ConditionThinker on child wants,
	// so merging parent's current makes costs available to OPA without special-casing.
	currentAll := want.GetAllCurrent()
	for k, v := range want.GetParentAllCurrent() {
		currentAll[k] = v
	}
	currentRaw := mergeOPAInput(currentAll, "current")

	// Step 2: Change detection — exclude opa_input_hash from the hash input to
	// avoid a circular dependency where the hash changes its own input every tick.
	currentForHash := make(map[string]any, len(currentRaw))
	for k, v := range currentRaw {
		if k != "opa_input_hash" {
			currentForHash[k] = v
		}
	}
	shouldRun, inputHash := ShouldRunAgent(want, "opa_input_hash", goalRaw, currentForHash)
	if !shouldRun {
		return nil
	}

	// Step 3: Write inputs to temp files
	goalBytes, _ := json.Marshal(goalRaw)
	currentBytes, _ := json.Marshal(currentRaw)
	
	tmpDir, err := os.MkdirTemp("", "opa-llm-thinker-*")
	if err != nil {
		want.DirectLog("[OPA-LLM-THINKER] ERROR creating temp dir: %v", err)
		return nil
	}
	defer os.RemoveAll(tmpDir)

	goalPath := filepath.Join(tmpDir, "goal.json")
	currentPath := filepath.Join(tmpDir, "current.json")

	if err := os.WriteFile(goalPath, goalBytes, 0600); err != nil {
		want.DirectLog("[OPA-LLM-THINKER] ERROR writing goal.json: %v", err)
		return nil
	}
	if err := os.WriteFile(currentPath, currentBytes, 0600); err != nil {
		want.DirectLog("[OPA-LLM-THINKER] ERROR writing current.json: %v", err)
		return nil
	}

	// Step 4: Build command arguments from params
	command := want.GetStringParam("opa_llm_planner_command", "opa-llm-planner")
	args := []string{"plan", "--goal", goalPath, "--current", currentPath}

	policyDir := want.GetStringParam("policy_dir", "")
	if policyDir != "" {
		args = append(args, "--policy", policyDir)
	}

	useLLM := want.GetBoolParam("use_llm", true)
	if useLLM {
		provider := want.GetStringParam("llm_provider", "anthropic")
		args = append(args, "--llm", "--llm-provider", provider)
	}

	want.DirectLog("[OPA-LLM-THINKER] Running: %s %v", command, args)

	// Step 5: Execute the planner command, inheriting the current process environment
	// so that env vars like ANTHROPIC_API_KEY are available to the planner.
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = os.Environ()
	stdout, err := cmd.Output()
	if err != nil {
		want.DirectLog("[OPA-LLM-THINKER] ERROR running planner: %v", err)
		return nil
	}

	// Step 6: Parse output and extract direction type names as strings.
	// OPA output format: {"actions": [{"type": "reserve_hotel", "status": "pending"}, ...]}
	var planResult map[string]any
	if err := json.Unmarshal(stdout, &planResult); err != nil {
		want.DirectLog("[OPA-LLM-THINKER] ERROR parsing planner output: %v", err)
		return nil
	}

	rawActions, _ := planResult["actions"].([]any)
	directionTypes := make([]any, 0, len(rawActions))
	for _, a := range rawActions {
		if aMap, ok := a.(map[string]any); ok {
			if t, ok := aMap["type"].(string); ok {
				directionTypes = append(directionTypes, t)
			}
		} else if s, ok := a.(string); ok {
			directionTypes = append(directionTypes, s)
		}
	}

	want.SetPlan("directions", directionTypes)
	want.SetCurrent("opa_input_hash", inputHash)
	want.DirectLog("[OPA-LLM-THINKER] Plan updated with %d directions: %v", len(directionTypes), directionTypes)

	return nil
}

// mergeOPAInput builds a flat map for OPA input from labeled state fields.
// The field named primaryKey (e.g. "goal" or "current") is a blob whose contents
// are merged to the top level so OPA sees input.goal.trip.X / input.current.hotel_cost
// directly.  All other fields (e.g. "costs", "opa_input_hash") are kept as named
// keys so OPA can access them as input.current.costs etc.
func mergeOPAInput(all map[string]any, primaryKey string) map[string]any {
	result := make(map[string]any, len(all))
	for k, v := range all {
		if k == primaryKey {
			if m, ok := v.(map[string]any); ok {
				for mk, mv := range m {
					result[mk] = mv
				}
				continue
			}
		}
		result[k] = v
	}
	return result
}
