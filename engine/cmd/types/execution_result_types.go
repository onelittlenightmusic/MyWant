package types

import (
	"fmt"
	. "mywant/engine/src"
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
	e.StoreLog("[INITIALIZE] %s - Resetting state for fresh execution\n", e.Metadata.Name)
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
		e.StoreLog("ERROR: Unknown phase: %s", locals.Phase)
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

	// Set agent requirement for command execution
	// This tells ExecuteAgents() to find agents that provide "command_execution" capability
	e.Spec.Requires = []string{"command_execution"}

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

	e.StoreLog("Initializing execution of command: %s", locals.Command)

	// Transition to executing phase
	locals.Phase = ExecutionPhaseExecuting
	e.updateLocals(locals)
}

// tryAgentExecution delegates command execution to ExecutionAgent
func (e *ExecutionResultWant) tryAgentExecution() (map[string]any, error) {
	// Store command parameters in state for agent to read
	e.StoreStateMulti(map[string]any{
		"shell":              e.Locals.(*ExecutionResultWantLocals).Shell,
		"timeout":            e.Locals.(*ExecutionResultWantLocals).Timeout,
		"working_directory":  e.Locals.(*ExecutionResultWantLocals).WorkingDirectory,
	})

	// Execute agents via framework
	if err := e.ExecuteAgents(); err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	// Retrieve agent result from state
	if result, exists := e.GetState("agent_result"); exists {
		if resultMap, ok := result.(map[string]any); ok {
			return resultMap, nil
		}
	}

	return nil, fmt.Errorf("no agent result found")
}

// handlePhaseExecuting handles the execution phase
func (e *ExecutionResultWant) handlePhaseExecuting(locals *ExecutionResultWantLocals) {
	// Delegate to ExecutionAgent
	result, err := e.tryAgentExecution()
	if err != nil {
		// Handle agent execution failure
		e.StoreState("status", "failed")
		e.StoreState("error_message", fmt.Sprintf("Agent execution error: %v", err))
		e.StoreLog("ERROR: Agent execution failed: %v", err)
		locals.Phase = ExecutionPhaseFailed
		e.updateLocals(locals)
		return
	}

	// Extract results from agent with type safety
	if result == nil {
		e.StoreState("status", "failed")
		e.StoreState("error_message", "Agent returned nil result")
		e.StoreLog("ERROR: Agent returned nil result")
		locals.Phase = ExecutionPhaseFailed
		e.updateLocals(locals)
		return
	}

	// Safely extract results with type handling
	var exitCode int
	if ec, ok := result["exit_code"].(int); ok {
		exitCode = ec
	} else if ec, ok := result["exit_code"].(float64); ok {
		exitCode = int(ec)
	} else {
		exitCode = -1
	}

	locals.ExitCode = exitCode
	if stdout, ok := result["stdout"].(string); ok {
		locals.Stdout = stdout
	}
	if stderr, ok := result["stderr"].(string); ok {
		locals.Stderr = stderr
	}

	// Handle execution_time_ms as int64 or float64
	if etm, ok := result["execution_time_ms"].(int64); ok {
		locals.ExecutionTimeMs = etm
	} else if etm, ok := result["execution_time_ms"].(float64); ok {
		locals.ExecutionTimeMs = int64(etm)
	}

	// Build final result
	finalResult := e.buildFinalResult(locals)

	// Build state updates batch
	stateUpdates := map[string]any{
		"completed":            true,
		"exit_code":            exitCode,
		"stdout":               locals.Stdout,
		"stderr":               locals.Stderr,
		"final_result":         finalResult,
		"execution_time_ms":    locals.ExecutionTimeMs,
		"started_at":           result["started_at"],
		"completed_at":         result["completed_at"],
		"achieving_percentage": 100,
	}

	// Add status based on exit code
	if exitCode == 0 {
		stateUpdates["status"] = "completed"
		e.StoreLog("Command executed successfully in %dms", locals.ExecutionTimeMs)
		locals.Phase = ExecutionPhaseCompleted
	} else {
		stateUpdates["status"] = "failed"
		stateUpdates["error_message"] = fmt.Sprintf("Exit code: %d", exitCode)
		e.StoreLog("Command failed with exit code %d", exitCode)
		locals.Phase = ExecutionPhaseFailed
	}

	// Store all results in batch
	e.StoreStateMulti(stateUpdates)
	e.updateLocals(locals)
	e.ProvideDone()
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

// RegisterExecutionAgents registers the ExecutionAgent with the agent registry
func RegisterExecutionAgents(registry *AgentRegistry) {
	if registry == nil {
		InfoLog("Warning: No agent registry found, skipping ExecutionAgent registration")
		return
	}

	// Register capability
	registry.RegisterCapability(Capability{
		Name:  "command_execution",
		Gives: []string{"execute_shell_command"},
	})

	// Register agent
	agent := NewExecutionAgent()
	registry.RegisterAgent(agent)

	InfoLog("[AGENT] Registered ExecutionAgent with capability: command_execution")
}
