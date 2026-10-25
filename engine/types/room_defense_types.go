package types

import (
	"math"
	"strconv"
	"time"

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
		// What a room holds to be defended (treasure.yaml).
		RegisterWantImplementation[TreasureWant, TreasureLocals]("treasure")
	})
}

// RoomDefenseWant is a battle in a room of the board — run by itself, in its
// own Progress (room_defense_battle.go): it brings its enemies in, joins the
// battle's wants to one another, and keeps how it is going (its state).
// Never achieved: a battle cleared is waiting to be begun again, and only a
// want still running hears someone step onto it.
type RoomDefenseWant struct{ Want }

func (d *RoomDefenseWant) GetLocals() *RoomDefenseLocals {
	return CheckLocalsInitialized[RoomDefenseLocals](&d.Want)
}

func (d *RoomDefenseWant) Initialize() {
	// Waiting for someone to stand on it.
	if phase, _ := d.GetCurrent("phase"); phase == nil || phase == "" {
		d.SetCurrent("phase", "ready")
	}
}

func (d *RoomDefenseWant) IsAchieved() bool { return false }

type TurretLocals struct {
	shot sendClock
	body body
}

// TurretWant is a want tile that hits enemies near it by itself. What it is
// aimed at is its connection (the battle's aim, or a deciding want's). How
// hard it hits and how often are its own: every `interval` seconds while it
// is aimed at something, one shot in its `strike`, for that one (its
// `damage`; a freeze's `hold` too) — see battle_send.go. Standing in a room,
// it is gone for too: its hp is its own, worn down by the blows sent to it
// and mended by what heals it, and at 0 it stops itself.
type TurretWant struct{ Want }

func (t *TurretWant) GetLocals() *TurretLocals {
	return CheckLocalsInitialized[TurretLocals](&t.Want)
}

func (t *TurretWant) Initialize() {
	keepHP(&t.Want, 10)
	t.GetLocals().shot.start(&t.Want, strikeField)
}

func (t *TurretWant) IsAchieved() bool { return false }

func (t *TurretWant) Progress() {
	l := t.GetLocals()
	if l.body.takeActs(&t.Want) {
		return
	}
	on := aimedAt(&t.Want)
	if on == "" {
		return
	}
	body := map[string]any{}
	impact := ImpactAttack
	if t.Metadata.Type == "freeze" {
		// A freeze holds what it hits, for a while; it may wear it a little.
		body["damage"] = t.GetFloatParam("damage", 0)
		body["hold"] = t.GetFloatParam("duration", 2.5)
		impact = ImpactFreeze
	} else {
		body["damage"] = t.GetFloatParam("damage", 1)
	}
	if l.shot.send(&t.Want, strikeField, t.GetFloatParam("interval", 1.5), body, []string{on}, impact, []string{on}) {
		t.SetCurrent("shots", l.shot.seq)
	}
}

type EnemyLocals struct {
	blow sendClock
	in   inbox
	// When it last took a step.
	lastStep time.Time
}

// EnemyWant is one enemy of a room defense: a want tile of its own, as a
// character has a want of its own — where it is is its tile's place. What it
// goes for is its connection (the battle's), and its blows are its own: every
// `interval` seconds while it goes for something and is not held, one in its
// `strike` for that one (its `damage`), which lands if it is next to it.
// What is left of it is its own too: the shots sent to it come off its hp,
// a freeze's hold keeps it `frozen` a while, and at 0 it is `defeated`.
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
	l := e.GetLocals()
	l.blow.start(&e.Want, strikeField)
	// Just come in: a step's time before the first.
	l.lastStep = time.Now()
}

func (e *EnemyWant) IsAchieved() bool { return false }

func (e *EnemyWant) Progress() {
	if GetCurrent(e, "defeated", false) {
		return
	}
	l := e.GetLocals()
	hp, _ := e.GetStateFloat64("hp", e.GetFloatParam("max_hp", 3))
	now := time.Now()
	l.in.take(&e.Want, func(a act) {
		hp = math.Max(0, hp-a.damage)
		if a.hold > 0 {
			e.SetCurrent("frozen", true)
			e.SetCurrent("frozen_until", now.Add(time.Duration(a.hold*float64(time.Second))).Format(time.RFC3339Nano))
		}
	})
	e.SetCurrent("hp", math.Round(hp*100)/100)
	if hp <= 0 {
		e.SetCurrent("defeated", true)
		return
	}
	// A hold runs out by itself.
	if GetCurrent(e, "frozen", false) {
		until, err := time.Parse(time.RFC3339Nano, GetCurrent(e, "frozen_until", ""))
		if err == nil && now.Before(until) {
			return
		}
		e.SetCurrent("frozen", false)
	}
	on := aimedAt(&e.Want)
	if GetCurrent(e, "target", "") != on {
		e.SetCurrent("target", on)
	}
	if on == "" {
		return
	}
	// A blow lands only next to what it goes for (diagonals included); from
	// further off it walks there instead, a cell at a time.
	if !nextTo(&e.Want, on, 1.5) {
		e.stepTo(on, now)
		return
	}
	l.blow.send(&e.Want, strikeField, e.GetFloatParam("interval", 1),
		map[string]any{"damage": e.GetFloatParam("damage", 1)}, []string{on}, ImpactAttack, []string{on})
}

// HealLocals is what a heal keeps between its turns.
type HealLocals struct {
	mend sendClock
	body body
}

