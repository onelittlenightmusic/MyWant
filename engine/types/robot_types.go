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
	Provider   string `mywant:"internal,provider"`
	SessionID  string `mywant:"internal,session_id"`
	ReqCount   int    `mywant:"internal,request_count"`
	TimeoutSec int    `mywant:"internal,timeout_seconds"`
	WorkingDir string `mywant:"internal,working_dir"`
}

// RobotWant is the always-on chat companion (backs the header interact bubble
// and the "robot" canvas character). It reuses the coding want's Monitor/Think/Do
// machinery unchanged (session/thread persistence, FIFO chat, idempotency — see
// coding_types.go) and, when asked to, follows a character around the canvas
// independent of chat phase (see follow).
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

// follow walks the robot toward whoever it has been told to follow, one cell
// at a time and a beat behind them.
//
// It used to wander instead: a one-cell drift every forty-five seconds, on a
// leash tied to wherever it was last put. Nobody asked it to go anywhere, and
// on a board people are arranging by hand a thing that moves on its own is
// something to keep finding again. So it stands still unless it is following,
// and following is something you ask for — the `follow` parameter, naming a
// character (or "cursor" for whoever is at the controls).
//
// Behind them, not on top of them: it waits followDelay after they step out of
// reach before it sets off, walks one cell per followStep, and stops once it is
// within follow_distance of them. That lag is the whole look of being followed
// — something that arrives the instant you do is attached, not following.
func (w *RobotWant) follow() {
	target := w.GetStringParam("follow", "")
	selfID := w.Metadata.ID
	if target == "" || LocateCharacter == nil {
		forgetFollow(selfID)
		return
	}
	tx, ty, ok := LocateCharacter(target)
	if !ok {
		return
	}
	x, errX := strconv.Atoi(w.GetLabel("mywant.io/canvas-x"))
	y, errY := strconv.Atoi(w.GetLabel("mywant.io/canvas-y"))
	if errX != nil || errY != nil {
		return
	}

	reach := w.GetIntParam("follow_distance", 1)
	if reach < 1 {
		reach = 1 // never onto the cell they are standing on
	}
	now := time.Now()
	if chebyshev(x-tx, y-ty) <= reach {
		forgetFollow(selfID)
		return
	}
	if !followDue(selfID, now,
		time.Duration(w.GetIntParam("follow_delay_ms", 600))*time.Millisecond,
		time.Duration(w.GetIntParam("follow_step_ms", 350))*time.Millisecond) {
		return
	}

	nx, ny := x+sign(tx-x), y+sign(ty-y)

	// Same wall / locked-door boundaries a player's cursor can't cross (see
	// WantCanvas.tsx's wallCells) — try the diagonal step, then slide along
	// one axis at a time (matching the player's own slide-along-wall
	// behavior), before giving up and staying put this step. A robot that
	// cannot get round a wall waits at it rather than phasing through.
	switch {
	case !isCanvasBlocked(nx, ny, selfID):
	case nx != x && !isCanvasBlocked(nx, y, selfID):
		ny = y
	case ny != y && !isCanvasBlocked(x, ny, selfID):
		nx = x
	default:
		return
	}

	if nx != x {
		w.SetLabel("mywant.io/canvas-x", strconv.Itoa(nx))
	}
	if ny != y {
		w.SetLabel("mywant.io/canvas-y", strconv.Itoa(ny))
	}
}

// ── How far behind the robot is ──────────────────────────────────────────────

// Held in memory rather than in state: it is a few hundred milliseconds of
// timing, and a restart that forgets it just means the robot hesitates once.
type followTiming struct{ outSince, lastStep time.Time }

var (
	followMu      sync.Mutex
	followTimings = map[string]followTiming{}
)

// followDue reports whether the robot may take a step now: the target has
// been out of reach for at least delay, and the last step was at least step
// ago. Records the step when it says yes.
func followDue(wantID string, now time.Time, delay, step time.Duration) bool {
	followMu.Lock()
	defer followMu.Unlock()
	t := followTimings[wantID]
	if t.outSince.IsZero() {
		t.outSince = now
		followTimings[wantID] = t
	}
	if now.Sub(t.outSince) < delay || now.Sub(t.lastStep) < step {
		return false
	}
	t.lastStep = now
	followTimings[wantID] = t
	return true
}

// forgetFollow resets the lag once the robot has caught up (or stopped
// following), so the next time its target walks off it hesitates again.
func forgetFollow(wantID string) {
	followMu.Lock()
	defer followMu.Unlock()
	delete(followTimings, wantID)
}

func chebyshev(dx, dy int) int {
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// isCanvasBlocked reports whether (x,y) is occupied by a wall or a locked
// door — the same boundaries WantCanvas.tsx's wallCells memo enforces for the
// player's own cursor — so the robot following someone can't cross them
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
// the follow step and a phase that never reaches "achieved" —
// the robot is a permanent system want, not a completable task.
func (w *RobotWant) Progress() {
	locals := w.GetLocals()

	w.follow()

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
