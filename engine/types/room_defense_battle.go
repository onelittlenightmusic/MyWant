package types

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	. "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
)

// A room defense runs its battle itself, in its own Progress.
//
// It is the third party that joins the battle's wants: it brings its
// enemies in (wants of their own, added through the chain builder), says
// what each of them goes for and what each turret is aimed at (a connection
// between the two — a subscription, drawn on the board as the road between
// them), and wires who can reach whom (each act exposed under its own key,
// imported by what it can land on — battle_send.go). All of that is
// configuration it asks the chain builder for; the reconcile puts it in place,
// outside every Progress. Its own state — the phase, the round, how many came
// in and fell — it writes itself. What anything does is that want's own:
// a turret's shots, an enemy's blows and steps, a heal's mends, a hit taken.
//
// Where the board's walls are and where a character stands are the server's
// to know: it hands them in through BlockedCells and CharacterAt.

// BlockedCells is the board's cells nothing may stand on (walls, locked
// doors), given every want — set by the server, which knows their shapes.
var BlockedCells func(all []*Want) map[[2]int]bool

// CharacterAt is where a character stands on the board now — set by the
// server, which keeps the characters' cursors.
var CharacterAt func(characterID string) (x, y float64, ok bool)

const (
	enemyWantType    = "enemy"
	bossWantType     = "boss"
	freezeWantType   = "freeze"
	treasureWantType = "treasure"
	characterMotion  = "character_motion"
	// What names the battle an enemy belongs to. A label, not an owner
	// reference: a want under another is drawn only when its owner is
	// opened, and an enemy is a tile out on the board.
	EnemyOfLabel = "mywant.io/enemy-of"
	canvasX      = "mywant.io/canvas-x"
	canvasY      = "mywant.io/canvas-y"
	// How often a battle looks round.
	battleEvery = 250 * time.Millisecond
	// How long an enemy brought down stays on the board, for its fall to be
	// seen.
	fallLinger = 900 * time.Millisecond
	// A character's attack holds while the enemy is no further than this.
	characterReach = 6.0
)

// cell is where a tile stands.
type cell struct{ x, y int }

func (c cell) dist(o cell) float64 { return math.Hypot(float64(c.x-o.x), float64(c.y-o.y)) }

func tileCell(w *Want) (cell, bool) {
	x, y, ok := cellOf(w)
	return cell{x, y}, ok
}

// running: a want in play.
func running(w *Want) bool {
	switch w.GetStatus() {
	case WantStatusSuspended, WantStatusFailed, WantStatusTerminated,
		WantStatusDeleting, WantStatusArchived, WantStatusCancelled:
		return false
	}
	return true
}

// defendable: a want a battle's enemies go for and a heal mends — one that
// looks after its own hp.
func defendable(w *Want) bool {
	switch w.Metadata.Type {
	case treasureWantType, "turret", freezeWantType, "heal":
		return w.GetMetadata().Labels["mywant.io/canvas-hidden"] != "true"
	}
	return false
}

func isEnemy(w *Want) bool { return w.Metadata.Type == enemyWantType || w.Metadata.Type == bossWantType }

// RoomDefenseLocals is what a battle keeps between its turns.
type RoomDefenseLocals struct {
	last         time.Time
	lastSpawn    time.Time
	stoodOn      bool
	resetStoodOn map[string]bool
	// Enemies brought down, and when — off the board after fallLinger.
	fallen map[string]time.Time
}

