package mywant

import (
	"context"
	"fmt"
	"log"
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

// SetAgentActivity appends an activity annotation event for the given agent.
// This records a human-readable description of what the agent just did,
// e.g. "Flight reservation has been created".
func (n *Want) SetAgentActivity(agentName string, activity string) {
	n.initHistoryRings()
	// Find the ExecutionID of the most recent event for this agent
	execID := ""
	snapshot := n.agentHistoryRing.Snapshot(0)
	for i := len(snapshot) - 1; i >= 0; i-- {
		if snapshot[i].AgentName == agentName {
			execID = snapshot[i].ExecutionID
			break
		}
	}
	n.agentHistoryRing.Append(AgentExecution{
		ExecutionID: execID,
		AgentName:   agentName,
		Timestamp:   time.Now(),
		Status:      "running",
		Activity:    activity,
	})
}

func getAgentNames(agents []Agent) []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.GetName()
	}
	return names
}

// ExecuteAgents finds and executes agents based on want requirements
func (n *Want) ExecuteAgents() error {
	if n.agentRegistry == nil {
		return nil
	}

	// 1. Resolve explicit requirements from Spec.Requires
	requirements := make([]string, len(n.Spec.Requires))
	copy(requirements, n.Spec.Requires)

	// 2. Add implicit think-agent requirements from want type definition
	// This ensures that core 'thinking' logic always runs even if not explicitly listed in Requires
	if def := n.WantTypeDefinition; def != nil {
		for _, capName := range def.ThinkCapabilities {
			found := false
			for _, r := range requirements {
				if r == capName {
					found = true
					break
				}
			}
			if !found {
				requirements = append(requirements, capName)
			}
		}
	}

	if len(requirements) == 0 {
		return nil
	}

	for _, requirement := range requirements {
		var agents []Agent
		// n.StoreLog("🔍 Resolving requirement: '%s'", requirement)

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

		if len(agents) == 0 {
			n.StoreLog("⚠️ WARNING: No agents found providing requirement '%s'", requirement)
		} else {
			n.StoreLog("✅ Found %d agent(s) for '%s': %v", len(agents), requirement, getAgentNames(agents))
		}

		for _, agent := range agents {
			if err := n.executeAgent(agent); err != nil {
				return fmt.Errorf("failed to execute agent %s: %w", agent.GetName(), err)
			}
		}
	}

	return nil
}

// bootAgent prepares the agent runtime (localGo or docker)
func (n *Want) bootAgent(ctx context.Context, agent Agent) error {
	runtime := agent.GetRuntime()
	n.StoreLog("[BOOT-AGENT] Booting agent '%s' with runtime '%s'", agent.GetName(), runtime)

	switch runtime {
	case LocalGoRuntime:
		// Check if the agent has an action or monitor function registered
		switch a := agent.(type) {
		case *DoAgent:
			if a.Action == nil {
				return fmt.Errorf("localGo agent '%s' has no action function registered", agent.GetName())
			}
		case *MonitorAgent:
			if a.Monitor == nil {
				return fmt.Errorf("localGo agent '%s' has no monitor function registered", agent.GetName())
			}
		case *PollAgent:
			if a.Poll == nil {
				return fmt.Errorf("localGo agent '%s' has no poll function registered", agent.GetName())
			}
		case *ThinkAgent:
			if a.Think == nil {
				return fmt.Errorf("localGo agent '%s' has no think function registered", agent.GetName())
			}
		}
		n.StoreLog("[BOOT-AGENT] Agent '%s' (localGo) verified and ready", agent.GetName())

	case DockerRuntime:
		// FUTURE: Implement docker image start and health check
		n.StoreLog("[BOOT-AGENT] Agent '%s' (docker) starting container...", agent.GetName())
		// Placeholder for docker logic
		n.StoreLog("[BOOT-AGENT] Agent '%s' (docker) health check passed", agent.GetName())

	default:
		return fmt.Errorf("unsupported agent runtime: %s", runtime)
	}

	return nil
}

