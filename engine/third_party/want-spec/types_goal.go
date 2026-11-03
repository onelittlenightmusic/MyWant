package want_spec

// WantTypePlan embeds planning instructions into a WantTypeDefinition.
//
// When a want of this type is deployed, the Planner reads this section and
// auto-derives + deploys child wants (monitors, intermediaries, terminal).
// The want type itself acts as the recipe declaration — no separate goal file needed.
//
// Example (inside wantType.yaml):
//
//	wantType:
//	  metadata:
//	    name: smartgolf_booking
//	    ...
//	  plan:
//	    achieve:
//	      - type: smartgolf_book
//	    monitor:
//	      - type: smartgolf_check_reserved
//	    hints:
//	      - for: smartgolf_book
//	        use: smartgolf_list_available
type WantTypePlan struct {
	Achieve     []PlanTarget     `yaml:"achieve" json:"achieve"`
	Monitor     []PlanTarget     `yaml:"monitor,omitempty" json:"monitor,omitempty"`
	Hints       []PlanHint       `yaml:"hints,omitempty" json:"hints,omitempty"`
	Constraints []PlanConstraint `yaml:"constraints,omitempty" json:"constraints,omitempty"`
}

// PlanTarget declares a child want type to deploy as part of a WantTypePlan.
type PlanTarget struct {
	// Type is the want type name (e.g. "smartgolf_book").
	Type string `yaml:"type" json:"type"`

	// Name is an optional instance name for the deployed want.
	// If omitted, the Planner generates a name from the type.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Params provides pre-filled parameter values for this target.
	// Values declared here override auto-wired values.
	Params map[string]any `yaml:"params,omitempty" json:"params,omitempty"`

	// Optional marks a target as non-critical: the overall plan does not fail
	// even if this target fails or is skipped.
	Optional bool `yaml:"optional,omitempty" json:"optional,omitempty"`

	// Description provides human-readable context for this target.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// When, if set on a monitor target, declares the "isSatisfied" condition.
	// The Planner automatically negates this condition and injects it as a
	// LabelCondition that gates every achieve-chain want via using: selectors.
	When *ConditionDef `yaml:"when,omitempty" json:"when,omitempty"`
}

// PlanHint provides optional guidance to influence which intermediate want type
// the Planner inserts between a provider and a consumer.
//
// Example:
//
//	hints:
//	  - for: smartgolf_book
//	    use: smartgolf_list_available
//	    note: "Use list + choice to let the user interactively pick a slot"
type PlanHint struct {
	// For is the achieve/terminal want type this hint applies to.
	For string `yaml:"for" json:"for"`

	// Use is the preferred want type to use as a provider for the "for" type.
	Use string `yaml:"use,omitempty" json:"use,omitempty"`

	// Note is a free-text explanation of the hint.
	Note string `yaml:"note,omitempty" json:"note,omitempty"`
}

// PlanConstraint expresses an optional condition that limits the plan's execution.
// Constraints are advisory; future versions may enforce them formally.
//
// Example:
//
//	constraints:
//	  - description: "Skip booking if already reserved"
//	    when:
//	      field: smartgolf_check_reserved.is_reserved
//	      operator: "=="
//	      value: true
type PlanConstraint struct {
	// Description is a human-readable explanation of the constraint.
	Description string `yaml:"description" json:"description"`

	// When is an optional declarative condition expression.
	When *ConditionDef `yaml:"when,omitempty" json:"when,omitempty"`
}

// ----- Planner output types -----

// PlannerResult holds the output of a single planning run.
// The Planner reads a WantTypePlan + the exposable index and produces a
// RecipeContent together with confidence metadata and a human-readable trace.
type PlannerResult struct {
	WantTypeName string        `json:"wantTypeName"`
	Recipe       RecipeContent `json:"recipe"`
	Confidence   string        `json:"confidence"` // "certain" | "inferred" | "unknown"
	Steps        []PlannerStep `json:"steps"`
	Warnings     []string      `json:"warnings,omitempty"`
}

// PlannerStep describes one inference step in the backward-chaining derivation.
type PlannerStep struct {
	// WantType is the want type being placed in the recipe.
	WantType string `json:"wantType"`

	// Role describes the want's role in the plan.
	// Values: "terminal" | "intermediate" | "monitor"
	Role string `json:"role"`

	// ProvidedBy is the upstream want type (if any) that supplies this want's inputs.
	ProvidedBy string `json:"providedBy,omitempty"`

	// Confidence describes how certain the Planner is about this step.
	// "certain"  — derived from exact name/type match on exposable fields.
	// "inferred" — derived from semantic similarity of descriptions.
	// "unknown"  — no match found; manual wiring required.
	Confidence string `json:"confidence"`

	// Reasoning is a human-readable explanation of how this step was derived.
	Reasoning string `json:"reasoning"`
}

// ExposableField is an entry in the Planner's capability index.
// Built by scanning all registered WantTypeDefinitions for exposable:true state fields.
type ExposableField struct {
	// WantType is the want type that exposes this field.
	WantType string `json:"wantType"`

	// Field is the StateDef.Name of the exposable field.
	Field string `json:"field"`

	// Description is StateDef.Description — used for semantic matching.
	Description string `json:"description"`

	// Type is the data type of the field (e.g. "string", "array", "boolean").
	Type string `json:"type"`
}
