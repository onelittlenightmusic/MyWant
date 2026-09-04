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

// The other half of the same bug, one level up: prepareForRestart correctly
// leaves a persistent field alone, but ScriptableWant.Initialize (called right
// after it on every restart) used to copy Spec.Params back over state for any
// param sharing a state field's name — persistent or not. A checklist's
// `items` state is persistent:true, but its `items` param is just the initial
// seed (defaulted to [] by Initialize's own fill-in when the deploy never set
// one), so that copy silently deleted the persisted list on every restart —
// the exact case reproduced here.
func TestScriptableWantInitializeSkipsParamCopyForPersistentField(t *testing.T) {
	def := &WantTypeDefinition{
		Parameters: []ParameterDef{
			{Name: "items", Default: []interface{}{}},
		},
		State: []StateDef{
			{Name: "items", Label: "current", Persistent: true, InitialValue: []interface{}{}},
		},
	}

	sw := &ScriptableWant{Want: Want{
		Metadata:           Metadata{ID: "w1", Name: "w1", Type: "test-doc"},
		StateLabels:        map[string]StateLabel{"items": LabelCurrent},
		WantTypeDefinition: def,
	}}
	// As if reloaded from a persisted snapshot: state already holds what the
	// user last saved, and the deploy never set an explicit `items` param.
	sw.SetCurrent("items", []interface{}{"user-item"})

	sw.Initialize()

	got, _ := sw.GetCurrent("items")
	gotSlice, ok := got.([]interface{})
	if !ok || len(gotSlice) != 1 || gotSlice[0] != "user-item" {
		t.Fatalf("expected the persistent items field to survive Initialize's param copy, got %#v", got)
	}
}

// The param copy still has to run for a field that ISN'T persistent — this
// guards that the new Persistent check doesn't turn the copy off altogether.
func TestScriptableWantInitializeStillCopiesNonPersistentParam(t *testing.T) {
	def := &WantTypeDefinition{
		Parameters: []ParameterDef{
			{Name: "label", Default: "fallback"},
		},
		State: []StateDef{
			{Name: "label", Label: "current", Persistent: false, InitialValue: ""},
		},
	}

	sw := &ScriptableWant{Want: Want{
		Metadata:           Metadata{ID: "w1", Name: "w1", Type: "test-doc"},
		StateLabels:        map[string]StateLabel{"label": LabelCurrent},
		WantTypeDefinition: def,
		Spec:               WantSpec{Params: map[string]any{"label": "from-param"}},
	}}

	sw.Initialize()

	if got, _ := sw.GetCurrent("label"); got != "from-param" {
		t.Fatalf("expected the non-persistent field to still be copied from its param, got %v", got)
	}
}
