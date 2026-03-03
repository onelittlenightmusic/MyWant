package types

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
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
		o.StoreState("goal", goal)
	}
	if current, ok := o.Spec.Params["current"]; ok && current != nil {
		o.StoreState("current", current)
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
	// Step 1: Read goal and current from state
	goalRaw, goalExists := want.GetState("goal")
	currentRaw, currentExists := want.GetState("current")
	if !goalExists || goalRaw == nil || !currentExists || currentRaw == nil {
		// Inputs not yet available; wait for next tick
		return nil
	}

	// Step 2: Change detection via MD5 hash of serialized inputs
	goalBytes, err := json.Marshal(goalRaw)
	if err != nil {
		want.DirectLog("[OPA-LLM-THINKER] ERROR marshalling goal: %v", err)
		return nil
	}
	currentBytes, err := json.Marshal(currentRaw)
	if err != nil {
		want.DirectLog("[OPA-LLM-THINKER] ERROR marshalling current: %v", err)
		return nil
	}

	combined := append(goalBytes, currentBytes...)
	hashBytes := md5.Sum(combined)
	inputHash := fmt.Sprintf("%x", hashBytes)

	prevHash, _ := want.GetStateString("_opa_input_hash", "")
	if prevHash == inputHash {
		// No changes detected; skip planning
		return nil
	}

	// Step 3: Write inputs to temp files
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

	want.StoreState("directions", directionTypes)
	want.StoreState("_opa_input_hash", inputHash)
	want.DirectLog("[OPA-LLM-THINKER] Plan updated with %d directions: %v", len(directionTypes), directionTypes)

	return nil
}
