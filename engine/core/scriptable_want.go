package mywant

import (
	"context"
	"fmt"
	"time"
)

// ScriptableWant is a generic Progressable that executes YAML-defined inline agents.
// It is used automatically when a WantTypeDefinition contains InlineAgents but has
// no registered Go implementation.
type ScriptableWant struct {
	Want
}

// ScriptableLocals holds no runtime locals; state is managed entirely via Want state.
type ScriptableLocals struct{}

// Initialize copies all parameters to state using their declared labels,
// then executes the onInitialize lifecycle hook if defined.
func (s *ScriptableWant) Initialize() {
	if s.WantTypeDefinition == nil {
		return
	}
	// Build a quick lookup of state field label by name.
	labels := make(map[string]string, len(s.WantTypeDefinition.State))
	for _, sd := range s.WantTypeDefinition.State {
		labels[sd.Name] = sd.Label
	}

	for k, v := range s.Spec.Params {
		switch labels[k] {
		case "goal":
			s.SetGoal(k, v)
		case "plan":
			s.SetPlan(k, v)
		default:
			if _, isStateDefined := labels[k]; isStateDefined {
				s.SetCurrent(k, v)
			}
		}
	}

	if s.WantTypeDefinition.OnInitialize != nil {
		s.execLifecycleHook(s.WantTypeDefinition.OnInitialize)
	}
}

// OnDelete executes the onDelete lifecycle hook if defined.
func (s *ScriptableWant) OnDelete() {
	if s.WantTypeDefinition == nil || s.WantTypeDefinition.OnDelete == nil {
		return
	}
	s.execLifecycleHook(s.WantTypeDefinition.OnDelete)
}

// execLifecycleHook applies state changes declared in a lifecycle hook and
// optionally calls ExecuteAgents.
func (s *ScriptableWant) execLifecycleHook(hook *LifecycleHookDef) {
	for stateKey, paramKey := range hook.Params {
		if v, ok := s.Spec.Params[paramKey]; ok {
			s.SetCurrent(stateKey, fmt.Sprintf("%v", v))
		}
	}
	for k, v := range hook.Current {
		s.SetCurrent(k, v)
	}
	for k, v := range hook.Plan {
		s.SetPlan(k, v)
	}
	for k, v := range hook.Goal {
		s.SetGoal(k, v)
	}
	if hook.ExecuteAgents {
		if err := s.ExecuteAgents(); err != nil {
			s.DirectLog("[ScriptableWant] ExecuteAgents error in lifecycle hook: %v", err)
		}
	}
}

// IsAchieved evaluates the declarative AchievedWhen condition if defined,
// otherwise falls back to checking the predefined `achieved` current state field.
func (s *ScriptableWant) IsAchieved() bool {
	if s.WantTypeDefinition != nil && s.WantTypeDefinition.AchievedWhen != nil {
		aw := s.WantTypeDefinition.AchievedWhen
		actual := GetCurrent[any](&s.Want, aw.Field, nil)
		return evaluateAchievedWhen(actual, aw.Operator, aw.Value)
	}
	return GetCurrent(&s.Want, "achieved", false)
}

// Progress calls ExecuteAgents for any do-type inline agents.
// Think and Monitor agents are started automatically by StartBackgroundAgents().
func (s *ScriptableWant) Progress() {
	if err := s.ExecuteAgents(); err != nil {
		s.DirectLog("[ScriptableWant] ExecuteAgents error: %v", err)
	}
}

// evaluateAchievedWhen compares actual (from state) against expected using the operator.
func evaluateAchievedWhen(actual any, operator string, expected any) bool {
	// Convert to float64 for numeric comparisons.
	af, aOK := toFloat64(actual)
	ef, eOK := toFloat64(expected)
	if aOK && eOK {
		switch operator {
		case "==":
			return af == ef
		case "!=":
			return af != ef
		case ">":
			return af > ef
		case ">=":
			return af >= ef
		case "<":
			return af < ef
		case "<=":
			return af <= ef
		}
	}
	// Fall back to string/equality comparison.
	switch operator {
	case "==":
		return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
	case "!=":
		return fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", expected)
	}
	return false
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case float32:
		return float64(x), true
	}
	return 0, false
}

// createScriptableFactory returns a WantFactory for a YAML-only want type.
func createScriptableFactory(def *WantTypeDefinition) WantFactory {
	return func(metadata Metadata, spec WantSpec) Progressable {
		locals := &ScriptableLocals{}
		baseWant := NewWantWithLocals(metadata, spec, locals, def.Metadata.Name)
		return &ScriptableWant{Want: *baseWant}
	}
}

