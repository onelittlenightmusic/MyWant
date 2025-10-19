package mywant

import (
	"context"
	"fmt"
	"sync"
)

// AgentType defines the type of agent for execution strategies.
type AgentType string

const (
	DoAgentType      AgentType = "do"
	MonitorAgentType AgentType = "monitor"
)

// Capability represents an agent's functional capability with its dependencies.
type Capability struct {
	Name  string   `yaml:"name"`
	Gives []string `yaml:"gives"`
}

// Agent defines the interface for all agent implementations.
type Agent interface {
	Exec(ctx context.Context, want *Want) error
	GetCapabilities() []string
	GetName() string
	GetType() AgentType
	GetUses() []string
}

// BaseAgent provides common functionality for all agent types.
type BaseAgent struct {
	Name         string    `yaml:"name"`
	Capabilities []string  `yaml:"capabilities"`
	Uses         []string  `yaml:"uses"`
	Type         AgentType `yaml:"type"`
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

func (a *BaseAgent) GetUses() []string {
	return a.Uses
}

// DoAgent implements an agent that performs specific actions on wants.
type DoAgent struct {
	BaseAgent
	Action func(ctx context.Context, want *Want) error
}

func (a *DoAgent) Exec(ctx context.Context, want *Want) error {
	if a.Action != nil {
		// Begin batching cycle for all state changes from agent execution
		// This includes agent-generated state changes and any subsequent SetSchedule() calls
		want.BeginExecCycle()

		err := a.Action(ctx, want)

		// Commit all staged state changes from the agent in a single batch
		want.CommitStateChanges()
		return err
	}
	return fmt.Errorf("no action defined for DoAgent %s", a.Name)
}

// MonitorAgent implements an agent that monitors want execution and state.
type MonitorAgent struct {
	BaseAgent
	Monitor func(ctx context.Context, want *Want) error
}

func (a *MonitorAgent) Exec(ctx context.Context, want *Want) error {
	if a.Monitor != nil {
		// Begin batching cycle for all state changes from agent execution
		// This includes agent-generated state changes and any subsequent SetSchedule() calls
		want.BeginExecCycle()

		err := a.Monitor(ctx, want)

		// Commit all staged state changes from the agent in a single batch
		want.CommitStateChanges()
		return err
	}
	return fmt.Errorf("no monitor function defined for MonitorAgent %s", a.Name)
}

// AgentRegistry manages agent registration and capability mapping.
type AgentRegistry struct {
	capabilities       map[string]Capability
	agents             map[string]Agent
	capabilityToAgents map[string][]string
	mutex              sync.RWMutex
}

// NewAgentRegistry creates a new agent registry for managing agents and capabilities.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		capabilities:       make(map[string]Capability),
		agents:             make(map[string]Agent),
		capabilityToAgents: make(map[string][]string),
	}
}

func (r *AgentRegistry) RegisterCapability(cap Capability) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.capabilities[cap.Name] = cap

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

	for _, capName := range agent.GetCapabilities() {
		if cap, exists := r.capabilities[capName]; exists {
			for _, gives := range cap.Gives {
				agentNames := r.capabilityToAgents[gives]
				r.capabilityToAgents[gives] = append(agentNames, agent.GetName())
			}
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

// GetAllAgents returns all registered agents
func (r *AgentRegistry) GetAllAgents() []Agent {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	agents := make([]Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	return agents
}

// GetAllCapabilities returns all registered capabilities
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

	// Remove agent from agents map
	delete(r.agents, name)

	// Remove agent from capability mappings
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

	// Remove capability from capabilities map
	delete(r.capabilities, name)

	// Remove capability mappings
	for _, gives := range cap.Gives {
		delete(r.capabilityToAgents, gives)
	}

	return true
}
