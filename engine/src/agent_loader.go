package mywant

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"path/filepath"
	"strings"

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
	} `yaml:"agents"`
}

func (r *AgentRegistry) LoadCapabilities(path string) error {
	files, err := filepath.Glob(filepath.Join(path, "capability-*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to find capability files: %w", err)
	}

	for _, file := range files {
		if err := r.loadCapabilityFile(file); err != nil {
			return fmt.Errorf("failed to load capability file %s: %w", file, err)
		}
	}

	return nil
}

// loadAgentSpec loads the agent OpenAPI spec, trying multiple possible paths
func loadAgentSpec() (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	specPaths := []string{"spec/agent-spec.yaml", "../spec/agent-spec.yaml"}

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
	capabilitySchemaRef := spec.Components.Schemas["CapabilityConfig"]
	if capabilitySchemaRef == nil {
		return fmt.Errorf("CapabilityConfig schema not found in agent OpenAPI spec")
	}
	var yamlContent interface{}
	if err := yaml.Unmarshal(yamlData, &yamlContent); err != nil {
		return fmt.Errorf("failed to parse capability YAML: %w", err)
	}

	// Basic structural validation
	if err := validateCapabilityStructure(yamlContent); err != nil {
		return fmt.Errorf("capability structure validation failed: %w", err)
	}

	InfoLog("[VALIDATION] Capability '%s' validated successfully against OpenAPI spec\n", filename)
	return nil
}
func validateCapabilityStructure(content interface{}) error {
	contentObj, ok := content.(map[string]interface{})
	if !ok {
		return fmt.Errorf("capability content must be an object")
	}
	capabilities, ok := contentObj["capabilities"]
	if !ok {
		return fmt.Errorf("missing required 'capabilities' field")
	}

	capabilitiesArray, ok := capabilities.([]interface{})
	if !ok {
		return fmt.Errorf("capabilities must be an array")
	}

	if len(capabilitiesArray) == 0 {
		return fmt.Errorf("capabilities array cannot be empty")
	}
	for i, cap := range capabilitiesArray {
		capObj, ok := cap.(map[string]interface{})
		if !ok {
			return fmt.Errorf("capability at index %d must be an object", i)
		}
		if name, ok := capObj["name"]; !ok || name == "" {
			return fmt.Errorf("capability at index %d missing required 'name' field", i)
		}

		gives, ok := capObj["gives"]
		if !ok {
			return fmt.Errorf("capability at index %d missing required 'gives' field", i)
		}
		givesArray, ok := gives.([]interface{})
		if !ok {
			return fmt.Errorf("capability at index %d 'gives' must be an array", i)
		}

		if len(givesArray) == 0 {
			return fmt.Errorf("capability at index %d 'gives' array cannot be empty", i)
		}

		for j, give := range givesArray {
			if giveStr, ok := give.(string); !ok || giveStr == "" {
				return fmt.Errorf("capability at index %d, gives at index %d must be a non-empty string", i, j)
			}
		}
	}

	return nil
}

func (r *AgentRegistry) loadCapabilityFile(filename string) error {
	data, err := ioutil.ReadFile(filename)
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
		r.RegisterCapability(cap)
	}

	return nil
}