// Progress: the battle's turn.
func (d *RoomDefenseWant) Progress() {
	l := d.GetLocals()
	now := time.Now()
	if now.Sub(l.last) < battleEvery {
		return
	}
	l.last = now
	if l.resetStoodOn == nil {
		l.resetStoodOn = map[string]bool{}
		l.fallen = map[string]time.Time{}
	}
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	all := cb.GetWants()
	var enemies []*Want
	for _, w := range all {
		if isEnemy(w) && w.GetMetadata().Labels[EnemyOfLabel] == d.Metadata.ID {
			enemies = append(enemies, w)
		}
	}

	// The fallen, gone once they have been seen to fall.
	standing := enemies[:0:0]
	var cleared []string
	for _, e := range enemies {
		at, down := l.fallen[e.Metadata.ID]
		switch {
		case !down:
			standing = append(standing, e)
		case now.Sub(at) >= fallLinger:
			cleared = append(cleared, e.Metadata.ID)
			delete(l.fallen, e.Metadata.ID)
		}
	}
	enemies = standing
	if len(cleared) > 0 {
		_ = cb.QueueWantDelete(cleared)
	}

	// A reset stood on puts this battle back as it was.
	for _, r := range all {
		if r.Metadata.Type != "battle_reset" {
			continue
		}
		if b := r.GetStringParam("battle", ""); b != d.Metadata.Name && b != d.Metadata.ID {
			continue
		}
		on := len(GetCurrent(r, "characters", []string{})) > 0
		got := on && !l.resetStoodOn[r.Metadata.ID]
		l.resetStoodOn[r.Metadata.ID] = on
		if got {
			d.reset(cb, all, enemies)
			enemies = nil
		}
	}

	// It says which round it is, from the first: an expose only publishes
	// what changes after it is there.
	exposeAs(&d.Want, "round", RoundKey(d.Metadata.Name))

	phase := GetCurrent(d, "phase", "ready")
	on := len(GetCurrent(d, "characters", []string{})) > 0
	got := on && !l.stoodOn
	l.stoodOn = on
	if got && phase != "fighting" {
		if !d.holdsAnything(all) {
			// Nothing standing to defend: a battle begun now would be lost
			// the turn it began. The tile says why instead.
			d.SetCurrent("say", "守るものがありません — 部屋の want を再開してください")
		} else {
			d.clearRoom(all)
			d.begin()
			phase = "fighting"
		}
	}
	if phase != "fighting" {
		d.clearRoom(all)
		// Not under way: its enemies leave the board with it.
		var gone []string
		for _, e := range enemies {
			gone = append(gone, e.Metadata.ID)
		}
		if len(gone) > 0 {
			_ = cb.QueueWantDelete(gone)
		}
		return
	}
	d.step(cb, all, enemies, now)
}

