package types

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	. "mywant/engine/core"
)

// A battle, on the board's own wiring.
//
// Every want in a battle is in charge of itself. One that acts — a heal's
// mend, a turret's shot, an enemy's blow, a character's attack — decides how
// much and how often (its params, its interval) and puts each act in its own
// state: one map, numbered (`seq`), saying who it is for (`to`). That field
// is exposed under a key of its own that does not depend on who it is for —
// "<its name>.attack", "<its name>.heal" (ActionKey) — so it can be imported
// before anyone knows what it will be aimed at.
//
// One that is acted on imports those keys (under a local key starting
// ActionImportPrefix) and reads them in its own Progress: an act numbered
// past the last one it took, with its own name in `to`, it takes — its own
// hp down or up, never past its max_hp; held a while, by a freeze. At hp 0 it
// stops itself. Nothing writes another want's state.
//
// The wiring — the exposes, and the imports added as enemies come and go — is
// configuration, put in place outside every Progress (the server's battle,
// through the chain builder), as every want's wiring is.

// ActionImportPrefix starts the local key an act is imported under.
const ActionImportPrefix = "action:"

// RoundImportKey is the local key a want in a battle's room imports its
// battle's round under: when it changes, a new round has begun, and the want
// makes itself whole again.
const RoundImportKey = "battle_round"

// RoundKey is the global key a battle's round is exposed under.
func RoundKey(battleName string) string { return battleName + ".round" }

// ActionKey is the global key a want's act is exposed under.
func ActionKey(wantName, act string) string { return wantName + "." + act }

// The acts, and the fields they are kept in.
const (
	actAttack   = "attack"
	actHeal     = "heal"
	strikeField = "strike"
	pulseField  = "pulse"
)

// sendClock is what a sender keeps between its turns: when it last sent,
// and that act's number.
//
// Locked: a want's loop can be running twice for a moment (a world opened
// again starts it afresh before the last one has let go), over the same
// locals.
type sendClock struct {
	mu   sync.Mutex
	last time.Time
	seq  int
}

// start puts a sender's field in place as it starts — its last act as it
// was, or one numbered 0 and for no one — written afresh, so what imports it
// has a value from the first: an import with nothing behind it holds the
// importer's Progress back, and an expose only publishes what changes.
func (c *sendClock) start(w *Want, field string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	body := map[string]any{"seq": 0, "to": []string{}}
	if v, ok := w.GetCurrent(field); ok {
		if last, isMap := v.(map[string]any); isMap {
			body = map[string]any{}
			for k, x := range last {
				body[k] = x
			}
			c.seq = int(numOf(last["seq"]))
		}
	}
	body["since"] = time.Now().Format(time.RFC3339Nano)
	w.StoreState(field, body)
}

// send puts the next act in w's `field` when `interval` seconds have passed
// since the last: `body` (what it carries) for `to` (a list of names, or for
// a heal, each name's share). Recorded as an impact on whoever it is for, so
// the board shows it. Whether it sent.
func (c *sendClock) send(w *Want, field string, interval float64, body map[string]any, to any, impact string, on []string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if interval < 0.1 {
		interval = 0.1
	}
	now := time.Now()
	if now.Sub(c.last).Seconds() < interval {
		return false
	}
	c.last = now
	c.seq++
	body["seq"] = c.seq
	body["to"] = to
	w.StoreState(field, body)
	if impact != "" && len(on) > 0 {
		w.RecordImpact(impact, on, nil)
	}
	return true
}

// aimedAt is the want w is connected to (its aim), by name; "" for none.
func aimedAt(w *Want) string {
	if spec := w.GetSpec(); spec != nil && len(spec.StateSubscriptions) > 0 {
		return spec.StateSubscriptions[0].WantName
	}
	return ""
}

// connectedTo is every want w is connected to, by name.
func connectedTo(w *Want) []string {
	spec := w.GetSpec()
	if spec == nil {
		return nil
	}
	var names []string
	for _, s := range spec.StateSubscriptions {
		names = append(names, s.WantName)
	}
	return names
}

// inbox is what a want acted on keeps between its turns: the number of the
// last act it took from each sender.
// Locked, for sendClock's reason.
type inbox struct {
	mu   sync.Mutex
	seen map[string]int
}

