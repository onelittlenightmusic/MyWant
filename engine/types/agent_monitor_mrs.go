package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "mywant/engine/core"
)

const mrsMonitorAgentName = "monitor_mrs_agent"
const mrsDoAgentName = "do_mrs_agent"

func init() {
	RegisterWithInit(func() {
		RegisterMonitorAgentType(
			mrsMonitorAgentName,
			[]Capability{Cap(mrsMonitorAgentName)},
			monitorMRSAgentFn,
		)
		RegisterDoAgentType(
			mrsDoAgentName,
			[]Capability{Cap(mrsDoAgentName)},
			doMRSAgentFn,
		)
	})
}

// mrsCheckRequiredParams checks whether all params listed in the "skill_required_params"
// current state field (space-separated) are non-empty.
//
// Resolution order (first non-empty wins):
//  1. Imported values — if the param name is an imported local key (Spec.Imports),
//     the live value from the parent/global state is used via GetAllState().
//  2. Spec.Params — the statically declared parameter value.
//
// Returns true if any required param is missing/empty, in which case the MRS agent
// should skip this tick. Also updates "summary" with a waiting message.
//
// This enables the "wait for params" pattern: create a want with empty params,
// and the agent will not execute until the user (or a thinker) populates them.
func mrsCheckRequiredParams(want *Want) bool {
	reqStr := GetCurrent(want, "skill_required_params", "")
	if reqStr == "" {
		return false // no guard configured
	}
	// Overlay imports so imported keys resolve through GetAllState.
	allState := want.GetAllState()
	var missing []string
	for _, p := range strings.Fields(reqStr) {
		// Priority 1: check imported / live state value
		if stateVal, ok := allState[p]; ok && stateVal != nil && strings.TrimSpace(fmt.Sprintf("%v", stateVal)) != "" {
			continue // provided via import or current state
		}
		// Priority 2: check Spec.Params
		val, exists := want.Spec.Params[p]
		if !exists || val == nil || strings.TrimSpace(fmt.Sprintf("%v", val)) == "" {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		want.StoreLog("[MRS] waiting for required params: %v", missing)
		want.SetCurrent("summary", "パラメータ待機中: "+strings.Join(missing, ", "))
		return true
	}
	return false
}

// mrsRebuildSkillArg rebuilds the "skill_json_arg" state from "skill_json_arg_template"
// by substituting %{param} placeholders with current values.
//
// Resolution order for each placeholder %{key} (first non-empty wins):
//  1. Imported / live state values from GetAllState() — covers keys declared in Spec.Imports.
//     If the imported value is a map (e.g. a selected_slot object), its sub-keys are also
//     available as %{key} so the template can reference nested fields directly.
//  2. Spec.Params — the statically declared parameter value.
//
// IMPORTANT: The placeholder syntax is %{key} (percent-brace), NOT ${key}.
// The onInitialize interpolation engine uses ${key} and would pre-expand those
// at want-creation time (replacing params with their init-time values, often "").
// Using %{key} avoids this clash: the template is stored literally by onInitialize
// and expanded here at each tick with the *current* values.
//
// This allows the want to pick up import changes (e.g. selected slot from a child choice
// want) without requiring re-initialization.
func mrsRebuildSkillArg(want *Want) {
	tmpl := GetCurrent(want, "skill_json_arg_template", "")
	if tmpl == "" {
		return // no template; keep existing skill_json_arg unchanged
	}

	// Build a merged params map: spec.params as base, overlaid by imported/live state values.
	merged := make(map[string]any)
	for k, v := range want.Spec.Params {
		merged[k] = v
	}
	// Overlay imported values (Priority 1): imported values take precedence over spec.params.
	allState := want.GetAllState()
	for k, v := range allState {
		if v != nil && strings.TrimSpace(fmt.Sprintf("%v", v)) != "" {
			merged[k] = v
			// If the imported value is a map, also expose its sub-keys so templates
			// can reference nested fields directly as %{subkey}.
			if m, ok := v.(map[string]any); ok {
				for subKey, subVal := range m {
					if _, alreadySet := merged[subKey]; !alreadySet {
						merged[subKey] = subVal
					}
				}
			}
		}
	}

	built := tmpl
	for k, v := range merged {
		built = strings.ReplaceAll(built, "%{"+k+"}", fmt.Sprintf("%v", v))
	}
	want.StoreState("skill_json_arg", built)
}

// monitorMRSAgentFn executes a Machine-Readable Skill script (no CLI args) and writes
// raw JSON output to the "mrs_raw_output" state field. EndProgressCycle then expands
// any state fields that declare fetchFrom+onFetchData automatically.
//
// Progress protocol: the script may emit {"_progress": <0-100>, "_message": "<text>"}
// lines to stdout at any point; these update achieving_percentage / summary in real time.
//
// Concurrent tick protection is now handled by PollingAgent via Want.TryStartAgentRun /
// FinishAgentRun, so no per-agent sync.Map guard is needed here.
//
// Timeout: reads "skill_timeout_seconds" from goal state (default: 120s).
func monitorMRSAgentFn(ctx context.Context, want *Want) (bool, error) {
	// Wait for required params before executing (supports param-driven doers).
	// Return false (shouldStop=false) so PollingAgent keeps polling until params arrive.
	if mrsCheckRequiredParams(want) {
		return false, nil // keep polling; retry on next tick
	}
	// Gate: check using.when conditions against live provider state.
	// This blocks execution when the gate condition is not met regardless of packet cache.
	if want.HasUsingWhenConditions() && !want.CheckUsingWhenConditions() {
		return false, nil
	}
	// Rebuild skill_json_arg from template so param updates are picked up each tick.
	mrsRebuildSkillArg(want)

	scriptPath, err := mrsSkillPath(want)
	if err != nil {
		want.StoreLog("[MRS-MONITOR] %v", err)
		want.RecordAgentResult("", mrsMonitorAgentName, string(MonitorAgentType), "error", err.Error())
		return false, nil
	}

	timeoutSec := GetGoal(want, "skill_timeout_seconds", 120)
	skillCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Pass CLI args if skill_json_arg is configured (supports param-driven monitor tasks).
	args := mrsBuildArgs(want)
	want.StoreLog("[MRS-MONITOR] executing skill: %s args=%v (timeout: %ds)", scriptPath, args, timeoutSec)
	raw, err := runMRSSkillWithArgs(skillCtx, scriptPath, args, func(pct int, msg string) {
		want.SetCurrent("achieving_percentage", pct)
		if msg != "" {
			want.SetCurrent("summary", msg)
		}
		want.StoreLog("[MRS-MONITOR] progress %d%%: %s", pct, msg)
	})
	if err != nil {
		// If the parent or skill context was cancelled externally (e.g., want restart via
		// StopAllBackgroundAgents), the script was killed intentionally — don't record this
		// as an error so the restarted want can begin fresh without a stale error state.
		if ctx.Err() != nil || skillCtx.Err() != nil {
			want.StoreLog("[MRS-MONITOR] skill interrupted by context cancellation (not an error)")
			return false, nil
		}
		want.StoreLog("[MRS-MONITOR] skill failed: %v", err)
		want.RecordAgentResult("", mrsMonitorAgentName, string(MonitorAgentType), "error", err.Error())
		want.SetCurrent("error", err.Error())
		return false, nil
	}

	want.SetCurrent("mrs_raw_output", raw)
	// Return true (shouldStop) so the PollingAgent goroutine exits immediately after
	// a successful execution. This prevents a buffered ticker tick (Go ticker channel
	// size=1) from starting a spurious second execution while the execution loop is
	// concurrently calling StopAllBackgroundAgents(), which would SIGKILL the script.
	return true, nil
}

// doMRSAgentFn executes a Machine-Readable Skill script with optional CLI arguments
// and writes raw JSON output to the "mrs_raw_output" state field.
//
// Argument resolution (in priority order):
//  1. skill_json_arg — a pre-built JSON string passed as a single CLI argument.
//     Set this via onInitialize with ${field} interpolation for structured inputs.
//  2. skill_args_keys — space-separated list of current state field names whose
//     values become positional CLI arguments (empty values are filtered out).
//
// Param-wait support: if "skill_required_params" is set in current state, execution
// is skipped (returns nil without running) until all listed params are non-empty.
// "skill_json_arg_template" is rebuilt from spec.params on each successful tick.
//
// Timeout: reads "skill_timeout_seconds" from goal state (default: 120s).
func doMRSAgentFn(ctx context.Context, want *Want) error {
	// Wait for required params before executing (supports param-driven doers).
	if mrsCheckRequiredParams(want) {
		return fmt.Errorf("waiting for required params") // triggers retry on next cycle (succeeded=false)
	}
	// Rebuild skill_json_arg from template so param updates are picked up.
	mrsRebuildSkillArg(want)

	scriptPath, err := mrsSkillPath(want)
	if err != nil {
		want.StoreLog("[MRS-DO] %v", err)
		want.RecordAgentResult("", mrsDoAgentName, string(DoAgentType), "error", err.Error())
		return nil
	}

	args := mrsBuildArgs(want)

	timeoutSec := GetGoal(want, "skill_timeout_seconds", 120)
	skillCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	want.StoreLog("[MRS-DO] executing skill: %s args=%v (timeout: %ds)", scriptPath, args, timeoutSec)
	raw, err := runMRSSkillWithArgs(skillCtx, scriptPath, args, func(pct int, msg string) {
		want.SetCurrent("achieving_percentage", pct)
		if msg != "" {
			want.SetCurrent("summary", msg)
		}
		want.StoreLog("[MRS-DO] progress %d%%: %s", pct, msg)
	})
	if err != nil {
		want.StoreLog("[MRS-DO] skill failed: %v", err)
		want.RecordAgentResult("", mrsDoAgentName, string(DoAgentType), "error", err.Error())
		want.SetCurrent("error", err.Error())
		return nil
	}

	want.SetCurrent("mrs_raw_output", raw)
	return nil
}

// mrsBuildArgs builds CLI argument list from want state.
// If skill_json_arg is set, it is returned as a single-element slice.
// Otherwise skill_args_keys is used: each named field value becomes an arg,
// with empty strings filtered out (supports optional trailing args).
func mrsBuildArgs(want *Want) []string {
	if jsonArg := GetCurrent(want, "skill_json_arg", ""); jsonArg != "" {
		return []string{jsonArg}
	}
	keys := strings.Fields(GetCurrent(want, "skill_args_keys", ""))
	args := make([]string, 0, len(keys))
	for _, key := range keys {
		val := fmt.Sprintf("%v", GetCurrent[any](want, key, nil))
		if val != "" && val != "<nil>" {
			args = append(args, val)
		}
	}
	return args
}

// mrsSkillPath resolves the skill script path from want state.
// Priority: skill_path (supports ~/) > {skill_base_dir}/{skill_name}/main.py
func mrsSkillPath(want *Want) (string, error) {
	if p := GetCurrent(want, "skill_path", ""); p != "" {
		return expandTilde(p), nil
	}
	skillName := GetCurrent(want, "skill_name", "")
	if skillName == "" {
		return "", fmt.Errorf("skill_name or skill_path must be set in want state")
	}
	baseDir := GetCurrent(want, "skill_base_dir", "")
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".claude", "skills")
	}
	return filepath.Join(expandTilde(baseDir), skillName, "main.py"), nil
}