// step is a turn of a battle under way.
func (d *RoomDefenseWant) step(cb *ChainBuilder, all, enemyList []*Want, now time.Time) {
	l := d.GetLocals()
	enemies := map[string]*Want{}
	for _, e := range enemyList {
		enemies[e.Metadata.ID] = e
	}

	// Who can reach whom, as wiring.
	d.wire(all, enemyList)

	// What the room holds, still standing.
	targets := map[string]*Want{}
	var names []string
	for _, w := range d.inRoom(all) {
		if defendable(w) && running(w) {
			targets[w.Metadata.ID] = w
			names = append(names, w.Metadata.Name)
		}
	}
	sort.Strings(names)
	if !sameNames(GetCurrent(d, "targets", []string{}), names) {
		d.SetCurrent("targets", names)
	}

	spawned := GetCurrent(d, "spawned", 0)
	defeated := GetCurrent(d, "defeated", 0)
	total := d.GetIntParam("enemies", 5)

	// Coming in, one after another — when the way in is clear.
	occupied := map[cell]bool{}
	for _, w := range all {
		if c, ok := tileCell(w); ok {
			occupied[c] = true
		}
	}
	spawnAt := cell{d.GetIntParam("spawn_x", 0), d.GetIntParam("spawn_y", 0)}
	if spawned < total && !occupied[spawnAt] && now.Sub(l.lastSpawn).Seconds() >= d.GetFloatParam("spawn_every", 4) {
		l.lastSpawn = now
		if err := cb.QueueWantAdd([]*Want{d.enemy(spawned+1, spawnAt)}); err == nil {
			spawned++
			d.SetCurrent("spawned", spawned)
		}
	}

	// An enemy brought down says so itself: counted, let go of, and left a
	// moment for its fall to be seen.
	for eid, e := range enemies {
		if !GetCurrent(e, "defeated", false) {
			continue
		}
		if _, counted := l.fallen[eid]; !counted {
			l.fallen[eid] = now
			SetAttack(e, nil)
			defeated++
			d.SetCurrent("defeated", defeated)
		}
		delete(enemies, eid)
	}

	// Each enemy goes for the nearest want in the room — its connection;
	// its steps and blows are its own (EnemyWant).
	goingFor := map[string]string{}
	for eid, e := range enemies {
		at, ok := tileCell(e)
		if !ok {
			continue
		}
		best, bestD := "", math.Inf(1)
		for tid, t := range targets {
			if c, ok := tileCell(t); ok {
				if dd := at.dist(c); dd < bestD {
					best, bestD = tid, dd
				}
			}
		}
		if best == "" {
			SetAttack(e, nil)
			continue
		}
		goingFor[eid] = best
		SetAttack(e, targets[best])
	}

	// Each turret is aimed: at the enemy it is already on while it is in
	// reach, or one its strategy picks.
	maxTarget := d.GetFloatParam("target_hp", 10)
	for _, t := range d.inRoom(all) {
		if t.Metadata.Type != "turret" && t.Metadata.Type != freezeWantType || !running(t) {
			continue
		}
		tc, ok := tileCell(t)
		if !ok {
			continue
		}
		var e *Want
		if eid := aimTurret(t, tc, enemies, goingFor, targets, maxTarget); eid != "" {
			e = enemies[eid]
		}
		SetAttack(t, e)
	}

	// A character locked on to one of these enemies holds while near enough,
	// and lets go of one gone.
	alive := map[string]bool{}
	for _, e := range enemies {
		alive[e.Metadata.Name] = true
	}
	for _, f := range all {
		if f.Metadata.Type != characterMotion {
			continue
		}
		on := aimedAt(f)
		if !strings.HasPrefix(on, d.Metadata.Name+"-") {
			continue // not one of this battle's
		}
		if !alive[on] {
			SetAttack(f, nil)
			continue
		}
		var ec cell
		for _, e := range enemies {
			if e.Metadata.Name == on {
				ec, _ = tileCell(e)
			}
		}
		if CharacterAt == nil {
			continue
		}
		if x, y, ok := CharacterAt(f.GetStringParam("character_id", "")); ok &&
			math.Hypot(x-float64(ec.x), y-float64(ec.y)) > characterReach {
			SetAttack(f, nil)
		}
	}

	// Over: every enemy in and down, or nothing left to defend.
	phase := ""
	switch {
	case spawned >= total && len(enemies) == 0:
		phase = "cleared"
	case len(targets) == 0 && spawned > 0:
		phase = "lost"
	}
	if phase != "" {
		d.SetCurrent("phase", phase)
		if phase == "cleared" {
			d.SetCurrent("say", fmt.Sprintf("守りきった！ %d 体倒しました", defeated))
		} else {
			d.SetCurrent("say", fmt.Sprintf("全部やられた… %d 体倒しました。want を再開してもう一度", defeated))
		}
		d.clearRoom(all)
	}
}

// begin starts a battle afresh, in a new round.
func (d *RoomDefenseWant) begin() {
	d.SetCurrent("round", GetCurrent(d, "round", 0)+1)
	d.SetCurrent("spawned", 0)
	d.SetCurrent("defeated", 0)
	d.SetCurrent("targets", []string{})
	d.SetCurrent("phase", "fighting")
	l := d.GetLocals()
	l.lastSpawn = time.Time{}
	l.fallen = map[string]time.Time{}
}

