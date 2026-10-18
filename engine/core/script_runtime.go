package mywant

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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
	JSONData       map[string]any `json:"json_data"`     // structured data for onFetchData JSON path extraction
	LabelUpdates   map[string]any `json:"label_updates"` // tile-* labels the script sets (see applyLabelUpdates)
}

// stateFiles is goal/current/plan state as the files a script reads:
// goal.json, current.json, plan.json, and all three in input.json.
func stateFiles(want *Want) (map[string][]byte, error) {
	goalAll := want.GetAllGoal()
	currentAll := want.GetAllCurrent()
	planAll := want.GetAllPlan()

	combined, err := json.Marshal(scriptInput{Goal: goalAll, Current: currentAll, Plan: planAll})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}
	goalBytes, _ := json.Marshal(goalAll)
	currentBytes, _ := json.Marshal(currentAll)
	planBytes, _ := json.Marshal(planAll)
	return map[string][]byte{
		"goal.json": goalBytes, "current.json": currentBytes,
		"plan.json": planBytes, "input.json": combined,
	}, nil
}

// stateFileEnv names the MYWANT_* variable that points at each state file.
var stateFileEnv = map[string]string{
	"MYWANT_GOAL_FILE":    "goal.json",
	"MYWANT_CURRENT_FILE": "current.json",
	"MYWANT_PLAN_FILE":    "plan.json",
	"MYWANT_INPUT_FILE":   "input.json",
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
// - directions    → plan state
// - result        → plan state
// - current_updates → current state (only for current-labeled fields)
// - json_data     → triggers onFetchData JSON path extraction for current-labeled state fields
// - label_updates → tile-* labels (applyLabelUpdates)
func applyScriptOutput(want *Want, out *scriptOutput, agentLabel string) {
	if out.LabelUpdates != nil {
		applyLabelUpdates(want, map[string]any{"label_updates": out.LabelUpdates}, agentLabel)
	}
	if out.Directions != nil {
		want.SetPlan("directions", out.Directions)
		want.StoreLog("[%s] directions updated: %v", agentLabel, out.Directions)
	}
	if out.Result != nil {
		want.SetPlan("result", out.Result)
	}
	for k, v := range out.CurrentUpdates {
		if label, exists := want.StateLabels[k]; exists && label == LabelCurrent {
			want.SetCurrent(k, v)
		} else {
			want.StoreLog("[%s] WARN: current_updates key %q is not current-labeled; skipped", agentLabel, k)
		}
	}
	if out.JSONData != nil && want.WantTypeDefinition != nil {
		for _, sd := range want.WantTypeDefinition.State {
			if sd.OnFetchData == "" || sd.Label != "current" {
				continue
			}
			value := extractJSONPath(out.JSONData, sd.OnFetchData)
			if value != nil {
				want.SetCurrent(sd.Name, value)
				want.StoreLog("[%s] onFetchData %q → %q = %v", agentLabel, sd.OnFetchData, sd.Name, value)
			}
		}
	}
}

// extractJSONPath extracts a value from data using dot-notation with optional array indices.
// Examples: "routes[0].departure", "summary", "routes[0].legs[1].line"
func extractJSONPath(data any, path string) any {
	if path == "" {
		return data
	}
	segments := splitJSONPathSegments(path)
	current := data
	for _, seg := range segments {
		if current == nil {
			return nil
		}
		// Check for array index syntax: "key[n]"
		if bracketIdx := strings.Index(seg, "["); bracketIdx >= 0 {
			key := seg[:bracketIdx]
			indexStr := strings.TrimSuffix(seg[bracketIdx+1:], "]")
			idx, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil
			}
			if key != "" {
				current = navigateMapKey(current, key)
				if current == nil {
					return nil
				}
			}
			current = navigateArrayIndex(current, idx)
		} else {
			current = navigateMapKey(current, seg)
		}
	}
	return current
}

// splitJSONPathSegments splits a dot-notation path into segments, preserving array indices.
// "routes[0].departure" → ["routes[0]", "departure"]
func splitJSONPathSegments(path string) []string {
	return strings.Split(path, ".")
}

// navigateMapKey returns data[key] for map types.
func navigateMapKey(data any, key string) any {
	switch d := data.(type) {
	case map[string]any:
		return d[key]
	case map[any]any:
		return d[key]
	}
	return nil
}

// navigateArrayIndex returns data[idx] for slice types.
func navigateArrayIndex(data any, idx int) any {
	switch d := data.(type) {
	case []any:
		if idx >= 0 && idx < len(d) {
			return d[idx]
		}
	}
	return nil
}

// NewAnsibleRuntime returns an AnsibleRuntime for use outside this package.
func NewAnsibleRuntime() ScriptRuntime {
	return &ansibleRuntime{}
}

// resolveRuntime returns the ScriptRuntime for the given runtime name.
func resolveRuntime(runtime string) ScriptRuntime {
	switch runtime {
	case "python":
		return &pythonRuntime{}
	case "rego":
		return &regoRuntime{}
	case "ansible":
		return &ansibleRuntime{}
	default: // "shell" and anything else
		return &shellRuntime{}
	}
}
