package types

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	mywant "mywant/engine/core"
)

// executeCommand performs the actual command execution
func executeCommand(ctx context.Context, want *mywant.Want) error {
	var commandStr, shellStr, workingDirStr string
	var timeoutSec int

	// Read parameters from want state by binding to local variables
	want.GetStateMulti(mywant.Dict{
		"command":           &commandStr,
		"shell":             &shellStr,
		"working_directory": &workingDirStr,
		"timeout":           &timeoutSec,
	})

	if shellStr == "" {
		shellStr = "/bin/bash"
	}
	if timeoutSec == 0 {
		timeoutSec = 30
	}

	want.StoreState("achieving_percentage", 50)
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

	// Build execution result
	result := map[string]any{
		"exit_code":         exitCode,
		"stdout":            stdout.String(),
		"stderr":            stderr.String(),
		"execution_time_ms": executionTimeMs,
		"started_at":        startedAt,
		"completed_at":      completedAt,
		"success":           exitCode == 0,
	}

	// Store result in standard agent_result key
	want.StoreState("agent_result", result)

	if exitCode == 0 {
		want.StoreLog("Command executed successfully in %dms", executionTimeMs)
	} else {
		want.StoreLog("Command failed with exit code %d", exitCode)
	}

	return nil // Return nil even on command failure (agent executed successfully)
}
