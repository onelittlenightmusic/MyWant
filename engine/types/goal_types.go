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
func (g *GoalWant) Initialize() {
	g.SetGoal("goal_text", g.GetStringParam("goal_text", ""))
	g.SetCurrent("interactive", true)
	g.SetCurrent("phase", "decomposing")
	g.SetCurrent("cc_messages", []any{})
	g.SetCurrent("cc_responses", []any{})
	g.SetCurrent("cc_message_count", 0)
	g.SetCurrent("proposed_breakdown", []any{})
	g.SetCurrent("proposed_response", "")
}

// IsAchieved always returns false — the GoalThinker ThinkAgent manages lifecycle.
func (g *GoalWant) IsAchieved() bool { return false }

// Progress is a no-op; the ThinkAgent handles all logic each tick.
func (g *GoalWant) Progress() {}
