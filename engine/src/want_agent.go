package mywant

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Agent execution methods for Want
func (n *Want) SetAgentRegistry(registry *AgentRegistry) {
	n.agentRegistry = registry
	if n.runningAgents == nil {
		n.runningAgents = make(map[string]context.CancelFunc)
	}
}
func (n *Want) GetAgentRegistry() *AgentRegistry {
	return n.agentRegistry
}

// Example: "Flight reservation has been created" or "Hotel booking confirmed" Will find the last execution record for this agent and set the activity
// regardless of current status (running, completed, or failed)
func (n *Want) SetAgentActivity(agentName string, activity string) {
	if n.History.AgentHistory == nil {
		return
	}
	for i := len(n.History.AgentHistory) - 1; i >= 0; i-- {
		if n.History.AgentHistory[i].AgentName == agentName {
			n.History.AgentHistory[i].Activity = activity
			break
		}
	}
}

// ExecuteAgents finds and executes agents based on want requirements
func (n *Want) ExecuteAgents() error {
	if n.agentRegistry == nil {
		return nil // No agent registry configured, skip agent execution
	}

	if len(n.Spec.Requires) == 0 {
		return nil // No requirements specified, skip agent execution
	}

	for _, requirement := range n.Spec.Requires {
		var agents []Agent

		// First, try to find agents by the requirement directly (if it's a "gives" value)
		agents = n.agentRegistry.FindAgentsByGives(requirement)

		// If not found, check if requirement is a capability name, then get agents for its "gives" values
		if len(agents) == 0 {
			if cap, exists := n.agentRegistry.GetCapability(requirement); exists {
				for _, givesValue := range cap.Gives {
					foundAgents := n.agentRegistry.FindAgentsByGives(givesValue)
					agents = append(agents, foundAgents...)
				}
			}
		}

		for _, agent := range agents {
			if err := n.executeAgent(agent); err != nil {
				return fmt.Errorf("failed to execute agent %s: %w", agent.GetName(), err)
			}
		}
	}

	return nil
}

