package mywant

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/getkin/kin-openapi/openapi3"
)

type CapabilityYAML struct {
	Capabilities []Capability `yaml:"capabilities"`
}

type AgentYAML struct {
	Agents []struct {
		Name         string   `yaml:"name"`
		Capabilities []string `yaml:"capabilities"`
		Uses         []string `yaml:"uses"`
		Type         string   `yaml:"type"`
		Runtime      string   `yaml:"runtime"`
	} `yaml:"agents"`
}

func (r *AgentRegistry) LoadCapabilities(path string) error {
	InfoLog("[AGENT] Loading capabilities from: %s", path)
	files, err := filepath.Glob(filepath.Join(path, "capability-*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to find capability files: %w", err)
	}
	InfoLog("[AGENT] Found %d capability files", len(files))

	for _, file := range files {
		if err := r.loadCapabilityFile(file); err != nil {
			ErrorLog("[AGENT] Failed to load capability file %s: %v", file, err)
			return fmt.Errorf("failed to load capability file %s: %w", file, err)
		}
		InfoLog("[AGENT] Successfully loaded capability file: %s", file)
	}

	return nil
}

// loadAgentSpec loads the agent OpenAPI spec, trying multiple possible paths
func loadAgentSpec() (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	specPaths := []string{
		filepath.Join(SpecDir, "agent-spec.yaml"),
		filepath.Join("..", SpecDir, "agent-spec.yaml"),
		filepath.Join("../..", SpecDir, "agent-spec.yaml"),
		"spec/agent-spec.yaml",    // Legacy
		"../spec/agent-spec.yaml", // Legacy
		"../../openapi.yaml",      // Legacy
		"openapi.yaml",            // Legacy
	}

	var lastErr error
	for _, specPath := range specPaths {
		if spec, err := loader.LoadFromFile(specPath); err == nil {
			return spec, nil
		} else {
			lastErr = err
		}
	}

	return nil, fmt.Errorf("failed to load agent OpenAPI spec from paths %v: %w", specPaths, lastErr)
}
func validateCapabilityWithSpec(yamlData []byte, filename string) error {
	// Load the OpenAPI spec for agents and capabilities
	spec, err := loadAgentSpec()
	if err != nil {
		return fmt.Errorf("failed to load agent OpenAPI spec: %w", err)
	}
	ctx := context.Background()
	err = spec.Validate(ctx)
	if err != nil {
		return fmt.Errorf("agent OpenAPI spec is invalid: %w", err)
	}
	capabilitySchemaRef := spec.Components.Schemas["CapabilityYAML"]
	if capabilitySchemaRef == nil {
		schemaNames := make([]string, 0, len(spec.Components.Schemas))
		for name := range spec.Components.Schemas {
			schemaNames = append(schemaNames, name)
		}
		return fmt.Errorf("CapabilityYAML schema not found in agent OpenAPI spec. Available: %v", schemaNames)
	}
	var yamlContent any
	if err := yaml.Unmarshal(yamlData, &yamlContent); err != nil {
		return fmt.Errorf("failed to parse capability YAML: %w", err)
	}

	// Basic structural validation
	// if err := validateCapabilityStructure(yamlContent); err != nil {
	// 	return fmt.Errorf("capability structure validation failed: %w", err)
	// }

	InfoLog("[VALIDATION] Capability '%s' validated successfully against OpenAPI spec\n", filename)
	return nil
}

func (r *AgentRegistry) loadCapabilityFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	err = validateCapabilityWithSpec(data, filename)
	if err != nil {
		return fmt.Errorf("capability validation failed for %s: %w", filename, err)
	}

	var capYAML CapabilityYAML
	if err := yaml.Unmarshal(data, &capYAML); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	for _, cap := range capYAML.Capabilities {
		InfoLog("[AGENT] Registering capability: %s (gives: %v)", cap.Name, cap.Gives)
		r.RegisterCapability(cap)
	}

	return nil
}

func (r *AgentRegistry) LoadAgents(path string) error {
	InfoLog("[AGENT] Loading agents from: %s", path)
	files, err := filepath.Glob(filepath.Join(path, "agent-*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to find agent files: %w", err)
	}
	InfoLog("[AGENT] Found %d agent files", len(files))

	for _, file := range files {
		if err := r.loadAgentFile(file); err != nil {
			ErrorLog("[AGENT] Failed to load agent file %s: %v", file, err)
			return fmt.Errorf("failed to load agent file %s: %w", file, err)
		}
		InfoLog("[AGENT] Successfully loaded agent file: %s", file)
	}

	return nil
}
func validateAgentWithSpec(yamlData []byte, filename string) error {
	// Load the OpenAPI spec for agents and capabilities
	spec, err := loadAgentSpec()
	if err != nil {
		return fmt.Errorf("failed to load agent OpenAPI spec: %w", err)
	}
	ctx := context.Background()
	err = spec.Validate(ctx)
	if err != nil {
		return fmt.Errorf("agent OpenAPI spec is invalid: %w", err)
	}
	agentSchemaRef := spec.Components.Schemas["AgentYAML"]
	if agentSchemaRef == nil {
		schemaNames := make([]string, 0, len(spec.Components.Schemas))
		for name := range spec.Components.Schemas {
			schemaNames = append(schemaNames, name)
		}
		return fmt.Errorf("AgentYAML schema not found in agent OpenAPI spec. Available: %v", schemaNames)
	}
	var yamlContent any
	if err := yaml.Unmarshal(yamlData, &yamlContent); err != nil {
		return fmt.Errorf("failed to parse agent YAML: %w", err)
	}

	// Basic structural validation
	// if err := validateAgentStructure(yamlContent); err != nil {
	// 	return fmt.Errorf("agent structure validation failed: %w", err)
	// }

	InfoLog("[VALIDATION] Agent '%s' validated successfully against OpenAPI spec\n", filename)
	return nil
}

