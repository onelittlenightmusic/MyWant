package server

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"

	ws "github.com/onelittlenightmusic/want-spec"
	mywant "mywant/engine/core"
)

// A battle on the board: a room_defense want, its enemies, and the turrets
// that fight them — every one of them a want, and everything about the battle
// their labels and their fields.
//
// An enemy is a want tile of its own (`enemy`, its battle on a label):
// where it is is the canvas-x/y every tile has, what is left of it its `hp`.
// It goes for the room's want tiles one cell at a time, and wears down the one
// it reaches — whose `hp` field is what is left of it. A turret is a want tile
// that hits an enemy in reach every so often.
//
// Who is attacking whom is a connection between the two wants: the attacker
// subscribes to its target's `hp` (Spec.StateSubscriptions), which the board
// draws as a road between them — an enemy to what it goes for, a turret to
// the enemy it hits. The connection is also how a turret is aimed: it keeps on
// the enemy it is subscribed to while that one is in reach, so a want that
// decides how to fight (reading the enemies' fields) aims a turret by changing
// what it subscribes to. Left to itself a turret picks by its strategy.
//
// Worn down to nothing, a want is suspended — a real one, resumed by hand once
// the battle is over (the battle's `suspended` says which).

const (
	enemyWantType = "enemy"
	// A want tile that holds an enemy where it is for a while when it hits it
	// (freeze.yaml): a turret whose shot stops rather than wears down.
	freezeWantType = "freeze"
	// The field saying an enemy is held by a freeze.
	frozenField = "frozen"
	// The field saying an enemy has been brought down (fallLinger).
	defeatedField = "defeated"
	// How long an enemy brought down stays on the board. Deleted in the tick
	// it fell, its tile was gone before the board heard of the hit that did
	// it — the one hit worth seeing land, and hearing, was never shown.
	fallLinger = 900 * time.Millisecond
	// The last to come in, when the battle has one: an enemy of its own kind
	// (its own face on the board), harder to bring down and harder-hitting.
	bossWantType = "boss"
	// How near an enemy has to be to what it is at to wear it down: next to
	// it, diagonals included.
	enemyReach = 1.5
	// A character's attack (the 攻撃 skill): one hit this often, while the
	// enemy it is connected to is no further than this — beyond it the
	// connection lets go.
	characterHitEvery = time.Second
	characterHit      = 1.0
	characterReach    = 6.0
	// The field a battle wears down — on an enemy, and on what it attacks.
	hpField    = "hp"
	maxHPField = "max_hp"
	// What a battle's connections just did, kept on the want that acted
	// (core's RecordImpact).
	impactAttack = mywant.ImpactAttack
	impactMend   = mywant.ImpactMend
	impactFreeze = mywant.ImpactFreeze
)

// impact records that `from`'s connections just had their effect: of what
// kind, and on which wants.
func impact(from *mywant.Want, kind string, on ...string) {
	from.RecordImpact(kind, on, nil)
}

// battleStoodOn remembers, per battle, whether someone was standing on it at
// the last tick: getting on is what starts one.
var battleStoodOn = map[string]bool{}

// battleRun is what a battle keeps between ticks in memory: when each enemy
// last took a step and each turret last fired.
type battleRun struct {
	lastSpawn time.Time
	lastStep  map[string]time.Time // enemy want id → when it last moved
	lastFired map[string]time.Time // turret or character want id → when it last hit
	frozenTil map[string]time.Time // enemy want id → until when a freeze holds it
}

var (
	battlesMu sync.Mutex
	battles   = map[string]*battleRun{} // room_defense want id → its run
	// Enemies brought down, and when: out of the battle at once, off the
	// board a moment later (fallLinger).
	fallen = map[string]time.Time{}

)

func (s *Server) startBattles() {
	go func() {
		ticker := time.NewTicker(thingMotionInterval)
		defer ticker.Stop()
		for range ticker.C {
			s.battleTick()
		}
	}()
}

// battleWantRunning: a want that is in play.
func battleWantRunning(w *mywant.Want) bool {
	switch w.GetStatus() {
	case mywant.WantStatusSuspended, mywant.WantStatusFailed, mywant.WantStatusTerminated,
		mywant.WantStatusDeleting, mywant.WantStatusArchived, mywant.WantStatusCancelled:
		return false
	}
	return true
}

// enemyOfLabel names the battle an enemy belongs to. A label, not an owner
// reference: a want under another is drawn only when its owner is opened,
// and an enemy is a tile out on the board.
const enemyOfLabel = "mywant.io/enemy-of"

