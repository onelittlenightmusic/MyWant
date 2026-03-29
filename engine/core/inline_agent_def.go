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