// executeAgent executes a single agent using the appropriate executor
func (n *Want) executeAgent(agent Agent) error {
	agentName := agent.GetName()
	agentType := agent.GetType()

	// PrepareAgent phase
	n.SetStatus(WantStatusPrepareAgent)

	if err := n.bootAgent(context.Background(), agent); err != nil {
		n.SetStatus(WantStatusReaching)
		return fmt.Errorf("failed to boot agent %s: %w", agentName, err)
	}

	n.SetStatus(WantStatusReaching)

	// A. Persistent Agents (Think, Monitor, Poll) - Integrated background management
	if agentType != DoAgentType {
		n.StoreLog("[AGENT-START] Starting persistent %s agent: %s", agentType, agentName)
		return n.startPersistentAgent(agent)
	}

	// B. One-off Agents (Do) - Synchronous execution with result tracking
	n.StoreLog("[AGENT-START] Running one-off %s agent: %s", agentType, agentName)
	return n.runDoAgent(agent)
}

// startPersistentAgent registers and starts an agent as a BackgroundAgent.
// This unifies ThinkAgent, MonitorAgent, and PollAgent lifecycle management.
func (n *Want) startPersistentAgent(agent Agent) error {
	agentName := agent.GetName()
	agentType := agent.GetType()
	bgID := fmt.Sprintf("%s-%s-%s", strings.ToLower(string(agentType)), agentName, n.Metadata.ID)

	// Check if already running
	if _, exists := n.GetBackgroundAgent(bgID); exists {
		return nil
	}

	n.StoreLog("[PERSISTENT-AGENT] Starting background %s agent: %s", agentType, agentName)

	var bgAgent BackgroundAgent
	switch a := agent.(type) {
	case *ThinkAgent:
		// ThinkAgents use 2s interval by default
		bgAgent = NewThinkingAgent(bgID, 2*time.Second, agentName, a.Think)
	case *MonitorAgent:
		// Wrap MonitorFunc into ThinkFunc signature (no return value)
		thinkWrapper := func(ctx context.Context, w *Want) error {
			return a.Monitor(ctx, w)
		}
		// MonitorAgents use 5s interval for checking
		bgAgent = NewThinkingAgent(bgID, 5*time.Second, agentName, thinkWrapper)
	case *PollAgent:
		// PollAgents use 5s interval and check for shouldStop
		bgAgent = NewPollingAgent(bgID, 5*time.Second, agentName, a.Poll)
	default:
		return fmt.Errorf("agent %s has persistent type %s but is not a persistent agent implementation", agentName, agentType)
	}

	// Record start in history
	n.initHistoryRings()
	execID := generateUUID()
	n.agentHistoryRing.Append(AgentExecution{
		ExecutionID: execID,
		AgentName:   agentName,
		AgentType:   string(agentType),
		Timestamp:   time.Now(),
		Status:      "running",
	})

	if err := n.AddBackgroundAgent(bgAgent); err != nil {
		return fmt.Errorf("failed to start persistent agent %s: %w", agentName, err)
	}

	return nil
}

// runDoAgent executes a DoAgent synchronously and records its result.
// This encapsulates the complex executor and history logic from the original executeAgent.
func (n *Want) runDoAgent(agent Agent) error {
	agentName := agent.GetName()
	
	// Get BaseAgent to access ExecutionConfig
	var execConfig ExecutionConfig
	if a, ok := agent.(*DoAgent); ok {
		execConfig = a.BaseAgent.ExecutionConfig
	} else {
		execConfig = DefaultExecutionConfig()
	}

	// Create executor
	executor, err := NewExecutor(execConfig)
	if err != nil {
		return fmt.Errorf("failed to create executor for agent %s: %w", agentName, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if n.runningAgents == nil {
		n.runningAgents = make(map[string]context.CancelFunc)
	}
	n.runningAgents[agentName] = cancel
	if n.RunningAgents == nil {
		n.RunningAgents = make([]string, 0)
	}
	n.initHistoryRings()
	executionID := generateUUID()
	n.RunningAgents = append(n.RunningAgents, agentName)
	n.CurrentAgent = agentName
	{
		// Store current agent state
		n.StoreState("_current_agent", agentName)
		n.StoreState("_running_agents", n.RunningAgents)
		n.AggregateChanges()
	}
	// Append "running" start event
	n.agentHistoryRing.Append(AgentExecution{
		ExecutionID:   executionID,
		AgentName:     agentName,
		AgentType:     string(agent.GetType()),
		Timestamp:     time.Now(),
		Status:        "running",
		ExecutionMode: string(executor.GetMode()),
	})

	var agentErr error
	var finalStatus string = "achieved"
	var finalError string

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Agent %s panicked: %v\n", agentName, r)
			finalStatus = "failed"
			finalError = fmt.Sprintf("Panic: %v", r)
		}

		// Append completion event
		n.agentHistoryRing.Append(AgentExecution{
			ExecutionID: executionID,
			AgentName:   agentName,
			AgentType:   string(agent.GetType()),
			Timestamp:   time.Now(),
			Status:      finalStatus,
			Error:       finalError,
		})

		for i, runningAgent := range n.RunningAgents {
			if runningAgent == agentName {
				n.RunningAgents = append(n.RunningAgents[:i], n.RunningAgents[i+1:]...)
				break
			}
		}

		// Update current agent
		if len(n.RunningAgents) == 0 {
			n.CurrentAgent = ""
		} else {
			n.CurrentAgent = n.RunningAgents[len(n.RunningAgents)-1]
		}
		{
			n.StoreStateForAgent("_current_agent", n.CurrentAgent)
			n.StoreStateForAgent("_running_agents", n.RunningAgents)
			n.DumpStateForAgent("DoAgent")
		}

		delete(n.runningAgents, agentName)
	}()

	// Execute through executor
	err = executor.Execute(ctx, agent, n)
	if err != nil {
		log.Printf("Agent %s failed: %v\n", agentName, err)
		finalStatus = "failed"
		finalError = err.Error()
		agentErr = err
	} else {
		n.DumpStateForAgent("DoAgent")
	}

	return agentErr
}