// ownerOf is the battle an enemy belongs to.
func ownerOf(w *mywant.Want) string { return w.Metadata.Labels[enemyOfLabel] }

func (s *Server) battleTick() {
	if s.globalBuilder == nil {
		return
	}
	all := s.globalBuilder.GetWants()
	var defenses, turrets, heals, fighters, resets []*mywant.Want
	enemiesOf := map[string][]*mywant.Want{}
	for _, w := range all {
		switch w.Metadata.Type {
		case "room_defense":
			defenses = append(defenses, w)
		case "turret", freezeWantType:
			if battleWantRunning(w) {
				turrets = append(turrets, w)
			}
		case enemyWantType, bossWantType:
			enemiesOf[ownerOf(w)] = append(enemiesOf[ownerOf(w)], w)
		case "heal":
			if battleWantRunning(w) {
				heals = append(heals, w)
			}
		case "battle_reset":
			resets = append(resets, w)
		case characterMotionWantType:
			// A character attacking: its own want connected to an enemy.
			if attackedBy(w) != "" {
				fighters = append(fighters, w)
			}
		}
	}

	battlesMu.Lock()
	defer battlesMu.Unlock()
	// The fallen are out of every battle; gone from the board once they have
	// been seen to fall.
	var cleared []string
	seen := map[string]bool{}
	for owner, list := range enemiesOf {
		standing := list[:0:0]
		for _, e := range list {
			seen[e.Metadata.ID] = true
			at, down := fallen[e.Metadata.ID]
			if !down {
				standing = append(standing, e)
				continue
			}
			if time.Since(at) >= fallLinger {
				cleared = append(cleared, e.Metadata.ID)
				delete(fallen, e.Metadata.ID)
			}
		}
		enemiesOf[owner] = standing
	}
	for id := range fallen {
		if !seen[id] {
			delete(fallen, id) // gone some other way: a reset, its battle over
		}
	}
	if len(cleared) > 0 {
		_ = s.globalBuilder.QueueWantDelete(cleared)
	}
	// A reset stood on puts its battle back as it was.
	for _, r := range resets {
		on := len(mywant.GetCurrent(r, "characters", []string{})) > 0
		gotOn := on && !battleStoodOn[r.Metadata.ID]
		battleStoodOn[r.Metadata.ID] = on
		if !gotOn {
			continue
		}
		name := r.GetStringParam("battle", "")
		for _, d := range defenses {
			if d.Metadata.Name == name || d.Metadata.ID == name {
				s.resetBattle(d, all, enemiesOf[d.Metadata.ID])
			}
		}
	}

	// What can never be worn down carries no hp — a character's chat, which
	// stands hidden on the character's own cell, and the like.
	// (Not an enemy: its hp is what it has left.)
	for _, w := range all {
		if !defendable(w) && !isEnemy(w) {
			clearBattleFields(w)
		}
	}
	live := map[string]bool{}
	for _, d := range defenses {
		phase, _ := d.GetStateString("phase", "ready")
		on := len(mywant.GetCurrent(d, "characters", []string{})) > 0
		gotOn := on && !battleStoodOn[d.Metadata.ID]
		battleStoodOn[d.Metadata.ID] = on
		// Someone getting on a battle that is waiting, or over: it begins.
		if gotOn && phase != "fighting" && battleWantRunning(d) {
			if !roomHoldsAnything(d, all) {
				// Nothing standing to defend — every want in it suspended by
				// the last battle, waiting to be resumed by hand: a battle
				// begun now would be lost the tick it began, and look like
				// nothing at all. The tile says why instead.
				d.SetCurrent("say", "守るものがありません — 部屋の want を再開してください")
			} else {
				clearRoom(d, all)
				startBattle(d)
				phase = "fighting"
			}
		}
		if !battleWantRunning(d) || phase != "fighting" {
			// Not under way: nothing in its room is worn — kept so each tick,
			// since a want suspended in the battle can write its fields back
			// after the battle has cleared them.
			clearRoom(d, all)
			continue
		}
		live[d.Metadata.ID] = true
		s.battleStep(d, all, turrets, fighters, enemiesOf[d.Metadata.ID])
	}
	// A character still connected to an enemy no battle has — defeated, or
	// its battle over: the connection lets go.
	for _, f := range fighters {
		on, alive := attackedBy(f), false
		for _, list := range enemiesOf {
			for _, e := range list {
				alive = alive || e.Metadata.Name == on
			}
		}
		if !alive {
			setAttack(f, nil)
		}
	}
	mendTick(heals, all)
	// A battle stopped, deleted or over: its enemies leave the board with it.
	var gone []string
	for owner, list := range enemiesOf {
		if !live[owner] {
			for _, e := range list {
				gone = append(gone, e.Metadata.ID)
			}
		}
	}
	if len(gone) > 0 {
		_ = s.globalBuilder.QueueWantDelete(gone)
	}
	for id := range battles {
		if !live[id] {
			delete(battles, id)
		}
	}
}

