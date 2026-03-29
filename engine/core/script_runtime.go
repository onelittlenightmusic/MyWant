package mywant

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ScriptRuntime executes inline scripts with want state I/O.
type ScriptRuntime interface {
	// ExecuteThink runs the script as a think-agent tick.
	// It reads goal/current state and writes back directions/result/current_updates.
	ExecuteThink(want *Want, script string) error

	// ExecuteDo runs the script as a synchronous do-agent action.
	ExecuteDo(want *Want, script string) error

	// ExecuteMonitor runs the script as a monitor-agent poll tick.
	// Returns shouldStop=true to signal that monitoring should cease.
	ExecuteMonitor(want *Want, script string) (shouldStop bool, err error)
}

// scriptInput is the combined JSON structure written to MYWANT_INPUT_FILE.
type scriptInput struct {
	Goal    map[string]any `json:"goal"`
	Current map[string]any `json:"current"`
	Plan    map[string]any `json:"plan"`
}

// scriptOutput is the parsed JSON from script stdout.
type scriptOutput struct {
	Result         any            `json:"result"`
	Directions     []any          `json:"directions"`
	CurrentUpdates map[string]any `json:"current_updates"`
	ShouldStop     bool           `json:"should_stop"`
}

// writeStateFiles writes goal/current/plan state to a temp directory and returns the file paths.
// The caller is responsible for removing the tmpDir.
func writeStateFiles(want *Want, tmpDir string) (goalPath, currentPath, planPath, inputPath string, err error) {
	goalAll := want.GetAllGoal()
	currentAll := want.GetAllCurrent()
	planAll := want.GetAllPlan()

	combined := scriptInput{Goal: goalAll, Current: currentAll, Plan: planAll}

	goalBytes, _ := json.Marshal(goalAll)
	currentBytes, _ := json.Marshal(currentAll)
	planBytes, _ := json.Marshal(planAll)
	combinedBytes, merr := json.Marshal(combined)
	if merr != nil {
		return "", "", "", "", fmt.Errorf("failed to marshal state: %w", merr)
	}

	goalPath = filepath.Join(tmpDir, "goal.json")
	currentPath = filepath.Join(tmpDir, "current.json")
	planPath = filepath.Join(tmpDir, "plan.json")
	inputPath = filepath.Join(tmpDir, "input.json")

	files := map[string][]byte{
		goalPath: goalBytes, currentPath: currentBytes,
		planPath: planBytes, inputPath: combinedBytes,
	}
	for path, data := range files {
		if werr := os.WriteFile(path, data, 0600); werr != nil {
			return "", "", "", "", fmt.Errorf("failed to write %s: %w", path, werr)
		}
	}
	return goalPath, currentPath, planPath, inputPath, nil
}

// stateEnv builds the MYWANT_* environment variable slice from file paths.
func stateEnv(goalPath, currentPath, planPath, inputPath string) []string {
	return []string{
		"MYWANT_GOAL_FILE=" + goalPath,
		"MYWANT_CURRENT_FILE=" + currentPath,
		"MYWANT_PLAN_FILE=" + planPath,
		"MYWANT_INPUT_FILE=" + inputPath,
	}
}

// parseScriptOutput unmarshals JSON stdout into scriptOutput.
func parseScriptOutput(stdout []byte) (*scriptOutput, error) {
	var out scriptOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return nil, fmt.Errorf("script stdout is not valid JSON: %w\nstdout:\n%s", err, string(stdout))
	}
	return &out, nil
}

// applyScriptOutput writes the script result back into want state.
// - directions → plan state
// - result     → plan state
// - current_updates → current state (only for current-labeled fields)
func applyScriptOutput(want *Want, out *scriptOutput, agentLabel string) {
	if out.Directions != nil {
		want.SetPlan("directions", out.Directions)
		want.DirectLog("[%s] directions updated: %v", agentLabel, out.Directions)
	}
	if out.Result != nil {
		want.SetPlan("result", out.Result)
	}
	for k, v := range out.CurrentUpdates {
		if label, exists := want.StateLabels[k]; exists && label == LabelCurrent {
			want.SetCurrent(k, v)
		} else {
			want.DirectLog("[%s] WARN: current_updates key %q is not current-labeled; skipped", agentLabel, k)
		}
	}
}

// resolveRuntime returns the ScriptRuntime for the given runtime name.
func resolveRuntime(runtime string) ScriptRuntime {
	switch runtime {
	case "python":
		return &pythonRuntime{}
	case "rego":
		return &regoRuntime{}
	default: // "shell" and anything else
		return &shellRuntime{}
	}
}
