package want_spec

import (
	"fmt"
	"strings"
)

// Plan is what is to be done about a question — a declaration, nothing done
// yet. A small on-device model works it out with the server's help
// (/plans/brief, /plans/params) and posts it; an Act that names it carries it
// out.
//
//	apiVersion: mywant.io/v1
//	kind: Plan
//	metadata:
//	  name: plan-1730…            # given by the server
//	  createdAt: 2026-11-03T…
//	spec:
//	  question: 最新のゴルフのスコアは？
//	  action: existing             # existing | new | none
//	  target: smartgolf-check-reserved-instance
//	  capability: rounds           # existing: what is asked of it
//	  type: ""                     # new: the type to make
//	  params: {}                   # new: its parameters
//
//	POST   /api/v1/plans          Plan (YAML or JSON) → Plan, named
//	GET    /api/v1/plans          PlanList, newest first (?limit=)
//	GET    /api/v1/plans/{name}   Plan
//	DELETE /api/v1/plans/{name}
//
// Read back as JSON, or YAML with ?format=yaml. See spec/plan-spec.yaml.
type Plan struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Metadata   ResourceMeta `yaml:"metadata" json:"metadata"`
	Spec       PlanSpec     `yaml:"spec" json:"spec"`
}

// PlanKind is a Plan's kind.
const PlanKind = "Plan"

// What a plan does.
const (
	// PlanActionExisting: a want already on the board answers — pointed at,
	// or a capability of it used.
	PlanActionExisting = "existing"
	// PlanActionNew: a want of Type is made, with Params.
	PlanActionNew = "new"
	// PlanActionNone: nothing on the board; the answer is words alone.
	PlanActionNone = "none"
)

// PlanNone is the decision's own word for nothing chosen.
const PlanNone = "-"

// PlanSpec is what is to be done.
type PlanSpec struct {
	Question string `yaml:"question" json:"question"`
	// Action is existing, new or none.
	Action string `yaml:"action" json:"action"`
	// Target is the want an existing plan is about.
	Target string `yaml:"target,omitempty" json:"target,omitempty"`
	// Capability is what is asked of Target: one of its type's capabilities,
	// or the standard restart. Empty: it is only pointed at.
	Capability string `yaml:"capability,omitempty" json:"capability,omitempty"`
	// Type is the want type a new plan makes.
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	// Params are the new want's parameters.
	Params map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
}

// PlanList is GET /plans.
type PlanList struct {
	Plans []Plan `yaml:"plans" json:"plans"`
}

// Normalize makes "-" (nothing chosen) empty.
func (s *PlanSpec) Normalize() {
	for _, f := range []*string{&s.Target, &s.Capability, &s.Type} {
		if strings.TrimSpace(*f) == PlanNone {
			*f = ""
		}
	}
}

// Validate says what is wrong with a plan as posted: another kind, or an
// action there is not.
func (p *Plan) Validate() error {
	if p.Kind != "" && p.Kind != PlanKind {
		return fmt.Errorf("kind must be %s", PlanKind)
	}
	switch p.Spec.Action {
	case PlanActionExisting, PlanActionNew, PlanActionNone:
		return nil
	}
	return fmt.Errorf("spec.action must be %s, %s or %s", PlanActionExisting, PlanActionNew, PlanActionNone)
}