// cell is where a tile stands.
type cell struct{ x, y int }

func tileCell(w *mywant.Want) (cell, bool) {
	x, errX := strconv.Atoi(w.Metadata.Labels[canvasLabelX])
	y, errY := strconv.Atoi(w.Metadata.Labels[canvasLabelY])
	return cell{x, y}, errX == nil && errY == nil
}

func (c cell) dist(o cell) float64 { return math.Hypot(float64(c.x-o.x), float64(c.y-o.y)) }

// hpOf is what is left of a want in the battle. A field any want can carry —
// the wants a room holds were not written with one — so it is kept with
// StoreState, not as a declared current field.
func hpOf(w *mywant.Want, fallback float64) float64 {
	v, _ := w.GetStateFloat64(hpField, fallback)
	return v
}

func setHP(w *mywant.Want, hp float64) {
	w.StoreState(hpField, math.Max(0, math.Round(hp*100)/100))
}

func (s *Server) battleStep(d *mywant.Want, all, turrets, fighters, enemyWants []*mywant.Want) {
	id := d.Metadata.ID
	run := battles[id]
	if run == nil {
		run = &battleRun{lastStep: map[string]time.Time{}, lastFired: map[string]time.Time{}, frozenTil: map[string]time.Time{}}
		battles[id] = run
	}
	now := time.Now()

	// Where every tile stands: what an enemy cannot step onto, and what the
	// room holds.
	occupied := map[cell]bool{}
	for _, w := range all {
		if c, ok := tileCell(w); ok {
			occupied[c] = true
		}
	}
	walls := s.blockedCellSnapshot()

	enemies := map[string]*mywant.Want{}
	for _, e := range enemyWants {
		enemies[e.Metadata.ID] = e
	}

	// What the room holds: its want tiles, still standing.
	rx, ry := d.GetIntParam("room_x", 0), d.GetIntParam("room_y", 0)
	rw, rh := d.GetIntParam("room_w", 8), d.GetIntParam("room_h", 6)
	inRoom := func(c cell) bool { return c.x >= rx && c.x < rx+rw && c.y >= ry && c.y < ry+rh }
	maxTarget := d.GetFloatParam("target_hp", 10)
	targets := map[string]*mywant.Want{}
	var names []string
	for _, w := range all {
		if !defendable(w) || !battleWantRunning(w) {
			continue
		}
		if c, ok := tileCell(w); ok && inRoom(c) {
			targets[w.Metadata.ID] = w
			names = append(names, w.Metadata.Name)
			if _, ok := w.GetStateFloat64(hpField, 0); !ok {
				setHP(w, maxTarget)
				w.StoreState(maxHPField, maxTarget)
			}
		}
	}
	if !sameStrings(mywant.GetCurrent(d, "targets", []any{}), names) {
		d.SetCurrent("targets", names)
	}

	spawned := mywant.GetCurrent(d, "spawned", 0)
	defeated := mywant.GetCurrent(d, "defeated", 0)
	total := d.GetIntParam("enemies", 5)

	// Coming in, one after another — when the way in is clear.
	spawnAt := cell{d.GetIntParam("spawn_x", 0), d.GetIntParam("spawn_y", 0)}
	if spawned < total && !occupied[spawnAt] && now.Sub(run.lastSpawn).Seconds() >= d.GetFloatParam("spawn_every", 4) {
		run.lastSpawn = now
		if e, err := s.spawnEnemy(d, spawned+1, spawnAt); err != nil {
			log.Printf("[BATTLE] %s: cannot bring in an enemy: %v", d.Metadata.Name, err)
		} else {
			spawned++
			d.SetCurrent("spawned", spawned)
			occupied[spawnAt] = true
			run.lastStep[e.Metadata.ID] = now
		}
	}

	// Each enemy goes for the nearest want in the room, a cell at a time,
	// and wears it down once next to it.
	damage := d.GetFloatParam("enemy_damage", 1) * thingMotionInterval.Seconds()
	stepEvery := 1 / math.Max(d.GetFloatParam("enemy_speed", 0.6), 0.05)
	goingFor := map[string]string{} // enemy want id → target want id
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
			setAttack(e, nil)
			continue
		}
		goingFor[eid] = best
		target := targets[best]
		setAttack(e, target)
		e.SetCurrent("target", target.Metadata.Name)
		// Held by a freeze: still going for it (its connection stays), but not
		// moving and not wearing anything down until the freeze lets go.
		if til, held := run.frozenTil[eid]; held {
			if now.Before(til) {
				continue
			}
			delete(run.frozenTil, eid)
			e.StoreState(frozenField, false)
		}
		if bestD <= enemyReach {
			hit := damage
			if e.Metadata.Type == bossWantType {
				hit = d.GetFloatParam("boss_damage", 3) * thingMotionInterval.Seconds()
			}
			left := hpOf(target, maxTarget) - hit
			setHP(target, left)
			impact(e, impactAttack, target.Metadata.Name)
			if left <= 0 {
				s.knockOut(d, target)
				delete(targets, best)
			}
			continue
		}
		if now.Sub(run.lastStep[eid]).Seconds() < stepEvery {
			continue
		}
		tc, _ := tileCell(target)
		if next, ok := stepToward(at, tc, func(c cell) bool { return occupied[c] || walls[[2]int{c.x, c.y}] }); ok {
			run.lastStep[eid] = now
			delete(occupied, at)
			occupied[next] = true
			_ = s.globalBuilder.QueueWantAddLabel(eid, canvasLabelX, strconv.Itoa(next.x))
			_ = s.globalBuilder.QueueWantAddLabel(eid, canvasLabelY, strconv.Itoa(next.y))
		}
	}

	// Turrets hit an enemy in reach: the one they are already on, or one
	// their strategy picks.
	for _, t := range turrets {
		tc, ok := tileCell(t)
		if !ok || now.Sub(run.lastFired[t.Metadata.ID]).Seconds() < t.GetFloatParam("interval", 1.5) {
			continue
		}
		eid := aimTurret(t, tc, enemies, goingFor, targets, maxTarget)
		if eid == "" {
			setAttack(t, nil)
			continue
		}
		e := enemies[eid]
		setAttack(t, e)
		run.lastFired[t.Metadata.ID] = now
		shots := mywant.GetCurrent(t, "shots", 0)
		t.SetCurrent("shots", shots+1)
		if t.Metadata.Type == freezeWantType {
			// A freeze holds what it hits, for a while; it may wear it a little.
			run.frozenTil[eid] = now.Add(time.Duration(t.GetFloatParam("duration", 2.5) * float64(time.Second)))
			e.StoreState(frozenField, true)
			impact(t, impactFreeze, e.Metadata.Name)
		} else {
			impact(t, impactAttack, e.Metadata.Name)
		}
		left := hpOf(e, d.GetFloatParam("enemy_hp", 3)) - t.GetFloatParam("damage", defaultDamageOf(t))
		setHP(e, left)
		go broadcastSSE("battle_shot", map[string]any{"from": t.Metadata.ID, "to": eid, "battle": id})
		if left <= 0 {
			fell(e)
			delete(enemies, eid)
			defeated++
			d.SetCurrent("defeated", defeated)
		}
	}

	// Characters hit the enemy they are connected to (the 攻撃 skill), while
	// near enough; walked too far away, they let go.
	for _, f := range fighters {
		on := attackedBy(f)
		var eid string
		for id, e := range enemies {
			if e.Metadata.Name == on {
				eid = id
			}
		}
		if eid == "" {
			continue // another battle's enemy, or gone (battleTick lets go)
		}
		me, ok := characterAt(f.GetStringParam("character_id", ""))
		ec, placed := tileCell(enemies[eid])
		if !ok || !placed {
			continue
		}
		if math.Hypot(me[0]-float64(ec.x), me[1]-float64(ec.y)) > characterReach {
			setAttack(f, nil)
			continue
		}
		if now.Sub(run.lastFired[f.Metadata.ID]) < characterHitEvery {
			continue
		}
		run.lastFired[f.Metadata.ID] = now
		left := hpOf(enemies[eid], d.GetFloatParam("enemy_hp", 3)) - characterHit
		setHP(enemies[eid], left)
		impact(f, impactAttack, enemies[eid].Metadata.Name)
		go broadcastSSE("battle_shot", map[string]any{"from": f.Metadata.ID, "to": eid, "battle": id})
		if left <= 0 {
			fell(enemies[eid])
			delete(enemies, eid)
			defeated++
			d.SetCurrent("defeated", defeated)
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
		// Said over the battle's tile: the enemies leave the board with the
		// battle, and their going all at once said nothing about why.
		if phase == "cleared" {
			d.SetCurrent("say", fmt.Sprintf("守りきった！ %d 体倒しました", defeated))
		} else {
			d.SetCurrent("say", fmt.Sprintf("全部やられた… %d 体倒しました。want を再開してもう一度", defeated))
		}
		// Over for everything in the room: the battle's fields go, and its
		// connections — standing or suspended (that is the battle's
		// `suspended`, and a want resumed by hand comes back whole).
		clearRoom(d, all)
		suspended, _ := d.GetCurrent("suspended")
		log.Printf("[BATTLE] %s: %s (%d defeated, suspended %v)", d.Metadata.Name, phase, defeated, suspended)
		go broadcastSSE("battle_over", map[string]any{"battle": id, "phase": phase})
	}
}

