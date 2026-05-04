package mywant

// InlineAgentDef defines an executable agent with inline script embedded in a YAML want type definition.
type InlineAgentDef struct {
	Name       string `json:"name" yaml:"name"`
	Type       string `json:"type" yaml:"type"`                                 // "think", "do", "monitor"
	Runtime    string `json:"runtime" yaml:"runtime"`                           // "rego", "shell", "python"
	Script     string `json:"script,omitempty" yaml:"script,omitempty"`         // inline script content
	ScriptFile string `json:"scriptFile,omitempty" yaml:"scriptFile,omitempty"` // path to external script file (relative to working dir)
	Interval   int    `json:"interval,omitempty" yaml:"interval,omitempty"`     // execution interval in seconds; 0 = default
}

// ConditionDef defines a single declarative state condition (field operator value).
type ConditionDef struct {
	Field    string `json:"field" yaml:"field"`       // current-labeled state field name
	Operator string `json:"operator" yaml:"operator"` // ==, !=, >, >=, <, <=
	Value    any    `json:"value" yaml:"value"`       // comparison value
}

// AchievedWhenDef is an alias kept for backward compatibility. Use ConditionDef directly.
type AchievedWhenDef = ConditionDef

// FinalizeWhen groups the conditions that determine how a ScriptableWant terminates.
// Both fields are optional; omitting one means that outcome is never triggered declaratively.
type FinalizeWhen struct {
	Achieved *ConditionDef `json:"achieved,omitempty" yaml:"achieved,omitempty"` // transition to WantStatusAchieved
	Failed   *ConditionDef `json:"failed,omitempty" yaml:"failed,omitempty"`     // transition to WantStatusFailed
}

// LifecycleHookDef defines actions executed at a want lifecycle event (onInitialize, onDelete, onAchieved).
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

	// MergeParent propagates key-value pairs to the parent want via MergeParentState().
	// Values support ${varName} interpolation from current state.
	MergeParent map[string]any `json:"mergeParent,omitempty" yaml:"mergeParent,omitempty"`

	// ExecuteAgents calls ExecuteAgents() after applying the state changes above.
	ExecuteAgents bool `json:"executeAgents,omitempty" yaml:"executeAgents,omitempty"`
}
