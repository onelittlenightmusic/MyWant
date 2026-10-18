package mywant

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ansibleRuntime executes Ansible playbooks as inline agent scripts.
//
// State I/O contract:
//   - Input:  MYWANT_GOAL_FILE, MYWANT_CURRENT_FILE, MYWANT_PLAN_FILE, MYWANT_INPUT_FILE
//     point to JSON files (stateFiles); see script_runner.go for where it runs.
//   - Output: playbook writes {"current_updates": {...}} to MYWANT_OUTPUT_FILE.
//     If the file is not written, no state updates are applied.
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
	// Fail early if ansible-playbook is not where the playbook would run.
	if _, err := exec.LookPath("ansible-playbook"); err != nil && MRSRunnerURL() == "" {
		return scriptErr(want, label,
			"ansible-playbook not found in PATH.\n"+
				"  Install Ansible and required collections:\n"+
				"    pip install ansible\n"+
				"    ansible-galaxy collection install community.docker\n"+
				"  Then restart the mywant server.")
	}

	r := inlineRun{
		Want:  want,
		Argv:  []string{"ansible-playbook", "playbook.yaml"},
		Files: map[string][]byte{"playbook.yaml": []byte(playbook)},
		// The playbook writes {"current_updates": {...}} to MYWANT_OUTPUT_FILE.
		FileEnv: map[string]string{"MYWANT_OUTPUT_FILE": "output.json"},
		Env: map[string]string{
			"ANSIBLE_HOST_KEY_CHECKING": "False",
			"ANSIBLE_LOCALHOST_WARNING": "False",
		},
		Outputs: []string{"output.json"},
		Timeout: 30 * time.Minute,
	}
	// Expand launch_env_* keys as real env vars for docker-compose substitution.
	for k, v := range want.GetAllCurrent() {
		if strings.HasPrefix(k, "launch_env_") {
			r.Env[strings.TrimPrefix(k, "launch_env_")] = fmt.Sprint(v)
		}
	}
	if err := r.withStateFiles(); err != nil {
		return scriptErr(want, label, err.Error())
	}

	want.StoreLog("[%s] running ansible-playbook", label)
	res, err := runInline(r)
	if err != nil {
		return scriptErr(want, label, err.Error())
	}
	if res.ExitCode != 0 {
		return scriptErr(want, label, fmt.Sprintf("ansible-playbook failed: exit status %d\nstdout:\n%s\nstderr:\n%s",
			res.ExitCode, res.Stdout, res.Stderr))
	}
	if out := strings.TrimSpace(string(res.Stdout)); out != "" {
		want.StoreLog("[%s] %s", label, out)
	}

	// Apply state updates written by the playbook to MYWANT_OUTPUT_FILE.
	if data, ok := res.Outputs["output.json"]; ok {
		out, perr := parseScriptOutput(data)
		if perr != nil {
			return scriptErr(want, label, fmt.Sprintf("output.json parse error: %v", perr))
		}
		apply(out)
	}
	return nil
}