// act is one act taken: who sent it, and what it carries for this want.
type act struct {
	from   string
	damage float64
	amount float64
	hold   float64
}

// take reads every act w imports and hands each new one for w to `each`.
func (in *inbox) take(w *Want, each func(a act)) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.seen == nil {
		in.seen = map[string]int{}
	}
	spec := w.GetSpec()
	if spec == nil {
		return
	}
	me := w.Metadata.Name
	for globalKey, localKey := range spec.Imports {
		if !strings.HasPrefix(localKey, ActionImportPrefix) {
			continue
		}
		raw, _ := GetGlobalState(globalKey)
		body, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		seq := int(numOf(body["seq"]))
		if seen, ok := in.seen[globalKey]; ok && seen == seq {
			continue
		}
		first := !hasKey(in.seen, globalKey)
		in.seen[globalKey] = seq
		// What was already there when it was first read was sent before this
		// want was listening: not for it.
		if first || seq == 0 {
			continue
		}
		a := act{from: strings.TrimSuffix(strings.TrimSuffix(globalKey, "."+actAttack), "."+actHeal)}
		switch to := body["to"].(type) {
		case []any:
			if !containsName(to, me) {
				continue
			}
			a.damage, a.hold = numOf(body["damage"]), numOf(body["hold"])
		case []string:
			if !containsString(to, me) {
				continue
			}
			a.damage, a.hold = numOf(body["damage"]), numOf(body["hold"])
		case map[string]any:
			share, ok := to[me]
			if !ok {
				continue
			}
			a.amount = numOf(share)
		default:
			continue
		}
		each(a)
	}
}

// body is a want that can be worn down and mended: its hp, kept by itself.
type body struct {
	in inbox
}

// keepHP puts w's hp in place when it has none — whole, at its max_hp param
// (the battle's target_hp is what a room gives the wants in it).
func keepHP(w *Want, fallback float64) {
	if _, ok := w.GetStateFloat64("hp", 0); ok {
		return
	}
	max := w.GetFloatParam("max_hp", fallback)
	w.StoreState("max_hp", max)
	w.StoreState("hp", max)
}

// takeActs takes what has been sent to w — blows off its hp, mends onto it,
// never past its max_hp, nothing at all once it is down — and stops w at hp
// 0. Whether it is down.
func (b *body) takeActs(w *Want) bool {
	b.newRound(w)
	hp, hasHP := w.GetStateFloat64("hp", 0)
	max, _ := w.GetStateFloat64("max_hp", hp)
	if !hasHP {
		return false
	}
	b.in.take(w, func(a act) {
		if hp <= 0 {
			return
		}
		hp = hp - a.damage + a.amount
		hp = math.Max(0, math.Min(max, hp))
	})
	hp = math.Round(hp*100) / 100
	w.StoreState("hp", hp)
	if hp > 0 {
		return false
	}
	// Down: it stops itself, until it is resumed by hand (or its battle is
	// put back).
	_ = w.SendControlCommand(&ControlCommand{
		Trigger: ControlTriggerSuspend, WantID: w.Metadata.ID, Timestamp: time.Now(), Reason: "worn down",
	})
	return true
}

// newRound: its battle has begun a round it has not been whole for — whole
// again, at its max_hp. The first round it hears of is only noted: what it
// has now is what it was given.
func (b *body) newRound(w *Want) {
	spec := w.GetSpec()
	if spec == nil {
		return
	}
	for gk, lk := range spec.Imports {
		if lk != RoundImportKey {
			continue
		}
		raw, _ := GetGlobalState(gk)
		round := int(numOf(raw))
		had, known := w.GetStateInt("round", 0)
		if known && had == round {
			return
		}
		w.StoreState("round", round)
		if known {
			max := w.GetFloatParam("max_hp", 10)
			w.StoreState("max_hp", max)
			w.StoreState("hp", max)
		}
		return
	}
}

func numOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

func hasKey(m map[string]int, k string) bool { _, ok := m[k]; return ok }

func containsName(list []any, name string) bool {
	for _, v := range list {
		if s, ok := v.(string); ok && s == name {
			return true
		}
	}
	return false
}
