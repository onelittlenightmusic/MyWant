package planner

import (
	"fmt"
	"strings"

	ws "github.com/onelittlenightmusic/want-spec"
)

// Planner derives a Recipe from a WantTypePlan by backward-chaining over the
// ExposableIndex. It uses formal name/type matching for "certain" steps and
// description-based inference for "inferred" steps.
type Planner struct {
	// index is the capability index built from registered want type definitions.
	index *ExposableIndex

	// defs is the full set of want type definitions, used to look up parameter requirements.
	defs map[string]*ws.WantTypeDefinition
}

// New creates a Planner backed by the given ExposableIndex and definition map.
func New(idx *ExposableIndex, defs map[string]*ws.WantTypeDefinition) *Planner {
	return &Planner{index: idx, defs: defs}
}

// PlanFromWantType derives a Recipe from a want type's Plan section.
// typeName is the name of the parent want type (used for recipe naming).
// category is used for metadata.
// plan contains the achieve/monitor/hints/constraints declarations.
// globalParams are pre-filled parameter values (e.g. from the want's spec.params).
func (p *Planner) PlanFromWantType(typeName, category string, plan *ws.WantTypePlan, globalParams map[string]any) ws.PlannerResult {
	if plan == nil {
		return ws.PlannerResult{
			WantTypeName: typeName,
			Confidence:   "certain",
		}
	}
	if globalParams == nil {
		globalParams = map[string]any{}
	}

	var steps []ws.PlannerStep
	var recipeWants []ws.RecipeWant
	var warnings []string
	overallConf := "certain"

	// ── Monitor targets ──────────────────────────────────────────────────────
	// Track gate labels for monitors that carry a When condition (isSatisfied-style).
	type gateInfo struct {
		labelKey   string
		labelValue string
		negated    ws.ConditionDef
	}
	var gates []gateInfo

	for _, mon := range plan.Monitor {
		w, step := p.buildMonitorWant(mon)
		if mon.When != nil {
			// Assign a unique gate label to this monitor want.
			gateLabelKey := "isSatisfied-gate"
			gateLabelValue := w.Metadata.Name
			if w.Metadata.Labels == nil {
				w.Metadata.Labels = make(map[string]string)
			}
			w.Metadata.Labels[gateLabelKey] = gateLabelValue
			gates = append(gates, gateInfo{
				labelKey:   gateLabelKey,
				labelValue: gateLabelValue,
				negated:    negateCondition(*mon.When),
			})
		}
		recipeWants = append(recipeWants, w)
		steps = append(steps, step)
	}

	// ── Achieve targets (backward chain) ────────────────────────────────────
	achieveStart := len(recipeWants) // index where achieve wants begin
	for _, target := range plan.Achieve {
		chain, warn := p.backwardChain(target, plan.Hints, globalParams)
		recipeWants = append(recipeWants, chain.wants...)
		steps = append(steps, chain.steps...)
		warnings = append(warnings, warn...)
		if lowerConf(chain.confidence, overallConf) {
			overallConf = chain.confidence
		}
	}

	// ── Gate wiring ──────────────────────────────────────────────────────────
	// For each isSatisfied gate, inject an import for the coordinator-managed
	// _achieve_gate_open key into every achieve-chain want. The coordinator sets
	// this key to true when the isSatisfied condition is NOT met (achieve chain
	// should run), and clears it to nil when the condition IS met (already
	// satisfied). hasUnresolvedImports() blocks Progress() while the key is nil.
	if len(gates) > 0 {
		for i := achieveStart; i < len(recipeWants); i++ {
			w := recipeWants[i]
			if w.Spec.Imports == nil {
				w.Spec.Imports = map[string]string{}
			}
			w.Spec.Imports["_achieve_gate_open"] = "_achieve_gate_open"
		}
	}

	recipeName := typeName + "-auto"
	recipe := ws.RecipeContent{
		Metadata: ws.GenericRecipeMetadata{
			Name:        recipeName,
			Description: fmt.Sprintf("Auto-derived from want type %q plan section", typeName),
			Version:     "1.0",
			Category:    category,
		},
		Wants: recipeWants,
	}

	return ws.PlannerResult{
		WantTypeName: typeName,
		Recipe:       recipe,
		Confidence:   overallConf,
		Steps:        steps,
		Warnings:     warnings,
	}
}

