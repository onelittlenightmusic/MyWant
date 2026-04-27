package types

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterWithInit(func() {
		mywant.RegisterDoAgent("execution_command", executeCommand)
	})
}

// executeCommand performs the actual command execution and writes results directly to want state.
// When runtime=="ansible" and scriptFile is set, delegates to ansibleRuntime instead of shell.
func executeCommand(ctx context.Context, want *mywant.Want) error {
	runtime := mywant.GetCurrent(want, "runtime", "shell")
	scriptFile := mywant.GetCurrent(want, "scriptFile", "")

	if runtime == "ansible" {
		return executeCommandAnsible(want, scriptFile)
	}

	// Read parameters from want state using generic GetCurrent
	commandStr := mywant.GetCurrent(want, "command", "")
	shellStr := mywant.GetCurrent(want, "shell", "/bin/bash")
	workingDirStr := mywant.GetCurrent(want, "working_directory", "")
	timeoutSec := mywant.GetCurrent(want, "timeout", 30)

	want.SetCurrent("achieving_percentage", 50)
	want.StoreLog("Starting command execution...")

	// Record start time
	startTime := time.Now()
	startedAt := startTime.Format(time.RFC3339)

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(execCtx, shellStr, "-c", commandStr)
	if workingDirStr != "" {
		cmd.Dir = workingDirStr
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()

	// Record execution time
	executionTime := time.Since(startTime)
	executionTimeMs := executionTime.Milliseconds()
	completedAt := time.Now().Format(time.RFC3339)

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			stderr.WriteString(fmt.Sprintf("\nCommand execution error: %v", err))
			want.StoreLog("ERROR executing command: %v", err)
		}
	}

	// Write results directly to want state.
	want.SetCurrent("exit_code", exitCode)
	want.SetCurrent("stdout", stdout.String())
	want.SetCurrent("stderr", stderr.String())
	want.SetCurrent("execution_time_ms", executionTimeMs)
	want.SetCurrent("started_at", startedAt)
	want.SetCurrent("completed_at", completedAt)
	want.SetCurrent("final_result", stdout.String())

	if exitCode == 0 {
		want.SetCurrent("completed", true)
		want.SetCurrent("status", "completed")
		want.SetCurrent("achieving_percentage", 100)
		want.StoreLog("Command executed successfully in %dms", executionTimeMs)
	} else {
		want.SetCurrent("completed", false)
		want.SetCurrent("status", "failed")
		want.SetCurrent("error_message", fmt.Sprintf("Exit code: %d", exitCode))
		want.StoreLog("Command failed with exit code %d", exitCode)
	}

	return nil // Return nil even on command failure (agent executed successfully)
}

// executeCommandAnsible runs an Ansible playbook via ansibleRuntime and maps the
// current_updates output back to the standard execution_result state fields.
// The playbook should write {"current_updates": {"completed": true, "stdout": "...", ...}}
// to the file at $MYWANT_OUTPUT_FILE.
func executeCommandAnsible(want *mywant.Want, scriptFile string) error {
	if scriptFile == "" {
		want.SetCurrent("completed", false)
		want.SetCurrent("status", "failed")
		want.SetCurrent("error_message", "runtime=ansible requires scriptFile parameter")
		return nil
	}

	playbook, err := os.ReadFile(scriptFile)
	if err != nil {
		want.SetCurrent("completed", false)
		want.SetCurrent("status", "failed")
		want.SetCurrent("error_message", fmt.Sprintf("scriptFile read error: %v", err))
		return nil
	}

	want.SetCurrent("achieving_percentage", 50)
	want.StoreLog("Starting Ansible playbook execution: %s", scriptFile)

	runtime := mywant.NewAnsibleRuntime()
	if err := runtime.ExecuteDo(want, string(playbook)); err != nil {
		want.SetCurrent("completed", false)
		want.SetCurrent("status", "failed")
		want.SetCurrent("error_message", err.Error())
		return nil
	}

	// If the playbook did not set completed, default to true (playbook ran without error).
	if mywant.GetCurrent(want, "completed", false) != true {
		want.SetCurrent("completed", true)
	}
	if mywant.GetCurrent(want, "status", "") == "" || mywant.GetCurrent(want, "status", "") == "pending" {
		want.SetCurrent("status", "completed")
	}
	want.SetCurrent("achieving_percentage", 100)
	return nil
}