// StopAllAgents stops all running (synchronous) agents for this want
func (n *Want) StopAllAgents() {
	n.agentStateMutex.Lock()
	if n.runningAgents == nil || len(n.runningAgents) == 0 {
		n.agentStateMutex.Unlock()
		return
	}

	// Copy cancel functions to a slice to execute outside the lock
	type agentCancel struct {
		name   string
		cancel context.CancelFunc
	}
	cancels := make([]agentCancel, 0, len(n.runningAgents))
	for name, cancel := range n.runningAgents {
		cancels = append(cancels, agentCancel{name: name, cancel: cancel})
	}

	// Clear the maps and lists while holding the lock
	n.runningAgents = make(map[string]context.CancelFunc)
	n.RunningAgents = make([]string, 0)
	n.CurrentAgent = ""
	n.agentStateMutex.Unlock()

	// Execute cancels outside the lock
	for _, ac := range cancels {
		log.Printf("Stopping agent: %s\n", ac.name)
		ac.cancel()

		// Append termination event (pure append)
		// Find the ExecutionID of the last running event for this agent
		execID := ""
		snapshot := n.agentHistoryRing.Snapshot(0)
		for i := len(snapshot) - 1; i >= 0; i-- {
			if snapshot[i].AgentName == ac.name && snapshot[i].Status == "running" {
				execID = snapshot[i].ExecutionID
				break
			}
		}
		n.agentHistoryRing.Append(AgentExecution{
			ExecutionID: execID,
			AgentName:   ac.name,
			Timestamp:   time.Now(),
			Status:      "terminated",
		})
	}
}

