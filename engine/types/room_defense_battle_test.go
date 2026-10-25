package types

import (
	"testing"

	. "mywant/engine/core"
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

func battleWant(name, typ string, x, y string, params map[string]any) *Want {
	return &Want{
		Metadata: Metadata{ID: name, Name: name, Type: typ,
			Labels: map[string]string{canvasX: x, canvasY: y}},
		Spec: WantSpec{Params: params},
	}
}

func TestAimTurret(t *testing.T) {
	near := battleWant("near", enemyWantType, "1", "0", nil)
	weak := battleWant("weak", enemyWantType, "2", "0", nil)
	far := battleWant("far", enemyWantType, "9", "0", nil)
	near.StoreState("hp", 3.0)
	weak.StoreState("hp", 1.0)
	far.StoreState("hp", 0.5)
	enemies := map[string]*Want{"near": near, "weak": weak, "far": far}

	turret := battleWant("t", "turret", "0", "0", map[string]any{"range": 3.0})
	if got := aimTurret(turret, cell{0, 0}, enemies, nil, nil, 10); got != "near" {
		t.Errorf("nearest: got %q", got)
	}
	turret.Spec.Params["strategy"] = "weakest"
	if got := aimTurret(turret, cell{0, 0}, enemies, nil, nil, 10); got != "weak" {
		t.Errorf("weakest in reach (not the far one): got %q", got)
	}
	// Connected to one in reach: that one, whatever the strategy says.
	SetAttack(turret, near)
	if got := aimTurret(turret, cell{0, 0}, enemies, nil, nil, 10); got != "near" {
		t.Errorf("connected: got %q", got)
	}
	// Connected to one out of reach: back to its strategy.
	SetAttack(turret, far)
	if got := aimTurret(turret, cell{0, 0}, enemies, nil, nil, 10); got != "weak" {
		t.Errorf("connected out of reach: got %q", got)
	}
}

func TestFreezeAimsAtTheUnheld(t *testing.T) {
	held := battleWant("held", enemyWantType, "1", "0", nil)
	free := battleWant("free", enemyWantType, "2", "0", nil)
	held.StoreState("frozen", true)
	enemies := map[string]*Want{"held": held, "free": free}
	f := battleWant("f", freezeWantType, "0", "0", map[string]any{"range": 3.0})
	if got := aimTurret(f, cell{0, 0}, enemies, nil, nil, 10); got != "free" {
		t.Errorf("freeze aimed at %q, want the one not held yet", got)
	}
	free.StoreState("frozen", true)
	if got := aimTurret(f, cell{0, 0}, enemies, nil, nil, 10); got != "held" {
		t.Errorf("all held: aimed at %q, want the nearest", got)
	}
}
