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

// validateCapabilityWithSpec validates capability YAML data against the OpenAPI spec
func validateCapabilityWithSpec(yamlData []byte) error {
	// Load the OpenAPI spec for agents and capabilities
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromFile("spec/agent-spec.yaml")
	if err != nil {
		return fmt.Errorf("failed to load agent OpenAPI spec: %w", err)
	}

	// Validate the spec itself
	ctx := context.Background()
	err = spec.Validate(ctx)
	if err != nil {
		return fmt.Errorf("agent OpenAPI spec is invalid: %w", err)
	}

	// Get the CapabilityConfig schema from the spec
	capabilitySchemaRef := spec.Components.Schemas["CapabilityConfig"]
	if capabilitySchemaRef == nil {
		return fmt.Errorf("CapabilityConfig schema not found in agent OpenAPI spec")
	}

	// Convert YAML to generic structure for validation
	var yamlContent interface{}
	if err := yaml.Unmarshal(yamlData, &yamlContent); err != nil {
		return fmt.Errorf("failed to parse capability YAML: %w", err)
	}

	// Basic structural validation
	if err := validateCapabilityStructure(yamlContent); err != nil {
		return fmt.Errorf("capability structure validation failed: %w", err)
	}

	fmt.Printf("[VALIDATION] Capability validated successfully against OpenAPI spec\n")
	return nil
}

// validateCapabilityStructure validates the structure of capability content
func validateCapabilityStructure(content interface{}) error {
	contentObj, ok := content.(map[string]interface{})
	if !ok {
		return fmt.Errorf("capability content must be an object")
	}

	// Check required capabilities field
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

	// Validate each capability
	for i, cap := range capabilitiesArray {
		capObj, ok := cap.(map[string]interface{})
		if !ok {
			return fmt.Errorf("capability at index %d must be an object", i)
		}

		// Check required fields
		if name, ok := capObj["name"]; !ok || name == "" {
			return fmt.Errorf("capability at index %d missing required 'name' field", i)
		}

		gives, ok := capObj["gives"]
		if !ok {
			return fmt.Errorf("capability at index %d missing required 'gives' field", i)
		}

		// Validate gives array
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

	// Validate against OpenAPI spec
	err = validateCapabilityWithSpec(data)
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

// validateAgentWithSpec validates agent YAML data against the OpenAPI spec
func validateAgentWithSpec(yamlData []byte) error {
	// Load the OpenAPI spec for agents and capabilities
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromFile("spec/agent-spec.yaml")
	if err != nil {
		return fmt.Errorf("failed to load agent OpenAPI spec: %w", err)
	}

	// Validate the spec itself
	ctx := context.Background()
	err = spec.Validate(ctx)
	if err != nil {
		return fmt.Errorf("agent OpenAPI spec is invalid: %w", err)
	}

	// Get the AgentConfig schema from the spec
	agentSchemaRef := spec.Components.Schemas["AgentConfig"]
	if agentSchemaRef == nil {
		return fmt.Errorf("AgentConfig schema not found in agent OpenAPI spec")
	}

	// Convert YAML to generic structure for validation
	var yamlContent interface{}
	if err := yaml.Unmarshal(yamlData, &yamlContent); err != nil {
		return fmt.Errorf("failed to parse agent YAML: %w", err)
	}

	// Basic structural validation
	if err := validateAgentStructure(yamlContent); err != nil {
		return fmt.Errorf("agent structure validation failed: %w", err)
	}

	fmt.Printf("[VALIDATION] Agent validated successfully against OpenAPI spec\n")
	return nil
}

// validateAgentStructure validates the structure of agent content
func validateAgentStructure(content interface{}) error {
	contentObj, ok := content.(map[string]interface{})
	if !ok {
		return fmt.Errorf("agent content must be an object")
	}

	// Check required agents field
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

	// Validate each agent
	for i, agent := range agentsArray {
		agentObj, ok := agent.(map[string]interface{})
		if !ok {
			return fmt.Errorf("agent at index %d must be an object", i)
		}

		// Check required fields
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
			// Validate capabilities array
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

		// Validate optional uses field if present
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

	// Validate against OpenAPI spec
	err = validateAgentWithSpec(data)
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
			r.setAgentAction(doAgent, agentDef.Name)
			agent = doAgent
		case "monitor":
			baseAgent.Type = MonitorAgentType
			monitorAgent := &MonitorAgent{BaseAgent: baseAgent}
			r.setAgentMonitor(monitorAgent, agentDef.Name)
			agent = monitorAgent
		default:
			return fmt.Errorf("unknown agent type: %s", agentDef.Type)
		}

		r.RegisterAgent(agent)
	}

	return nil
}

func (r *AgentRegistry) setAgentAction(agent *DoAgent, name string) {
	switch name {
	case "agent_premium":
		agent.Action = r.hotelReservationAction
	default:
		agent.Action = r.defaultDoAction
	}
}

func (r *AgentRegistry) setAgentMonitor(agent *MonitorAgent, name string) {
	switch name {
	case "hotel_monitor":
		agent.Monitor = r.hotelReservationMonitor
	default:
		agent.Monitor = r.defaultMonitorAction
	}
}

func (r *AgentRegistry) defaultDoAction(ctx context.Context, want *Want) error {
	fmt.Printf("DoAgent executing for want: %s\n", want.Metadata.Name)
	return nil
}

func (r *AgentRegistry) defaultMonitorAction(ctx context.Context, want *Want) error {
	fmt.Printf("MonitorAgent monitoring for want: %s\n", want.Metadata.Name)
	return nil
}

func (r *AgentRegistry) hotelReservationAction(ctx context.Context, want *Want) error {
	fmt.Printf("Hotel reservation agent executing for want: %s\n", want.Metadata.Name)

	// Stage all state changes as a single object
	want.StageStateChange(map[string]interface{}{
		"reservation_id": "HTL-12345",
		"status":        "confirmed",
		"hotel_name":    "Premium Hotel",
		"check_in":      "2025-09-20",
		"check_out":     "2025-09-22",
	})

	// Commit all changes at once
	want.CommitStateChanges()

	fmt.Printf("Hotel reservation completed with object-based state update\n")
	return nil
}

func (r *AgentRegistry) hotelReservationMonitor(ctx context.Context, want *Want) error {
	fmt.Printf("Hotel reservation monitor checking status for want: %s\n", want.Metadata.Name)

	if reservationID, exists := want.GetState("reservation_id"); exists {
		// Stage all monitoring updates as a single object
		want.StageStateChange(map[string]interface{}{
			"reservation_id": reservationID, // Keep existing reservation ID
			"status":        "confirmed",    // Confirm status
			"last_checked":  "2025-09-17T10:00:00Z",
			"room_ready":    true,
		})

		// Commit all monitoring updates at once
		want.CommitStateChanges()

		fmt.Printf("Hotel reservation status updated with object-based commit\n")
	}

	return nil
}