package want_spec

// StateLabel is the set of valid label values for a StateDef.
type StateLabel string

const (
	StateLabelGoal     StateLabel = "goal"
	StateLabelCurrent  StateLabel = "current"
	StateLabelPlan     StateLabel = "plan"
	StateLabelInternal StateLabel = "internal"
)

// WantTypeDefinition represents a complete want type definition
type WantTypeDefinition struct {
	Metadata            WantTypeMetadata       `json:"metadata" yaml:"metadata"`
	Parameters          []ParameterDef         `json:"parameters" yaml:"parameters"`
	State               []StateDef             `json:"state" yaml:"state"`
	Connectivity        ConnectivityDef        `json:"connectivity" yaml:"connectivity"`
	Connect             *RequireSpec           `json:"connect,omitempty" yaml:"connect,omitempty"`
	Require             *RequireSpec           `json:"require,omitempty" yaml:"require,omitempty"`
	UsageLimit          *UsageLimitSpec        `json:"usageLimit,omitempty" yaml:"usageLimit,omitempty"`
	Requires            []string               `json:"requires,omitempty" yaml:"requires,omitempty"`
	MonitorCapabilities []MonitorCapabilityDef `json:"monitorCapabilities,omitempty" yaml:"monitorCapabilities,omitempty"`
	Capabilities        []WantCapabilityDef    `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	FinalResultField    string                 `json:"finalResultField,omitempty" yaml:"finalResultField,omitempty"`
	GlobalOverrideFrom  string                 `json:"globalOverrideFrom,omitempty" yaml:"globalOverrideFrom,omitempty"`
	Agents              []AgentDef             `json:"agents" yaml:"agents"`
	InlineAgents        []InlineAgentDef       `json:"inlineAgents,omitempty" yaml:"inlineAgents,omitempty"`
	FinalizeWhen        *FinalizeWhen          `json:"finalizeWhen,omitempty" yaml:"finalizeWhen,omitempty"`
	AchievedWhen        *ConditionDef          `json:"achievedWhen,omitempty" yaml:"achievedWhen,omitempty"`
	OnInitialize        *LifecycleHookDef      `json:"onInitialize,omitempty" yaml:"onInitialize,omitempty"`
	OnDelete            *LifecycleHookDef      `json:"onDelete,omitempty" yaml:"onDelete,omitempty"`
	OnAchieved          *LifecycleHookDef      `json:"onAchieved,omitempty" yaml:"onAchieved,omitempty"`
	Constraints         []ConstraintDef        `json:"constraints" yaml:"constraints"`
	Examples            []ExampleDef           `json:"examples" yaml:"examples"`
	RelatedTypes        []string               `json:"relatedTypes" yaml:"relatedTypes"`
	SeeAlso             []string               `json:"seeAlso" yaml:"seeAlso"`
	GoType              string                 `json:"goType,omitempty" yaml:"goType,omitempty"`
}

// WantTypeMetadata contains want type identity and classification
type WantTypeMetadata struct {
	Name        string            `json:"name" yaml:"name"`
	Title       string            `json:"title" yaml:"title"`
	Description string            `json:"description" yaml:"description"`
	Version     string            `json:"version" yaml:"version"`
	Category    string            `json:"category" yaml:"category"`
	Pattern     string            `json:"pattern" yaml:"pattern"`
	SystemType  bool              `json:"system_type,omitempty" yaml:"system_type,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// ParameterDef defines a parameter for want type configuration
type ParameterDef struct {
	Name                       string          `json:"name" yaml:"name"`
	Description                string          `json:"description" yaml:"description"`
	Type                       string          `json:"type" yaml:"type"`
	Label                      string          `json:"label,omitempty" yaml:"label,omitempty"`
	Default                    any             `json:"default,omitempty" yaml:"default,omitempty"`
	DefaultGlobalParameter     string          `json:"defaultGlobalParameter,omitempty" yaml:"defaultGlobalParameter,omitempty"`
	DefaultGlobalParameterFrom string          `json:"defaultGlobalParameterFrom,omitempty" yaml:"defaultGlobalParameterFrom,omitempty"`
	Required                   bool            `json:"required" yaml:"required"`
	Validation                 ValidationRules `json:"validation,omitempty" yaml:"validation,omitempty"`
	Example                    any             `json:"example,omitempty" yaml:"example,omitempty"`
	// SubType declares a semantic category for recording things and autocomplete.
	// Examples: "location", "city", "date", "time", "port"
	SubType string `json:"subType,omitempty" yaml:"subType,omitempty"`
	// Accepts widens what may be PUT here without changing what a value entered
	// here IS. SubType is one answer to two questions — what this field takes,
	// and what a value typed into it gets remembered as — and they are not
	// always the same question. A route's `from` takes a station, a named place,
	// a coordinate or an address, and a name typed into it is still a station.
	//
	// SubType is always accepted and need not be repeated here. Empty means the
	// field takes its SubType and nothing else, which is what every parameter
	// written before this meant.
	Accepts []string `json:"accepts,omitempty" yaml:"accepts,omitempty"`
	// RecordThing controls whether user-entered values are remembered as things.
	// Defaults to true when SubType is set. Set to false for sensitive values (e.g. secrets).
	RecordThing *bool `json:"recordThing,omitempty" yaml:"recordThing,omitempty"`
	// RecordMemo is what RecordThing was called before things were called
	// things. A want type YAML in the wild still says `recordMemo`, and it means
	// the same, so it is read — and never written back. Use ShouldRecordThing.
	//
	// Deprecated: set RecordThing.
	RecordMemo *bool `json:"recordMemo,omitempty" yaml:"recordMemo,omitempty"`
	// Title is what the parameter is called on screen, when its name — an
	// identifier, ASCII by rule — is not what anybody would call it. A web
	// want's field saved from a Japanese page is `field_1` by name and
	// 「ポスト本文」 by title.
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	// BackgroundImage is a picture of what this parameter stands for, drawn
	// behind its card: a URL, or a data: URL. A web want's parameters are the
	// objects saved on its page, and each is shown as it looked there.
	BackgroundImage string `json:"backgroundImage,omitempty" yaml:"backgroundImage,omitempty"`
}

// AcceptedSubTypes is everything this parameter takes: its own SubType first,
// then whatever Accepts adds. Callers matching a value against a parameter walk
// this rather than SubType, so a parameter that widened what it takes does not
// have to be special-cased anywhere.
func (p ParameterDef) AcceptedSubTypes() []string {
	if p.SubType == "" && len(p.Accepts) == 0 {
		return nil
	}
	out := make([]string, 0, len(p.Accepts)+1)
	seen := map[string]bool{}
	for _, s := range append([]string{p.SubType}, p.Accepts...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// ShouldRecordThing reports whether a value entered for this parameter is
// remembered. The new spelling wins; the old one is honoured; silence means yes,
// because a parameter that declares a SubType is asking to be remembered.
func (p ParameterDef) ShouldRecordThing() bool {
	if p.RecordThing != nil {
		return *p.RecordThing
	}
	if p.RecordMemo != nil {
		return *p.RecordMemo
	}
	return true
}

// ValidationRules defines validation constraints for parameters
type ValidationRules struct {
	Min     *float64 `json:"min,omitempty" yaml:"min,omitempty"`
	Max     *float64 `json:"max,omitempty" yaml:"max,omitempty"`
	Pattern string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Enum    []any    `json:"enum,omitempty" yaml:"enum,omitempty"`
}

// StateDef defines a state key for a want type
type StateDef struct {
	Name         string     `json:"name" yaml:"name"`
	Description  string     `json:"description" yaml:"description"`
	Type         string     `json:"type" yaml:"type"`
	SubType      string     `json:"subType,omitempty" yaml:"subType,omitempty"`
	Label        StateLabel `json:"label,omitempty" yaml:"label,omitempty"`
	Persistent   bool       `json:"persistent" yaml:"persistent"`
	InitialValue any        `json:"initialValue,omitempty" yaml:"initialValue,omitempty"`
	Example      any        `json:"example,omitempty" yaml:"example,omitempty"`
	OnFetchData  string     `json:"onFetchData,omitempty" yaml:"onFetchData,omitempty"`
	FetchFrom    string     `json:"fetchFrom,omitempty" yaml:"fetchFrom,omitempty"`
	Exposable    bool       `json:"exposable,omitempty" yaml:"exposable,omitempty"`
	// Importable marks the mirror of Exposable: a slot this want type wants
	// filled from somewhere else, rather than a value it offers. Exposable said
	// what a want can give; nothing said what it is asking for, so a want with
	// an obvious input had no way to advertise it and the matcher had to guess
	// from names and types alone.
	Importable bool `json:"importable,omitempty" yaml:"importable,omitempty"`
	Volatile   bool `json:"volatile,omitempty" yaml:"volatile,omitempty"`
}

// ConnectivityDef defines input/output patterns for a want type
type ConnectivityDef struct {
	Inputs  []ChannelDef `json:"inputs" yaml:"inputs"`
	Outputs []ChannelDef `json:"outputs" yaml:"outputs"`
}

// ChannelDef defines an input or output channel
type ChannelDef struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	DataType    string `json:"data_type,omitempty" yaml:"data_type,omitempty"`
	Description string `json:"description" yaml:"description"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Multiple    bool   `json:"multiple,omitempty" yaml:"multiple,omitempty"`
}

// ConnectionSpec represents a single connection definition
type ConnectionSpec struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	DataType    string `json:"data_type,omitempty" yaml:"data_type,omitempty"`
	Description string `json:"description" yaml:"description"`
	Required    bool   `json:"required" yaml:"required"`
	Multiple    bool   `json:"multiple" yaml:"multiple"`
}

// RequireSpec defines structured connectivity requirements
type RequireSpec struct {
	Type      string           `json:"type" yaml:"type"`
	Providers []ConnectionSpec `json:"providers,omitempty" yaml:"providers,omitempty"`
	Users     []ConnectionSpec `json:"users,omitempty" yaml:"users,omitempty"`
}

// UsageLimitSpec defines want usage limits in YAML format
type UsageLimitSpec struct {
	Providers struct {
		Min int `json:"min" yaml:"min"`
		Max int `json:"max" yaml:"max"`
	} `json:"providers" yaml:"providers"`
	Users struct {
		Min int `json:"min" yaml:"min"`
		Max int `json:"max" yaml:"max"`
	} `json:"users" yaml:"users"`
	Description string `json:"description" yaml:"description"`
}

// MonitorCapabilityDef describes a MonitorAgent capability
type MonitorCapabilityDef struct {
	Capability      string `json:"capability" yaml:"capability"`
	IntervalSeconds int    `json:"intervalSeconds,omitempty" yaml:"intervalSeconds,omitempty"`
}

// WantCapabilityDef is one thing a want of a type can answer beyond its own
// result: said in words for the robot to choose by (Description), and done by
// the server — the data named in Reads gathered and handed with Prompt to the
// model that words the answer, or Skill run. Not an agent's capability
// (Requires, MonitorCapabilities): those are what a want needs, these what it
// offers.
type WantCapabilityDef struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	// Reads: answers | answer_photos | state.
	Reads  []string `json:"reads,omitempty" yaml:"reads,omitempty"`
	Prompt string   `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	// Skill is a Machine-Readable Skill, relative to the type's directory.
	Skill string `json:"skill,omitempty" yaml:"skill,omitempty"`
}

// AgentDef defines agent integration for a want type
type AgentDef struct {
	Name        string `json:"name" yaml:"name"`
	Role        string `json:"role" yaml:"role"`
	Description string `json:"description" yaml:"description"`
	Example     string `json:"example,omitempty" yaml:"example,omitempty"`
}

// InlineAgentDef defines an executable agent with inline script
type InlineAgentDef struct {
	Name       string `json:"name" yaml:"name"`
	Type       string `json:"type" yaml:"type"`
	Runtime    string `json:"runtime" yaml:"runtime"`
	Script     string `json:"script,omitempty" yaml:"script,omitempty"`
	ScriptFile string `json:"scriptFile,omitempty" yaml:"scriptFile,omitempty"`
	Interval   int    `json:"interval,omitempty" yaml:"interval,omitempty"`
}

// ConditionDef defines a single declarative state condition
type ConditionDef struct {
	Field    string `json:"field" yaml:"field"`
	Operator string `json:"operator" yaml:"operator"`
	Value    any    `json:"value" yaml:"value"`
}

// FinalizeWhen groups the conditions that determine how a ScriptableWant terminates
type FinalizeWhen struct {
	Achieved *ConditionDef `json:"achieved,omitempty" yaml:"achieved,omitempty"`
	Failed   *ConditionDef `json:"failed,omitempty" yaml:"failed,omitempty"`
}

// LifecycleHookDef defines actions executed at a want lifecycle event
type LifecycleHookDef struct {
	Params        map[string]string `json:"params,omitempty" yaml:"params,omitempty"`
	Current       map[string]any    `json:"current,omitempty" yaml:"current,omitempty"`
	Plan          map[string]any    `json:"plan,omitempty" yaml:"plan,omitempty"`
	Goal          map[string]any    `json:"goal,omitempty" yaml:"goal,omitempty"`
	MergeParent   map[string]any    `json:"mergeParent,omitempty" yaml:"mergeParent,omitempty"`
	ExecuteAgents bool              `json:"executeAgents,omitempty" yaml:"executeAgents,omitempty"`
}

// ConstraintDef defines business logic constraints
type ConstraintDef struct {
	Description string `json:"description" yaml:"description"`
	Validation  string `json:"validation" yaml:"validation"`
}

// ExampleDef defines an example usage of a want type
type ExampleDef struct {
	Name             string         `json:"name" yaml:"name"`
	Description      string         `json:"description" yaml:"description"`
	Want             map[string]any `json:"want" yaml:"want"`
	ExpectedBehavior string         `json:"expectedBehavior" yaml:"expectedBehavior"`
}

// WantTypeWrapper is the top-level YAML structure for wantType-only files
type WantTypeWrapper struct {
	WantType WantTypeDefinition `yaml:"wantType"`
}

// ConnectivityMetadata defines want connectivity requirements and constraints
type ConnectivityMetadata struct {
	RequiredInputs  int    `json:"required_inputs"`
	RequiredOutputs int    `json:"required_outputs"`
	MaxInputs       int    `json:"max_inputs"`
	MaxOutputs      int    `json:"max_outputs"`
	WantType        string `json:"want_type"`
	Description     string `json:"description"`
}
