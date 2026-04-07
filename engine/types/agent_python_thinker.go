package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	. "mywant/engine/core"
)

const pythonThinkerAgentName = "python"

func init() {
	RegisterWantImplementation[PythonThinkerWant, PythonThinkerLocals]("python")
	RegisterThinkAgentType(pythonThinkerAgentName, []Capability{
		{Name: "python_thinking", Gives: []string{"python_thinking"}, Description: "Runs a Python script as a think agent each tick"},
	}, pythonThinkerThink)
}

// PythonThinkerLocals holds type-specific local state (no runtime locals needed).
type PythonThinkerLocals struct{}

// PythonThinkerWant is a passive coordinator whose planning logic is entirely
// handled by the PythonThinker ThinkAgent. The want itself never self-completes.
type PythonThinkerWant struct {
	Want
}

func (o *PythonThinkerWant) GetLocals() *PythonThinkerLocals {
	return CheckLocalsInitialized[PythonThinkerLocals](&o.Want)
}

// Initialize copies goal/current from params to state.
func (o *PythonThinkerWant) Initialize() {
	if goal, ok := o.Spec.GetParam("goal"); ok && goal != nil {
		o.SetGoal("goal", goal)
	}
	if current, ok := o.Spec.GetParam("current"); ok && current != nil {
		o.SetCurrent("current", current)
	}
	// Copy config params → state so the thinker reads from GetCurrent instead of GetStringParam
	o.SetCurrent("python_script_path", o.GetStringParam("python_script_path", ""))
	o.SetCurrent("python_script", o.GetStringParam("python_script", ""))
	o.SetCurrent("python_command", o.GetStringParam("python_command", "python3"))
}

// IsAchieved always returns false — the thinker runs indefinitely.
func (o *PythonThinkerWant) IsAchieved() bool { return false }

// Progress is a no-op; the ThinkAgent handles all logic each tick.
func (o *PythonThinkerWant) Progress() {}

