package types

import (
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[CharacterMotionWant, CharacterMotionLocals]("character_motion")
	})
}

// CharacterMotionLocals holds this want's own pacing clock. In-memory only,
// on purpose: a restart is itself a fine moment to start re-pacing from
// zero, and nothing about a paused clock needs to survive one.
type CharacterMotionLocals struct {
	lastTickAt time.Time
}

// CharacterMotionWant is what actually moves a character.
//
// "going" is this character's own going/stopped flag — set by a footstep on
// any "going" want (see button_occupancy.go's toggleGoingOnStep), and by
// nothing else, so one character stepping on a going want can never start
// or stop a different character who also happens to be near it. The
// gear/direction votes cast by whatever wants this character is currently
// standing on or steering are, by contrast, read fresh every tick — see
// driveOneCharacterTick. All of it is resolved once every
// CharacterMotionInterval by CharacterMotionTick (see character_motion_hook.go
// for why that's a hook and not a direct call), and the result lands in the
// ephemeral cursor store — the same place it always has. x/y here are a
// mirror of that, purely so this want's own card can show, at a glance,
// where its character last resolved to; they are not themselves read back
// by anything.
//
// One per character (see ensureCharacterMotionWants in the server package),
// driven by the engine's normal reconcile loop like every other want — not
// by a separate ticker goroutine outside it. That used to be the shape:
// a single shared ticker recomputed every driven character on every tick,
// which is exactly the kind of thing that races against a want doing the
// same work on its own schedule. There is now only the one mechanism.
type CharacterMotionWant struct {
	Want
}

func (w *CharacterMotionWant) GetLocals() *CharacterMotionLocals {
	return CheckLocalsInitialized[CharacterMotionLocals](&w.Want)
}

func (w *CharacterMotionWant) Initialize() {
	w.SetCurrent("x", 0.0)
	w.SetCurrent("y", 0.0)
	// Only on first init, not on every restart — same guard as going/
	// direction's own dx/dy/going, and for the same reason: a character who
	// was going when the server went down should still be going when it
	// comes back up, not silently reset to stopped.
	if _, ok := w.GetCurrent("going"); !ok {
		w.SetCurrent("going", false)
	}
	// Written once, not every tick: a character never finishes being able to
	// move, so this never has anywhere else to go. Progress() runs on the
	// engine's own 100ms reconcile cadence, far more often than this want
	// actually does anything (see CharacterMotionInterval) — restating an
	// unchanging value there would just be log noise on every tick that has
	// nothing to say.
	w.SetCurrent("achieving_percentage", 50)
}

// Progress moves the character at most once every CharacterMotionInterval,
// no matter how often the engine calls this — the engine reconciles every
// want far more often than that (GlobalReconcileInterval, 100ms), and each
// real move is a whole grid cell, so calling the mover on every reconcile
// tick unpaced would move a character several times too fast.
func (w *CharacterMotionWant) Progress() {
	locals := w.GetLocals()
	now := time.Now()
	if !locals.lastTickAt.IsZero() && now.Sub(locals.lastTickAt) < CharacterMotionInterval {
		return
	}
	locals.lastTickAt = now

	// Nil before the server has wired it up (or in a context with no server
	// at all, e.g. a unit test) — a no-op tick, same defensive style as the
	// GetGlobalChainBuilder() == nil checks throughout this codebase.
	if CharacterMotionTick == nil {
		return
	}
	characterID := w.GetStringParam("character_id", "")
	if characterID == "" {
		return
	}
	x, y, moved := CharacterMotionTick(characterID, &w.Want)
	if moved {
		w.SetCurrent("x", x)
		w.SetCurrent("y", y)
	}
}

// IsAchieved is always false: a character never finishes being able to
// move. Same reasoning as character_chat's own IsAchieved.
func (w *CharacterMotionWant) IsAchieved() bool { return false }
