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
	// Step 1: Collect all goal-labeled and current-labeled state fields, then
	// merge their values into flat maps so OPA sees input.goal.X / input.current.X
	// directly (not input.goal.<fieldName>.X).
	goalRaw := mergeOPAInput(want.GetAllGoal())
	currentRaw := mergeOPAInput(want.GetAllCurrent())
	if len(goalRaw) == 0 {
		// Goal not yet available; wait for next tick
		return nil
	}

	// Step 2: Change detection via helper
	shouldRun, inputHash := ShouldRunAgent(want, "opa_input_hash", goalRaw, currentRaw)
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

// mergeOPAInput flattens a map of labeled state fields into a single map for OPA input.
// Map-valued fields have their contents merged in (supporting the blob-per-field pattern),
// so OPA sees input.goal.X / input.current.X directly rather than input.goal.<fieldName>.X.
// Scalar-valued fields are included by their field name as-is.
func mergeOPAInput(all map[string]any) map[string]any {
	result := make(map[string]any, len(all))
	for k, v := range all {
		if m, ok := v.(map[string]any); ok {
			for mk, mv := range m {
				result[mk] = mv
			}
		} else {
			result[k] = v
		}
	}
	return result
}