// pythonThinkerThink executes a Python script with goal/current/plan state as JSON inputs,
// then stores the script's structured output back to state.
// It uses a hash to skip execution when inputs have not changed since the last run.
// Script failures set WantStatusModuleError so the user is immediately notified.
func pythonThinkerThink(ctx context.Context, want *Want) error {
	// Step 1: Collect all goal-labeled and current-labeled state fields.
	// The named primary blob ("goal"/"current") is expanded so the script
	// sees its contents directly, matching the OPA thinker convention.
	goalAll := want.GetAllGoal()
	if parentGoal := want.GetParentAllGoal(); len(parentGoal) > 0 {
		goalAll = parentGoal
	}
	goalRaw := mergeOPAInput(goalAll, "goal")
	if len(goalRaw) == 0 {
		// Goal not yet available; wait for next tick.
		return nil
	}

	currentAll := want.GetAllCurrent()
	for k, v := range want.GetParentAllCurrent() {
		currentAll[k] = v
	}
	currentRaw := mergeOPAInput(currentAll, "current")

	planAll := want.GetAllPlan()
	planRaw := mergeOPAInput(planAll, "plan")

	// Step 2: Change detection — exclude python_input_hash to avoid circular dependency.
	currentForHash := make(map[string]any, len(currentRaw))
	for k, v := range currentRaw {
		if k != "python_input_hash" {
			currentForHash[k] = v
		}
	}
	shouldRun, inputHash := ShouldRunAgent(want, "python_input_hash", goalRaw, currentForHash)
	if !shouldRun {
		return nil
	}

	// Step 3: Resolve the Python script to execute (read from state set by Initialize).
	scriptPath := GetCurrent(want, "python_script_path", "")
	inlineCode := GetCurrent(want, "python_script", "")
	if scriptPath == "" && inlineCode == "" {
		return pythonThinkerError(want, "neither python_script nor python_script_path is set")
	}

	// Step 4: Write all inputs to a temp directory.
	tmpDir, err := os.MkdirTemp("", "python-thinker-*")
	if err != nil {
		return pythonThinkerError(want, fmt.Sprintf("failed to create temp dir: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	// If inline code provided, write it to a temp script file.
	if inlineCode != "" {
		scriptPath = filepath.Join(tmpDir, "script.py")
		if err := os.WriteFile(scriptPath, []byte(inlineCode), 0700); err != nil {
			return pythonThinkerError(want, fmt.Sprintf("failed to write inline script: %v", err))
		}
	}

	goalBytes, _ := json.Marshal(goalRaw)
	currentBytes, _ := json.Marshal(currentRaw)
	planBytes, _ := json.Marshal(planRaw)

	combinedInput := map[string]any{
		"goal":    goalRaw,
		"current": currentRaw,
		"plan":    planRaw,
	}
	combinedBytes, _ := json.Marshal(combinedInput)

	goalPath := filepath.Join(tmpDir, "goal.json")
	currentPath := filepath.Join(tmpDir, "current.json")
	planPath := filepath.Join(tmpDir, "plan.json")
	inputPath := filepath.Join(tmpDir, "input.json")

	for path, data := range map[string][]byte{
		goalPath: goalBytes, currentPath: currentBytes,
		planPath: planBytes, inputPath: combinedBytes,
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			return pythonThinkerError(want, fmt.Sprintf("failed to write %s: %v", path, err))
		}
	}

	// Step 5: Build and execute the command (read from state set by Initialize).
	pythonCmd := GetCurrent(want, "python_command", "python3")
	want.DirectLog("[PYTHON-THINKER] Running: %s %s", pythonCmd, scriptPath)

	cmd := exec.CommandContext(ctx, pythonCmd, scriptPath)
	cmd.Env = append(os.Environ(),
		"MYWANT_GOAL_FILE="+goalPath,
		"MYWANT_CURRENT_FILE="+currentPath,
		"MYWANT_PLAN_FILE="+planPath,
		"MYWANT_INPUT_FILE="+inputPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return pythonThinkerError(want, fmt.Sprintf(
			"script exited with error: %v\nstderr:\n%s", err, stderr.String()))
	}

	// Step 6: Parse output JSON.
	// Expected format:
	//   { "result": <any>, "directions": [...], "current_updates": {"key": val} }
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return pythonThinkerError(want, fmt.Sprintf(
			"script stdout is not valid JSON: %v\nstdout:\n%s", err, stdout.String()))
	}

	// Store result to plan state.
	if result, ok := output["result"]; ok {
		want.SetPlan("result", result)
	}

	// Extract directions — support both string array and OPA-style action objects.
	if rawDirs, ok := output["directions"].([]any); ok {
		directionTypes := make([]any, 0, len(rawDirs))
		for _, d := range rawDirs {
			if s, ok := d.(string); ok {
				directionTypes = append(directionTypes, s)
			} else if m, ok := d.(map[string]any); ok {
				if t, ok := m["type"].(string); ok {
					directionTypes = append(directionTypes, t)
				}
			}
		}
		want.SetPlan("directions", directionTypes)
		want.DirectLog("[PYTHON-THINKER] Plan updated with %d directions: %v", len(directionTypes), directionTypes)
	}

	// Apply current_updates — only for keys already labeled LabelCurrent to prevent
	// accidental overwrite of internal or plan-labeled state.
	if updates, ok := output["current_updates"].(map[string]any); ok {
		for k, v := range updates {
			if label, exists := want.StateLabels[k]; exists && label == LabelCurrent {
				want.SetCurrent(k, v)
			} else {
				want.DirectLog("[PYTHON-THINKER] WARN: current_updates key %q is not a current-labeled field; skipped", k)
			}
		}
	}

	// Step 7: Record input hash so the script is not re-run until inputs change.
	want.SetCurrent("python_input_hash", inputHash)

	return nil
}

// pythonThinkerError logs the error message, sets the want to ModuleError status,
// and returns the error so the think loop can record it.
func pythonThinkerError(want *Want, msg string) error {
	err := fmt.Errorf("[PYTHON-THINKER] %s", msg)
	want.DirectLog("%v", err)
	want.SetStatus(WantStatusModuleError)
	return err
}