// executeAgent executes a single agent in a goroutine
func (n *Want) executeAgent(agent Agent) error {
	ctx, cancel := context.WithCancel(context.Background())
	n.runningAgents[agent.GetName()] = cancel
	if n.RunningAgents == nil {
		n.RunningAgents = make([]string, 0)
	}
	if n.History.AgentHistory == nil {
		n.History.AgentHistory = make([]AgentExecution, 0)
	}
	n.RunningAgents = append(n.RunningAgents, agent.GetName())
	n.CurrentAgent = agent.GetName()
	{
		n.BeginProgressCycle()
		n.StoreState("_current_agent", agent.GetName())
		n.StoreState("_running_agents", n.RunningAgents)
		n.EndProgressCycle()
	}
	agentExec := AgentExecution{
		AgentName: agent.GetName(),
		AgentType: string(agent.GetType()),
		StartTime: time.Now(),
		Status:    "running",
	}
	n.History.AgentHistory = append(n.History.AgentHistory, agentExec)

	// Execute agent - synchronously for DO agents, asynchronously for MONITOR agents
	executeFunc := func() {
		defer func() {
			// Update agent execution record
			for i := range n.History.AgentHistory {
				if n.History.AgentHistory[i].AgentName == agent.GetName() && n.History.AgentHistory[i].EndTime == nil {
					endTime := time.Now()
					n.History.AgentHistory[i].EndTime = &endTime
					break
				}
			}
			for i, runningAgent := range n.RunningAgents {
				if runningAgent == agent.GetName() {
					n.RunningAgents = append(n.RunningAgents[:i], n.RunningAgents[i+1:]...)
					break
				}
			}

			// Update current agent (set to empty if no more agents running)
			if len(n.RunningAgents) == 0 {
				n.CurrentAgent = ""
			} else {
				n.CurrentAgent = n.RunningAgents[len(n.RunningAgents)-1]
			}
			{
				n.StoreStateForAgent("_current_agent", n.CurrentAgent)
				n.StoreStateForAgent("_running_agents", n.RunningAgents)
				n.DumpStateForAgent("DoAgent") // Mark as AgentExecution for history tracking
			}

			if r := recover(); r != nil {
				fmt.Printf("Agent %s panicked: %v\n", agent.GetName(), r)
				// Update agent execution record with panic info
				for i := range n.History.AgentHistory {
					if n.History.AgentHistory[i].AgentName == agent.GetName() && n.History.AgentHistory[i].Status == "running" {
						n.History.AgentHistory[i].Status = "failed"
						n.History.AgentHistory[i].Error = fmt.Sprintf("Panic: %v", r)
						break
					}
				}
				// AgentHistory is now managed separately from state
			}
			delete(n.runningAgents, agent.GetName())
		}()

		// FRAMEWORK-LEVEL: Wrap agent execution in exec cycle This ensures all agent state changes are batched into a single history entry
		// Individual agents should NOT call BeginProgressCycle/EndProgressCycle themselves

		if err := agent.Exec(ctx, n); err != nil {
			fmt.Printf("Agent %s failed: %v\n", agent.GetName(), err)
			// Update agent execution record with error
			for i := range n.History.AgentHistory {
				if n.History.AgentHistory[i].AgentName == agent.GetName() && n.History.AgentHistory[i].Status == "running" {
					n.History.AgentHistory[i].Status = "failed"
					n.History.AgentHistory[i].Error = err.Error()
					break
				}
			}
			// AgentHistory is now managed separately from state
		} else {
			for i := range n.History.AgentHistory {
				if n.History.AgentHistory[i].AgentName == agent.GetName() && n.History.AgentHistory[i].Status == "running" {
					n.History.AgentHistory[i].Status = "achieved"
					break
				}
			}
			// AgentHistory is now managed separately from state
		}

		// FRAMEWORK-LEVEL: Commit all agent state changes
		n.DumpStateForAgent("DoAgent")
	}

	// Execute synchronously for DO agents, asynchronously for MONITOR agents
	if agent.GetType() == DoAgentType {
		// DO agents execute synchronously to return results immediately
		executeFunc()
	} else {
		// MONITOR agents execute asynchronously to run in background
		go executeFunc()
	}

	return nil
}

// StopAllAgents stops all running agents for this want
func (n *Want) StopAllAgents() {
	if n.runningAgents == nil {
		return
	}

	for agentName, cancel := range n.runningAgents {
		fmt.Printf("Stopping agent: %s\n", agentName)
		cancel()

		// Update agent execution records
		for i := range n.History.AgentHistory {
			if n.History.AgentHistory[i].AgentName == agentName && n.History.AgentHistory[i].Status == "running" {
				endTime := time.Now()
				n.History.AgentHistory[i].EndTime = &endTime
				n.History.AgentHistory[i].Status = "terminated"
				break
			}
		}
	}

	// Clear the maps and lists
	n.runningAgents = make(map[string]context.CancelFunc)
	n.RunningAgents = make([]string, 0)
	n.CurrentAgent = ""
}

// StopAgent stops a specific running agent
func (n *Want) StopAgent(agentName string) {
	if n.runningAgents == nil {
		return
	}

	if cancel, exists := n.runningAgents[agentName]; exists {
		fmt.Printf("Stopping agent: %s\n", agentName)
		cancel()
		delete(n.runningAgents, agentName)

		// Update agent execution record
		for i := range n.History.AgentHistory {
			if n.History.AgentHistory[i].AgentName == agentName && n.History.AgentHistory[i].Status == "running" {
				endTime := time.Now()
				n.History.AgentHistory[i].EndTime = &endTime
				n.History.AgentHistory[i].Status = "terminated"
				break
			}
		}
		for i, runningAgent := range n.RunningAgents {
			if runningAgent == agentName {
				n.RunningAgents = append(n.RunningAgents[:i], n.RunningAgents[i+1:]...)
				break
			}
		}

		// Update current agent
		if n.CurrentAgent == agentName {
			if len(n.RunningAgents) == 0 {
				n.CurrentAgent = ""
			} else {
				n.CurrentAgent = n.RunningAgents[len(n.RunningAgents)-1]
			}
		}
	}
}