// fell brings an enemy down: out of the battle now (battleTick passes it
// over), its connection let go, and off the board after fallLinger.
func fell(e *mywant.Want) {
	fallen[e.Metadata.ID] = time.Now()
	setAttack(e, nil)
	e.StoreState(defeatedField, true)
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
	dx, dy := to.x-from.x, to.y-from.y
	horiz := cell{from.x + sign(dx), from.y}
	vert := cell{from.x, from.y + sign(dy)}
	tries := []cell{horiz, vert}
	if absInt(dy) > absInt(dx) {
		tries = []cell{vert, horiz}
	}
	for _, c := range tries {
		if c != from && !blocked(c) {
			return c, true
		}
	}
	return from, false
}

// setAttack makes the attacker's connection the one to its target — a
// subscription to the target's hp, which the board draws as a road between
// the two — or none. Only touched when it changes.
func setAttack(attacker, target *mywant.Want) {
	var to []*mywant.Want
	if target != nil {
		to = []*mywant.Want{target}
	}
	setLinks(attacker, to)
}

// setLinks makes a want's connections the ones to `to` — a subscription to
// each one's hp, drawn as a road from it to each.
func setLinks(from *mywant.Want, to []*mywant.Want) {
	attacker := from
	spec := attacker.GetSpec() // a copy
	if spec == nil {
		return
	}
	var want []ws.StateSubscription
	for _, t := range to {
		want = append(want, ws.StateSubscription{WantName: t.Metadata.Name, StateKeys: []string{hpField}})
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
	cb := mywant.GetGlobalChainBuilder()
	if cb == nil {
		// No board to tell (a test): the want itself.
		attacker.Spec.StateSubscriptions = want
		return
	}
	// UpdateWant adds a want it cannot find: one being deleted must not be
	// brought back by having its connection changed on the way out.
	live, _, found := cb.FindWantByID(attacker.Metadata.ID)
	if !found || live.GetStatus() == mywant.WantStatusDeleting {
		return
	}
	cb.UpdateWant(&mywant.Want{Metadata: attacker.GetMetadata(), Spec: *spec})
}

// attackedBy is the want an attacker is connected to, by name.
func attackedBy(attacker *mywant.Want) string {
	if spec := attacker.GetSpec(); spec != nil && len(spec.StateSubscriptions) > 0 {
		return spec.StateSubscriptions[0].WantName
	}
	return ""
}

// aimTurret is which enemy in reach a turret hits: the one it is already
// connected to while that one is in reach — so whatever connects it decides
// — or else by its strategy:
//
//	nearest  the closest (the default)
//	weakest  the one with least left — fewest hits to take it off the board
//	guard    the one going for whatever in the room has least left
func aimTurret(t *mywant.Want, at cell, enemies map[string]*mywant.Want, goingFor map[string]string,
	targets map[string]*mywant.Want, maxTarget float64) string {
	reach := t.GetFloatParam("range", 3)
	inReach := func(e *mywant.Want) bool { c, ok := tileCell(e); return ok && at.dist(c) <= reach }
	// A freeze keeps to what is not held yet: holding one twice is a shot spent.
	if t.Metadata.Type == freezeWantType {
		free := map[string]*mywant.Want{}
		for eid, e := range enemies {
			if frozen, _ := e.GetStateBool(frozenField, false); !frozen {
				free[eid] = e
			}
		}
		if len(free) > 0 {
			enemies = free
		}
	}
	if on := attackedBy(t); on != "" {
		for eid, e := range enemies {
			if e.Metadata.Name == on && inReach(e) {
				return eid
			}
		}
	}
	score := func(e *mywant.Want) float64 { c, _ := tileCell(e); return at.dist(c) }
	switch t.GetStringParam("strategy", "nearest") {
	case "weakest":
		score = func(e *mywant.Want) float64 { return hpOf(e, 0) }
	case "guard":
		score = func(e *mywant.Want) float64 {
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

// spawnEnemy brings one enemy in at the battle's entrance: a want tile out on
// the board, its battle on its label.
func (s *Server) spawnEnemy(d *mywant.Want, n int, at cell) (*mywant.Want, error) {
	maxHP := d.GetFloatParam("enemy_hp", 3)
	name := fmt.Sprintf("%s-enemy-%d", d.Metadata.Name, n)
	typ := enemyWantType
	// The last of them, the boss — when the battle has one.
	if n == d.GetIntParam("enemies", 5) && d.GetBoolParam("boss", true) {
		typ, maxHP = bossWantType, d.GetFloatParam("boss_hp", 30)
		name = fmt.Sprintf("%s-boss", d.Metadata.Name)
		d.SetCurrent("say", "ボスが現れた！")
	}
	w := &mywant.Want{
		Metadata: mywant.Metadata{
			ID:   fmt.Sprintf("enemy-%s-%d", d.Metadata.ID, n),
			Name: name,
			Type: typ,
			Labels: map[string]string{
				canvasLabelX: strconv.Itoa(at.x),
				canvasLabelY: strconv.Itoa(at.y),
				enemyOfLabel: d.Metadata.ID,
			},
		},
		Spec: mywant.WantSpec{Params: map[string]any{"max_hp": maxHP}},
	}
	// The rest are small — half a tile — so the boss, whole, stands over them.
	if typ == enemyWantType {
		w.Metadata.Labels["mywant.io/canvas-size"] = "0.5"
	}
	if _, err := s.globalBuilder.AddWantsAsyncWithTracking([]*mywant.Want{w}); err != nil {
		return nil, err
	}
	return w, nil
}

// knockOut puts a want worn down to nothing out of play — suspended — and
// says so on the battle.
func (s *Server) knockOut(d *mywant.Want, target *mywant.Want) {
	if err := s.globalBuilder.QueueWantSuspend([]string{target.Metadata.ID}); err != nil {
		log.Printf("[BATTLE] cannot suspend %s: %v", target.Metadata.Name, err)
		return
	}
	list := mywant.GetCurrent(d, "suspended", []any{})
	d.SetCurrent("suspended", append(list, target.Metadata.Name))
	go broadcastSSE("battle_knockout", map[string]any{"id": target.Metadata.ID, "battle": d.Metadata.ID})
}

func sameStrings(have any, want []string) bool {
	list, ok := have.([]string)
	if !ok {
		if anys, ok := have.([]any); ok {
			for _, a := range anys {
				s, _ := a.(string)
				list = append(list, s)
			}
		}
	}
	if len(list) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, s := range list {
		seen[s]++
	}
	for _, s := range want {
		if seen[s] == 0 {
			return false
		}
		seen[s]--
	}
	return true
}

// mendTick: what each heal want mends.
//
// Connected to wants (its own connections — a subscription to each one's hp,
// made with the 接続 skill), it mends those, by itself, whether anyone is
// standing on it or not; its amount is the whole of it, shared among them —
// connecting it to a second turret halves what each gets, so healing more
// is never more healing. Connected to nothing, it is the button it always
// was: while stood on, it mends every hurt want in reach. Either way the
// names of what it is mending now are its `healing`, which the board draws
// as the green line.
func mendTick(heals, all []*mywant.Want) {
	dt := thingMotionInterval.Seconds()
	byName := map[string]*mywant.Want{}
	for _, w := range all {
		byName[w.Metadata.Name] = w
	}
	hurt := func(w *mywant.Want) (hp, max float64, ok bool) {
		max, hasMax := w.GetStateFloat64(maxHPField, 0)
		hp, hasHP := w.GetStateFloat64(hpField, 0)
		// Nothing left is out of play, not hurt: resumed by hand.
		return hp, max, hasMax && hasHP && hp > 0 && hp < max && defendable(w) && battleWantRunning(w)
	}
	for _, h := range heals {
		amount := h.GetFloatParam("amount", 2) * dt
		var names []string
		if spec := h.GetSpec(); spec != nil && len(spec.StateSubscriptions) > 0 {
			// Shared among everything it is connected to, hurt or not: the
			// sharing is the cost of reaching further.
			share := amount / float64(len(spec.StateSubscriptions))
			for _, sub := range spec.StateSubscriptions {
				w := byName[sub.WantName]
				if w == nil {
					continue
				}
				if hp, max, ok := hurt(w); ok {
					setHP(w, math.Min(max, hp+share))
					names = append(names, w.Metadata.Name)
				}
			}
		} else if hc, placed := tileCell(h); placed &&
			(len(mywant.GetCurrent(h, "characters", []string{})) > 0 || len(mywant.GetCurrent(h, "things", []string{})) > 0) {
			reach := h.GetFloatParam("range", 2)
			for _, w := range all {
				c, ok := tileCell(w)
				if w.Metadata.ID == h.Metadata.ID || !ok || hc.dist(c) > reach {
					continue
				}
				if hp, max, ok := hurt(w); ok {
					setHP(w, math.Min(max, hp+amount))
					names = append(names, w.Metadata.Name)
				}
			}
		}
		impact(h, impactMend, names...)
		if !sameStrings(mywant.GetCurrent(h, "healing", []string{}), names) {
			h.SetCurrent("healing", names)
		}
	}
}

// startBattle begins a battle afresh: nothing come in yet, nothing defeated,
// nothing suspended by it.
func startBattle(d *mywant.Want) {
	d.SetCurrent("spawned", 0)
	d.SetCurrent("defeated", 0)
	d.SetCurrent("suspended", []string{})
	d.SetCurrent("targets", []string{})
	d.SetCurrent("phase", "fighting")
	delete(battles, d.Metadata.ID)
	log.Printf("[BATTLE] %s: begins", d.Metadata.Name)
}

// characterAt is where a character is on the board now (the cursor store).
func characterAt(characterID string) ([2]float64, bool) {
	if characterID == "" {
		return [2]float64{}, false
	}
	cursorsMu.RLock()
	e, ok := cursors[characterID]
	cursorsMu.RUnlock()
	return [2]float64{e.X, e.Y}, ok
}

// POST /api/v1/wants/{id}/attack  {"target": "<want id>"}  — "" to stop.
//
// This want attacks that one: its connection becomes the one to the target's
// hp (setAttack), which the battle reads — a character's own want connected
// to an enemy is the 攻撃 skill, a turret's is its aim.
func (s *Server) setWantAttack(w http.ResponseWriter, r *http.Request) {
	if s.globalBuilder == nil {
		s.JSONError(w, r, http.StatusServiceUnavailable, "Server not ready", "")
		return
	}
	attacker, _, found := s.globalBuilder.FindWantByID(mux.Vars(r)["id"])
	if !found {
		s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
		return
	}
	var body struct {
		Target string `json:"target"`
		// Add: one more connection rather than the only one — a heal mends
		// several. Already connected, it is taken off instead.
		Add bool `json:"add"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if body.Target == "" {
		setAttack(attacker, nil)
		s.JSONResponse(w, http.StatusOK, map[string]any{"attacking": ""})
		return
	}
	target, _, found := s.globalBuilder.FindWantByID(body.Target)
	if !found {
		s.JSONError(w, r, http.StatusNotFound, "Target not found", "")
		return
	}
	if !body.Add {
		setAttack(attacker, target)
		s.JSONResponse(w, http.StatusOK, map[string]any{"attacking": target.Metadata.Name})
		return
	}
	var to []*mywant.Want
	removed := false
	if spec := attacker.GetSpec(); spec != nil {
		for _, sub := range spec.StateSubscriptions {
			if sub.WantName == target.Metadata.Name {
				removed = true
				continue
			}
			if t, ok := s.globalBuilder.FindWantByName(sub.WantName); ok {
				to = append(to, t)
			}
		}
	}
	if !removed {
		to = append(to, target)
	}
	setLinks(attacker, to)
	names := make([]string, 0, len(to))
	for _, t := range to {
		names = append(names, t.Metadata.Name)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"connected": names, "removed": removed})
}

// defendable: a want a battle's enemies go for, and that a heal mends — one
// standing on the board as a tile of its own. Not an enemy or a battle, not
// what the board itself is built of (a wall, a door), and not a system want:
// a character's chat stands hidden on the character's own cell, and a battle
// suspending it took the person's chat away.
func defendable(w *mywant.Want) bool {
	switch w.Metadata.Type {
	case enemyWantType, bossWantType, "room_defense", "wall", "door", "rpg_door":
		return false
	}
	return !w.Metadata.IsSystemWant && w.Metadata.Labels["mywant.io/canvas-hidden"] != "true"
}

// clearBattleFields takes a battle's fields off a want: whole again.
func clearBattleFields(w *mywant.Want) {
	if _, ok := w.GetStateFloat64(hpField, 0); ok {
		w.DeleteState(hpField)
	}
	if _, ok := w.GetStateFloat64(maxHPField, 0); ok {
		w.DeleteState(maxHPField)
	}
	if v, _ := w.GetCurrent(mywant.ImpactField); v != nil {
		w.DeleteState(mywant.ImpactField)
	}
}

// clearRoom: everything in a battle's room whole again, and its turrets and
// heals connected to nothing.
func clearRoom(d *mywant.Want, all []*mywant.Want) {
	rx, ry := d.GetIntParam("room_x", 0), d.GetIntParam("room_y", 0)
	rw, rh := d.GetIntParam("room_w", 8), d.GetIntParam("room_h", 6)
	for _, w := range all {
		c, ok := tileCell(w)
		if !ok || c.x < rx || c.x >= rx+rw || c.y < ry || c.y >= ry+rh || isEnemy(w) {
			continue
		}
		clearBattleFields(w)
		// A turret's connection is its aim in this battle; a heal's are what it
		// was connected to by hand, and outlast the battle.
		if w.Metadata.Type == "turret" {
			setLinks(w, nil)
		}
	}
}

// roomHoldsAnything: some want in the battle's room is standing to be
// defended.
func roomHoldsAnything(d *mywant.Want, all []*mywant.Want) bool {
	rx, ry := d.GetIntParam("room_x", 0), d.GetIntParam("room_y", 0)
	rw, rh := d.GetIntParam("room_w", 8), d.GetIntParam("room_h", 6)
	for _, w := range all {
		c, ok := tileCell(w)
		if ok && c.x >= rx && c.x < rx+rw && c.y >= ry && c.y < ry+rh && defendable(w) && battleWantRunning(w) {
			return true
		}
	}
	return false
}

// resetBattle puts a room defense back as it was: its enemies off the board,
// every want in its room it (or any battle) suspended resumed, what was worn
// whole again, and the battle ready to begin.
func (s *Server) resetBattle(d *mywant.Want, all, enemyWants []*mywant.Want) {
	var gone []string
	for _, e := range enemyWants {
		gone = append(gone, e.Metadata.ID)
	}
	if len(gone) > 0 {
		_ = s.globalBuilder.QueueWantDelete(gone)
	}
	rx, ry := d.GetIntParam("room_x", 0), d.GetIntParam("room_y", 0)
	rw, rh := d.GetIntParam("room_w", 8), d.GetIntParam("room_h", 6)
	var resume []string
	for _, w := range all {
		c, ok := tileCell(w)
		if ok && c.x >= rx && c.x < rx+rw && c.y >= ry && c.y < ry+rh && defendable(w) &&
			w.GetStatus() == mywant.WantStatusSuspended {
			resume = append(resume, w.Metadata.ID)
		}
	}
	if len(resume) > 0 {
		if err := s.globalBuilder.QueueWantResume(resume); err != nil {
			log.Printf("[BATTLE] %s: cannot resume %v: %v", d.Metadata.Name, resume, err)
		}
	}
	clearRoom(d, all)
	d.SetCurrent("spawned", 0)
	d.SetCurrent("defeated", 0)
	d.SetCurrent("suspended", []string{})
	d.SetCurrent("targets", []string{})
	d.SetCurrent("phase", "ready")
	delete(battles, d.Metadata.ID)
	d.SetCurrent("say", fmt.Sprintf("元に戻しました（%d 個を再開）— 乗ると始まります", len(resume)))
	log.Printf("[BATTLE] %s: reset (resumed %d)", d.Metadata.Name, len(resume))
}

// isEnemy: one of a battle's enemies — an enemy, or its boss.
func isEnemy(w *mywant.Want) bool {
	return w.Metadata.Type == enemyWantType || w.Metadata.Type == bossWantType
}

// defaultDamageOf: a turret's hit wears an enemy down; a freeze's holds it.
func defaultDamageOf(t *mywant.Want) float64 {
	if t.Metadata.Type == freezeWantType {
		return 0
	}
	return 1
}
