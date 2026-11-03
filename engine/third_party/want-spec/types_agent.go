package want_spec

// AgentType defines the type of agent for execution strategies.
type AgentType string

const (
	DoAgentType      AgentType = "do"
	MonitorAgentType AgentType = "monitor"
	ThinkAgentType   AgentType = "think"
)

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
	AccessTypeRead       AccessType = "read"       // Agent may read from this field
	AccessTypeReadUpdate AccessType = "readUpdate" // Agent may read and write (default)
)

// StateAccessField declares a state field that an agent with this capability can access.
type StateAccessField struct {
	Name        string     `yaml:"name" json:"name"`
	Type        string     `yaml:"type,omitempty" json:"type,omitempty"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	AccessType  AccessType `yaml:"accessType,omitempty" json:"accessType,omitempty"` // default: readUpdate
}

// Capability represents an agent's functional capability with its dependencies.
type Capability struct {
	Name              string             `yaml:"name" json:"name"`
	Gives             []string           `yaml:"gives" json:"gives"`
	Description       string             `yaml:"description,omitempty" json:"description,omitempty"`
	StateAccess       []StateAccessField `yaml:"stateAccess,omitempty" json:"stateAccess,omitempty"`
	ParentStateAccess []StateAccessField `yaml:"parentStateAccess,omitempty" json:"parentStateAccess,omitempty"`
}

// CapabilityYAML is the root wrapper for capability-*.yaml files.
type CapabilityYAML struct {
	Capabilities []Capability `yaml:"capabilities" json:"capabilities"`
}

// AgentEntryDef describes an agent registered via an agent-*.yaml file.
type AgentEntryDef struct {
	Name         string     `yaml:"name" json:"name"`
	Type         AgentType  `yaml:"type" json:"type"`
	Runtime      AgentRuntime `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Capabilities []string   `yaml:"capabilities" json:"capabilities"`
	Uses         []string   `yaml:"uses,omitempty" json:"uses,omitempty"`
}

// AgentYAML is the root wrapper for agent-*.yaml files.
type AgentYAML struct {
	Agents []AgentEntryDef `yaml:"agents" json:"agents"`
}

// MRSAgentDef describes a Machine-Readable Skill agent declared in a plugin agent.yaml.
// It is parsed from the top-level `agent:` key.
type MRSAgentDef struct {
	Metadata     MRSAgentMetadata `yaml:"metadata" json:"metadata"`
	Script       MRSScriptDef     `yaml:"script" json:"script"`
	StateUpdates []MRSStateUpdate `yaml:"state_updates,omitempty" json:"state_updates,omitempty"`
}

// MRSAgentMetadata identifies the agent and its capability.
type MRSAgentMetadata struct {
	Name       string `yaml:"name,omitempty" json:"name,omitempty"` // defaults to Capability if empty
	Capability string `yaml:"capability" json:"capability"`
	Type       string `yaml:"type" json:"type"` // "monitor" | "do"
}

// MRSScriptDef describes the Python script executed by the agent.
type MRSScriptDef struct {
	Path           string `yaml:"path" json:"path"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"` // 0 → default 120s
}

// MRSStateUpdate declares a state field that the plugin agent writes, along with
// the JSON path used to extract the value from the script's output.
type MRSStateUpdate struct {
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type" json:"type"`
	Label       string `yaml:"label,omitempty" json:"label,omitempty"` // current | goal | plan | internal
	Persistent  bool   `yaml:"persistent,omitempty" json:"persistent,omitempty"`
	OnFetchData string `yaml:"onFetchData,omitempty" json:"onFetchData,omitempty"`
}
