package types

import (
	"fmt"
	. "mywant/engine/core"
	"time"
)

func init() {
	RegisterWantImplementation[ExecutionResultWant, ExecutionResultWantLocals]("command")
	RegisterDoAgent("execution_command", executeCommand)
}

// ExecutionResultWantLocals holds type-specific local state for ExecutionResultWant
type ExecutionResultWantLocals struct {
	Command          string
	Timeout          int // seconds
	WorkingDirectory string
	Shell            string
	Phase            string `mywant:"internal,_phase"`
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

func (e *ExecutionResultWant) GetLocals() *ExecutionResultWantLocals {
	return CheckLocalsInitialized[ExecutionResultWantLocals](&e.Want)
}

// Initialize resets execution state before starting
func (e *ExecutionResultWant) Initialize() {
	// Reset completion state for fresh execution using batch update
	e.SetCurrent("completed", false)
	e.SetCurrent("status", "pending")
	e.SetCurrent("stdout", "")
	e.SetCurrent("stderr", "")
	e.SetCurrent("error_message", "")
	e.SetCurrent("exit_code", 0)
	e.SetCurrent("started_at", "")
	e.SetCurrent("completed_at", "")
	e.SetCurrent("execution_time_ms", 0)
	e.SetCurrent("achieving_percentage", 0)

	// Get locals (guaranteed to be initialized by framework)
	locals := e.GetLocals()
	locals.Phase = ExecutionPhaseInitial
	locals.Timeout = 30
	locals.Shell = "/bin/bash"
}

// IsAchieved checks if execution is completed
func (e *ExecutionResultWant) IsAchieved() bool {
	return GetCurrent(e, "completed", false)
}

// Progress implements Progressable for ExecutionResultWant
func (e *ExecutionResultWant) Progress() {
	// Get locals (guaranteed to be initialized by framework)
	locals := e.GetLocals()

	switch locals.Phase {
	case ExecutionPhaseInitial:
		e.handlePhaseInitial(locals)

	case ExecutionPhaseExecuting:
		e.handlePhaseExecuting(locals)

	case ExecutionPhaseCompleted:
		// Already completed, nothing more to do

	case ExecutionPhaseFailed:
		// Already failed, nothing more to do

	default:
		e.SetModuleError("Phase", fmt.Sprintf("Unknown phase: %s", locals.Phase))
	}
}

// handlePhaseInitial handles the initial phase
func (e *ExecutionResultWant) handlePhaseInitial(locals *ExecutionResultWantLocals) {
	// Validate command parameter using ConfigError pattern
	command, ok := e.Spec.Params["command"]
	if !ok || command == "" {
		e.SetConfigError("command", "Missing required parameter 'command'")
		locals.Phase = ExecutionPhaseFailed
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
	e.SetCurrent("status", "pending")
	e.SetCurrent("command", locals.Command)
	e.SetCurrent("completed", false)
	e.SetCurrent("achieving_percentage", 0)

	// Transition to executing phase
	locals.Phase = ExecutionPhaseExecuting
}

// tryAgentExecution delegates command execution to ExecutionAgent
func (e *ExecutionResultWant) tryAgentExecution() (map[string]any, error) {
	locals := e.GetLocals()
	// Store command parameters in state for agent to read
	e.SetCurrent("shell", locals.Shell)
	e.SetCurrent("timeout", locals.Timeout)
	e.SetCurrent("working_directory", locals.WorkingDirectory)

	// Execute agents via framework
	if err := e.ExecuteAgents(); err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	// Retrieve agent result from state
	result := GetCurrent(e, "agent_result", map[string]any{})
	if len(result) > 0 {
		return result, nil
	}

	return nil, fmt.Errorf("no agent result found")
}

// handlePhaseExecuting handles the execution phase
func (e *ExecutionResultWant) handlePhaseExecuting(locals *ExecutionResultWantLocals) {
	// Delegate to ExecutionAgent
	result, err := e.tryAgentExecution()
	if err != nil {
		// Handle agent execution failure
		e.SetCurrent("status", "failed")
		e.SetCurrent("error_message", fmt.Sprintf("Agent execution error: %v", err))
		e.StoreLog("ERROR: Agent execution failed: %v", err)
		locals.Phase = ExecutionPhaseFailed
		return
	}

	// Extract results from agent with type safety
	if result == nil {
		e.SetCurrent("status", "failed")
		e.SetCurrent("error_message", "Agent returned nil result")
		e.StoreLog("ERROR: Agent returned nil result")
		locals.Phase = ExecutionPhaseFailed
		return
	}

	// Safely extract results with type handling
	exitCode := ToInt(result["exit_code"], -1)

	locals.ExitCode = exitCode
	locals.Stdout = ToString(result["stdout"], "")
	locals.Stderr = ToString(result["stderr"], "")

	// Handle execution_time_ms
	locals.ExecutionTimeMs = int64(ToFloat64(result["execution_time_ms"], 0))

	// Update state
	e.SetCurrent("exit_code", exitCode)
	e.SetCurrent("stdout", locals.Stdout)
	e.SetCurrent("stderr", locals.Stderr)
	e.SetCurrent("execution_time_ms", locals.ExecutionTimeMs)
	e.SetCurrent("started_at", result["started_at"])
	e.SetCurrent("completed_at", result["completed_at"])

	// Set status based on exit code
	if exitCode == 0 {
		e.SetCurrent("completed", true)
		e.SetCurrent("status", "completed")
		e.SetCurrent("achieving_percentage", 100)
		e.StoreLog("Command executed successfully in %dms", locals.ExecutionTimeMs)
		locals.Phase = ExecutionPhaseCompleted
	} else {
		e.SetCurrent("completed", false)
		e.SetCurrent("status", "failed")
		e.SetCurrent("error_message", fmt.Sprintf("Exit code: %d", exitCode))
		e.StoreLog("Command failed with exit code %d", exitCode)
		locals.Phase = ExecutionPhaseFailed
		e.SetStatus(WantStatusFailed)
	}

	e.ProvideDone()
}