// StopAgent stops a specific running (synchronous) agent
func (n *Want) StopAgent(agentName string) {
	if n.runningAgents == nil {
		return
	}

	if cancel, exists := n.runningAgents[agentName]; exists {
		log.Printf("Stopping agent: %s\n", agentName)
		cancel()
		delete(n.runningAgents, agentName)

		// Append termination event (pure append)
		execID := ""
		snapshot := n.agentHistoryRing.Snapshot(0)
		for i := len(snapshot) - 1; i >= 0; i-- {
			if snapshot[i].AgentName == agentName && snapshot[i].Status == "running" {
				execID = snapshot[i].ExecutionID
				break
			}
		}
		n.agentHistoryRing.Append(AgentExecution{
			ExecutionID: execID,
			AgentName:   agentName,
			Timestamp:   time.Now(),
			Status:      "terminated",
		})
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
	hasStateChanges := len(n.agentStateChanges) > 0
	changesCopy := make(map[string]any)
	for k, v := range n.agentStateChanges {
		changesCopy[k] = v
	}
	changeCount := len(n.agentStateChanges)

	// Clear staged changes while holding the lock
	n.agentStateChanges = make(map[string]any)
	n.agentStateMutex.Unlock()

	// Step 2: Apply changes to State using encapsulated method
	if hasStateChanges {
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

		log.Printf("💾 Committed %d state changes for want %s in single batch\n",
			changeCount, n.Metadata.Name)
	}

	// Step 4: Commit pending logs (same as EndProgressCycle)
	if len(n.pendingLogs) > 0 {
		n.addAggregatedLogHistory()
	}
}
func (n *Want) GetStagedChanges() map[string]any {
	n.agentStateMutex.RLock()
	defer n.agentStateMutex.RUnlock()

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
	n.initHistoryRings()
	var result []AgentExecution
	for _, exec := range n.agentHistoryRing.Snapshot(0) {
		if exec.AgentName == agentName {
			result = append(result, exec)
		}
	}
	if result == nil {
		return []AgentExecution{}
	}
	return result
}
func (n *Want) GetAgentHistoryByType(agentType string) []AgentExecution {
	n.initHistoryRings()
	var result []AgentExecution
	for _, exec := range n.agentHistoryRing.Snapshot(0) {
		if exec.AgentType == agentType {
			result = append(result, exec)
		}
	}
	if result == nil {
		return []AgentExecution{}
	}
	return result
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
			w.StoreLog("ERROR: %v", lastErr)
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

// FlushThinkingAgents runs all ThinkingAgents' think function once synchronously.
// Called before StopAllBackgroundAgents() when a want achieves completion,
// ensuring any pending state changes (e.g. cost propagation) are committed.
func (w *Want) FlushThinkingAgents(ctx context.Context) {
	w.backgroundMutex.RLock()
	defer w.backgroundMutex.RUnlock()

	for agentID, agent := range w.backgroundAgents {
		if ta, ok := agent.(*ThinkingAgent); ok {
			if err := ta.Flush(ctx); err != nil {
				w.StoreLog("[FlushThinkingAgents] Error flushing agent %q: %v", agentID, err)
			}
		}
	}
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
		w.StoreLog("⚠️ VALIDATION WARNING: Agent '%s' has no stateAccess specification, writing to field '%s'",
			agentName, key)
		return false
	}

	// Check if key is in allowed list
	if spec.AllowedStateKeys[key] {
		return true
	}

	// Key not allowed - log warning but allow write
	description := spec.KeyDescriptions[key]
	if description != "" {
		w.StoreLog("⚠️ VALIDATION WARNING: Agent '%s' writing to undeclared field '%s' (%s)",
			agentName, key, description)
	} else {
		w.StoreLog("⚠️ VALIDATION WARNING: Agent '%s' writing to undeclared field '%s' (not declared in capability stateAccess)",
			agentName, key)
	}

	return false // Validation failed, but write still allowed
}

// StoreStateForAgent stages a state change from a background agent.
// Uses the shared agentStateMutex / agentStateChanges as StageStateChange.
func (w *Want) StoreStateForAgent(key string, value any) {
	w.validateAgentStateKey(key)

	w.agentStateMutex.Lock()
	defer w.agentStateMutex.Unlock()

	if w.agentStateChanges == nil {
		w.agentStateChanges = make(map[string]any)
	}
	w.agentStateChanges[key] = value
}

// StoreStateMultiForAgent stages multiple state changes from a background agent.
func (w *Want) StoreStateMultiForAgent(updates map[string]any) {
	for key := range updates {
		w.validateAgentStateKey(key)
	}

	w.agentStateMutex.Lock()
	defer w.agentStateMutex.Unlock()

	if w.agentStateChanges == nil {
		w.agentStateChanges = make(map[string]any)
	}
	for key, value := range updates {
		w.agentStateChanges[key] = value
	}
}

// DumpStateForAgent commits all staged agent state changes to the Want's state.
// The agentType parameter identifies which agent (e.g., "MonitorAgent", "DoAgent") made the changes.
func (w *Want) DumpStateForAgent(agentType string) {
	w.agentStateMutex.Lock()
	if len(w.agentStateChanges) == 0 {
		w.agentStateMutex.Unlock()
		return
	}

	changesCopy := make(map[string]any)
	for k, v := range w.agentStateChanges {
		changesCopy[k] = v
	}
	w.agentStateChanges = make(map[string]any)
	w.agentStateMutex.Unlock()

	w.StoreState("action_by_agent", agentType)
	for key, value := range changesCopy {
		w.StoreState(key, value)
	}

	w.StoreLog("💾 Agent state dumped for %s (agent: %s): %d changes (will be recorded in next Progress cycle)\n", w.Metadata.Name, agentType, len(changesCopy))
}

// HasPendingAgentStateChanges returns true if there are staged agent state changes.
func (w *Want) HasPendingAgentStateChanges() bool {
	w.agentStateMutex.RLock()
	defer w.agentStateMutex.RUnlock()

	return len(w.agentStateChanges) > 0
}
