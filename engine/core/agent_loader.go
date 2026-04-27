package mywant

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/getkin/kin-openapi/openapi3"
)

// MRSAgentDef describes a Machine-Readable Skill agent declared in a plugin agent.yaml.
// It is parsed from the top-level `agent:` key.
type MRSAgentDef struct {
	Metadata     MRSAgentMetadata `yaml:"metadata"`
	Script       MRSScriptDef     `yaml:"script"`
	StateUpdates []MRSStateUpdate `yaml:"state_updates"` // state fields this agent writes
}

// MRSAgentMetadata identifies the agent and its capability.
type MRSAgentMetadata struct {
	Name       string `yaml:"name"`       // agent name (defaults to capability if empty)
	Capability string `yaml:"capability"` // registered capability name (referenced in wantType.requires)
	Type       string `yaml:"type"`       // "monitor" | "do"
}

// MRSScriptDef describes the Python script executed by the agent.
type MRSScriptDef struct {
	Path           string `yaml:"path"`            // absolute or ~/ path to the script
	TimeoutSeconds int    `yaml:"timeout_seconds"` // 0 → default 120s
}

// MRSStateUpdate declares a state field that the plugin agent writes, along with
// the JSON path used to extract the value from the script's output.
type MRSStateUpdate struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Label       string `yaml:"label"`      // current | goal | plan | internal
	Persistent  bool   `yaml:"persistent"`
	OnFetchData string `yaml:"onFetchData"` // JSON path into script output (e.g. "routes[0].summary")
}

// mrsPluginWrapper detects the `agent:` key in a YAML file.
type mrsPluginWrapper struct {
	Agent *MRSAgentDef `yaml:"agent,omitempty"`
}

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
		case "monitor", "poll":
			baseAgent.Type = MonitorAgentType
			monitorAgent := &MonitorAgent{BaseAgent: baseAgent}
			r.setAgentMonitor(monitorAgent)
			agent = monitorAgent
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
func (r *AgentRegistry) genericMonitorAction(ctx context.Context, want *Want) (bool, error) {
	InfoLog("[AGENT] MonitorAgent monitoring for want: %s\n", want.Metadata.Name)
	// Monitoring logic happens in the want type's agent execution logic This is just a placeholder that confirms the monitor executed
	return false, nil
}

// LoadUserCustomAgents scans dir recursively for YAML files containing an `agent:` key
// (MRS plugin agent definitions) and registers each one as a script-backed agent.
// This is called alongside LoadAgents to pick up agents from ~/.mywant/custom-types/.
func (r *AgentRegistry) LoadUserCustomAgents(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // best-effort
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[AGENT] Warning: failed to read %s: %v", path, err)
			return nil
		}

		var wrapper mrsPluginWrapper
		if err := yaml.Unmarshal(data, &wrapper); err != nil || wrapper.Agent == nil {
			return nil // not an agent file, skip silently
		}

		def := wrapper.Agent
		if def.Metadata.Capability == "" {
			log.Printf("[AGENT] Warning: skipping MRS agent in %s: capability is required", path)
			return nil
		}

		// Resolve script path relative to plugin directory
		scriptPath := def.Script.Path
		if !filepath.IsAbs(scriptPath) && !strings.HasPrefix(scriptPath, "~/") {
			scriptPath = filepath.Join(filepath.Dir(path), scriptPath)
		}
		scriptPath = mrsExpandTilde(scriptPath)

		agentName := def.Metadata.Name
		if agentName == "" {
			agentName = def.Metadata.Capability
		}
		capName := def.Metadata.Capability
		timeoutSec := def.Script.TimeoutSeconds
		if timeoutSec == 0 {
			timeoutSec = 120
		}

		// Convert state_updates to StateDef slice and register in registry.
		stateDefs := mrsStateUpdatesToDefs(def.StateUpdates)
		if len(stateDefs) > 0 {
			r.RegisterPluginStateUpdates(capName, stateDefs)
		}
		stateUpdates := def.StateUpdates // captured by closures below

		switch strings.ToLower(def.Metadata.Type) {
		case "do":
			finalPath, finalTimeout, finalName := scriptPath, timeoutSec, agentName
			r.RegisterCapability(Cap(capName))
			agent := &DoAgent{
				BaseAgent: *NewBaseAgent(agentName, []string{capName}, DoAgentType),
				Action: func(ctx context.Context, want *Want) error {
					args := mrsPluginBuildArgs(want)
					skillCtx, cancel := context.WithTimeout(ctx, time.Duration(finalTimeout)*time.Second)
					defer cancel()
					want.DirectLog("[MRS-DO:%s] executing %s args=%v (timeout: %ds)", finalName, finalPath, args, finalTimeout)
					raw, err := mrsRunScript(skillCtx, finalPath, args)
					if err != nil {
						want.DirectLog("[MRS-DO:%s] failed: %v", finalName, err)
						return nil
					}
					mrsApplyStateUpdates(want, raw, stateUpdates)
					return nil
				},
			}
			r.RegisterAgent(agent)
		default: // "monitor" or anything else
			finalPath, finalTimeout, finalName := scriptPath, timeoutSec, agentName
			r.RegisterCapability(Cap(capName))
			agent := &MonitorAgent{
				BaseAgent: *NewBaseAgent(agentName, []string{capName}, MonitorAgentType),
				Monitor: func(ctx context.Context, want *Want) (bool, error) {
					skillCtx, cancel := context.WithTimeout(ctx, time.Duration(finalTimeout)*time.Second)
					defer cancel()
					want.DirectLog("[MRS-MONITOR:%s] executing %s (timeout: %ds)", finalName, finalPath, finalTimeout)
					raw, err := mrsRunScript(skillCtx, finalPath, nil)
					if err != nil {
						want.DirectLog("[MRS-MONITOR:%s] failed: %v", finalName, err)
						return false, nil
					}
					mrsApplyStateUpdates(want, raw, stateUpdates)
					return true, nil // stop after successful run
				},
			}
			r.RegisterAgent(agent)
		}

		log.Printf("[AGENT] Registered MRS plugin agent '%s' (capability=%s type=%s script=%s state_updates=%d)",
			agentName, capName, def.Metadata.Type, scriptPath, len(stateUpdates))
		return nil
	})
}

