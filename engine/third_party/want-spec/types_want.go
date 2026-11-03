package want_spec

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// UsingEntry is one entry in spec.using.
// All YAML/JSON keys except "when" are treated as label matchers for the provider.
// The optional "when" block declares a condition on the provider's data:
// Progress() is only unblocked when the received data satisfies the condition.
//
// Example YAML:
//
//	using:
//	  - name: smartgolf-check
//	    when:
//	      field: is_reserved
//	      operator: "=="
//	      value: false
type UsingEntry struct {
	// Labels holds all non-"when" keys (e.g. name, role, category, …)
	Labels map[string]string `json:"-" yaml:"-"`
	// When is an optional condition evaluated against the provider's data.
	When *ConditionDef `json:"when,omitempty" yaml:"-"`
}

// Label returns the value of a specific label key (helper used by engine).
func (e UsingEntry) Label(key string) string { return e.Labels[key] }

// ToLabelMap returns the label portion as a plain map (for PubSub matching).
func (e UsingEntry) ToLabelMap() map[string]string { return e.Labels }

// UnmarshalYAML handles: all non-"when" keys → Labels, "when" → When.
func (e *UsingEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("UsingEntry must be a YAML mapping")
	}
	e.Labels = make(map[string]string)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]
		if key == "when" {
			var cond ConditionDef
			if err := val.Decode(&cond); err != nil {
				return fmt.Errorf("using.when: %w", err)
			}
			e.When = &cond
		} else {
			e.Labels[key] = val.Value
		}
	}
	return nil
}

// MarshalYAML emits label keys at the top level and "when" nested.
func (e UsingEntry) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for k, v := range e.Labels {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: k},
			&yaml.Node{Kind: yaml.ScalarNode, Value: v},
		)
	}
	if e.When != nil {
		whenNode := &yaml.Node{}
		if err := whenNode.Encode(e.When); err != nil {
			return nil, err
		}
		// whenNode.Encode() makes whenNode itself a MappingNode (not a DocumentNode
		// wrapper), so whenNode is the mapping directly — not whenNode.Content[0].
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "when"},
			whenNode,
		)
	}
	return node, nil
}

// UnmarshalJSON handles: all non-"when" keys → Labels, "when" → When.
func (e *UsingEntry) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Labels = make(map[string]string)
	for k, v := range raw {
		if k == "when" {
			var cond ConditionDef
			if err := json.Unmarshal(v, &cond); err != nil {
				return fmt.Errorf("using.when: %w", err)
			}
			e.When = &cond
		} else {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				s = string(v) // fallback: keep raw value
			}
			e.Labels[k] = s
		}
	}
	return nil
}

// MarshalJSON emits label keys at top level and "when" nested.
func (e UsingEntry) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{}, len(e.Labels)+1)
	for k, v := range e.Labels {
		m[k] = v
	}
	if e.When != nil {
		m["when"] = e.When
	}
	return json.Marshal(m)
}

// OwnerReference represents a reference to an owner object
type OwnerReference struct {
	APIVersion         string `json:"apiVersion" yaml:"apiVersion"`
	Kind               string `json:"kind" yaml:"kind"`
	Name               string `json:"name" yaml:"name"`
	ID                 string `json:"id" yaml:"id"`
	Controller         bool   `json:"controller,omitempty" yaml:"controller,omitempty"`
	BlockOwnerDeletion bool   `json:"blockOwnerDeletion,omitempty" yaml:"blockOwnerDeletion,omitempty"`
}

// CorrelationEntry represents the correlation relationship between two Wants.
type CorrelationEntry struct {
	WantID   string   `json:"wantID"             yaml:"wantID"`
	Labels   []string `json:"labels"             yaml:"labels"`
	Rate     int      `json:"rate"               yaml:"rate"`
	DataType string   `json:"dataType,omitempty" yaml:"dataType,omitempty"`
}

// Metadata contains want identification and classification info
type Metadata struct {
	ID              string             `json:"id,omitempty" yaml:"id,omitempty"`
	Name            string             `json:"name" yaml:"name"`
	Type            string             `json:"type" yaml:"type"`
	Labels          map[string]string  `json:"labels" yaml:"labels"`
	OwnerReferences []OwnerReference   `json:"ownerReferences,omitempty" yaml:"ownerReferences,omitempty"`
	UpdatedAt       int64              `json:"updatedAt" yaml:"-"`
	IsSystemWant    bool               `json:"isSystemWant,omitempty" yaml:"isSystemWant,omitempty"`
	OrderKey        string             `json:"orderKey,omitempty" yaml:"orderKey,omitempty"`
	Correlation     []CorrelationEntry `json:"correlation,omitempty" yaml:"correlation,omitempty"`
	Series          string             `json:"series,omitempty" yaml:"series,omitempty"`
	Version         int                `json:"version,omitempty" yaml:"version,omitempty"`
}

// AlertRule is one promise about a want: while it holds, the want needs the
// person who placed it. Exactly one of When, Status or SilentFor is set.
//
//   - When: a state field compares as written (the ConditionDef a want
//     type's achievedWhen uses).
//   - Status: the want is in one of these statuses (failed, waiting_user_action…).
//   - SilentFor: nothing has changed for this long ("6h", "2d") — in Field
//     if it is given, in any of the want's state if not. It stopped without
//     failing.
type AlertRule struct {
	Name      string        `json:"name" yaml:"name"`
	When      *ConditionDef `json:"when,omitempty" yaml:"when,omitempty"`
	Status    []string      `json:"status,omitempty" yaml:"status,omitempty"`
	SilentFor string        `json:"silentFor,omitempty" yaml:"silentFor,omitempty"`
	Field     string        `json:"field,omitempty" yaml:"field,omitempty"`
}