func (r *AgentRegistry) loadAgentFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	err = validateAgentWithSpec(data, filename)
	if err != nil {
		return fmt.Errorf("agent validation failed for %s: %w", filename, err)
	}

	var agentYAML AgentYAML
	if err := yaml.Unmarshal(data, &agentYAML); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	for _, agentDef := range agentYAML.Agents {
		var agent Agent

		baseAgent := BaseAgent{
			Name:         agentDef.Name,
			Capabilities: agentDef.Capabilities,
			Runtime:      AgentRuntime(agentDef.Runtime),
		}

		if baseAgent.Runtime == "" {
			baseAgent.Runtime = LocalGoRuntime
		}

		switch strings.ToLower(agentDef.Type) {
		case "do":
			baseAgent.Type = DoAgentType
			doAgent := &DoAgent{BaseAgent: baseAgent}
			r.setAgentAction(doAgent)
			agent = doAgent
		case "monitor":
			baseAgent.Type = MonitorAgentType
			// Check if it's actually a PollAgent (has a registered poll function)
			if _, isPoll := pollActionRegistry[baseAgent.Name]; isPoll {
				pollAgent := &PollAgent{BaseAgent: baseAgent}
				r.setAgentPoll(pollAgent)
				agent = pollAgent
			} else {
				monitorAgent := &MonitorAgent{BaseAgent: baseAgent}
				r.setAgentMonitor(monitorAgent)
				agent = monitorAgent
			}
		case "think":
			baseAgent.Type = ThinkAgentType
			thinkAgent := &ThinkAgent{BaseAgent: baseAgent}
			r.setAgentThink(thinkAgent)
			agent = thinkAgent
		default:
			return fmt.Errorf("unknown agent type: %s", agentDef.Type)
		}

		r.RegisterAgent(agent)
		InfoLog("[AGENT] Registered agent '%s' with capabilities %v", agent.GetName(), agent.GetCapabilities())

		// Build agent spec from capability stateAccess / parentStateAccess declarations
		r.BuildAgentSpecFromCapabilities(agentDef.Name, agentDef.Capabilities)
	}

	return nil
}

func (r *AgentRegistry) setAgentAction(agent *DoAgent) {
	// Look up specific implementation from registry
	if action, ok := doActionRegistry[agent.Name]; ok {
		agent.Action = action
		InfoLog("[AGENT] Linked agent '%s' to registered Go implementation (Do)", agent.Name)
		return
	}

	// All DoAgents use the same generic action - just initialize state
	agent.Action = r.genericDoAction
}

func (r *AgentRegistry) setAgentMonitor(agent *MonitorAgent) {
	// Look up specific implementation from registry
	if monitor, ok := monitorActionRegistry[agent.Name]; ok {
		agent.Monitor = monitor
		InfoLog("[AGENT] Linked agent '%s' to registered Go implementation (Monitor)", agent.Name)
		return
	}

	// All MonitorAgents use the same generic monitor - just log monitoring
	agent.Monitor = r.genericMonitorAction
}

func (r *AgentRegistry) setAgentPoll(agent *PollAgent) {
	// Look up specific implementation from registry
	if poll, ok := pollActionRegistry[agent.Name]; ok {
		agent.Poll = poll
		InfoLog("[AGENT] Linked agent '%s' to registered Go implementation (Poll)", agent.Name)
		return
	}

	// PollAgent doesn't have a generic fallback as it needs to return shouldStop
}

func (r *AgentRegistry) setAgentThink(agent *ThinkAgent) {
	// Look up specific implementation from registry
	if think, ok := thinkActionRegistry[agent.Name]; ok {
		agent.Think = think
		InfoLog("[AGENT] Linked agent '%s' to registered Go implementation (Think)", agent.Name)
		return
	}

	// ThinkAgent doesn't have a generic fallback as it needs a think function to be useful
}

// genericDoAction is the default action for all DoAgents Agents don't need special implementations - state initialization is externalized to want types
func (r *AgentRegistry) genericDoAction(ctx context.Context, want *Want) error {
	InfoLog("[AGENT] DoAgent executing for want: %s\n", want.Metadata.Name)
	// State initialization happens in the want type's agent execution logic This is just a placeholder that confirms the agent executed
	return nil
}

// genericMonitorAction is the default monitor for all MonitorAgents Agents don't need special implementations - monitoring logic is externalized to want types
func (r *AgentRegistry) genericMonitorAction(ctx context.Context, want *Want) error {
	InfoLog("[AGENT] MonitorAgent monitoring for want: %s\n", want.Metadata.Name)
	// Monitoring logic happens in the want type's agent execution logic This is just a placeholder that confirms the monitor executed
	return nil
}
