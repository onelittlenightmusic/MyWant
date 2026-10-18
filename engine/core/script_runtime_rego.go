package mywant

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
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

	inputBytes, err := json.Marshal(map[string]any{"goal": goalAll, "current": currentAll})
	if err != nil {
		return scriptErr(want, "REGO-THINK", fmt.Sprintf("failed to marshal input: %v", err))
	}
	eval := func(query string) (*inlineResult, error) {
		return runInline(inlineRun{
			Want: want,
			Argv: []string{"opa", "eval", "--format", "raw",
				"--data", "policy.rego", "--input", "input.json", query},
			Files:   map[string][]byte{"policy.rego": []byte(script), "input.json": inputBytes},
			Timeout: 30 * time.Second,
		})
	}

	// Evaluate `directions` rule.
	want.StoreLog("[REGO-THINK] evaluating policy")
	res, err := eval("data.directions")
	if err != nil {
		return scriptErr(want, "REGO-THINK", fmt.Sprintf("opa eval failed: %v", err))
	}
	if res.ExitCode != 0 {
		// `opa eval` exits non-zero when the query has no result (undefined).
		// Treat undefined as empty directions rather than an error.
		if len(res.Stdout) == 0 {
			want.SetPlan("directions", []any{})
			want.SetCurrent("think_input_hash", inputHash)
			return nil
		}
		return scriptErr(want, "REGO-THINK", fmt.Sprintf("opa eval failed: exit status %d\nstderr:\n%s", res.ExitCode, res.Stderr))
	}

	// Parse result — OPA `--format raw` returns the value directly.
	// directions can be a set (printed as array) or array.
	raw := bytes.TrimSpace(res.Stdout)
	var directions []any
	if err := json.Unmarshal(raw, &directions); err != nil {
		// Could be a single value or empty — try wrapping.
		want.StoreLog("[REGO-THINK] WARN: could not parse directions as array: %v (raw: %s)", err, raw)
		want.SetPlan("directions", []any{})
	} else {
		want.SetPlan("directions", directions)
		want.StoreLog("[REGO-THINK] directions updated: %v", directions)
	}

	// Also evaluate optional `result` rule.
	if res, err := eval("data.result"); err == nil && res.ExitCode == 0 && len(bytes.TrimSpace(res.Stdout)) > 0 {
		var result any
		if json.Unmarshal(bytes.TrimSpace(res.Stdout), &result) == nil {
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
