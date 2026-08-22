package mywant

import "testing"

// This is the actual bug: a state field declared `persistent: true` in a
// want type's YAML — direction's dx/dy, going's own going flag, gear's
// value, all of them — is exactly the schema's own way of saying "this
// outlives a restart". prepareForRestart's reset-to-initialValue pass never
// read that flag, so every persistent field was wiped back to its
// initialValue on every server restart regardless — the persisted value on
// disk was correct, prepareForRestart threw it away seconds after loading it.
func TestPrepareForRestartSkipsPersistentFields(t *testing.T) {
	cb := NewChainBuilder(nil)
	cb.wantTypeDefinitions = map[string]*WantTypeDefinition{
		"test-type": {
			State: []StateDef{
				{Name: "persisted_field", Label: "current", Persistent: true, InitialValue: "default"},
				{Name: "volatile_field", Label: "current", Persistent: false, InitialValue: "default"},
			},
		},
	}
	prevCB := GetGlobalChainBuilder()
	SetGlobalChainBuilder(cb)
	t.Cleanup(func() { SetGlobalChainBuilder(prevCB) })

	w := &Want{
		Metadata: Metadata{ID: "w1", Name: "w1", Type: "test-type"},
		StateLabels: map[string]StateLabel{
			"persisted_field": LabelCurrent,
			"volatile_field":  LabelCurrent,
		},
	}
	w.SetCurrent("persisted_field", "user-set")
	w.SetCurrent("volatile_field", "user-set")

	w.prepareForRestart()

	if got, _ := w.GetCurrent("persisted_field"); got != "user-set" {
		t.Fatalf("expected the persistent field to survive prepareForRestart, got %v", got)
	}
	if got, _ := w.GetCurrent("volatile_field"); got != "default" {
		t.Fatalf("expected the non-persistent field to reset to its initialValue, got %v", got)
	}
}

// Goal-labeled fields were already excluded before this fix, for an
// unrelated reason (Initialize() reads them back as its own fallback) — this
// guards that the new Persistent check doesn't change that.
func TestPrepareForRestartStillSkipsGoalFields(t *testing.T) {
	cb := NewChainBuilder(nil)
	cb.wantTypeDefinitions = map[string]*WantTypeDefinition{
		"test-type": {
			State: []StateDef{
				{Name: "goal_field", Label: "goal", Persistent: false, InitialValue: "default"},
			},
		},
	}
	prevCB := GetGlobalChainBuilder()
	SetGlobalChainBuilder(cb)
	t.Cleanup(func() { SetGlobalChainBuilder(prevCB) })

	w := &Want{
		Metadata:    Metadata{ID: "w1", Name: "w1", Type: "test-type"},
		StateLabels: map[string]StateLabel{"goal_field": LabelGoal},
	}
	w.SetGoal("goal_field", "user-set")

	w.prepareForRestart()

	if got, _ := w.GetGoal("goal_field"); got != "user-set" {
		t.Fatalf("expected the goal field to survive prepareForRestart, got %v", got)
	}
}

// ResetOnRestart: false must still bypass the whole reset pass, persistent
// flag or not — this was the only escape hatch before this fix, and it must
// keep working for want types that rely on it for other fields.
func TestPrepareForRestartResetOnRestartFalseSkipsEverything(t *testing.T) {
	cb := NewChainBuilder(nil)
	cb.wantTypeDefinitions = map[string]*WantTypeDefinition{
		"test-type": {
			State: []StateDef{
				{Name: "volatile_field", Label: "current", Persistent: false, InitialValue: "default"},
			},
		},
	}
	prevCB := GetGlobalChainBuilder()
	SetGlobalChainBuilder(cb)
	t.Cleanup(func() { SetGlobalChainBuilder(prevCB) })

	no := false
	w := &Want{
		Metadata:    Metadata{ID: "w1", Name: "w1", Type: "test-type"},
		StateLabels: map[string]StateLabel{"volatile_field": LabelCurrent},
		Spec:        WantSpec{ResetOnRestart: &no},
	}
	w.SetCurrent("volatile_field", "user-set")

	w.prepareForRestart()

	if got, _ := w.GetCurrent("volatile_field"); got != "user-set" {
		t.Fatalf("expected resetOnRestart:false to skip the reset entirely, got %v", got)
	}
}