// StateSubscription defines what state changes to monitor
type StateSubscription struct {
	WantName   string   `json:"wantName" yaml:"wantName"`
	StateKeys  []string `json:"stateKeys,omitempty" yaml:"stateKeys,omitempty"`
	Conditions []string `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	BufferSize int      `json:"bufferSize,omitempty" yaml:"bufferSize,omitempty"`
}

// ParamEntry represents a single parameter entry in array format
type ParamEntry struct {
	Key   string `json:"key" yaml:"key"`
	Value any    `json:"value" yaml:"value"`
}

// ExposeEntry declares a parameter or state exposure between scope levels.
//
// Supported combinations:
//   - Param + As:              top-down — receive a param from upper scope (global or parent) into local param.
//   - CurrentState + As:       bottom-up — push local current state to parent's state (or global state if no parent).
//   - CurrentState + AsGoal:   bottom-up — push local current state to parent's Goal-labeled state via SetGoal.
//   - CurrentState + AsPlan:   bottom-up — push local current state to parent's Plan-labeled state via SetPlan.
//   - CurrentState + AsGlobalParam: write local current state directly to a named global parameter.
type ExposeEntry struct {
	Param         string `json:"param,omitempty" yaml:"param,omitempty"`
	CurrentState  string `json:"currentState,omitempty" yaml:"currentState,omitempty"`
	As            string `json:"as,omitempty" yaml:"as,omitempty"`
	AsGoal        string `json:"asGoal,omitempty" yaml:"asGoal,omitempty"`
	AsPlan        string `json:"asPlan,omitempty" yaml:"asPlan,omitempty"`
	AsGlobalParam string `json:"asGlobalParam,omitempty" yaml:"asGlobalParam,omitempty"`
}

// NotificationFilter allows filtering received notifications
type NotificationFilter struct {
	SourcePattern string   `json:"sourcePattern" yaml:"sourcePattern"`
	StateKeys     []string `json:"stateKeys,omitempty" yaml:"stateKeys,omitempty"`
	ValuePattern  string   `json:"valuePattern,omitempty" yaml:"valuePattern,omitempty"`
}

// ParamRef allows a spec.params value to reference a global parameter by name,
// using the same fromGlobalParam convention as WhenSpec.
// Example YAML: budget: {fromGlobalParam: global_budget}
type ParamRef struct {
	FromGlobalParam string `json:"fromGlobalParam" yaml:"fromGlobalParam"`
}

// WhenSpec defines a scheduled execution time for a Want
type WhenSpec struct {
	At              string `json:"at,omitempty" yaml:"at,omitempty"`
	Every           string `json:"every,omitempty" yaml:"every,omitempty"`
	FromGlobalParam string `json:"fromGlobalParam,omitempty" yaml:"fromGlobalParam,omitempty"`
}

// Want is the canonical DTO representing a single want.
// It contains only the client-mutable fields (Metadata and Spec).
// Runtime-only fields (goroutines, channels, state maps) live in the engine's internal Want type.
type Want struct {
	Metadata Metadata `json:"metadata" yaml:"metadata"`
	Spec     WantSpec `json:"spec" yaml:"spec"`
}

// WantSpec contains the desired state configuration for a want
type WantSpec struct {
	Params              map[string]any       `json:"params" yaml:"params"`
	Exposes             []ExposeEntry        `json:"exposes,omitempty" yaml:"exposes,omitempty"`
	// Imports maps global state keys to internal state keys.
	// Imported fields are read-only: reads transparently return the current global state value;
	// writes are silently blocked. Values are never copied — they are always resolved live.
	Imports             map[string]string    `json:"imports,omitempty" yaml:"imports,omitempty"`
	Using               []UsingEntry         `json:"using,omitempty" yaml:"using,omitempty"`
	Recipe              string               `json:"recipe,omitempty" yaml:"recipe,omitempty"`
	StateSubscriptions  []StateSubscription  `json:"stateSubscriptions,omitempty" yaml:"stateSubscriptions,omitempty"`
	NotificationFilters []NotificationFilter `json:"notificationFilters,omitempty" yaml:"notificationFilters,omitempty"`
	Requires            []string             `json:"requires,omitempty" yaml:"requires,omitempty"`
	When                []WhenSpec           `json:"when,omitempty" yaml:"when,omitempty"`
	FinalResultField    string               `json:"finalResultField,omitempty" yaml:"finalResultField,omitempty"`
	ResetOnRestart      *bool                `json:"resetOnRestart,omitempty" yaml:"resetOnRestart,omitempty"`
	// AutoExpose, when true, automatically creates expose handlers for every state
	// field marked exposable in the want's WantTypeDefinition, writing each field's
	// value to the controlling parent want's state under the same key name.
	// Equivalent to listing all exposable fields in spec.exposes with as: <fieldName>.
	AutoExpose          bool                 `json:"autoExpose,omitempty" yaml:"autoExpose,omitempty"`
	// Alerts are the promises the person who placed this want made about it:
	// each one says when the want needs them. A want with no alert holding is
	// one they do not have to come back for.
	Alerts []AlertRule `json:"alerts,omitempty" yaml:"alerts,omitempty"`

	// UnknownFields is populated by UnmarshalJSON / UnmarshalYAML when the input
	// contains keys that this version of the spec does not recognise.  It is
	// intentionally omitted from JSON/YAML output so it does not round-trip.
	// Consumers (e.g. mywant engine) should log a warning when this is non-empty.
	UnknownFields []string `json:"-" yaml:"-"`
}
