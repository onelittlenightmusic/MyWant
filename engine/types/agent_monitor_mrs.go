package types

import (
	"context"
	"fmt"
	"os"
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
	if MRSCheckRequiredParams(want) {
		return false, nil // keep polling; retry on next tick
	}
	// Gate: check using.when conditions against live provider state.
	// This blocks execution when the gate condition is not met regardless of packet cache.
	if want.HasUsingWhenConditions() && !want.CheckUsingWhenConditions() {
		return false, nil
	}
	// Rebuild skill_json_arg from template so param updates are picked up each tick.
	MRSRebuildSkillArg(want)

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
	DebugLog("[%s] [MRS-MONITOR] executing skill: %s args=%v (timeout: %ds)", want.Metadata.Name, scriptPath, args, timeoutSec)
	raw, err := runMRSSkillForWant(skillCtx, want, scriptPath, args, func(pct int, msg string) {
		want.SetCurrent("achieving_percentage", pct)
		if msg != "" {
			want.SetCurrent("summary", msg)
		}
		DebugLog("[%s] [MRS-MONITOR] progress %d%%: %s", want.Metadata.Name, pct, msg)
	})
	if err != nil {
		// If the parent or skill context was cancelled externally (e.g., want restart via
		// StopAllBackgroundAgents), the script was killed intentionally — don't record this
		// as an error so the restarted want can begin fresh without a stale error state.
		if ctx.Err() != nil || skillCtx.Err() != nil {
			DebugLog("[%s] [MRS-MONITOR] skill interrupted by context cancellation", want.Metadata.Name)
			return false, nil
		}
		want.StoreLog("[MRS-MONITOR] skill failed: %v", err)
		want.RecordAgentResult("", mrsMonitorAgentName, string(MonitorAgentType), "error", err.Error())
		want.SetCurrent("error", err.Error())
		return false, nil
	}

	want.SetCurrent("mrs_raw_output", raw)
	// Return false so the PollingAgent stays alive and fires on the next ticker tick.
	// Concurrent-execution protection is already provided by TryStartAgentRun inside
	// PollingAgent.runPoll, and context-cancellation SIGKILL is handled above.
	return false, nil
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
	if MRSCheckRequiredParams(want) {
		return fmt.Errorf("waiting for required params") // triggers retry on next cycle (succeeded=false)
	}
	// Rebuild skill_json_arg from template so param updates are picked up.
	MRSRebuildSkillArg(want)

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
	raw, err := runMRSSkillForWant(skillCtx, want, scriptPath, args, func(pct int, msg string) {
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
	return RunMRSScript(ctx, scriptPath, MRSRunOptions{Args: args, OnProgress: onProgress})
}

// runMRSSkillForWant is runMRSSkillWithArgs plus the residency options a
// skill_path-style want can ask for. agent.yaml plugins declare these under
// `script:`; these wants carry them as state fields instead, so a want type
// can opt into residency without being migrated to the newer plugin format.
//
//	skill_serve         bool — keep one interpreter alive (script must support it)
//	skill_cache_ttl_ms  int  — reuse a result for a repeat call with equal args
//	skill_max_procs     int  — resident processes for this script (0 → 1)
func runMRSSkillForWant(ctx context.Context, want *Want, scriptPath string, args []string, onProgress func(int, string)) (map[string]any, error) {
	return RunMRSScript(ctx, scriptPath, MRSRunOptions{
		Args:       args,
		OnProgress: onProgress,
		Serve:      GetCurrent(want, "skill_serve", false),
		CacheTTL:   time.Duration(GetCurrent(want, "skill_cache_ttl_ms", 0)) * time.Millisecond,
		MaxProcs:   GetCurrent(want, "skill_max_procs", 0),
	})
}
