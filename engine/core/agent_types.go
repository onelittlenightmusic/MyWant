package mywant

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AgentType defines the type of agent for execution strategies.
type AgentType string

const (
	DoAgentType      AgentType = "do"
	MonitorAgentType AgentType = "monitor"
	ThinkAgentType   AgentType = "think"
)

// ThinkFunc defines the signature for think logic executed by a ThinkingAgent.
// Unlike PollFunc, it does not return shouldStop - the agent runs indefinitely.
type ThinkFunc func(ctx context.Context, want *Want) error

// AgentRuntime defines the runtime environment for the agent.
type AgentRuntime string

const (
	LocalGoRuntime AgentRuntime = "localGo"
	DockerRuntime  AgentRuntime = "docker"
)

// AccessType defines how an agent can access a state field.
type AccessType string

const (
	AccessTypeUpdate     AccessType = "update"     // Agent may write to this field
	AccessTypeRead       AccessType = "read"        // Agent may read from this field
	AccessTypeReadUpdate AccessType = "readUpdate"  // Agent may read and write (default)
)

// StateAccessField declares a state field that an agent with this capability can access.
type StateAccessField struct {
	Name        string     `yaml:"name"`
	Type        string     `yaml:"type"`
	Description string     `yaml:"description"`
	AccessType  AccessType `yaml:"accessType,omitempty"` // default: readUpdate
}

// Capability represents an agent's functional capability with its dependencies.
type Capability struct {
	Name              string             `yaml:"name"`
	Gives             []string           `yaml:"gives"`
	Description       string             `yaml:"description,omitempty"`
	StateAccess       []StateAccessField `yaml:"stateAccess,omitempty"`
	ParentStateAccess []StateAccessField `yaml:"parentStateAccess,omitempty"`
}

// Agent defines the interface for all agent implementations.
type Agent interface {
	Exec(ctx context.Context, want *Want) (shouldStop bool, err error)
	GetCapabilities() []string
	GetName() string
	GetType() AgentType
	GetRuntime() AgentRuntime
}

// BaseAgent provides common functionality for all agent types.
type BaseAgent struct {
	Name         string       `yaml:"name"`
	Capabilities []string     `yaml:"capabilities"`
	Type         AgentType    `yaml:"type"`
	Runtime      AgentRuntime `yaml:"runtime"`

	// Execution configuration (local, webhook, rpc)
	ExecutionConfig ExecutionConfig `yaml:"execution" json:"execution"`
}

// NewBaseAgent creates a new BaseAgent with the given parameters. This is the canonical constructor for creating agents.
func NewBaseAgent(name string, capabilities []string, agentType AgentType) *BaseAgent {
	return &BaseAgent{
		Name:            name,
		Capabilities:    capabilities,
		Type:            agentType,
		Runtime:         LocalGoRuntime,
		ExecutionConfig: DefaultExecutionConfig(), // Default to local execution
	}
}

func (a *BaseAgent) GetCapabilities() []string {
	return a.Capabilities
}

func (a *BaseAgent) GetName() string {
	return a.Name
}

func (a *BaseAgent) GetType() AgentType {
	return a.Type
}

func (a *BaseAgent) GetRuntime() AgentRuntime {
	if a.Runtime == "" {
		return LocalGoRuntime
	}
	return a.Runtime
}

// DoAgent implements an agent that performs specific actions on wants.
type DoAgent struct {
	BaseAgent
	Action func(ctx context.Context, want *Want) error
}

// NewDoAgent creates a new DoAgent with the given parameters. This is the canonical constructor for DoAgent-based agents.
func NewDoAgent(name string, capabilities []string) DoAgent {
	return DoAgent{
		BaseAgent: *NewBaseAgent(name, capabilities, DoAgentType),
	}
}

func (a *DoAgent) Exec(ctx context.Context, want *Want) (bool, error) {
	if a.Action != nil {
		err := a.Action(ctx, want)

		// Commit agent state changes
		want.CommitStateChanges()
		return false, err // DoAgents don't stop monitoring
	}
	return false, fmt.Errorf("no action defined for DoAgent %s", a.Name)
}

// MonitorAgent implements an agent that monitors want execution and state.
type MonitorAgent struct {
	BaseAgent
	Monitor func(ctx context.Context, want *Want) error
}