// reset puts the battle back as it was: its enemies off the board, what
// stopped itself in its room running again (each makes itself whole when it
// hears the new round), and ready to begin.
func (d *RoomDefenseWant) reset(cb *ChainBuilder, all, enemies []*Want) {
	var gone, resume []string
	for _, e := range enemies {
		gone = append(gone, e.Metadata.ID)
	}
	if len(gone) > 0 {
		_ = cb.QueueWantDelete(gone)
	}
	for _, w := range d.inRoom(all) {
		if defendable(w) && w.GetStatus() == WantStatusSuspended {
			resume = append(resume, w.Metadata.ID)
		}
	}
	if len(resume) > 0 {
		_ = cb.QueueWantResume(resume)
	}
	d.clearRoom(all)
	d.SetCurrent("round", GetCurrent(d, "round", 0)+1)
	d.SetCurrent("spawned", 0)
	d.SetCurrent("defeated", 0)
	d.SetCurrent("targets", []string{})
	d.SetCurrent("phase", "ready")
	d.SetCurrent("say", fmt.Sprintf("元に戻しました（%d 個を再開）— 乗ると始まります", len(resume)))
}

// enemy is the n-th enemy, at the way in: a want tile of its own, its battle
// on its label, the battle's numbers its own params — and its act exposed
// from the first.
func (d *RoomDefenseWant) enemy(n int, at cell) *Want {
	maxHP := d.GetFloatParam("enemy_hp", 3)
	damage := d.GetFloatParam("enemy_damage", 1)
	name := fmt.Sprintf("%s-enemy-%d", d.Metadata.Name, n)
	typ := enemyWantType
	if n == d.GetIntParam("enemies", 5) && d.GetBoolParam("boss", true) {
		typ, maxHP, damage = bossWantType, d.GetFloatParam("boss_hp", 30), d.GetFloatParam("boss_damage", 3)
		name = fmt.Sprintf("%s-boss", d.Metadata.Name)
		d.SetCurrent("say", "ボスが現れた！")
	}
	labels := map[string]string{
		canvasX:      strconv.Itoa(at.x),
		canvasY:      strconv.Itoa(at.y),
		EnemyOfLabel: d.Metadata.ID,
	}
	// The rest are small — half a tile — so the boss, whole, stands over them.
	if typ == enemyWantType {
		labels["mywant.io/canvas-size"] = "0.5"
	}
	return &Want{
		Metadata: Metadata{ID: fmt.Sprintf("enemy-%s-%d", d.Metadata.ID, n), Name: name, Type: typ, Labels: labels},
		Spec: WantSpec{
			Params: map[string]any{
				"max_hp": maxHP, "damage": damage, "interval": 1.0,
				"speed": d.GetFloatParam("enemy_speed", 0.6),
			},
			Exposes: []ws.ExposeEntry{{CurrentState: strikeField, As: ActionKey(name, actAttack)}},
		},
	}
}

// inRoom is every want standing in the battle's room, enemies aside.
func (d *RoomDefenseWant) inRoom(all []*Want) []*Want {
	rx, ry := d.GetIntParam("room_x", 0), d.GetIntParam("room_y", 0)
	rw, rh := d.GetIntParam("room_w", 8), d.GetIntParam("room_h", 6)
	var in []*Want
	for _, w := range all {
		if c, ok := tileCell(w); ok && c.x >= rx && c.x < rx+rw && c.y >= ry && c.y < ry+rh && !isEnemy(w) {
			in = append(in, w)
		}
	}
	return in
}

// holdsAnything: something in the room stands to be defended.
func (d *RoomDefenseWant) holdsAnything(all []*Want) bool {
	for _, w := range d.inRoom(all) {
		if defendable(w) && running(w) {
			return true
		}
	}
	return false
}

// clearRoom: turrets in a battle not under way are aimed at nothing (a
// heal's connections were made by hand, and outlast the battle).
func (d *RoomDefenseWant) clearRoom(all []*Want) {
	for _, w := range d.inRoom(all) {
		if w.Metadata.Type == "turret" || w.Metadata.Type == freezeWantType {
			SetLinks(w, nil)
		}
	}
}

