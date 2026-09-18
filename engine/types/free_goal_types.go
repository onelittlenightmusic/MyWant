package types

import (
	. "mywant/engine/core"
)

// A request said in plain words, kept as a want.
//
// The robot could answer questions and, lately, move things. What it could not
// do was hold a job: "Parasomniaを(9,-9)に移動して" takes a look, then a move,
// then a word back, and each step depends on what the one before it found. All
// of that was happening inside the on-device model, which meant the plan
// existed nowhere — not on the board, not in a card, not afterwards. A wrong
// step could only be described once it had happened.
//
// So the request is a want like any other. Its parameter is the sentence; its
// state is the plan and what each step printed; its tile shows how far it has
// got. The model is asked one short question at a time (see
// agent_free_goal.go) and this want decides what actually runs — which is the
// right way round, because the board is what the commands change and the board
// is here.

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[FreeGoalWant, FreeGoalLocals]("free_goal")
	})
}

// FreeGoalLocals holds nothing: everything this want knows is worth keeping,
// so all of it is state.
type FreeGoalLocals struct{}

type FreeGoalWant struct{ Want }

func (g *FreeGoalWant) GetLocals() *FreeGoalLocals {
	return CheckLocalsInitialized[FreeGoalLocals](&g.Want)
}

func (g *FreeGoalWant) Initialize() {
	// A goal that has already run is not run again.
	//
	// Initialize runs on every start, including a server restart, and it used to
	// set the phase back to "planning" and call the agent — so every goal ever
	// asked for was carried out again on every restart. One restart moved a tile
	// that had been put back hours earlier, four times over, because four old
	// goals all said to move it. A request is a thing that happened once.
	if phase := GetCurrent(&g.Want, "phase", ""); phase != "" && phase != "planning" {
		return
	}

	request := g.GetStringParam("request", "")
	g.SetCurrent("request", request)
	g.SetCurrent("asked_by", g.GetStringParam("asked_by", ""))
	g.SetCurrent("goal_context", g.GetStringParam("context", ""))
	g.SetCurrent("max_steps", g.GetIntParam("max_steps", 4))
	g.SetCurrent("dry_run", g.GetBoolParam("dry_run", false))
	g.SetCurrent("phase", "planning")
	g.SetCurrent("steps", []any{})
	g.SetCurrent("pending_command", "")
	g.SetCurrent("answer", "")
	g.SetCurrent("error", "")
	g.SetCurrent("achieving_percentage", 0)

	// A goal with nothing asked is not a goal. Said here rather than left to
	// the agent, so the card shows why it will never move.
	if request == "" {
		g.SetCurrent("phase", "failed")
		g.SetCurrent("error", "何をすればいいのか書かれていません (request が空です)")
		return
	}
	g.ExecuteAgents() //nolint:errcheck
}

// IsAchieved is true once the goal has been carried out and there is something
// to say about it.
func (g *FreeGoalWant) IsAchieved() bool {
	return GetCurrent(&g.Want, "phase", "") == "done"
}

// IsFailed is true when the goal stopped without doing what was asked. Waiting
// for a person to confirm is not failure — it is the want doing its job.
func (g *FreeGoalWant) IsFailed() bool {
	return GetCurrent(&g.Want, "error", "") != ""
}

// Progress keeps the tile honest about how far along the job is: how many of
// the steps allowed have been run.
func (g *FreeGoalWant) Progress() {
	switch GetCurrent(&g.Want, "phase", "") {
	case "done":
		g.SetCurrent("achieving_percentage", 100)
	case "waiting_confirmation":
		g.SetCurrent("achieving_percentage", 90)
	default:
		steps := GetCurrent(&g.Want, "steps", []any{})
		max := GetCurrent(&g.Want, "max_steps", 4)
		if max <= 0 {
			max = 4
		}
		percent := len(steps) * 100 / (max + 1)
		if percent > 90 {
			percent = 90
		}
		g.SetCurrent("achieving_percentage", percent)
	}
}
