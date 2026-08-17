package types

import (
	"strconv"
	"sync"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[RobotWant, RobotLocals]("robot")
	})
}

// RobotLocals holds type-specific local state for the robot want.
type RobotLocals struct {
	WebhookLocals
	Provider          string `mywant:"internal,provider"`
	SessionID         string `mywant:"internal,session_id"`
	ReqCount          int    `mywant:"internal,request_count"`
	TimeoutSec        int    `mywant:"internal,timeout_seconds"`
	WorkingDir        string `mywant:"internal,working_dir"`
	WanderIntervalSec int    `mywant:"internal,wander_interval_seconds"`
}

// RobotWant is the always-on chat companion (backs the header interact bubble
// and the "robot" canvas character). It reuses the coding want's Monitor/Think/Do
// machinery unchanged (session/thread persistence, FIFO chat, idempotency — see
// coding_types.go) and adds slow autonomous wandering across the canvas,
// independent of chat phase, so it visibly moves like a character.
type RobotWant struct {
	Want
}

func (w *RobotWant) GetLocals() *RobotLocals {
	return CheckLocalsInitialized[RobotLocals](&w.Want)
}

func (w *RobotWant) Initialize() {
	w.StoreLog("[ROBOT] Initializing: %s", w.Metadata.Name)

	if err := w.StopAllBackgroundAgents(); err != nil {
		w.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	locals := w.GetLocals()

	locals.Provider = w.GetStringParam("provider", "claude_code")
	locals.SessionID = w.GetStringParam("session_id", "")
	if locals.SessionID == "" {
		locals.SessionID = GetGoal(&w.Want, "session_id", "")
	}
	locals.TimeoutSec = w.GetIntParam("timeout_seconds", 300)
	locals.WorkingDir = w.GetStringParam("working_dir", "")
	locals.WanderIntervalSec = w.GetIntParam("wander_interval_seconds", 45)

	existingCount := GetCurrent(&w.Want, "request_count", -1)
	if existingCount < 0 {
		locals.ReqCount = 0
	} else {
		locals.ReqCount = existingCount
	}

	w.SetGoal("provider", locals.Provider)
	w.SetGoal("model", w.GetStringParam("model", ""))
	w.SetGoal("session_id", locals.SessionID)
	w.SetGoal("auto_request", "")
	w.SetGoal("max_requests", 0) // robot chat is unlimited — it never completes
	w.SetGoal("working_dir", locals.WorkingDir)
	w.SetGoal("permission_mode", w.GetStringParam("permission_mode", "bypassPermissions"))
	w.SetGoal("allowed_tools", w.GetStringParam("allowed_tools", ""))

	// trigger_on is always webhook (user-driven chat, same as coding)
	w.SetGoal("trigger_on", "webhook")
	w.SetGoal("watch_pattern", "")

	SetCCPhase(&w.Want, CCPhaseMonitoring)
	w.SetCurrent("request_count", locals.ReqCount)
	w.SetCurrent("timeout_seconds", locals.TimeoutSec)
	w.SetCurrent("interactive", true)

	w.ensureCanvasPosition()

	InitializeWebhook(&w.Want, ccWebhookConfig, &locals.WebhookLocals)
}

// ensureCanvasPosition assigns default canvas-x/y/rotation/length labels the
// first time the robot want is created, without ever overwriting a position
// it (or a user drag) already resumed from — labels are ordinary Want
// metadata, so they already persist and restore across restarts on their own.
func (w *RobotWant) ensureCanvasPosition() {
	if _, err := strconv.Atoi(w.GetLabel("mywant.io/canvas-x")); err != nil {
		w.SetLabel("mywant.io/canvas-x", strconv.Itoa(w.GetIntParam("spawn_x", 5)))
	}
	if _, err := strconv.Atoi(w.GetLabel("mywant.io/canvas-y")); err != nil {
		w.SetLabel("mywant.io/canvas-y", strconv.Itoa(w.GetIntParam("spawn_y", 5)))
	}
	if w.GetLabel("mywant.io/canvas-rotation") == "" {
		w.SetLabel("mywant.io/canvas-rotation", "0")
	}
	if w.GetLabel("mywant.io/canvas-length") == "" {
		w.SetLabel("mywant.io/canvas-length", "0")
	}
}

// wander nudges the robot's canvas position by at most one cell every
// wander_interval_seconds, independent of chat phase, so it wanders even
// while idle or mid-conversation. Ticks (Progress calls) are frequent
// relative to the interval, so this is a plain elapsed-time gate, not a timer.
func (w *RobotWant) wander(locals *RobotLocals) {
	interval := locals.WanderIntervalSec
	if interval <= 0 {
		interval = 45
	}
	now := time.Now().Unix()
	last := GetCurrent(&w.Want, "last_wander_at", int64(0))
	if now-last < int64(interval) {
		return
	}
	w.SetCurrent("last_wander_at", now)

	x, errX := strconv.Atoi(w.GetLabel("mywant.io/canvas-x"))
	y, errY := strconv.Atoi(w.GetLabel("mywant.io/canvas-y"))
	if errX != nil || errY != nil {
		return
	}

	step := (time.Now().UnixNano() / int64(time.Millisecond)) % 9 // 0..8 -> one of 9 moves (incl. staying put)
	dx := int(step%3) - 1
	dy := int(step/3) - 1

	// One cell, on a leash — but tied to where it was last put, not to the
	// origin.
	//
	// Both extremes were wrong. Clamping into an absolute box (0..bound) yanked
	// a robot that had been called to somebody standing outside it straight back
	// inside, so being called looked like the robot flying off somewhere nobody
	// asked for. Removing the leash entirely fixed that and introduced a slower
	// version of the same complaint: with nothing to hold it, a one-cell drift
	// every forty-five seconds walked the robot clean off the board — 448 steps
	// took it from (7,-4) to (-16,-51), which is not wandering, it is leaving.
	//
	// So it wanders around wherever it was last placed. anchorFor works that out
	// by noticing when the robot is somewhere wander did not leave it: only
	// something else — a call, a take, an agent — moves it that way, and that is
	// exactly the event that should re-tie the leash.
	bound := w.GetIntParam("wander_bound", 6)
	ax, ay := anchorFor(w.Metadata.ID, x, y)
	nx := clampInt(x+dx, ax-bound, ax+bound)
	ny := clampInt(y+dy, ay-bound, ay+bound)

	// And inside the board, whatever the leash says.
	//
	// The leash is relative — it holds the robot near wherever it was last put —
	// so it has nothing to say about where the board IS. That was enough while
	// the leash held, and when it slipped (see rememberWanderLeftAt below) the
	// robot walked to (201, -166) with every other want between (-4,-4) and
	// (14,14). Nothing was broken by it standing there; what broke was
	// everything that asks how big the board is. The canvas sizes itself to hold
	// every want, so one wanderer 160 cells out stretched a 19×19 board to
	// 207×181 — the minimap drew that whole empty expanse, and the opening
	// camera framed its middle, which is nowhere near anything.
	//
	// So the robot is not allowed to be the want that defines the board. It
	// wanders within what the others already span; if it is the only thing
	// placed there is no board to leave, and the leash alone decides.
	if minX, minY, maxX, maxY, ok := canvasBounds(w.Metadata.ID); ok {
		nx = clampInt(nx, minX, maxX)
		ny = clampInt(ny, minY, maxY)
	}

	// Same wall / locked-door boundaries a player's cursor can't cross (see
	// WantCanvas.tsx's wallCells) — try the diagonal move, then slide along
	// one axis at a time (matching the player's own slide-along-wall
	// behavior), before giving up and staying put this tick. This is what
	// keeps the robot inside a room enclosed by walls instead of phasing
	// through them.
	selfID := w.Metadata.ID
	switch {
	case !isCanvasBlocked(nx, ny, selfID):
		// diagonal (or straight) move is clear as-is
	case !isCanvasBlocked(nx, y, selfID):
		ny = y
	case !isCanvasBlocked(x, ny, selfID):
		nx = x
	default:
		nx, ny = x, y
	}

	// Record the move before writing it, and for a move on either axis.
	//
	// This used to sit inside the `nx != x` branch, so a step that only changed
	// y was never remembered. anchorFor then compared the robot's new y against
	// a leftY that had not moved, decided somebody else must have put it there,
	// and re-tied the leash to where the robot had just walked itself. A leash
	// that follows is not a leash: every vertical step re-anchored, the anchor
	// crept along behind the robot, and the drift the bound was meant to stop
	// resumed at full speed in both axes.
	if nx != x || ny != y {
		rememberWanderLeftAt(w.Metadata.ID, nx, ny)
	}
	if nx != x {
		w.SetLabel("mywant.io/canvas-x", strconv.Itoa(nx))
	}
	if ny != y {
		w.SetLabel("mywant.io/canvas-y", strconv.Itoa(ny))
	}
}

// canvasBounds is the rectangle the OTHER placed wants span, in grid cells.
//
// This is the board as far as anything that draws it is concerned: the canvas
// and the minimap both size themselves to hold every want, so this is exactly
// the region the robot must stay inside to avoid resizing them. Reports ok =
// false when nothing else carries a position, which is a board with no extent
// rather than an empty one.
func canvasBounds(selfID string) (minX, minY, maxX, maxY int, ok bool) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return 0, 0, 0, 0, false
	}
	for _, sib := range cb.GetWants() {
		if sib.Metadata.ID == selfID {
			continue
		}
		x, errX := strconv.Atoi(sib.GetLabel("mywant.io/canvas-x"))
		y, errY := strconv.Atoi(sib.GetLabel("mywant.io/canvas-y"))
		if errX != nil || errY != nil {
			continue
		}
		if !ok {
			minX, maxX, minY, maxY, ok = x, x, y, y, true
			continue
		}
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	return minX, minY, maxX, maxY, ok
}

