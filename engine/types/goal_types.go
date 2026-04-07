package types

import . "mywant/engine/core"

func init() {
	RegisterWantImplementation[GoalWant, GoalLocals]("goal")
}

// GoalLocals holds type-specific local state (no runtime locals needed).
type GoalLocals struct{}

// GoalWant represents a user's goal that gets decomposed into sub-wants
// by the GoalThinker ThinkAgent. The want itself runs indefinitely.
type GoalWant struct{ Want }

func (g *GoalWant) GetLocals() *GoalLocals {
	return CheckLocalsInitialized[GoalLocals](&g.Want)
}

// Initialize sets up the initial state from params.
// On restart (phase already advanced beyond decomposing), preserve progress.
func (g *GoalWant) Initialize() {
	existingPhase := GetCurrent(&g.Want, "phase", "")
	if existingPhase == "monitoring" || existingPhase == "awaiting_approval" || existingPhase == "re_planning" {
		// Restore non-persistent config fields that Initialize normally sets.
		g.SetCurrent("interactive", true)
		g.SetCurrent("opa_llm_planner_command", g.GetStringParam("opa_llm_planner_command", "opa-llm-planner"))
		g.SetCurrent("policy_dir", g.GetStringParam("policy_dir", "yaml/policies/goal"))
		g.SetCurrent("use_llm", g.GetBoolParam("use_llm", true))
		g.SetCurrent("llm_provider", g.GetStringParam("llm_provider", "anthropic"))
		g.SetCurrent("auto_approve", g.GetBoolParam("auto_approve", false))
		return
	}

	g.SetGoal("goal_text", g.GetStringParam("goal_text", ""))
	g.SetCurrent("interactive", true)
	g.SetCurrent("phase", "decomposing")
	g.SetCurrent("cc_messages", []any{})
	g.SetCurrent("cc_responses", []any{})
	g.SetCurrent("cc_message_count", 0)
	g.SetCurrent("proposed_breakdown", []any{})
	g.SetCurrent("proposed_response", "")

	// Static breakdown: if provided, skip LLM decomposition entirely.
	// auto_approve: if true, also skip the reaction queue approval step.
	if ib, ok := g.Spec.GetParam("initial_breakdown"); ok && ib != nil {
		g.SetCurrent("initial_breakdown", ib)
	}
	g.SetCurrent("auto_approve", g.GetBoolParam("auto_approve", false))

	// OPA LLM Planner config params
	g.SetCurrent("opa_llm_planner_command", g.GetStringParam("opa_llm_planner_command", "opa-llm-planner"))
	g.SetCurrent("policy_dir", g.GetStringParam("policy_dir", "yaml/policies/goal"))
	g.SetCurrent("use_llm", g.GetBoolParam("use_llm", true))
	g.SetCurrent("llm_provider", g.GetStringParam("llm_provider", "anthropic"))
}

// IsAchieved always returns false — the GoalThinker ThinkAgent manages lifecycle.
func (g *GoalWant) IsAchieved() bool { return false }

// Progress syncs direction_map_json into Spec.Params and calls InterpretDirections
// when in the monitoring phase (OPA planner manages child want dispatching).
// It also ensures the DispatchThinker background agent is running.
func (g *GoalWant) Progress() {
	// Ensure DispatchThinker is running to realize desired_dispatch requests.
	dispatchThinkerID := DispatchThinkerName + "-" + g.Metadata.ID
	if _, running := g.GetBackgroundAgent(dispatchThinkerID); !running {
		agent := NewDispatchThinker(dispatchThinkerID)
		if err := g.AddBackgroundAgent(agent); err != nil {
			g.DirectLog("[GoalWant] ERROR starting DispatchThinker: %v", err)
		}
	}

	phase := GetCurrent(&g.Want, "phase", "decomposing")
	if phase != "monitoring" {
		return
	}

	InterpretDirectionsCoordinator(&g.Want)
}