// negateCondition returns the logical negation of a ConditionDef.
// An empty field defaults to "final_result"; an empty operator defaults to "== true".
func negateCondition(c ws.ConditionDef) ws.ConditionDef {
	neg := c
	if neg.Field == "" {
		neg.Field = "final_result"
	}
	switch c.Operator {
	case "", "==":
		// Negate "== X" as "== (not X)" rather than "!= X".
		// Using "!= X" would match null/unset values (null != true == true),
		// causing the gate to open before the check want has run.
		// "== false" correctly requires the field to be explicitly false.
		neg.Operator = "=="
		if c.Value == true || c.Operator == "" {
			neg.Value = false
		} else if c.Value == false {
			neg.Value = true
		} else {
			neg.Operator = "!="
		}
	case "!=":
		neg.Operator = "=="
	case ">":
		neg.Operator = "<="
	case ">=":
		neg.Operator = "<"
	case "<":
		neg.Operator = ">="
	case "<=":
		neg.Operator = ">"
	}
	return neg
}

// ─── internal chain result ────────────────────────────────────────────────────

type chainResult struct {
	wants      []ws.RecipeWant
	steps      []ws.PlannerStep
	confidence string
	warnings   []string
}

// ─── monitor ─────────────────────────────────────────────────────────────────

func (p *Planner) buildMonitorWant(target ws.PlanTarget) (ws.RecipeWant, ws.PlannerStep) {
	name := target.Name
	if name == "" {
		name = slugify(target.Type) + "-monitor"
	}

	// Expose all exposable fields from this want type.
	exposes := p.buildExposes(target.Type)

	w := ws.RecipeWant{
		Metadata: ws.Metadata{
			Name: name,
			Type: target.Type,
		},
		Spec: ws.WantSpec{
			Params:  target.Params,
			Exposes: exposes,
		},
	}
	step := ws.PlannerStep{
		WantType:   target.Type,
		Role:       "monitor",
		Confidence: "certain",
		Reasoning:  fmt.Sprintf("%s is a monitor target — added as independent want", target.Type),
	}
	return w, step
}

// ─── backward chain ──────────────────────────────────────────────────────────

func (p *Planner) backwardChain(target ws.PlanTarget, hints []ws.PlanHint, globalParams map[string]any) (chainResult, []string) {
	var result chainResult
	result.confidence = "certain"

	targetName := target.Name
	if targetName == "" {
		targetName = slugify(target.Type)
	}

	def := p.defs[target.Type]
	if def == nil {
		result.confidence = "unknown"
		result.warnings = append(result.warnings,
			fmt.Sprintf("want type %q not found in registry — cannot plan", target.Type))
		return result, result.warnings
	}

	// Find required parameters not already provided.
	missingParams := p.missingRequiredParams(def, target.Params, globalParams)

	// Check if there's a planning hint for this target.
	hintWantType := p.findHintFor(target.Type, hints)

	var intermediates []ws.RecipeWant
	var intermediateSteps []ws.PlannerStep
	usingEntries := []ws.UsingEntry{}
	var terminalImports map[string]string

	if len(missingParams) > 0 || hintWantType != "" {
		if hintWantType != "" {
			// Hint-guided: use a specific want type as provider.
			providerChain := p.buildHintedProvider(hintWantType, target.Type, targetName, hints, globalParams)
			intermediates = append(intermediates, providerChain.wants...)
			intermediateSteps = append(intermediateSteps, providerChain.steps...)
			usingEntries = providerChain.usingEntries
			terminalImports = providerChain.terminalImports
			if lowerConf(providerChain.confidence, result.confidence) {
				result.confidence = providerChain.confidence
			}
			result.warnings = append(result.warnings, providerChain.warnings...)
		} else {
			// Formal backward chain: match param names to exposable fields.
			// Deduplicate providers so the same want type is only added once.
			addedProviders := map[string]bool{}
			for _, param := range missingParams {
				provWant, step, conf := p.findProvider(param, target.Type)
				if provWant != nil {
					// Propagate confidence downward (inferred < certain).
					if lowerConf(conf, result.confidence) {
						result.confidence = conf
					}
					if !addedProviders[provWant.Metadata.Name] {
						intermediates = append(intermediates, *provWant)
						intermediateSteps = append(intermediateSteps, step)
						addedProviders[provWant.Metadata.Name] = true
					}
					usingEntries = append(usingEntries, ws.UsingEntry{
						Labels: map[string]string{"name": provWant.Metadata.Name, "select": param},
					})
				} else {
					result.confidence = "unknown"
					result.warnings = append(result.warnings,
						fmt.Sprintf("no provider found for param %q required by %s", param, target.Type))
					_ = step
					_ = conf
				}
			}
		}
	}

	// Assemble: intermediates first, then the terminal want.
	result.wants = append(result.wants, intermediates...)
	result.steps = append(result.steps, intermediateSteps...)

	terminalWant := ws.RecipeWant{
		Metadata: ws.Metadata{
			Name: targetName,
			Type: target.Type,
		},
		Spec: ws.WantSpec{
			Params:  target.Params,
			Using:   usingEntries,
			Imports: terminalImports,
		},
	}
	result.wants = append(result.wants, terminalWant)
	result.steps = append(result.steps, ws.PlannerStep{
		WantType:   target.Type,
		Role:       "terminal",
		Confidence: result.confidence,
		Reasoning:  fmt.Sprintf("%s is the terminal achieve target", target.Type),
	})

	return result, result.warnings
}