// wire: each want that acts exposes its act under its own key; each want
// acted on imports the acts that can reach it — the room's wants: the
// enemies' blows and the heals' mends; an enemy: the turrets', freezes' and
// characters' attacks. An import is added only once there is something
// behind it, so it never holds the importer back.
func (d *RoomDefenseWant) wire(all, enemies []*Want) {
	var defenders, shooters, heals, characters []*Want
	for _, w := range d.inRoom(all) {
		switch w.Metadata.Type {
		case "turret", freezeWantType:
			shooters = append(shooters, w)
		case "heal":
			heals = append(heals, w)
		}
		if defendable(w) {
			defenders = append(defenders, w)
		}
	}
	for _, w := range all {
		if w.Metadata.Type == characterMotion {
			characters = append(characters, w)
		}
	}
	for _, w := range append(append(append([]*Want(nil), shooters...), characters...), enemies...) {
		exposeAs(w, strikeField, ActionKey(w.Metadata.Name, actAttack))
	}
	for _, w := range heals {
		exposeAs(w, pulseField, ActionKey(w.Metadata.Name, actHeal))
	}

	round := RoundKey(d.Metadata.Name)
	var blows, shots []string
	for _, e := range enemies {
		blows = append(blows, ActionKey(e.Metadata.Name, actAttack))
	}
	for _, h := range heals {
		blows = append(blows, ActionKey(h.Metadata.Name, actHeal))
	}
	for _, w := range append(append([]*Want(nil), shooters...), characters...) {
		shots = append(shots, ActionKey(w.Metadata.Name, actAttack))
	}
	for _, w := range defenders {
		if v, ok := GetGlobalState(round); ok && v != nil {
			importAs(w, round, RoundImportKey)
		}
		importActs(w, blows)
	}
	for _, e := range enemies {
		importActs(e, shots)
	}
}

// aimTurret is which enemy in reach a turret hits: the one it is already
// connected to while that one is in reach — so whatever connects it decides
// — or else by its strategy:
//
//	nearest  the closest (the default)
//	weakest  the one with least left — fewest hits to take it off the board
//	guard    the one going for whatever in the room has least left
func aimTurret(t *Want, at cell, enemies map[string]*Want, goingFor map[string]string,
	targets map[string]*Want, maxTarget float64) string {
	reach := t.GetFloatParam("range", 3)
	inReach := func(e *Want) bool { c, ok := tileCell(e); return ok && at.dist(c) <= reach }
	// A freeze keeps to what is not held yet: holding one twice is a shot spent.
	if t.Metadata.Type == freezeWantType {
		free := map[string]*Want{}
		for eid, e := range enemies {
			if frozen, _ := e.GetStateBool("frozen", false); !frozen {
				free[eid] = e
			}
		}
		if len(free) > 0 {
			enemies = free
		}
	}
	if on := aimedAt(t); on != "" {
		for eid, e := range enemies {
			if e.Metadata.Name == on && inReach(e) {
				return eid
			}
		}
	}
	hpOf := func(w *Want, fallback float64) float64 { v, _ := w.GetStateFloat64("hp", fallback); return v }
	score := func(e *Want) float64 { c, _ := tileCell(e); return at.dist(c) }
	switch t.GetStringParam("strategy", "nearest") {
	case "weakest":
		score = func(e *Want) float64 { return hpOf(e, 0) }
	case "guard":
		score = func(e *Want) float64 {
			if tw, ok := targets[goingFor[e.Metadata.ID]]; ok {
				return hpOf(tw, maxTarget)
			}
			return math.Inf(1)
		}
	}
	best, bestScore := "", math.Inf(1)
	for eid, e := range enemies {
		if !inReach(e) {
			continue
		}
		if sc := score(e); best == "" || sc < bestScore {
			best, bestScore = eid, sc
		}
	}
	return best
}