// StageStateChange stages state changes for later commit (used by agents) Can be called with either:
// 1. Single key-value: StageStateChange("key", "value") 2. Object with multiple pairs: StageStateChange(map[string]any{"key1": "value1", "key2": "value2"})
func (n *Want) StageStateChange(keyOrObject any, value ...any) error {
	n.agentStateMutex.Lock()
	defer n.agentStateMutex.Unlock()

	if n.agentStateChanges == nil {
		n.agentStateChanges = make(map[string]any)
	}
	if len(value) == 0 {
		if stateObject, ok := keyOrObject.(map[string]any); ok {
			for k, v := range stateObject {
				n.agentStateChanges[k] = v
			}
			return nil
		}
		// Invalid usage - no value provided and not a map
		return fmt.Errorf("StageStateChange: when called with single argument, it must be map[string]any")
	}
	if len(value) == 1 {
		if key, ok := keyOrObject.(string); ok {
			n.agentStateChanges[key] = value[0]
			return nil
		}
		// Invalid usage - first arg is not a string
		return fmt.Errorf("StageStateChange: when called with two arguments, first must be string")
	}

	// Invalid usage - too many arguments
	return fmt.Errorf("StageStateChange: accepts either 1 argument (map) or 2 arguments (key, value)")
}

// CommitStateChanges commits all staged state changes in a single atomic operation
func (n *Want) CommitStateChanges() {
	// Step 1: Copy staged changes while holding agentStateMutex
	n.agentStateMutex.Lock()
	if len(n.agentStateChanges) == 0 {
		n.agentStateMutex.Unlock()
		return
	}
	changesCopy := make(map[string]any)
	for k, v := range n.agentStateChanges {
		changesCopy[k] = v
	}
	changeCount := len(n.agentStateChanges)

	// Clear staged changes while holding the lock
	n.agentStateChanges = make(map[string]any)
	n.agentStateMutex.Unlock()

	// Step 2: Apply changes to State using encapsulated method
	n.SetStateAtomic(changesCopy)

	// Step 3: Add single history entry with all changes with stateMutex protection
	historyEntry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: changesCopy,
		Timestamp:  time.Now(),
	}

	n.stateMutex.Lock()
	if n.History.StateHistory == nil {
		n.History.StateHistory = make([]StateHistoryEntry, 0)
	}
	n.History.StateHistory = append(n.History.StateHistory, historyEntry)
	n.stateMutex.Unlock()

	fmt.Printf("💾 Committed %d state changes for want %s in single batch\n",
		changeCount, n.Metadata.Name)
}
func (n *Want) GetStagedChanges() map[string]any {
	n.agentStateMutex.RLock()
	defer n.agentStateMutex.Unlock()

	if n.agentStateChanges == nil {
		return make(map[string]any)
	}

	staged := make(map[string]any)
	for k, v := range n.agentStateChanges {
		staged[k] = v
	}
	return staged
}
func (n *Want) GetAgentHistoryByName(agentName string) []AgentExecution {
	if n.History.AgentHistory == nil {
		return []AgentExecution{}
	}

	var result []AgentExecution
	for _, exec := range n.History.AgentHistory {
		if exec.AgentName == agentName {
			result = append(result, exec)
		}
	}
	return result
}
func (n *Want) GetAgentHistoryByType(agentType string) []AgentExecution {
	if n.History.AgentHistory == nil {
		return []AgentExecution{}
	}

	var result []AgentExecution
	for _, exec := range n.History.AgentHistory {
		if exec.AgentType == agentType {
			result = append(result, exec)
		}
	}
	return result
}
func (n *Want) GetAgentHistoryGroupedByName() map[string][]AgentExecution {
	if n.History.AgentHistory == nil {
		return make(map[string][]AgentExecution)
	}

	grouped := make(map[string][]AgentExecution)
	for _, exec := range n.History.AgentHistory {
		grouped[exec.AgentName] = append(grouped[exec.AgentName], exec)
	}
	return grouped
}
func (n *Want) GetAgentHistoryGroupedByType() map[string][]AgentExecution {
	if n.History.AgentHistory == nil {
		return make(map[string][]AgentExecution)
	}

	grouped := make(map[string][]AgentExecution)
	for _, exec := range n.History.AgentHistory {
		grouped[exec.AgentType] = append(grouped[exec.AgentType], exec)
	}
	return grouped
}