func (a *MonitorAgent) Exec(ctx context.Context, want *Want) (bool, error) {
	if a.Monitor != nil {
		err := a.Monitor(ctx, want)

		// Commit agent state changes
		want.CommitStateChanges()
		return false, err // Default: continue monitoring
	}
	return false, fmt.Errorf("no monitor function defined for MonitorAgent %s", a.Name)
}

// PollAgent implements an agent that polls with stop-signal support.
// Unlike MonitorAgent (always continues), PollAgent can signal termination via shouldStop.
type PollAgent struct {
	BaseAgent
	Poll PollFunc
}

func (a *PollAgent) Exec(ctx context.Context, want *Want) (bool, error) {
	if a.Poll != nil {
		shouldStop, err := a.Poll(ctx, want)
		want.CommitStateChanges()
		return shouldStop, err
	}
	return false, fmt.Errorf("no poll function defined for PollAgent %s", a.Name)
}

// ThinkAgent implements an agent that reacts to state changes by running a ThinkFunc.
// It is executed synchronously (like DoAgent) but registers a ThinkingAgent as a BackgroundAgent.
type ThinkAgent struct {
	BaseAgent
	Think ThinkFunc
}

func (a *ThinkAgent) Exec(ctx context.Context, want *Want) (bool, error) {
	if a.Think == nil {
		return false, fmt.Errorf("no think function defined for ThinkAgent %s", a.Name)
	}

	id := "think-" + a.Name + "-" + want.Metadata.ID
	if _, exists := want.GetBackgroundAgent(id); exists {
		// Already running - skip registration
		return false, nil
	}

	thinkingAgent := NewThinkingAgent(id, 2*time.Second, a.Name, a.Think)
	if err := want.AddBackgroundAgent(thinkingAgent); err != nil {
		return false, fmt.Errorf("failed to add ThinkingAgent %s: %w", id, err)
	}
	return false, nil
}

// AgentSpec holds specification for state field validation
type AgentSpec struct {
	Name             string
	AllowedStateKeys map[string]bool    // O(1) lookup: key -> allowed
	KeyDescriptions  map[string]string  // For logging: key -> description
	StateAccessTypes map[string]AccessType // key -> access type

	// Parent state access (for want.StoreParentState / GetParentState)
	AllowedParentKeys     map[string]bool
	ParentKeyDescriptions map[string]string
	ParentAccessTypes     map[string]AccessType
}

// AgentRegistry manages agent registration and capability mapping.
type AgentRegistry struct {
	capabilities       map[string]Capability
	agents             map[string]Agent
	capabilityToAgents map[string][]string
	agentSpecs         map[string]*AgentSpec // NEW: agent specs for validation
	mutex              sync.RWMutex
}

// NewAgentRegistry creates a new agent registry for managing agents and capabilities.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		capabilities:       make(map[string]Capability),
		agents:             make(map[string]Agent),
		capabilityToAgents: make(map[string][]string),
		agentSpecs:         make(map[string]*AgentSpec), // NEW
	}
}

func (r *AgentRegistry) RegisterCapability(cap Capability) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.capabilities[cap.Name] = cap
	InfoLog("[REGISTRY] Registered capability: %s (gives: %v)", cap.Name, cap.Gives)

	for _, gives := range cap.Gives {
		if r.capabilityToAgents[gives] == nil {
			r.capabilityToAgents[gives] = make([]string, 0)
		}
	}
}

func (r *AgentRegistry) RegisterAgent(agent Agent) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.agents[agent.GetName()] = agent
	InfoLog("[REGISTRY] Registered agent: %s (type: %s, capabilities: %v)", agent.GetName(), agent.GetType(), agent.GetCapabilities())

	for _, capName := range agent.GetCapabilities() {
		if cap, exists := r.capabilities[capName]; exists {
			for _, gives := range cap.Gives {
				agentNames := r.capabilityToAgents[gives]
				r.capabilityToAgents[gives] = append(agentNames, agent.GetName())
				InfoLog("[REGISTRY] Linked agent '%s' to capability value '%s' (via %s)", agent.GetName(), gives, capName)
			}
		} else {
			InfoLog("[REGISTRY] Agent '%s' refers to unknown capability '%s'", agent.GetName(), capName)
		}
	}
}

func (r *AgentRegistry) FindAgentsByGives(gives string) []Agent {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	agentNames, exists := r.capabilityToAgents[gives]
	if !exists {
		return nil
	}

	agents := make([]Agent, 0, len(agentNames))
	for _, name := range agentNames {
		if agent, exists := r.agents[name]; exists {
			agents = append(agents, agent)
		}
	}

	return agents
}

