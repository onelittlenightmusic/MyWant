package server

import (
	"math"
	"testing"

	mywant "mywant/engine/core"
)

func TestStepToward(t *testing.T) {
	free := func(cell) bool { return false }
	if got, _ := stepToward(cell{0, 0}, cell{5, 1}, free); got != (cell{1, 0}) {
		t.Errorf("the longer way first: got %v", got)
	}
	if got, _ := stepToward(cell{0, 0}, cell{1, 5}, free); got != (cell{0, 1}) {
		t.Errorf("the longer way first: got %v", got)
	}
	wall := func(c cell) bool { return c == cell{1, 0} }
	if got, _ := stepToward(cell{0, 0}, cell{5, 1}, wall); got != (cell{0, 1}) {
		t.Errorf("round a wall: got %v", got)
	}
	if _, ok := stepToward(cell{0, 0}, cell{5, 0}, wall); ok {
		t.Error("no way round on a straight line: should stay")
	}
}

func battleWant(name, typ string, x, y string, params map[string]any) *mywant.Want {
	return &mywant.Want{
		Metadata: mywant.Metadata{ID: name, Name: name, Type: typ,
			Labels: map[string]string{canvasLabelX: x, canvasLabelY: y}},
		Spec: mywant.WantSpec{Params: params},
	}
}

func TestAimTurret(t *testing.T) {
	near := battleWant("near", enemyWantType, "1", "0", nil)
	weak := battleWant("weak", enemyWantType, "2", "0", nil)
	far := battleWant("far", enemyWantType, "9", "0", nil)
	setHP(near, 3)
	setHP(weak, 1)
	setHP(far, 0.5)
	enemies := map[string]*mywant.Want{"near": near, "weak": weak, "far": far}

	turret := battleWant("t", "turret", "0", "0", map[string]any{"range": 3.0})
	if got := aimTurret(turret, cell{0, 0}, enemies, nil, nil, 10); got != "near" {
		t.Errorf("nearest: got %q", got)
	}
	turret.Spec.Params["strategy"] = "weakest"
	if got := aimTurret(turret, cell{0, 0}, enemies, nil, nil, 10); got != "weak" {
		t.Errorf("weakest in reach (not the far one): got %q", got)
	}
	// Connected to one in reach: that one, whatever the strategy says.
	setAttack(turret, near)
	if got := aimTurret(turret, cell{0, 0}, enemies, nil, nil, 10); got != "near" {
		t.Errorf("connected: got %q", got)
	}
	// Connected to one out of reach: back to its strategy.
	setAttack(turret, far)
	if got := aimTurret(turret, cell{0, 0}, enemies, nil, nil, 10); got != "weak" {
		t.Errorf("connected out of reach: got %q", got)
	}
}

func TestMendShared(t *testing.T) {
	heal := battleWant("heal", "heal", "0", "0", map[string]any{"amount": 2.0})
	a := battleWant("a", "turret", "1", "0", nil)
	b := battleWant("b", "turret", "9", "9", nil) // far: connected, so reach does not matter
	for _, w := range []*mywant.Want{a, b} {
		setHP(w, 5)
		w.StoreState(maxHPField, 10.0)
	}
	setLinks(heal, []*mywant.Want{a, b})
	mendTick([]*mywant.Want{heal}, []*mywant.Want{heal, a, b})
	dt := thingMotionInterval.Seconds()
	for _, w := range []*mywant.Want{a, b} {
		if got, want := hpOf(w, 0), 5+math.Round(2*dt/2*100)/100; got != want {
			t.Errorf("%s: hp %v, want %v (half the amount each)", w.Metadata.Name, got, want)
		}
	}

	// Connected to nothing and stood on by no one: nothing mends.
	setLinks(heal, nil)
	setHP(a, 5)
	mendTick([]*mywant.Want{heal}, []*mywant.Want{heal, a, b})
	if got := hpOf(a, 0); got != 5 {
		t.Errorf("unconnected, unstood heal mended: hp %v", got)
	}
}

func TestFreezeAimsAtTheUnheld(t *testing.T) {
	held := battleWant("held", enemyWantType, "1", "0", nil)
	free := battleWant("free", enemyWantType, "2", "0", nil)
	held.StoreState(frozenField, true)
	enemies := map[string]*mywant.Want{"held": held, "free": free}
	f := battleWant("f", freezeWantType, "0", "0", map[string]any{"range": 3.0})
	if got := aimTurret(f, cell{0, 0}, enemies, nil, nil, 10); got != "free" {
		t.Errorf("freeze aimed at %q, want the one not held yet", got)
	}
	// All held: it still has something to hit.
	free.StoreState(frozenField, true)
	if got := aimTurret(f, cell{0, 0}, enemies, nil, nil, 10); got != "held" {
		t.Errorf("all held: aimed at %q, want the nearest", got)
	}
}