// ============================================================================
// Background Agent Management
// ============================================================================

// AddBackgroundAgent registers and starts a background agent
// The agent's Start() method is called immediately
func (w *Want) AddBackgroundAgent(agent BackgroundAgent) error {
	if agent == nil {
		return fmt.Errorf("background agent cannot be nil")
	}

	w.backgroundMutex.Lock()
	defer w.backgroundMutex.Unlock()

	// Initialize background agents map if needed
	if w.backgroundAgents == nil {
		w.backgroundAgents = make(map[string]BackgroundAgent)
	}

	agentID := agent.ID()

	// Check if agent already exists
	if _, exists := w.backgroundAgents[agentID]; exists {
		return fmt.Errorf("background agent with ID %q already exists", agentID)
	}

	// Start the agent with background context
	if err := agent.Start(context.Background(), w); err != nil {
		return fmt.Errorf("failed to start background agent %q: %w", agentID, err)
	}

	// Store the agent
	w.backgroundAgents[agentID] = agent

	return nil
}

// DeleteBackgroundAgent stops and removes a background agent by ID
func (w *Want) DeleteBackgroundAgent(agentID string) error {
	w.backgroundMutex.Lock()
	defer w.backgroundMutex.Unlock()

	agent, exists := w.backgroundAgents[agentID]
	if !exists {
		return fmt.Errorf("background agent with ID %q not found", agentID)
	}

	// Stop the agent
	if err := agent.Stop(); err != nil {
		return fmt.Errorf("failed to stop background agent %q: %w", agentID, err)
	}

	// Remove from map
	delete(w.backgroundAgents, agentID)

	return nil
}

// StopAllBackgroundAgents stops all running background agents
// Called automatically when a want completes
func (w *Want) StopAllBackgroundAgents() error {
	w.backgroundMutex.Lock()
	defer w.backgroundMutex.Unlock()

	if w.backgroundAgents == nil || len(w.backgroundAgents) == 0 {
		return nil
	}

	var lastErr error
	for agentID, agent := range w.backgroundAgents {
		if err := agent.Stop(); err != nil {
			lastErr = fmt.Errorf("failed to stop background agent %q: %w", agentID, err)
			w.StoreLog(fmt.Sprintf("ERROR: %v", lastErr))
		}
	}

	// Clear all agents
	w.backgroundAgents = make(map[string]BackgroundAgent)

	return lastErr
}

// GetBackgroundAgent returns a specific background agent by ID
func (w *Want) GetBackgroundAgent(agentID string) (BackgroundAgent, bool) {
	w.backgroundMutex.RLock()
	defer w.backgroundMutex.RUnlock()

	agent, exists := w.backgroundAgents[agentID]
	return agent, exists
}

// GetBackgroundAgentCount returns the number of active background agents
func (w *Want) GetBackgroundAgentCount() int {
	w.backgroundMutex.RLock()
	defer w.backgroundMutex.RUnlock()

	return len(w.backgroundAgents)
}