// isCanvasBlocked reports whether (x,y) is occupied by a wall or a locked
// door — the same boundaries WantCanvas.tsx's wallCells memo enforces for the
// player's own cursor — so the robot's autonomous wandering can't cross them
// either. Reuses wantFootprint (aura_types.go) for multi-cell wall/door spans.
func isCanvasBlocked(x, y int, selfID string) bool {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return false
	}
	for _, sib := range cb.GetWants() {
		if sib.Metadata.ID == selfID {
			continue
		}
		blocking := sib.Metadata.Type == "wall"
		if sib.Metadata.Type == "door" {
			blocking = true // fail closed: unknown/missing "locked" defaults to blocking, matching door.yaml's initialValue
			if locked, ok := sib.GetCurrent("locked"); ok {
				if b, isBool := locked.(bool); isBool {
					blocking = b
				}
			}
		}
		if !blocking {
			continue
		}
		for _, cell := range wantFootprint(sib) {
			if cell[0] == x && cell[1] == y {
				return true
			}
		}
	}
	return false
}

// Progress reads ThinkAgent's Plan decisions and executes state transitions.
// Identical phase machine to CodingWant.Progress (see coding_types.go), plus
// the unconditional wander step and a phase that never reaches "achieved" —
// the robot is a permanent system want, not a completable task.
func (w *RobotWant) Progress() {
	locals := w.GetLocals()

	w.wander(locals)

	w.SetCurrent("achieving_percentage", 50)

	phase := GetCurrent(&w.Want, "phase", CCPhaseMonitoring)
	nextAction := GetPlan(&w.Want, "next_action", "")

	switch phase {
	case CCPhaseMonitoring:
		if nextAction == "send_request" {
			SetCCPhase(&w.Want, CCPhaseTriggerReady)
		}

	case CCPhaseTriggerReady:
		// Allow DoAgent to re-run for each new message (chat mode).
		// DoAgent itself clears webhook_auto_request after reading it.
		w.FinishAgentRun(ccDoAgentName, false)
		SetCCPhase(&w.Want, CCPhaseRequesting)
		if err := w.ExecuteAgents(); err != nil {
			w.StoreLog("ERROR: DoAgent execution failed: %v", err)
			SetCCPhase(&w.Want, CCPhaseError)
			w.SetCurrent("last_error", err.Error())
			return
		}
		SetCCPhase(&w.Want, CCPhaseAwaitingResponse)
		w.SetPlan("next_action", "")

	case CCPhaseAwaitingResponse:
		if nextAction == "process_response" {
			SetCCPhase(&w.Want, CCPhaseResponseReceived)
		} else if nextAction == "handle_timeout" {
			w.StoreLog("[ROBOT] Response timeout, resuming monitoring")
			SetCCPhase(&w.Want, CCPhaseMonitoring)
			w.SetPlan("next_action", "")
		}

	case CCPhaseResponseReceived:
		locals.ReqCount++
		w.SetCurrent("request_count", locals.ReqCount)
		w.StoreLog("[ROBOT] Request %d completed", locals.ReqCount)
		SetCCPhase(&w.Want, CCPhaseMonitoring)
		w.SetPlan("next_action", "")

	case CCPhaseError:
		if nextAction == "retry" {
			SetCCPhase(&w.Want, CCPhaseMonitoring)
			w.SetPlan("next_action", "")
		}

	case CCPhaseRequesting:
		// See the identical case in coding_types.go's CodingWant.Progress —
		// reaching this phase at the start of a Progress() call means a
		// previous request was interrupted mid-flight (e.g. a server
		// restart during ExecuteAgents()). Recover via the error/retry path.
		w.StoreLog("[ROBOT] Found stale 'requesting' phase (likely interrupted by a server restart) — recovering")
		SetCCPhase(&w.Want, CCPhaseError)
		w.SetCurrent("last_error", "request was interrupted (e.g. server restart) before completing")
	}
}