// HealWant mends, and is the one that says how much and when.
//
// Every `interval` seconds, while it has something to mend, it sends one
// pulse in its own `pulse` — for each want it mends, its share (`to`): its
// amount per second × its interval in all, shared among them, so mending more
// is never more mending. What it mends is what it is connected to (the 接続
// skill); connected to nothing, it is a button, and while someone or
// something is on it (its `characters`, `things`, filled by footsteps as
// every button's are) it mends the hurt wants in its reach. Each one mended
// takes its share itself (battle_send.go). Standing in a room it is gone for
// too, and its hp is its own.
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
	keepHP(&h.Want, 10)
	h.GetLocals().mend.start(&h.Want, pulseField)
}

func (h *HealWant) IsAchieved() bool { return false }

func (h *HealWant) Progress() {
	l := h.GetLocals()
	if l.body.takeActs(&h.Want) {
		return
	}
	names := connectedTo(&h.Want)
	if len(names) == 0 && (len(GetCurrent(h, "characters", []string{})) > 0 || len(GetCurrent(h, "things", []string{})) > 0) {
		names = hurtInReach(&h.Want, h.GetFloatParam("range", 2))
	}
	if len(names) == 0 {
		if len(GetCurrent(h, "healing", []string{})) > 0 {
			h.SetCurrent("healing", []string{})
		}
		return
	}
	interval := h.GetFloatParam("interval", 1)
	share := h.GetFloatParam("amount", 2) * interval / float64(len(names))
	to := map[string]any{}
	for _, n := range names {
		to[n] = share
	}
	if l.mend.send(&h.Want, pulseField, interval, map[string]any{"amount": share * float64(len(names))}, to, ImpactMend, names) {
		h.SetCurrent("healing", names)
	}
}

// stepTo takes one step toward the want named `on`, at its own speed — round
// what stands in the way and the board's walls — by asking for its own tile
// to be moved.
func (e *EnemyWant) stepTo(on string, now time.Time) {
	l := e.GetLocals()
	if now.Sub(l.lastStep).Seconds() < 1/math.Max(e.GetFloatParam("speed", 0.6), 0.05) {
		return
	}
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	target, found := cb.FindWantByName(on)
	if !found {
		return
	}
	from, ok := tileCell(&e.Want)
	to, ok2 := tileCell(target)
	if !ok || !ok2 {
		return
	}
	all := cb.GetWants()
	occupied := map[cell]bool{}
	for _, w := range all {
		if c, ok := tileCell(w); ok {
			occupied[c] = true
		}
	}
	var walls map[[2]int]bool
	if BlockedCells != nil {
		walls = BlockedCells(all)
	}
	next, moved := stepToward(from, to, func(c cell) bool { return occupied[c] || walls[[2]int{c.x, c.y}] })
	if !moved {
		return
	}
	l.lastStep = now
	_ = cb.QueueWantAddLabel(e.Metadata.ID, canvasX, strconv.Itoa(next.x))
	_ = cb.QueueWantAddLabel(e.Metadata.ID, canvasY, strconv.Itoa(next.y))
}

// hurtInReach is the wants within `reach` cells of w on the board that have
// hp to mend — read off them, never written.
func hurtInReach(w *Want, reach float64) []string {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil
	}
	hx, hy, ok := cellOf(w)
	if !ok {
		return nil
	}
	var names []string
	for _, o := range cb.GetWants() {
		if o.Metadata.ID == w.Metadata.ID {
			continue
		}
		x, y, ok := cellOf(o)
		if !ok || math.Hypot(float64(x-hx), float64(y-hy)) > reach {
			continue
		}
		hp, hasHP := o.GetStateFloat64("hp", 0)
		max, hasMax := o.GetStateFloat64("max_hp", 0)
		if hasHP && hasMax && hp > 0 && hp < max {
			names = append(names, o.Metadata.Name)
		}
	}
	return names
}

// nextTo: the want named `name` stands within `reach` cells of w — read
// off its labels, never written.
func nextTo(w *Want, name string, reach float64) bool {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return false
	}
	other, found := cb.FindWantByName(name)
	if !found {
		return false
	}
	x, y, ok := cellOf(w)
	ox, oy, ok2 := cellOf(other)
	return ok && ok2 && math.Hypot(float64(x-ox), float64(y-oy)) <= reach
}

// cellOf is where a want's tile stands on the board, from its labels.
func cellOf(w *Want) (int, int, bool) {
	labels := w.GetMetadata().Labels
	x, errX := strconv.Atoi(labels["mywant.io/canvas-x"])
	y, errY := strconv.Atoi(labels["mywant.io/canvas-y"])
	return x, y, errX == nil && errY == nil
}

// TreasureLocals is what a treasure keeps between its turns.
type TreasureLocals struct{ body body }

// TreasureWant is what a room holds to be defended: a want with something
// written on it (its `content`, as a note's), and an hp of its own — worn
// down by the blows sent to it, mended by what heals it, and at 0 it stops
// itself, until it is resumed (or its battle put back).
type TreasureWant struct{ Want }

func (t *TreasureWant) GetLocals() *TreasureLocals {
	return CheckLocalsInitialized[TreasureLocals](&t.Want)
}

func (t *TreasureWant) Initialize()      { keepHP(&t.Want, 10) }
func (t *TreasureWant) IsAchieved() bool { return false }
func (t *TreasureWant) Progress()        { t.GetLocals().body.takeActs(&t.Want) }

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