// hintedProviderChain is an intermediate result for hint-guided planning.
type hintedProviderChain struct {
	wants           []ws.RecipeWant
	steps           []ws.PlannerStep
	usingEntries    []ws.UsingEntry
	terminalImports map[string]string // imports to inject into the terminal want
	confidence      string
	warnings        []string
}

// buildHintedProvider constructs intermediate wants using a hint-specified provider type.
// Currently implements the smartgolf pattern:
//
//	provider (e.g. smartgolf_list_available) → choice → terminal
func (p *Planner) buildHintedProvider(providerType, targetType, targetName string, hints []ws.PlanHint, globalParams map[string]any) hintedProviderChain {
	var chain hintedProviderChain
	chain.confidence = "inferred"

	providerName := slugify(providerType)
	def := p.defs[providerType]
	if def == nil {
		chain.confidence = "unknown"
		chain.warnings = append(chain.warnings,
			fmt.Sprintf("hint provider type %q not found in registry", providerType))
		return chain
	}

	// Step 1: Add the hint-specified provider want with exposes.
	providerExposes := p.buildExposes(providerType)
	providerWant := ws.RecipeWant{
		Metadata: ws.Metadata{Name: providerName, Type: providerType},
		Spec:     ws.WantSpec{Exposes: providerExposes},
	}
	chain.wants = append(chain.wants, providerWant)
	chain.steps = append(chain.steps, ws.PlannerStep{
		WantType:   providerType,
		Role:       "intermediate",
		ProvidedBy: "",
		Confidence: "inferred",
		Reasoning:  fmt.Sprintf("hint: use %s to provide data for %s", providerType, targetType),
	})

	// Step 2: Check if there's an array exposable field → insert choice intermediary.
	arrayFields := p.index.FindByWantType(providerType)
	var choiceImportKey string
	for _, f := range arrayFields {
		if f.DataType == "array" {
			choiceImportKey = f.Field
			break
		}
	}

	if choiceImportKey != "" && p.defs["choice"] != nil {
		// Add a choice want that imports the array and exposes selected to the
		// coordinator's current state as "selected_slot". The terminal want then
		// imports "selected_slot" from the coordinator — both go through coordinator
		// state so imports polling in hasUnresolvedImports() can see the value.
		choiceName := slugify(targetType) + "-choice"
		choiceWant := ws.RecipeWant{
			Metadata: ws.Metadata{Name: choiceName, Type: "choice"},
			Spec: ws.WantSpec{
				Imports: map[string]string{choiceImportKey: "choices"},
				Exposes: []ws.ExposeEntry{
					{CurrentState: "selected", As: "selected_slot"},
				},
			},
		}
		chain.wants = append(chain.wants, choiceWant)
		chain.steps = append(chain.steps, ws.PlannerStep{
			WantType:   "choice",
			Role:       "intermediate",
			ProvidedBy: providerType,
			Confidence: "inferred",
			Reasoning: fmt.Sprintf(
				"choice imports %s.%s (array) → user picks one → exposes selected as coordinator.selected_slot",
				providerType, choiceImportKey),
		})
		// Terminal want imports selected_slot from coordinator state.
		chain.terminalImports = map[string]string{"selected_slot": "selected_slot"}
		chain.usingEntries = nil
	}

	return chain
}