// registerInlineAgents registers Go agent functions for each InlineAgentDef in the definition
// into both agentFactoryRegistry (for deferred registration) and, if registry != nil,
// directly into the live AgentRegistry.
//
// Passing a non-nil registry is required when this is called AFTER
// RegisterAllKnownAgentImplementations() has already run (e.g., on server startup where
// StoreWantTypeDefinition is called after the agent registry has been fully populated).
//
// It also appends generated capability names to def.Requires so that
// StartBackgroundAgents() and ExecuteAgents() can resolve them automatically.
func registerInlineAgents(def *WantTypeDefinition, registry *AgentRegistry) {
	for _, ia := range def.InlineAgents {
		// Capability name is scoped to the want type to avoid collisions.
		capName := def.Metadata.Name + "__" + ia.Name
		runtime := resolveRuntime(ia.Runtime)

		// Capture loop variables for use in closures.
		agentScript := ia.Script
		agentRuntime := runtime
		capabilityName := capName

		interval := defaultInterval(ia.Type, ia.Interval)
		caps := []Capability{{Name: capabilityName, Gives: []string{capabilityName}}}

		switch ia.Type {
		case "think":
			thinkFn := func(_ context.Context, want *Want) error {
				return agentRuntime.ExecuteThink(want, agentScript)
			}
			RegisterThinkAgentType(ia.Name, caps, thinkFn)
			if registry != nil {
				registerThinkAgentDirect(registry, ia.Name, caps, thinkFn)
			}
			// Record custom interval.
			if interval != 2*time.Second {
				def.MonitorCapabilities = append(def.MonitorCapabilities, MonitorCapabilityDef{
					Capability:      capabilityName,
					IntervalSeconds: int(interval.Seconds()),
				})
			}

		case "do":
			doFn := func(_ context.Context, want *Want) error {
				return agentRuntime.ExecuteDo(want, agentScript)
			}
			RegisterDoAgentType(ia.Name, caps, doFn)
			if registry != nil {
				registerDoAgentDirect(registry, ia.Name, caps, doFn)
			}

		case "monitor":
			monFn := func(_ context.Context, want *Want) (bool, error) {
				return agentRuntime.ExecuteMonitor(want, agentScript)
			}
			RegisterMonitorAgentType(ia.Name, caps, monFn)
			if registry != nil {
				registerMonitorAgentDirect(registry, ia.Name, caps, monFn)
			}
		}

		// Add to Requires so the standard agent resolution picks it up.
		def.Requires = append(def.Requires, capabilityName)
	}
}

// registerThinkAgentDirect creates and registers a ThinkAgent directly into the registry.
func registerThinkAgentDirect(registry *AgentRegistry, name string, caps []Capability, think ThinkFunc) {
	capNames := make([]string, len(caps))
	for i, c := range caps {
		capNames[i] = c.Name
		registry.RegisterCapability(c)
	}
	agent := &ThinkAgent{
		BaseAgent: *NewBaseAgent(name, capNames, ThinkAgentType),
		Think:     think,
	}
	registry.RegisterAgent(agent)
}

// registerDoAgentDirect creates and registers a DoAgent directly into the registry.
func registerDoAgentDirect(registry *AgentRegistry, name string, caps []Capability, action func(context.Context, *Want) error) {
	capNames := make([]string, len(caps))
	for i, c := range caps {
		capNames[i] = c.Name
		registry.RegisterCapability(c)
	}
	agent := &DoAgent{
		BaseAgent: *NewBaseAgent(name, capNames, DoAgentType),
		Action:    action,
	}
	registry.RegisterAgent(agent)
}

// registerMonitorAgentDirect creates and registers a MonitorAgent directly into the registry.
func registerMonitorAgentDirect(registry *AgentRegistry, name string, caps []Capability, monitor func(context.Context, *Want) (bool, error)) {
	capNames := make([]string, len(caps))
	for i, c := range caps {
		capNames[i] = c.Name
		registry.RegisterCapability(c)
	}
	agent := &MonitorAgent{
		BaseAgent: *NewBaseAgent(name, capNames, MonitorAgentType),
		Monitor:   monitor,
	}
	registry.RegisterAgent(agent)
}

// defaultInterval returns the execution interval for an inline agent.
// Falls back to the standard defaults when interval == 0.
func defaultInterval(agentType string, intervalSec int) time.Duration {
	if intervalSec > 0 {
		return time.Duration(intervalSec) * time.Second
	}
	switch agentType {
	case "monitor":
		return 5 * time.Second
	default: // "think"
		return 2 * time.Second
	}
}
