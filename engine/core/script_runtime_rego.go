package mywant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// regoRuntime evaluates OPA Rego policies as think-agent ticks.
// It calls the `opa eval` CLI so no Go dependency on OPA is needed.
// The Rego policy must define a `directions` rule (set or array of strings).
type regoRuntime struct{}

// ExecuteThink evaluates the Rego script against the current goal/current state
// and writes directions back to plan state.
func (r *regoRuntime) ExecuteThink(want *Want, script string) error {
	goalAll := want.GetAllGoal()
	currentAll := want.GetAllCurrent()

	// Change detection — skip execution if inputs haven't changed.
	currentForHash := make(map[string]any, len(currentAll))
	for k, v := range currentAll {
		if k != "think_input_hash" {
			currentForHash[k] = v
		}
	}
	shouldRun, inputHash := ShouldRunAgent(want, "think_input_hash", goalAll, currentForHash)
	if !shouldRun {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "rego-runtime-*")
	if err != nil {
		return scriptErr(want, "REGO-THINK", fmt.Sprintf("failed to create temp dir: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	// Write policy file.
	policyPath := filepath.Join(tmpDir, "policy.rego")
	if err := os.WriteFile(policyPath, []byte(script), 0600); err != nil {
		return scriptErr(want, "REGO-THINK", fmt.Sprintf("failed to write policy: %v", err))
	}

	// Build OPA input.
	input := map[string]any{
		"goal":    goalAll,
		"current": currentAll,
	}
	inputBytes, _ := json.Marshal(input)
	inputPath := filepath.Join(tmpDir, "input.json")
	if err := os.WriteFile(inputPath, inputBytes, 0600); err != nil {
		return scriptErr(want, "REGO-THINK", fmt.Sprintf("failed to write input: %v", err))
	}

	// Evaluate `directions` rule.
	want.DirectLog("[REGO-THINK] evaluating policy")
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "opa", "eval",
		"--format", "raw",
		"--data", policyPath,
		"--input", inputPath,
		"data.directions",
	)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// `opa eval` exits non-zero when the query has no result (undefined).
		// Treat undefined as empty directions rather than an error.
		if stdout.Len() == 0 {
			want.SetPlan("directions", []any{})
			want.SetCurrent("think_input_hash", inputHash)
			return nil
		}
		return scriptErr(want, "REGO-THINK", fmt.Sprintf("opa eval failed: %v\nstderr:\n%s", err, stderr.String()))
	}

	// Parse result — OPA `--format raw` returns the value directly.
	// directions can be a set (printed as array) or array.
	raw := bytes.TrimSpace(stdout.Bytes())
	var directions []any
	if err := json.Unmarshal(raw, &directions); err != nil {
		// Could be a single value or empty — try wrapping.
		want.DirectLog("[REGO-THINK] WARN: could not parse directions as array: %v (raw: %s)", err, raw)
		want.SetPlan("directions", []any{})
	} else {
		want.SetPlan("directions", directions)
		want.DirectLog("[REGO-THINK] directions updated: %v", directions)
	}

	// Also evaluate optional `result` rule.
	resultCmd := exec.CommandContext(ctx, "opa", "eval",
		"--format", "raw",
		"--data", policyPath,
		"--input", inputPath,
		"data.result",
	)
	resultCmd.Env = os.Environ()
	if resultOut, err := resultCmd.Output(); err == nil && len(bytes.TrimSpace(resultOut)) > 0 {
		var result any
		if json.Unmarshal(bytes.TrimSpace(resultOut), &result) == nil {
			want.SetPlan("result", result)
		}
	}

	want.SetCurrent("think_input_hash", inputHash)
	return nil
}

// ExecuteDo is not supported for the Rego runtime.
func (r *regoRuntime) ExecuteDo(want *Want, _ string) error {
	return scriptErr(want, "REGO-DO", "rego runtime does not support do agents; use shell or python")
}

// ExecuteMonitor is not supported for the Rego runtime.
func (r *regoRuntime) ExecuteMonitor(want *Want, _ string) (bool, error) {
	return false, scriptErr(want, "REGO-MONITOR", "rego runtime does not support monitor agents; use shell or python")
}
