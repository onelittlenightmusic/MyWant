package types

import (
	"bytes"
	"context"
	"fmt"
	. "mywant/engine/src"
	"os/exec"
	"strings"
	"time"
)

// ExecutionResultWantLocals holds type-specific local state for ExecutionResultWant
type ExecutionResultWantLocals struct {
	Command          string
	Timeout          int // seconds
	WorkingDirectory string
	Shell            string
	Phase            string
	StartTime        time.Time
	ExecutionTimeMs  int64
	ExitCode         int
	Stdout           string
	Stderr           string
}

// ExecutionResult represents command execution result
type ExecutionResult struct {
	Command         string `json:"command"`
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExecutionTimeMs int64  `json:"execution_time_ms"`
	StartedAt       string `json:"started_at"`
	CompletedAt     string `json:"completed_at"`
}

// Phase constants
const (
	ExecutionPhaseInitial   = "initial"
	ExecutionPhaseExecuting = "executing"
	ExecutionPhaseCompleted = "completed"
	ExecutionPhaseFailed    = "failed"
)

// ExecutionResultWant represents a want that executes shell commands
type ExecutionResultWant struct {
	Want
}

// NewExecutionResultWant creates a new ExecutionResultWant
func NewExecutionResultWant(want *Want) *ExecutionResultWant {
	return &ExecutionResultWant{Want: *want}
}

// Initialize resets execution state before starting
func (e *ExecutionResultWant) Initialize() {
	InfoLog("[INITIALIZE] %s - Resetting state for fresh execution\n", e.Metadata.Name)
	// Reset completion state for fresh execution using batch update
	e.StoreStateMulti(map[string]any{
		"completed":            false,
		"status":               "pending",
		"stdout":               "",
		"stderr":               "",
		"error_message":        "",
		"exit_code":            0,
		"final_result":         "",
		"started_at":           "",
		"completed_at":         "",
		"execution_time_ms":    0,
		"achieving_percentage": 0,
		"_phase":               string(ExecutionPhaseInitial),
	})

	// Also reset the in-memory Locals struct to ensure Progress() loop starts fresh
	e.Locals = &ExecutionResultWantLocals{
		Phase:   ExecutionPhaseInitial,
		Timeout: 30,
		Shell:   "/bin/bash",
	}
}

// IsAchieved checks if execution is completed
func (e *ExecutionResultWant) IsAchieved() bool {
	completed, _ := e.GetStateBool("completed", false)
	return completed
}

// Progress implements Progressable for ExecutionResultWant
func (e *ExecutionResultWant) Progress() {
	// Get or initialize locals
	locals := e.getOrInitializeLocals()

	switch locals.Phase {
	case ExecutionPhaseInitial:
		e.handlePhaseInitial(locals)

	case ExecutionPhaseExecuting:
		e.handlePhaseExecuting(locals)

	case ExecutionPhaseCompleted:
		e.handlePhaseCompleted(locals)

	case ExecutionPhaseFailed:
		e.handlePhaseFailed(locals)

	default:
		e.StoreLog(fmt.Sprintf("ERROR: Unknown phase: %s", locals.Phase))
		locals.Phase = ExecutionPhaseFailed
		e.updateLocals(locals)
	}
}

// handlePhaseInitial handles the initial phase
func (e *ExecutionResultWant) handlePhaseInitial(locals *ExecutionResultWantLocals) {
	// Validate command parameter
	command, ok := e.Spec.Params["command"]
	if !ok || command == "" {
		e.StoreLog("ERROR: Missing required parameter 'command'")
		e.StoreState("status", "failed")
		e.StoreState("error_message", "Missing required parameter 'command'")
		e.StoreState("completed", true)
		locals.Phase = ExecutionPhaseFailed
		e.updateLocals(locals)
		return
	}

	locals.Command = fmt.Sprintf("%v", command)

	// Get optional parameters
	if timeout, ok := e.Spec.Params["timeout"]; ok {
		if timeoutVal, ok := timeout.(float64); ok {
			locals.Timeout = int(timeoutVal)
		}
	}
	if locals.Timeout == 0 {
		locals.Timeout = 30 // default 30 seconds
	}

	if wd, ok := e.Spec.Params["working_directory"]; ok {
		locals.WorkingDirectory = fmt.Sprintf("%v", wd)
	}

	if shell, ok := e.Spec.Params["shell"]; ok {
		locals.Shell = fmt.Sprintf("%v", shell)
	}
	if locals.Shell == "" {
		locals.Shell = "/bin/bash" // default shell
	}

	// Initialize state
	e.StoreStateMulti(map[string]any{
		"status":               "pending",
		"command":              locals.Command,
		"completed":            false,
		"achieving_percentage": 0,
	})

	e.StoreLog(fmt.Sprintf("Initializing execution of command: %s", locals.Command))

	// Transition to executing phase
	locals.Phase = ExecutionPhaseExecuting
	e.updateLocals(locals)
}