func (r *AgentRegistry) GetAgent(name string) (Agent, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	agent, exists := r.agents[name]
	return agent, exists
}

func (r *AgentRegistry) GetCapability(name string) (Capability, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	cap, exists := r.capabilities[name]
	return cap, exists
}

// FindMonitorAgentsByCapabilityName returns all MonitorAgents (or PollAgents) that
// declare the given capability name in their Capabilities list.
// Unlike FindAgentsByGives (which searches by capability gives values), this searches
// by the capability name itself and filters to MonitorAgentType only.
func (r *AgentRegistry) FindMonitorAgentsByCapabilityName(capName string) []Agent {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var result []Agent
	for _, agent := range r.agents {
		if agent.GetType() != MonitorAgentType {
			continue
		}
		for _, cap := range agent.GetCapabilities() {
			if cap == capName {
				result = append(result, agent)
				break
			}
		}
	}
	return result
}

// BuildAgentSpecFromCapabilities builds an agent's state validation spec by aggregating
// stateAccess and parentStateAccess declared in its capabilities.
func (r *AgentRegistry) BuildAgentSpecFromCapabilities(agentName string, capabilityNames []string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	spec := &AgentSpec{
		Name:                  agentName,
		AllowedStateKeys:      make(map[string]bool),
		KeyDescriptions:       make(map[string]string),
		StateAccessTypes:      make(map[string]AccessType),
		AllowedParentKeys:     make(map[string]bool),
		ParentKeyDescriptions: make(map[string]string),
		ParentAccessTypes:     make(map[string]AccessType),
	}

	for _, capName := range capabilityNames {
		if cap, exists := r.capabilities[capName]; exists {
			for _, field := range cap.StateAccess {
				spec.AllowedStateKeys[field.Name] = true
				spec.KeyDescriptions[field.Name] = field.Description
				accessType := field.AccessType
				if accessType == "" {
					accessType = AccessTypeReadUpdate
				}
				spec.StateAccessTypes[field.Name] = accessType
			}
			for _, field := range cap.ParentStateAccess {
				spec.AllowedParentKeys[field.Name] = true
				spec.ParentKeyDescriptions[field.Name] = field.Description
				accessType := field.AccessType
				if accessType == "" {
					accessType = AccessTypeReadUpdate
				}
				spec.ParentAccessTypes[field.Name] = accessType
			}
		}
	}

	r.agentSpecs[agentName] = spec

	stateCount := len(spec.AllowedStateKeys)
	parentCount := len(spec.AllowedParentKeys)
	if stateCount > 0 || parentCount > 0 {
		InfoLog("[AGENT SPEC] Built spec for agent '%s': %d state fields, %d parent state fields\n",
			agentName, stateCount, parentCount)
	}
}

// GetAgentSpec retrieves an agent's specification
func (r *AgentRegistry) GetAgentSpec(agentName string) (*AgentSpec, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	spec, exists := r.agentSpecs[agentName]
	return spec, exists
}

func (r *AgentRegistry) GetAllAgents() []Agent {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	agents := make([]Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	return agents
}
func (r *AgentRegistry) GetAllCapabilities() []Capability {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	capabilities := make([]Capability, 0, len(r.capabilities))
	for _, cap := range r.capabilities {
		capabilities = append(capabilities, cap)
	}
	return capabilities
}

// UnregisterAgent removes an agent from the registry
func (r *AgentRegistry) UnregisterAgent(name string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	agent, exists := r.agents[name]
	if !exists {
		return false
	}
	delete(r.agents, name)
	for _, capName := range agent.GetCapabilities() {
		if cap, exists := r.capabilities[capName]; exists {
			for _, gives := range cap.Gives {
				agentNames := r.capabilityToAgents[gives]
				for i, agentName := range agentNames {
					if agentName == name {
						r.capabilityToAgents[gives] = append(agentNames[:i], agentNames[i+1:]...)
						break
					}
				}
			}
		}
	}

	return true
}

// UnregisterCapability removes a capability from the registry
func (r *AgentRegistry) UnregisterCapability(name string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	cap, exists := r.capabilities[name]
	if !exists {
		return false
	}
	delete(r.capabilities, name)
	for _, gives := range cap.Gives {
		delete(r.capabilityToAgents, gives)
	}

	return true
}
