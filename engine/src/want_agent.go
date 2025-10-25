package mywant

import (
	"context"
	"fmt"
	"time"
)

// Agent execution methods for Want

// SetAgentRegistry sets the agent registry for this want
func (n *Want) SetAgentRegistry(registry *AgentRegistry) {
	n.agentRegistry = registry
	if n.runningAgents == nil {
		n.runningAgents = make(map[string]context.CancelFunc)
	}
}

// GetAgentRegistry returns the agent registry for this want
func (n *Want) GetAgentRegistry() *AgentRegistry {
	return n.agentRegistry
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

	// Store cancel function for later cleanup
	n.runningAgents[agent.GetName()] = cancel

	// Initialize agent tracking fields if needed
	if n.RunningAgents == nil {
		n.RunningAgents = make([]string, 0)
	}
	if n.History.AgentHistory == nil {
		n.History.AgentHistory = make([]AgentExecution, 0)
	}

	// Add to running agents list
	n.RunningAgents = append(n.RunningAgents, agent.GetName())
	n.CurrentAgent = agent.GetName()

	// Store agent information in state for history tracking (batched to reduce history bloat)
	{
		n.BeginExecCycle()
		n.StoreState("current_agent", agent.GetName())
		n.StoreState("running_agents", n.RunningAgents)
		n.EndExecCycle()
	}

	// Create agent execution record
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

			// Remove from running agents list
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
				// Set to the last running agent
				n.CurrentAgent = n.RunningAgents[len(n.RunningAgents)-1]
			}

			// Update state with current agent and running agents info (batched)
			{
				n.BeginExecCycle()
				n.StoreState("current_agent", n.CurrentAgent)
				n.StoreState("running_agents", n.RunningAgents)
				n.EndExecCycle()
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
			// Remove from running agents when done
			delete(n.runningAgents, agent.GetName())
		}()

		// FRAMEWORK-LEVEL: Wrap agent execution in exec cycle
		// This ensures all agent state changes are batched into a single history entry
		// Individual agents should NOT call BeginExecCycle/EndExecCycle themselves
		n.BeginExecCycle()

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
			// Mark as completed
			for i := range n.History.AgentHistory {
				if n.History.AgentHistory[i].AgentName == agent.GetName() && n.History.AgentHistory[i].Status == "running" {
					n.History.AgentHistory[i].Status = "completed"
					break
				}
			}
			// AgentHistory is now managed separately from state
		}

		// FRAMEWORK-LEVEL: Commit all agent state changes
		n.EndExecCycle()
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

		// Remove from running agents list
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

// StageStateChange stages state changes for later commit (used by agents)
// Can be called with either:
// 1. Single key-value: StageStateChange("key", "value")
// 2. Object with multiple pairs: StageStateChange(map[string]interface{}{"key1": "value1", "key2": "value2"})
func (n *Want) StageStateChange(keyOrObject interface{}, value ...interface{}) error {
	n.agentStateMutex.Lock()
	defer n.agentStateMutex.Unlock()

	if n.agentStateChanges == nil {
		n.agentStateChanges = make(map[string]interface{})
	}

	// Handle object case: StageStateChange(map[string]interface{}{...})
	if len(value) == 0 {
		if stateObject, ok := keyOrObject.(map[string]interface{}); ok {
			for k, v := range stateObject {
				n.agentStateChanges[k] = v
			}
			return nil
		}
		// Invalid usage - no value provided and not a map
		return fmt.Errorf("StageStateChange: when called with single argument, it must be map[string]interface{}")
	}

	// Handle single key-value case: StageStateChange("key", "value")
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
	n.agentStateMutex.Lock()
	defer n.agentStateMutex.Unlock()

	if len(n.agentStateChanges) == 0 {
		return
	}

	// Create a single aggregated state history entry
	if n.State == nil {
		n.State = make(map[string]interface{})
	}

	// Apply all changes to current state
	for key, value := range n.agentStateChanges {
		n.State[key] = value
	}

	// Add single history entry with all changes (one entry instead of multiple)
	historyEntry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: n.agentStateChanges,
		Timestamp:  time.Now(),
	}
	if n.History.StateHistory == nil {
		n.History.StateHistory = make([]StateHistoryEntry, 0)
	}
	n.History.StateHistory = append(n.History.StateHistory, historyEntry)

	fmt.Printf("💾 Committed %d state changes for want %s in single batch\n",
		len(n.agentStateChanges), n.Metadata.Name)

	// Clear staged changes
	n.agentStateChanges = make(map[string]interface{})
}

// GetStagedChanges returns a copy of currently staged changes (for debugging)
func (n *Want) GetStagedChanges() map[string]interface{} {
	n.agentStateMutex.RLock()
	defer n.agentStateMutex.RUnlock()

	if n.agentStateChanges == nil {
		return make(map[string]interface{})
	}

	staged := make(map[string]interface{})
	for k, v := range n.agentStateChanges {
		staged[k] = v
	}
	return staged
}
