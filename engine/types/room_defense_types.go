package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[RoomDefenseWant, RoomDefenseLocals]("room_defense")
		RegisterWantImplementation[TurretWant, TurretLocals]("turret")
		// A freeze is a turret whose hit holds rather than wears (freeze.yaml).
		RegisterWantImplementation[TurretWant, TurretLocals]("freeze")
		RegisterWantImplementation[EnemyWant, EnemyLocals]("enemy")
		// A boss is an enemy with a face of its own (boss.yaml).
		RegisterWantImplementation[EnemyWant, EnemyLocals]("boss")
		RegisterWantImplementation[HealWant, HealLocals]("heal")
		RegisterWantImplementation[BattleResetWant, BattleResetLocals]("battle_reset")
	})
}

type RoomDefenseLocals struct{}

// RoomDefenseWant is a battle in a room of the board. What happens in it —
// enemies coming in, going for the room's tiles and things, being hit — is
// the server's battle tick (engine/server/battle.go), which needs the board:
// where everything stands, the things, the characters. This want holds what
// the battle is (its params) and how it is going (its state), and is achieved
// when the tick says every enemy is down.
type RoomDefenseWant struct{ Want }

func (d *RoomDefenseWant) GetLocals() *RoomDefenseLocals {
	return CheckLocalsInitialized[RoomDefenseLocals](&d.Want)
}

func (d *RoomDefenseWant) Initialize() {
	// Waiting for someone to stand on it (the battle tick starts it then).
	if phase, _ := d.GetCurrent("phase"); phase == nil || phase == "" {
		d.SetCurrent("phase", "ready")
	}
}

func (d *RoomDefenseWant) IsAchieved() bool {
	phase, _ := d.GetCurrent("phase")
	return phase == "cleared"
}

func (d *RoomDefenseWant) Progress() {}

type TurretLocals struct{}

// TurretWant is a want tile that hits enemies near it by itself — see the
// battle tick, which knows where it stands and where the enemies are.
type TurretWant struct{ Want }

func (t *TurretWant) GetLocals() *TurretLocals {
	return CheckLocalsInitialized[TurretLocals](&t.Want)
}

// Its aim is its connection: it hits the enemy it subscribes to while that one
// is in reach (see the battle tick), so a want deciding how to fight aims it
// by changing what it subscribes to.
func (t *TurretWant) Initialize()      {}
func (t *TurretWant) IsAchieved() bool { return false }
func (t *TurretWant) Progress()        {}

type EnemyLocals struct{}

// EnemyWant is one enemy of a room defense: a want tile of its own, as a
// character has a want of its own — where it is is its tile's place, what is
// left of it its hp, and the battle tick moves it and wears it down.
type EnemyWant struct{ Want }

func (e *EnemyWant) GetLocals() *EnemyLocals {
	return CheckLocalsInitialized[EnemyLocals](&e.Want)
}

func (e *EnemyWant) Initialize() {
	if _, ok := e.GetCurrent("hp"); !ok {
		max := e.GetFloatParam("max_hp", 3)
		e.SetCurrent("max_hp", max)
		e.SetCurrent("hp", max)
	}
}

func (e *EnemyWant) IsAchieved() bool { return false }
func (e *EnemyWant) Progress()        {}

type HealLocals struct{}

// HealWant mends the wants near it while it is stood on: a button, in that
// who is on it is its `characters` (and `things`), filled by footsteps the
// way every button's is; the battle tick does the mending while either is
// non-empty.
type HealWant struct{ Want }

func (h *HealWant) GetLocals() *HealLocals {
	return CheckLocalsInitialized[HealLocals](&h.Want)
}

func (h *HealWant) Initialize() {
	if chars := h.GetStringSliceParam("characters"); len(chars) > 0 {
		h.SetCurrent("characters", chars)
	}
	if things := h.GetStringSliceParam("things"); len(things) > 0 {
		h.SetCurrent("things", things)
	}
}

func (h *HealWant) IsAchieved() bool { return false }
func (h *HealWant) Progress()        {}

type BattleResetLocals struct{}

// BattleResetWant puts a room defense back as it was when stood on — a
// button, its `characters` filled by footsteps; the battle tick does the
// putting back (resetBattle).
type BattleResetWant struct{ Want }

func (b *BattleResetWant) GetLocals() *BattleResetLocals {
	return CheckLocalsInitialized[BattleResetLocals](&b.Want)
}

func (b *BattleResetWant) Initialize()      {}
func (b *BattleResetWant) IsAchieved() bool { return false }
func (b *BattleResetWant) Progress()        {}
