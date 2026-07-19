package types

import (
	"strconv"
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
	w.SetGoal("session_id", locals.SessionID)
	w.SetGoal("auto_request", "")
	w.SetGoal("max_requests", 0) // robot chat is unlimited — it never completes
	w.SetGoal("working_dir", locals.WorkingDir)
	w.SetGoal("permission_mode", w.GetStringParam("permission_mode", "bypassPermissions"))
	w.SetGoal("allowed_tools", w.GetStringParam("allowed_tools", ""))

	// trigger_on is always webhook (user-driven chat, same as coding)
	w.SetGoal("trigger_on", "webhook")
	w.SetGoal("watch_pattern", "")

	w.SetCurrent("phase", CCPhaseMonitoring)
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
	if w.Metadata.Labels == nil {
		w.Metadata.Labels = make(map[string]string)
	}
	if _, err := strconv.Atoi(w.Metadata.Labels["mywant.io/canvas-x"]); err != nil {
		w.Metadata.Labels["mywant.io/canvas-x"] = strconv.Itoa(w.GetIntParam("spawn_x", 5))
	}
	if _, err := strconv.Atoi(w.Metadata.Labels["mywant.io/canvas-y"]); err != nil {
		w.Metadata.Labels["mywant.io/canvas-y"] = strconv.Itoa(w.GetIntParam("spawn_y", 5))
	}
	if w.Metadata.Labels["mywant.io/canvas-rotation"] == "" {
		w.Metadata.Labels["mywant.io/canvas-rotation"] = "0"
	}
	if w.Metadata.Labels["mywant.io/canvas-length"] == "" {
		w.Metadata.Labels["mywant.io/canvas-length"] = "0"
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

	x, errX := strconv.Atoi(w.Metadata.Labels["mywant.io/canvas-x"])
	y, errY := strconv.Atoi(w.Metadata.Labels["mywant.io/canvas-y"])
	if errX != nil || errY != nil {
		return
	}

	step := (time.Now().UnixNano() / int64(time.Millisecond)) % 9 // 0..8 -> one of 9 moves (incl. staying put)
	dx := int(step%3) - 1
	dy := int(step/3) - 1

	maxCoord := w.GetIntParam("wander_bound", 20)
	nx := clampInt(x+dx, 0, maxCoord)
	ny := clampInt(y+dy, 0, maxCoord)

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

	if nx != x {
		w.Metadata.Labels["mywant.io/canvas-x"] = strconv.Itoa(nx)
	}
	if ny != y {
		w.Metadata.Labels["mywant.io/canvas-y"] = strconv.Itoa(ny)
	}
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

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
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
			w.SetCurrent("phase", CCPhaseTriggerReady)
		}

	case CCPhaseTriggerReady:
		// Allow DoAgent to re-run for each new message (chat mode).
		// DoAgent itself clears webhook_auto_request after reading it.
		w.FinishAgentRun(ccDoAgentName, false)
		w.SetCurrent("phase", CCPhaseRequesting)
		if err := w.ExecuteAgents(); err != nil {
			w.StoreLog("ERROR: DoAgent execution failed: %v", err)
			w.SetCurrent("phase", CCPhaseError)
			w.SetCurrent("last_error", err.Error())
			return
		}
		w.SetCurrent("phase", CCPhaseAwaitingResponse)
		w.SetPlan("next_action", "")

	case CCPhaseAwaitingResponse:
		if nextAction == "process_response" {
			w.SetCurrent("phase", CCPhaseResponseReceived)
		} else if nextAction == "handle_timeout" {
			w.StoreLog("[ROBOT] Response timeout, resuming monitoring")
			w.SetCurrent("phase", CCPhaseMonitoring)
			w.SetPlan("next_action", "")
		}

	case CCPhaseResponseReceived:
		locals.ReqCount++
		w.SetCurrent("request_count", locals.ReqCount)
		w.StoreLog("[ROBOT] Request %d completed", locals.ReqCount)
		w.SetCurrent("phase", CCPhaseMonitoring)
		w.SetPlan("next_action", "")

	case CCPhaseError:
		if nextAction == "retry" {
			w.SetCurrent("phase", CCPhaseMonitoring)
			w.SetPlan("next_action", "")
		}

	case CCPhaseRequesting:
		// See the identical case in coding_types.go's CodingWant.Progress —
		// reaching this phase at the start of a Progress() call means a
		// previous request was interrupted mid-flight (e.g. a server
		// restart during ExecuteAgents()). Recover via the error/retry path.
		w.StoreLog("[ROBOT] Found stale 'requesting' phase (likely interrupted by a server restart) — recovering")
		w.SetCurrent("phase", CCPhaseError)
		w.SetCurrent("last_error", "request was interrupted (e.g. server restart) before completing")
	}
}

// IsAchieved always returns false: the robot is a permanent system want.
func (w *RobotWant) IsAchieved() bool {
	return false
}
