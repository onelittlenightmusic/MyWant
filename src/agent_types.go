package mywant

import (
	"context"
	"fmt"
	"sync"
)

type AgentType string

const (
	DoAgentType      AgentType = "do"
	MonitorAgentType AgentType = "monitor"
)

type Capability struct {
	Name  string   `yaml:"name"`
	Gives []string `yaml:"gives"`
}

type Agent interface {
	Exec(ctx context.Context, want *Want) error
	GetCapabilities() []string
	GetName() string
	GetType() AgentType
	GetUses() []string
}

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

type DoAgent struct {
	BaseAgent
	Action func(ctx context.Context, want *Want) error
}

func (a *DoAgent) Exec(ctx context.Context, want *Want) error {
	if a.Action != nil {
		return a.Action(ctx, want)
	}
	return fmt.Errorf("no action defined for DoAgent %s", a.Name)
}

type MonitorAgent struct {
	BaseAgent
	Monitor func(ctx context.Context, want *Want) error
}

func (a *MonitorAgent) Exec(ctx context.Context, want *Want) error {
	if a.Monitor != nil {
		return a.Monitor(ctx, want)
	}
	return fmt.Errorf("no monitor function defined for MonitorAgent %s", a.Name)
}

type AgentRegistry struct {
	capabilities       map[string]Capability
	agents            map[string]Agent
	capabilityToAgents map[string][]string
	mutex             sync.RWMutex
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		capabilities:       make(map[string]Capability),
		agents:            make(map[string]Agent),
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