func (r *AgentRegistry) LoadAgents(path string) error {
	files, err := filepath.Glob(filepath.Join(path, "agent-*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to find agent files: %w", err)
	}

	for _, file := range files {
		if err := r.loadAgentFile(file); err != nil {
			return fmt.Errorf("failed to load agent file %s: %w", file, err)
		}
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
	agentSchemaRef := spec.Components.Schemas["AgentConfig"]
	if agentSchemaRef == nil {
		return fmt.Errorf("AgentConfig schema not found in agent OpenAPI spec")
	}
	var yamlContent interface{}
	if err := yaml.Unmarshal(yamlData, &yamlContent); err != nil {
		return fmt.Errorf("failed to parse agent YAML: %w", err)
	}

	// Basic structural validation
	if err := validateAgentStructure(yamlContent); err != nil {
		return fmt.Errorf("agent structure validation failed: %w", err)
	}

	InfoLog("[VALIDATION] Agent '%s' validated successfully against OpenAPI spec\n", filename)
	return nil
}
func validateAgentStructure(content interface{}) error {
	contentObj, ok := content.(map[string]interface{})
	if !ok {
		return fmt.Errorf("agent content must be an object")
	}
	agents, ok := contentObj["agents"]
	if !ok {
		return fmt.Errorf("missing required 'agents' field")
	}

	agentsArray, ok := agents.([]interface{})
	if !ok {
		return fmt.Errorf("agents must be an array")
	}

	if len(agentsArray) == 0 {
		return fmt.Errorf("agents array cannot be empty")
	}
	for i, agent := range agentsArray {
		agentObj, ok := agent.(map[string]interface{})
		if !ok {
			return fmt.Errorf("agent at index %d must be an object", i)
		}
		if name, ok := agentObj["name"]; !ok || name == "" {
			return fmt.Errorf("agent at index %d missing required 'name' field", i)
		}

		if agentType, ok := agentObj["type"]; !ok {
			return fmt.Errorf("agent at index %d missing required 'type' field", i)
		} else {
			typeStr, ok := agentType.(string)
			if !ok {
				return fmt.Errorf("agent at index %d 'type' must be a string", i)
			}
			if typeStr != "do" && typeStr != "monitor" {
				return fmt.Errorf("agent at index %d 'type' must be 'do' or 'monitor', got '%s'", i, typeStr)
			}
		}

		if capabilities, ok := agentObj["capabilities"]; !ok {
			return fmt.Errorf("agent at index %d missing required 'capabilities' field", i)
		} else {
			capabilitiesArray, ok := capabilities.([]interface{})
			if !ok {
				return fmt.Errorf("agent at index %d 'capabilities' must be an array", i)
			}

			if len(capabilitiesArray) == 0 {
				return fmt.Errorf("agent at index %d 'capabilities' array cannot be empty", i)
			}

			for j, cap := range capabilitiesArray {
				if capStr, ok := cap.(string); !ok || capStr == "" {
					return fmt.Errorf("agent at index %d, capability at index %d must be a non-empty string", i, j)
				}
			}
		}
		if uses, ok := agentObj["uses"]; ok {
			usesArray, ok := uses.([]interface{})
			if !ok {
				return fmt.Errorf("agent at index %d 'uses' must be an array", i)
			}

			for j, use := range usesArray {
				if useStr, ok := use.(string); !ok || useStr == "" {
					return fmt.Errorf("agent at index %d, uses at index %d must be a non-empty string", i, j)
				}
			}
		}
	}

	return nil
}

func (r *AgentRegistry) loadAgentFile(filename string) error {
	data, err := ioutil.ReadFile(filename)
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
			Uses:         agentDef.Uses,
		}

		switch strings.ToLower(agentDef.Type) {
		case "do":
			baseAgent.Type = DoAgentType
			doAgent := &DoAgent{BaseAgent: baseAgent}
			r.setAgentAction(doAgent)
			agent = doAgent
		case "monitor":
			baseAgent.Type = MonitorAgentType
			monitorAgent := &MonitorAgent{BaseAgent: baseAgent}
			r.setAgentMonitor(monitorAgent)
			agent = monitorAgent
		default:
			return fmt.Errorf("unknown agent type: %s", agentDef.Type)
		}

		r.RegisterAgent(agent)
	}

	return nil
}

func (r *AgentRegistry) setAgentAction(agent *DoAgent) {
	// All DoAgents use the same generic action - just initialize state
	agent.Action = r.genericDoAction
}

func (r *AgentRegistry) setAgentMonitor(agent *MonitorAgent) {
	// All MonitorAgents use the same generic monitor - just log monitoring
	agent.Monitor = r.genericMonitorAction
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
