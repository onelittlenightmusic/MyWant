package mywant

// mrs_params.go — deciding whether a Machine-Readable Skill runs this tick, and
// with what.
//
// The gate and the template expansion used to live beside the generic
// skill_path agent, in package types, which meant the plugin agents declared in
// an agent.yaml (package mywant, see agent_loader.go) could not reach them:
// types dot-imports core, not the other way round. So the newer, declarative
// half of MRS had no "wait for these params" and no %{...} expansion, and a
// plugin monitor could not be passed arguments at all.
//
// They are the same two questions for both halves — is this want ready, and
// what does the script get — so they live here, where both can ask them.

import (
	"fmt"
	"strings"
)

// MRSCheckRequiredParams checks whether all params listed in the "skill_required_params"
// current state field (space-separated) are non-empty.
//
// Resolution order (first non-empty wins):
//  1. Imported values — if the param name is an imported local key (Spec.Imports),
//     the live value from the parent/global state is used via GetAllState().
//  2. Spec.Params — the statically declared parameter value.
//
// Returns true if any required param is missing/empty, in which case the MRS agent
// should skip this tick. Also updates "summary" with a waiting message.
//
// This enables the "wait for params" pattern: create a want with empty params,
// and the agent will not execute until the user (or a thinker) populates them.
func MRSCheckRequiredParams(want *Want, declared ...string) bool {
	required := append([]string{}, declared...)
	required = append(required, strings.Fields(GetCurrent(want, "skill_required_params", ""))...)
	if len(required) == 0 {
		return false // no guard configured
	}
	// Overlay imports so imported keys resolve through GetAllState.
	allState := want.GetAllState()
	var missing []string
	for _, p := range required {
		// Priority 1: check imported / live state value
		if stateVal, ok := allState[p]; ok && stateVal != nil && strings.TrimSpace(fmt.Sprintf("%v", stateVal)) != "" {
			continue // provided via import or current state
		}
		// Priority 2: the parameter's effective value — GetParameter, not
		// Spec.Params, so a {fromGlobalParam} reference reads as the value it
		// resolves to rather than as the declaration.
		val, exists := want.GetParameter(p)
		if !exists || val == nil || strings.TrimSpace(fmt.Sprintf("%v", val)) == "" {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		want.StoreLog("[MRS] waiting for required params: %v", missing)
		want.SetCurrent("summary", "パラメータ待機中: "+strings.Join(missing, ", "))
		return true
	}
	return false
}

// MRSRebuildSkillArg rebuilds the "skill_json_arg" state from "skill_json_arg_template"
// by substituting %{param} placeholders with current values.
//
// Resolution order for each placeholder %{key} (first non-empty wins):
//  1. Imported / live state values from GetAllState() — covers keys declared in Spec.Imports.
//     If the imported value is a map (e.g. a selected_slot object), its sub-keys are also
//     available as %{key} so the template can reference nested fields directly.
//  2. Spec.Params — the statically declared parameter value.
//  3. The want's own identity: %{want_name} and %{want_id}. Scripts that call back
//     into the API (OAuth state, state PUTs) need to name the want they belong to,
//     which is otherwise invisible to them. Lowest priority, so a param or state
//     field of the same name still wins.
//
// IMPORTANT: The placeholder syntax is %{key} (percent-brace), NOT ${key}.
// The onInitialize interpolation engine uses ${key} and would pre-expand those
// at want-creation time (replacing params with their init-time values, often "").
// Using %{key} avoids this clash: the template is stored literally by onInitialize
// and expanded here at each tick with the *current* values.
//
// This allows the want to pick up import changes (e.g. selected slot from a child choice
// want) without requiring re-initialization.
func MRSRebuildSkillArg(want *Want) {
	tmpl := GetCurrent(want, "skill_json_arg_template", "")
	if tmpl == "" {
		return // no template; keep existing skill_json_arg unchanged
	}

	// Build a merged params map: want identity as base, then spec.params,
	// overlaid by imported/live state values.
	merged := make(map[string]any)
	if want.Metadata.Name != "" {
		merged["want_name"] = want.Metadata.Name
	}
	if want.Metadata.ID != "" {
		merged["want_id"] = want.Metadata.ID
	}
	// GetParameter, not Spec.Params: a param declared as
	// {fromGlobalParam: key} keeps that reference in Spec.Params by design, and
	// its resolved value lives beside it. Reading the raw map put the reference
	// object itself into the skill arguments.
	for k := range want.Spec.Params {
		if v, ok := want.GetParameter(k); ok {
			merged[k] = v
		}
	}
	// Overlay imported values (Priority 1): imported values take precedence over spec.params.
	allState := want.GetAllState()
	for k, v := range allState {
		if v != nil && strings.TrimSpace(fmt.Sprintf("%v", v)) != "" {
			merged[k] = v
			// If the imported value is a map, also expose its sub-keys so templates
			// can reference nested fields directly as %{subkey}.
			if m, ok := v.(map[string]any); ok {
				for subKey, subVal := range m {
					if _, alreadySet := merged[subKey]; !alreadySet {
						merged[subKey] = subVal
					}
				}
			}
		}
	}

	built := tmpl
	for k, v := range merged {
		built = strings.ReplaceAll(built, "%{"+k+"}", fmt.Sprintf("%v", v))
	}
	want.StoreState("skill_json_arg", built)
}