// validateAgentStateKey validates a state key against the agent's specification
// Returns true if validation passes, false otherwise
// Logs warnings for invalid keys but does not reject them (backward compatibility)
func (w *Want) validateAgentStateKey(key string) bool {
	// Skip validation for internal framework fields (underscore prefix)
	if strings.HasPrefix(key, "_") {
		return true
	}

	// Get current agent name
	agentName := w.CurrentAgent
	if agentName == "" {
		// No agent context - allow write (backward compatibility)
		return true
	}

	// Check if agent registry is available
	if w.agentRegistry == nil {
		// No registry - allow write (backward compatibility)
		return true
	}

	// Get agent specification
	spec, exists := w.agentRegistry.GetAgentSpec(agentName)
	if !exists || spec == nil {
		// STRICT MODE: Agent has no specification - warn on all writes
		w.StoreLog(fmt.Sprintf("⚠️ VALIDATION WARNING: Agent '%s' has no tracked_status_fields specification, writing to field '%s'",
			agentName, key))
		return false
	}

	// Check if key is in allowed list
	if spec.AllowedStateKeys[key] {
		return true
	}

	// Key not allowed - log warning but allow write
	description := spec.KeyDescriptions[key]
	if description != "" {
		w.StoreLog(fmt.Sprintf("⚠️ VALIDATION WARNING: Agent '%s' writing to undeclared field '%s' (%s)",
			agentName, key, description))
	} else {
		w.StoreLog(fmt.Sprintf("⚠️ VALIDATION WARNING: Agent '%s' writing to undeclared field '%s' (not in tracked_status_fields)",
			agentName, key))
	}

	return false // Validation failed, but write still allowed
}

// StoreStateForAgent stores state changes from background agents in a separate queue
// These changes are tracked independently from the Want's Progress cycle
func (w *Want) StoreStateForAgent(key string, value any) {
	// Validate key against agent specification
	w.validateAgentStateKey(key)

	w.agentStateChangesMutex.Lock()
	defer w.agentStateChangesMutex.Unlock()

	if w.pendingAgentStateChanges == nil {
		w.pendingAgentStateChanges = make(map[string]any)
	}
	w.pendingAgentStateChanges[key] = value
}

// StoreStateMultiForAgent stores multiple state changes from background agents in a separate queue
// This is a convenience method for storing multiple key-value pairs at once
func (w *Want) StoreStateMultiForAgent(updates map[string]any) {
	// Validate all keys against agent specification
	for key := range updates {
		w.validateAgentStateKey(key)
	}

	w.agentStateChangesMutex.Lock()
	defer w.agentStateChangesMutex.Unlock()

	if w.pendingAgentStateChanges == nil {
		w.pendingAgentStateChanges = make(map[string]any)
	}
	for key, value := range updates {
		w.pendingAgentStateChanges[key] = value
	}
}

// DumpStateForAgent commits pending agent state changes to the Want's state
// Agent state changes are applied to State and will be recorded in StateHistory
// by the next Progress cycle's addAggregatedStateHistory() call
// This consolidates all state history recording through addAggregatedStateHistory()
// DumpStateForAgent applies pending agent state changes and stores the agent type
// The agentType parameter identifies which agent (e.g., "MonitorAgent", "DoAgent") made the changes
func (w *Want) DumpStateForAgent(agentType string) {
	w.agentStateChangesMutex.Lock()
	if len(w.pendingAgentStateChanges) == 0 {
		w.agentStateChangesMutex.Unlock()
		return
	}

	// Copy pending changes
	changesCopy := make(map[string]any)
	for k, v := range w.pendingAgentStateChanges {
		changesCopy[k] = v
	}

	// Clear pending changes
	w.pendingAgentStateChanges = make(map[string]any)
	w.agentStateChangesMutex.Unlock()

	// Store the agent type for use in StateHistory recording
	w.stateMutex.Lock()
	if w.State == nil {
		w.State = make(map[string]any)
	}
	// Store agent type for tracking in history
	w.State["action_by_agent"] = agentType
	for key, value := range changesCopy {
		w.State[key] = value
	}
	w.stateMutex.Unlock()

	fmt.Printf("💾 Agent state dumped for %s (agent: %s): %d changes (will be recorded in next Progress cycle)\n", w.Metadata.Name, agentType, len(changesCopy))
}

// HasPendingAgentStateChanges returns true if there are pending agent state changes to dump
func (w *Want) HasPendingAgentStateChanges() bool {
	w.agentStateChangesMutex.RLock()
	defer w.agentStateChangesMutex.RUnlock()

	return len(w.pendingAgentStateChanges) > 0
}
