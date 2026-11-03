package want_spec

// RecipeWant represents a want in recipe format (aligned with Want structure)
type RecipeWant struct {
	Metadata Metadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Spec     WantSpec `yaml:"spec,omitempty" json:"spec,omitempty"`

	// Legacy flattened fields for backward compatibility
	Name        string              `yaml:"name,omitempty" json:"name,omitempty"`
	Type        string              `yaml:"type,omitempty" json:"type,omitempty"`
	Labels      map[string]string   `yaml:"labels,omitempty" json:"labels,omitempty"`
	Params      map[string]any      `yaml:"params,omitempty" json:"params,omitempty"`
	Using       []UsingEntry        `yaml:"using,omitempty" json:"using,omitempty"`
	Requires    []string            `yaml:"requires,omitempty" json:"requires,omitempty"`
	RecipeAgent bool                `yaml:"recipeAgent,omitempty" json:"recipeAgent,omitempty"`
}

// GenericRecipe represents the top-level recipe structure
type GenericRecipe struct {
	Recipe RecipeContent `yaml:"recipe" json:"recipe"`
}

// RecipeResult defines how to compute results from recipe execution
type RecipeResult []RecipeResultSpec

// RecipeResultSpec specifies which want and state field to use for result computation
type RecipeResultSpec struct {
	WantName    string `yaml:"want_name" json:"want_name"`
	StateField  string `yaml:"state_field,omitempty" json:"state_field,omitempty"`
	StatName    string `yaml:"stat_name,omitempty" json:"stat_name,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// RecipeExample represents example deployment configuration for one-click deployment
type RecipeExample struct {
	Wants []RecipeWant `yaml:"wants,omitempty" json:"wants,omitempty"`
}

// RecipeExampleDef defines a named example usage of a recipe with pre-filled parameters
type RecipeExampleDef struct {
	Name             string         `json:"name" yaml:"name"`
	Description      string         `json:"description" yaml:"description"`
	Params           map[string]any `json:"params" yaml:"params"`
	ExpectedBehavior string         `json:"expectedBehavior,omitempty" yaml:"expectedBehavior,omitempty"`
}

// RecipeContent contains the actual recipe data
type RecipeContent struct {
	Metadata         GenericRecipeMetadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Wants            []RecipeWant          `yaml:"wants,omitempty" json:"wants,omitempty"`
	Parameters       []ParameterDef        `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Result           *RecipeResult         `yaml:"result,omitempty" json:"result,omitempty"`
	Example          *RecipeExample        `yaml:"example,omitempty" json:"example,omitempty"`
	Examples         []RecipeExampleDef    `yaml:"examples,omitempty" json:"examples,omitempty"`
	State            []StateDef            `yaml:"state,omitempty" json:"state,omitempty"`
	FinalResultField string                `yaml:"finalResultField,omitempty" json:"finalResultField,omitempty"`

	// Achieve declares terminal goals for Planner-based auto-derivation of Wants.
	// When non-empty, the Planner derives the child wants automatically.
	// Manually-specified Wants (above) are appended after the derived ones.
	Achieve []PlanTarget `yaml:"achieve,omitempty" json:"achieve,omitempty"`

	// IsSatisfied declares a pre-check want that short-circuits the recipe:
	// if the When condition evaluates to true, the recipe is immediately marked
	// as achieved (the Achieve chain is skipped).
	IsSatisfied *RecipeIsSatisfied `yaml:"isSatisfied,omitempty" json:"isSatisfied,omitempty"`

	// Hints provides Planner guidance for intermediate want type selection.
	Hints []PlanHint `yaml:"hints,omitempty" json:"hints,omitempty"`

	// LabelConditions maps label selectors to conditions that are automatically
	// injected into any using: entry matching those labels.
	// Avoids repeating the same when: on every subscriber.
	LabelConditions []LabelCondition `yaml:"labelConditions,omitempty" json:"labelConditions,omitempty"`
}

// LabelCondition maps a label selector to a condition that is automatically
// injected into any using: entry whose labels contain the given match.
// Defined once in the recipe; applies to ALL subscribers using those labels.
type LabelCondition struct {
	// Match is the label key-value pairs to match against using: entries.
	Match map[string]string `yaml:"match" json:"match"`
	// When is the condition to inject when the match succeeds.
	When ConditionDef `yaml:"when" json:"when"`
}

// RecipeIsSatisfied defines a pre-check want and the condition that means
// "goal already achieved — skip the Achieve chain."
type RecipeIsSatisfied struct {
	// Type is the want type to run as a satisfaction check (e.g. "smartgolf_check_reserved").
	Type string `yaml:"type" json:"type"`

	// Name is an optional instance name for the check want.
	// Defaults to slugify(Type) + "-satisfied-check".
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// When is the condition on the check want's (exposed) state that indicates satisfaction.
	// Example: {field: is_reserved, operator: "==", value: true}
	When ConditionDef `yaml:"when" json:"when"`
}

// ParameterDefsToMap converts []ParameterDef to a name→default map for substitution.
func ParameterDefsToMap(params []ParameterDef) map[string]any {
	m := make(map[string]any, len(params))
	for _, p := range params {
		if p.Default != nil {
			m[p.Name] = p.Default
		}
	}
	return m
}

// GenericRecipeMetadata contains recipe information
type GenericRecipeMetadata struct {
	ID          string `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Version     string `yaml:"version" json:"version"`
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	CustomType  string `yaml:"custom_type,omitempty" json:"custom_type,omitempty"`
	Category    string `yaml:"category,omitempty" json:"category,omitempty"`
}