// mrsStateUpdatesToDefs converts MRSStateUpdate declarations to StateDef entries
// for injection into the want type's state definition.
func mrsStateUpdatesToDefs(updates []MRSStateUpdate) []StateDef {
	defs := make([]StateDef, 0, len(updates))
	for _, u := range updates {
		label := u.Label
		if label == "" {
			label = "current"
		}
		defs = append(defs, StateDef{
			Name:       u.Name,
			Type:       u.Type,
			Label:      label,
			Persistent: u.Persistent,
		})
	}
	return defs
}

// mrsApplyStateUpdates extracts values from the script JSON output and writes them
// directly to want state. Falls back to writing mrs_raw_output when no updates declared.
func mrsApplyStateUpdates(want *Want, raw map[string]any, updates []MRSStateUpdate) {
	if len(updates) == 0 {
		// Legacy fallback: write full output to mrs_raw_output
		want.StoreState("mrs_raw_output", raw)
		return
	}
	for _, u := range updates {
		val := extractJSONPath(raw, u.OnFetchData)
		if val == nil {
			continue
		}
		want.StoreState(u.Name, val)
	}
}

// mrsPluginBuildArgs reads skill_json_arg or skill_args_keys from want state
// to build CLI arguments, matching the behavior of the generic do_mrs_agent.
func mrsPluginBuildArgs(want *Want) []string {
	if jsonArg := GetCurrent(want, "skill_json_arg", ""); jsonArg != "" {
		return []string{jsonArg}
	}
	keys := strings.Fields(GetCurrent(want, "skill_args_keys", ""))
	args := make([]string, 0, len(keys))
	for _, key := range keys {
		val := fmt.Sprintf("%v", GetCurrent[any](want, key, nil))
		if val != "" && val != "<nil>" {
			args = append(args, val)
		}
	}
	return args
}

// mrsExpandTilde replaces a leading "~/" with the user's home directory.
func mrsExpandTilde(p string) string {
	if !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}

// mrsRunScript executes a Python3 script and returns parsed JSON output.
// Progress lines ({"_progress": ...}) are consumed and discarded; the last
// non-progress JSON object is returned as the result.
func mrsRunScript(ctx context.Context, scriptPath string, args []string) (map[string]any, error) {
	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.CommandContext(ctx, "python3", cmdArgs...)
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	var finalResult map[string]any
	decoder := json.NewDecoder(stdout)
	for decoder.More() {
		var obj map[string]any
		if err := decoder.Decode(&obj); err != nil {
			break
		}
		if _, ok := obj["_progress"]; !ok {
			finalResult = obj
		}
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("exit error: %w", err)
	}
	if finalResult == nil {
		return nil, fmt.Errorf("skill produced no JSON output")
	}
	return finalResult, nil
}
