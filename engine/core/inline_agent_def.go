package mywant

// InlineAgentDef defines an executable agent with inline script embedded in a YAML want type definition.
type InlineAgentDef struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`                             // "think", "do", "monitor"
	Runtime  string `json:"runtime" yaml:"runtime"`                       // "rego", "shell", "python"
	Script   string `json:"script" yaml:"script"`                         // inline script content
	Interval int    `json:"interval,omitempty" yaml:"interval,omitempty"` // execution interval in seconds; 0 = default
}

// AchievedWhenDef defines a declarative achievement condition for ScriptableWant.
// Replaces the need for a Go IsAchieved() implementation.
type AchievedWhenDef struct {
	Field    string `json:"field" yaml:"field"`       // current-labeled state field name
	Operator string `json:"operator" yaml:"operator"` // ==, !=, >, >=, <, <=
	Value    any    `json:"value" yaml:"value"`        // comparison value
}

// LifecycleHookDef defines actions executed at a want lifecycle event (onInitialize, onDelete).
type LifecycleHookDef struct {
	// Params copies want params into current state.
	// Key = current state field name, Value = param name to read from Spec.Params.
	Params map[string]string `json:"params,omitempty" yaml:"params,omitempty"`

	// Current sets current state fields to literal values.
	Current map[string]any `json:"current,omitempty" yaml:"current,omitempty"`

	// Plan sets plan state fields to literal values.
	Plan map[string]any `json:"plan,omitempty" yaml:"plan,omitempty"`

	// Goal sets goal state fields to literal values.
	Goal map[string]any `json:"goal,omitempty" yaml:"goal,omitempty"`

	// ExecuteAgents calls ExecuteAgents() after applying the state changes above.
	ExecuteAgents bool `json:"executeAgents,omitempty" yaml:"executeAgents,omitempty"`
}
