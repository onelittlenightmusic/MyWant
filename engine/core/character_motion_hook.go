package mywant

import "time"

// CharacterMotionInterval is how often a "character_motion" want actually
// moves its character — self-paced (see that want type's own Progress()),
// since the engine reconciles every want far more often than this
// (GlobalReconcileInterval, 100ms) and each real move is a whole grid cell:
// calling the mover on every reconcile tick without pacing would move a
// character 2.5x too fast.
const CharacterMotionInterval = 250 * time.Millisecond

// CharacterMotionTick is set by the server package at startup to the
// function that actually resolves and applies one character's motion for a
// tick (drive_engine.go's driveOneCharacterTick) — updating the ephemeral
// cursor/presence store, syncing form-type:button occupancy, and
// broadcasting the result over SSE. It reports back the resolved position
// (and whether anything actually moved) purely so the calling want can
// mirror it into its own state for observability — the ephemeral cursor
// store, not this, stays the authoritative position.
//
// Passed the calling "character_motion" want itself (motionWant) so the
// tick can read/write that want's own "going" field directly — going is the
// character's own state, kept on their own want, not shared with any going
// want they might currently be touching (see that field's own doc and
// button_occupancy.go's toggleGoingOnStep).
//
// It lives here, not in the server package, purely to break an import
// cycle: the "character_motion" want type (engine/types) is what calls
// this, self-paced to roughly once every 250ms (see its own Progress()),
// but engine/types cannot import engine/server — engine/server already
// imports engine/types to trigger every want type's registration. A plain
// function variable set once at startup is the whole bridge; nil before the
// server has started (or in a context with no server at all, e.g. a unit
// test), in which case a "character_motion" want's Progress() is simply a
// no-op tick.
var CharacterMotionTick func(characterID string, motionWant *Want) (x, y float64, moved bool)