// IsAchieved always returns false: the robot is a permanent system want.
func (w *RobotWant) IsAchieved() bool {
	return false
}

// ── Where the robot is tethered while it wanders ─────────────────────────────

// Its leash is tied to wherever it was last put by something other than its own
// wandering: called to somebody, taken, moved by an agent. Held in memory
// rather than in state because a leash is not worth persisting — a restart
// simply re-ties it wherever the robot is standing, which is the same answer
// anybody would give.
type wanderAnchor struct{ anchorX, anchorY, leftX, leftY int }

var (
	wanderAnchorsMu sync.Mutex
	wanderAnchors   = map[string]wanderAnchor{}
)

// anchorFor returns the cell to wander around, re-tying to (x, y) when the
// robot is not where wander left it — the mark of somebody else having moved it.
func anchorFor(wantID string, x, y int) (int, int) {
	wanderAnchorsMu.Lock()
	defer wanderAnchorsMu.Unlock()
	a, known := wanderAnchors[wantID]
	if !known || a.leftX != x || a.leftY != y {
		a = wanderAnchor{anchorX: x, anchorY: y, leftX: x, leftY: y}
		wanderAnchors[wantID] = a
	}
	return a.anchorX, a.anchorY
}

// rememberWanderLeftAt records where this tick's wander put the robot, so the
// next tick can tell its own move apart from anybody else's.
func rememberWanderLeftAt(wantID string, x, y int) {
	wanderAnchorsMu.Lock()
	defer wanderAnchorsMu.Unlock()
	a := wanderAnchors[wantID]
	a.leftX, a.leftY = x, y
	wanderAnchors[wantID] = a
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