// handlePhaseExecuting handles the execution phase
func (e *ExecutionResultWant) handlePhaseExecuting(locals *ExecutionResultWantLocals) {
	e.StoreLog("Starting command execution...")
	e.StoreState("achieving_percentage", 50)

	// Record start time
	locals.StartTime = time.Now()
	startedAt := locals.StartTime.Format(time.RFC3339)

	// Execute command with timeout
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(locals.Timeout)*time.Second,
	)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, locals.Shell, "-c", locals.Command)

	// Set working directory if specified
	if locals.WorkingDirectory != "" {
		cmd.Dir = locals.WorkingDirectory
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()

	// Record execution time
	executionTime := time.Since(locals.StartTime)
	locals.ExecutionTimeMs = executionTime.Milliseconds()
	completedAt := time.Now().Format(time.RFC3339)

	// Capture output
	locals.Stdout = stdout.String()
	locals.Stderr = stderr.String()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Command failed to start or timeout
			exitCode = -1
			locals.Stderr = fmt.Sprintf("Command execution error: %v", err)
			e.StoreLog(fmt.Sprintf("ERROR executing command: %v", err))
		}
	}

	locals.ExitCode = exitCode

	// Build final result
	finalResult := e.buildFinalResult(locals)

	// Build state updates batch with all execution results
	stateUpdates := map[string]any{
		"completed":         true,
		"exit_code":         exitCode,
		"stdout":            locals.Stdout,
		"stderr":            locals.Stderr,
		"final_result":      finalResult,
		"execution_time_ms": locals.ExecutionTimeMs,
		"started_at":        startedAt,
		"completed_at":      completedAt,
		"achieving_percentage": 100,
	}

	// Add status based on exit code
	if exitCode == 0 {
		stateUpdates["status"] = "completed"
		e.StoreLog(fmt.Sprintf("Command executed successfully in %dms", locals.ExecutionTimeMs))
		locals.Phase = ExecutionPhaseCompleted
	} else {
		stateUpdates["status"] = "failed"
		stateUpdates["error_message"] = fmt.Sprintf("Exit code: %d", exitCode)
		e.StoreLog(fmt.Sprintf("Command failed with exit code %d", exitCode))
		locals.Phase = ExecutionPhaseFailed
	}

	// Store all results in batch
	e.StoreStateMulti(stateUpdates)
	e.updateLocals(locals)
}

// handlePhaseCompleted handles the completed phase
func (e *ExecutionResultWant) handlePhaseCompleted(locals *ExecutionResultWantLocals) {
	// Already completed, nothing more to do
	e.StoreLog("Execution completed")
}

// handlePhaseFailed handles the failed phase
func (e *ExecutionResultWant) handlePhaseFailed(locals *ExecutionResultWantLocals) {
	// Already failed, nothing more to do
	e.StoreLog("Execution failed")
}

// buildFinalResult combines stdout and stderr into final result
func (e *ExecutionResultWant) buildFinalResult(locals *ExecutionResultWantLocals) string {
	var result strings.Builder

	if locals.Stdout != "" {
		result.WriteString(locals.Stdout)
	}

	if locals.Stderr != "" {
		if result.Len() > 0 && !strings.HasSuffix(result.String(), "\n") {
			result.WriteString("\n")
		}
		result.WriteString(locals.Stderr)
	}

	return result.String()
}

// getOrInitializeLocals gets or initializes locals for this want
func (e *ExecutionResultWant) getOrInitializeLocals() *ExecutionResultWantLocals {
	if e.Locals == nil {
		e.Locals = &ExecutionResultWantLocals{
			Phase:   ExecutionPhaseInitial,
			Timeout: 30,
			Shell:   "/bin/bash",
		}
		return e.Locals.(*ExecutionResultWantLocals)
	}

	locals, ok := e.Locals.(*ExecutionResultWantLocals)
	if !ok {
		e.StoreLog("ERROR: Locals is not ExecutionResultWantLocals")
		return &ExecutionResultWantLocals{
			Phase:   ExecutionPhaseInitial,
			Timeout: 30,
			Shell:   "/bin/bash",
		}
	}

	return locals
}

// updateLocals updates the locals
func (e *ExecutionResultWant) updateLocals(locals *ExecutionResultWantLocals) {
	e.Locals = locals
}

// RegisterExecutionResultWantType registers the execution_result want type with the chain builder
func RegisterExecutionResultWantType(builder *ChainBuilder) {
	builder.RegisterWantType("execution_result", func(metadata Metadata, spec WantSpec) Progressable {
		want := &Want{
			Metadata: metadata,
			Spec:     spec,
		}
		return NewExecutionResultWant(want)
	})
}