// expandTilde replaces a leading "~/" with the user's home directory.
func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}

// runMRSSkillWithArgs executes the skill script with optional CLI args and returns
// the parsed JSON output. Pass nil or empty slice for no args.
//
// Progress protocol: the script may write {"_progress": <0-100>, "_message": "<text>"}
// lines to stdout at any time during execution. These lines are forwarded to onProgress
// (if non-nil) and are NOT included in the returned result. The last non-progress JSON
// line is returned as the final result.
func runMRSSkillWithArgs(ctx context.Context, scriptPath string, args []string, onProgress func(int, string)) (map[string]any, error) {
	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.CommandContext(ctx, "python3", cmdArgs...)
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	var finalResult map[string]any
	decoder := json.NewDecoder(stdout)
	for decoder.More() {
		var obj map[string]any
		if err := decoder.Decode(&obj); err != nil {
			break // unrecoverable parse error; check stderr below
		}
		if pct, ok := obj["_progress"]; ok {
			if onProgress != nil {
				onProgress(int(toMRSFloat64(pct)), mrsString(obj["_message"]))
			}
		} else {
			finalResult = obj
		}
	}

	if err := cmd.Wait(); err != nil {
		// Prefer structured error from the script's own JSON output
		if finalResult != nil {
			if msg, ok := finalResult["error"].(string); ok && msg != "" {
				return nil, fmt.Errorf("%s", msg)
			}
		}
		if stderr := strings.TrimSpace(stderrBuf.String()); stderr != "" {
			return nil, fmt.Errorf("exit error: %w\nstderr: %s", err, stderr)
		}
		return nil, fmt.Errorf("exit error: %w", err)
	}

	if finalResult == nil {
		return nil, fmt.Errorf("skill produced no JSON output")
	}
	return finalResult, nil
}

func toMRSFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func mrsString(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
