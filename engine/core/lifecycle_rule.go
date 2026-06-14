package mywant

// LifecycleRuleMatch specifies which wants trigger a filtered lifecycle rule.
type LifecycleRuleMatch struct {
	Type   string            `json:"type,omitempty"`
	Owner  string            `json:"owner,omitempty"`
	Name   string            `json:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

// RegisterLifecycleRule and UnregisterLifecycleRule are set by the server package
// at startup, allowing types (agents) to register filtered webhook rules without
// creating an import cycle.  Before the server sets them, calls are no-ops.
var (
	RegisterLifecycleRule   func(event, targetURL string, match LifecycleRuleMatch, metadata map[string]any) string
	UnregisterLifecycleRule func(id string) bool
)