// findProvider searches the exposable index for a want type that provides a value
// matching paramName. Returns the recipe want to add and the confidence level.
func (p *Planner) findProvider(paramName, consumerType string) (*ws.RecipeWant, ws.PlannerStep, string) {
	// Try exact match first.
	exact := p.index.FindByExactName(paramName)
	for _, f := range exact {
		if f.WantType == consumerType {
			continue // skip self
		}
		name := slugify(f.WantType)
		exposes := p.buildExposes(f.WantType)
		w := ws.RecipeWant{
			Metadata: ws.Metadata{Name: name, Type: f.WantType},
			Spec:     ws.WantSpec{Exposes: exposes},
		}
		step := ws.PlannerStep{
			WantType:   f.WantType,
			Role:       "intermediate",
			Confidence: "certain",
			Reasoning:  fmt.Sprintf("%s.%s exactly matches param %q of %s", f.WantType, f.Field, paramName, consumerType),
		}
		return &w, step, "certain"
	}

	// Try suffix match (e.g. "first_room" last token "room" satisfies "room").
	suffix := p.index.FindBySuffix(paramName)
	for _, f := range suffix {
		if f.WantType == consumerType {
			continue
		}
		name := slugify(f.WantType)
		exposes := p.buildExposes(f.WantType)
		w := ws.RecipeWant{
			Metadata: ws.Metadata{Name: name, Type: f.WantType},
			Spec:     ws.WantSpec{Exposes: exposes},
		}
		step := ws.PlannerStep{
			WantType:   f.WantType,
			Role:       "intermediate",
			Confidence: "inferred",
			Reasoning:  fmt.Sprintf("%s.%s suffix-matches param %q of %s", f.WantType, f.Field, paramName, consumerType),
		}
		return &w, step, "inferred"
	}

	return nil, ws.PlannerStep{}, "unknown"
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// missingRequiredParams returns the names of required parameters that are not
// already satisfied by either target-level params or globalParams.
func (p *Planner) missingRequiredParams(def *ws.WantTypeDefinition, targetParams, globalParams map[string]any) []string {
	var missing []string
	for _, pd := range def.Parameters {
		if !pd.Required {
			continue
		}
		if _, ok := targetParams[pd.Name]; ok {
			continue
		}
		if _, ok := globalParams[pd.Name]; ok {
			continue
		}
		missing = append(missing, pd.Name)
	}
	return missing
}

// buildExposes creates an ExposeEntry slice for all exposable fields of a want type.
func (p *Planner) buildExposes(wantType string) []ws.ExposeEntry {
	fields := p.index.FindByWantType(wantType)
	exposes := make([]ws.ExposeEntry, 0, len(fields))
	for _, f := range fields {
		exposes = append(exposes, ws.ExposeEntry{
			CurrentState: f.Field,
			As:           f.Field,
		})
	}
	return exposes
}

// findHintFor returns the "use" want type from hints that target the given type.
func (p *Planner) findHintFor(targetType string, hints []ws.PlanHint) string {
	for _, h := range hints {
		if h.For == targetType && h.Use != "" {
			return h.Use
		}
	}
	return ""
}

// slugify converts a want type name to a kebab-case instance name.
func slugify(s string) string {
	return strings.ReplaceAll(s, "_", "-")
}

// lowerConf returns true if a is "lower" than b in the confidence ordering:
// certain > inferred > unknown.
func lowerConf(a, b string) bool {
	order := map[string]int{"certain": 2, "inferred": 1, "unknown": 0}
	return order[a] < order[b]
}