// stepToward is the next cell from `from` toward `to`: along the longer way
// first, the other if that one is blocked.
func stepToward(from, to cell, blocked func(cell) bool) (cell, bool) {
	sign := func(v int) int {
		switch {
		case v > 0:
			return 1
		case v < 0:
			return -1
		}
		return 0
	}
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	dx, dy := to.x-from.x, to.y-from.y
	horiz := cell{from.x + sign(dx), from.y}
	vert := cell{from.x, from.y + sign(dy)}
	tries := []cell{horiz, vert}
	if abs(dy) > abs(dx) {
		tries = []cell{vert, horiz}
	}
	for _, c := range tries {
		if c != from && !blocked(c) {
			return c, true
		}
	}
	return from, false
}

// SetAttack makes a want's connection the one to its target — or none.
func SetAttack(attacker, target *Want) {
	var to []*Want
	if target != nil {
		to = []*Want{target}
	}
	SetLinks(attacker, to)
}

// SetLinks makes a want's connections the ones to `to` — a subscription to
// each one's hp, drawn as a road from it to each — asked of the chain
// builder, and only when it changes.
func SetLinks(from *Want, to []*Want) {
	spec := from.GetSpec()
	if spec == nil {
		return
	}
	var want []ws.StateSubscription
	for _, t := range to {
		want = append(want, ws.StateSubscription{WantName: t.Metadata.Name, StateKeys: []string{"hp"}})
	}
	have := spec.StateSubscriptions
	same := len(have) == len(want)
	for i := 0; same && i < len(want); i++ {
		same = have[i].WantName == want[i].WantName
	}
	if same {
		return
	}
	spec.StateSubscriptions = want
	updateSpec(from, spec)
}

// importAs: w imports `key` as its local `local`, if it does not.
func importAs(w *Want, key, local string) {
	spec := w.GetSpec()
	if spec == nil || spec.Imports[key] == local {
		return
	}
	if spec.Imports == nil {
		spec.Imports = map[string]string{}
	}
	spec.Imports[key] = local
	updateSpec(w, spec)
}

// exposeAs: w exposes its state field under `key`, if it does not.
func exposeAs(w *Want, field, key string) {
	spec := w.GetSpec()
	if spec == nil {
		return
	}
	for _, e := range spec.Exposes {
		if e.CurrentState == field && e.As == key {
			return
		}
	}
	spec.Exposes = append(spec.Exposes, ws.ExposeEntry{CurrentState: field, As: key})
	updateSpec(w, spec)
}

// importActs: w imports exactly these acts — each that has something behind
// it — under action-prefixed local keys; its other imports are its own.
func importActs(w *Want, keys []string) {
	spec := w.GetSpec()
	if spec == nil {
		return
	}
	want := map[string]string{}
	for gk, lk := range spec.Imports {
		if !strings.HasPrefix(lk, ActionImportPrefix) {
			want[gk] = lk
		}
	}
	for _, k := range keys {
		if v, ok := GetGlobalState(k); ok && v != nil {
			want[k] = ActionImportPrefix + k
		}
	}
	if len(want) == len(spec.Imports) {
		same := true
		for k, v := range want {
			if spec.Imports[k] != v {
				same = false
				break
			}
		}
		if same {
			return
		}
	}
	spec.Imports = want
	updateSpec(w, spec)
}

// updateSpec asks the chain builder for w's new spec — with its state as it
// is now, so applying the change does not put an older copy back — unless it
// is on its way out.
func updateSpec(w *Want, spec *WantSpec) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		// No board to ask (a test): the want itself.
		w.Spec = *spec
		return
	}
	live, _, found := cb.FindWantByID(w.Metadata.ID)
	if !found || live.GetStatus() == WantStatusDeleting {
		return
	}
	cfg := &Want{Metadata: live.GetMetadata(), Spec: *spec}
	StoreStateMulti(cfg, live.GetAllStateDeep())
	cb.UpdateWant(cfg)
}

func sameNames(have []string, want []string) bool {
	if len(have) != len(want) {
		return false
	}
	h := append([]string(nil), have...)
	sort.Strings(h)
	for i := range h {
		if h[i] != want[i] {
			return false
		}
	}
	return true
}
