package types

import (
	"testing"
	"time"

	. "mywant/engine/core"
)

func TestSendClockKeepsItsInterval(t *testing.T) {
	w := &Want{}
	var c sendClock
	if !c.send(w, "strike", 1, map[string]any{"damage": 2.0}, []string{"a"}, "", nil) {
		t.Fatal("the first did not go")
	}
	if c.send(w, "strike", 1, map[string]any{"damage": 2.0}, []string{"a"}, "", nil) {
		t.Error("sent again before its interval")
	}
	c.last = time.Now().Add(-1100 * time.Millisecond)
	if !c.send(w, "strike", 1, map[string]any{"damage": 2.0}, []string{"a"}, "", nil) {
		t.Error("its interval came round and nothing went")
	}
	if c.seq != 2 {
		t.Errorf("numbered %d, want 2", c.seq)
	}
}

// A want acted on: named, with hp, importing two senders' acts.
func actedOn(name string, hp, max float64) *Want {
	w := &Want{Metadata: Metadata{ID: name, Name: name}}
	w.Spec.Imports = map[string]string{
		"enemy-1.attack": ActionImportPrefix + "enemy-1.attack",
		"泉.heal":         ActionImportPrefix + "泉.heal",
	}
	w.StoreState("hp", hp)
	w.StoreState("max_hp", max)
	return w
}

func hpOf(w *Want) float64 { v, _ := w.GetStateFloat64("hp", -1); return v }

func TestABodyTakesWhatIsForIt(t *testing.T) {
	SetGlobalChainBuilder(NewChainBuilder([]*Want{}))
	defer SetGlobalChainBuilder(nil)
	StoreGlobalState("enemy-1.attack", map[string]any{"seq": 3, "to": []any{"帳簿"}, "damage": 1.0})
	StoreGlobalState("泉.heal", map[string]any{"seq": 1, "to": map[string]any{"帳簿": 1.0, "地図": 1.0}})
	w := actedOn("帳簿", 6, 10)
	var b body

	// What was there before it was listening is not for it.
	b.takeActs(w)
	if got := hpOf(w); got != 6 {
		t.Fatalf("took what was sent before it listened: hp %v", got)
	}

	// A blow for it, and one for someone else.
	StoreGlobalState("enemy-1.attack", map[string]any{"seq": 4, "to": []any{"帳簿"}, "damage": 2.0})
	b.takeActs(w)
	b.takeActs(w) // the same blow again: nothing
	if got := hpOf(w); got != 4 {
		t.Errorf("after one blow of 2: hp %v, want 4", got)
	}
	StoreGlobalState("enemy-1.attack", map[string]any{"seq": 5, "to": []any{"地図"}, "damage": 2.0})
	b.takeActs(w)
	if got := hpOf(w); got != 4 {
		t.Errorf("took a blow for 地図: hp %v", got)
	}

	// Its share of a mend — never past its max_hp.
	StoreGlobalState("泉.heal", map[string]any{"seq": 2, "to": map[string]any{"帳簿": 100.0, "地図": 1.0}})
	b.takeActs(w)
	if got := hpOf(w); got != 10 {
		t.Errorf("mended past or short of max: hp %v, want 10", got)
	}
}

func TestABodyDownTakesNoMore(t *testing.T) {
	SetGlobalChainBuilder(NewChainBuilder([]*Want{}))
	defer SetGlobalChainBuilder(nil)
	StoreGlobalState("enemy-1.attack", map[string]any{"seq": 1, "to": []any{"地図"}, "damage": 1.0})
	StoreGlobalState("泉.heal", map[string]any{"seq": 1, "to": map[string]any{}})
	w := actedOn("地図", 1, 10)
	var b body
	b.takeActs(w) // listening now
	StoreGlobalState("enemy-1.attack", map[string]any{"seq": 2, "to": []any{"地図"}, "damage": 5.0})
	if !b.takeActs(w) {
		t.Fatal("hp 0 and not down")
	}
	StoreGlobalState("泉.heal", map[string]any{"seq": 2, "to": map[string]any{"地図": 3.0}})
	b.takeActs(w)
	if got := hpOf(w); got != 0 {
		t.Errorf("mended while down: hp %v", got)
	}
}
