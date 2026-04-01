package mywant

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ansibleRuntime executes Ansible playbooks as inline agent scripts.
//
// State I/O contract:
//   - Input:  MYWANT_GOAL_FILE, MYWANT_CURRENT_FILE, MYWANT_PLAN_FILE, MYWANT_INPUT_FILE
//             point to JSON files written by writeStateFiles.
//   - Output: playbook writes {"current_updates": {...}} to MYWANT_OUTPUT_FILE.
//             If the file is not written, no state updates are applied.
//
// launch_env_* current-state keys are automatically expanded to real environment
// variables (e.g. launch_env_OTP_DATA_DIR → OTP_DATA_DIR=value) so that
// docker-compose ${VAR} substitutions work without extra playbook boilerplate.
type ansibleRuntime struct{}

func (a *ansibleRuntime) ExecuteThink(want *Want, script string) error {
	return a.run(want, script, "ANSIBLE-THINK", func(out *scriptOutput) {
		applyScriptOutput(want, out, "ANSIBLE-THINK")
	})
}

func (a *ansibleRuntime) ExecuteDo(want *Want, script string) error {
	return a.run(want, script, "ANSIBLE-DO", func(out *scriptOutput) {
		applyScriptOutput(want, out, "ANSIBLE-DO")
	})
}

func (a *ansibleRuntime) ExecuteMonitor(want *Want, script string) (bool, error) {
	var shouldStop bool
	err := a.run(want, script, "ANSIBLE-MONITOR", func(out *scriptOutput) {
		applyScriptOutput(want, out, "ANSIBLE-MONITOR")
		shouldStop = out.ShouldStop
	})
	return shouldStop, err
}

func (a *ansibleRuntime) run(want *Want, playbook, label string, apply func(*scriptOutput)) error {
	// Fail early if ansible-playbook is not available.
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		return scriptErr(want, label,
			"ansible-playbook not found in PATH.\n"+
				"  Install Ansible and required collections:\n"+
				"    pip install ansible\n"+
				"    ansible-galaxy collection install community.docker\n"+
				"  Then restart the mywant server.")
	}

	tmpDir, err := os.MkdirTemp("", "ansible-runtime-*")
	if err != nil {
		return scriptErr(want, label, fmt.Sprintf("failed to create temp dir: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	goalPath, currentPath, planPath, inputPath, err := writeStateFiles(want, tmpDir)
	if err != nil {
		return scriptErr(want, label, err.Error())
	}

	// Write the playbook YAML to a temp file.
	playbookPath := filepath.Join(tmpDir, "playbook.yaml")
	if err := os.WriteFile(playbookPath, []byte(playbook), 0600); err != nil {
		return scriptErr(want, label, fmt.Sprintf("failed to write playbook: %v", err))
	}

	// Playbook writes {"current_updates": {...}} here.
	outputPath := filepath.Join(tmpDir, "output.json")

	env := append(os.Environ(), stateEnv(goalPath, currentPath, planPath, inputPath)...)
	env = append(env,
		"MYWANT_OUTPUT_FILE="+outputPath,
		"ANSIBLE_HOST_KEY_CHECKING=False",
		"ANSIBLE_LOCALHOST_WARNING=False",
	)
	// Expand launch_env_* keys as real env vars for docker-compose substitution.
	for k, v := range want.GetAllCurrent() {
		if strings.HasPrefix(k, "launch_env_") {
			varName := strings.TrimPrefix(k, "launch_env_")
			env = append(env, fmt.Sprintf("%s=%v", varName, v))
		}
	}

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "ansible-playbook", playbookPath)
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	want.DirectLog("[%s] running ansible-playbook", label)
	if err := cmd.Run(); err != nil {
		return scriptErr(want, label, fmt.Sprintf("ansible-playbook failed: %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout.String(), stderr.String()))
	}
	if out := strings.TrimSpace(stdout.String()); out != "" {
		want.DirectLog("[%s] %s", label, out)
	}

	// Apply state updates written by the playbook to MYWANT_OUTPUT_FILE.
	if data, rerr := os.ReadFile(outputPath); rerr == nil {
		out, perr := parseScriptOutput(data)
		if perr != nil {
			return scriptErr(want, label, fmt.Sprintf("output.json parse error: %v", perr))
		}
		apply(out)
	}
	return nil
}
