package types

import (
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
}

// monitorMRSAgentFn executes a Machine-Readable Skill script (no CLI args) and writes
// raw JSON output to the "mrs_raw_output" state field. EndProgressCycle then expands
// any state fields that declare fetchFrom+onFetchData automatically.
//
// Timeout: reads "skill_timeout_seconds" from goal state (default: 120s).
func monitorMRSAgentFn(ctx context.Context, want *Want) (bool, error) {
	scriptPath, err := mrsSkillPath(want)
	if err != nil {
		want.DirectLog("[MRS-MONITOR] %v", err)
		mrsSetStatus(want, "failed", err.Error())
		return false, nil
	}

	timeoutSec := GetGoal(want, "skill_timeout_seconds", 120)
	skillCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	want.DirectLog("[MRS-MONITOR] executing skill: %s (timeout: %ds)", scriptPath, timeoutSec)
	raw, err := runMRSSkillWithArgs(skillCtx, scriptPath, nil)
	if err != nil {
		want.DirectLog("[MRS-MONITOR] skill failed: %v", err)
		mrsSetStatus(want, "failed", err.Error())
		return false, nil
	}

	want.SetCurrent("mrs_raw_output", raw)
	mrsSetStatus(want, "done", "")
	return want.GetStatus() == WantStatusAchieved, nil
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
// Timeout: reads "skill_timeout_seconds" from goal state (default: 120s).
func doMRSAgentFn(ctx context.Context, want *Want) error {
	scriptPath, err := mrsSkillPath(want)
	if err != nil {
		want.DirectLog("[MRS-DO] %v", err)
		mrsSetStatus(want, "failed", err.Error())
		return nil
	}

	args := mrsBuildArgs(want)

	timeoutSec := GetGoal(want, "skill_timeout_seconds", 120)
	skillCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	want.DirectLog("[MRS-DO] executing skill: %s args=%v (timeout: %ds)", scriptPath, args, timeoutSec)
	raw, err := runMRSSkillWithArgs(skillCtx, scriptPath, args)
	if err != nil {
		want.DirectLog("[MRS-DO] skill failed: %v", err)
		mrsSetStatus(want, "failed", err.Error())
		return nil
	}

	want.SetCurrent("mrs_raw_output", raw)
	mrsSetStatus(want, "done", "")
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

// mrsSetStatus writes to "status" and "error" current state fields if declared.
func mrsSetStatus(want *Want, status, errMsg string) {
	if want.StateLabels["status"] == LabelCurrent {
		want.SetCurrent("status", status)
	}
	if errMsg != "" && want.StateLabels["error"] == LabelCurrent {
		want.SetCurrent("error", errMsg)
	}
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
func runMRSSkillWithArgs(ctx context.Context, scriptPath string, args []string) (map[string]any, error) {
	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.CommandContext(ctx, "python3", cmdArgs...)
	cmd.Env = os.Environ()

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exit error: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("skill output is not valid JSON: %w", err)
	}
	return result, nil
}
