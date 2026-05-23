package types

import (
	"testing"

	. "mywant/engine/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeStateChangeSyncForTest temporarily sets the StateChange event processing mode
// to synchronous so expose handlers fire inline during test assertions.
// Returns a cleanup function that restores the original (async) mode.
func makeStateChangeSyncForTest(t *testing.T) func() {
	t.Helper()
	uss := GetGlobalSubscriptionSystem()
	uss.SetProcessingMode(EventTypeStateChange, ProcessSync)
	return func() {
		uss.SetProcessingMode(EventTypeStateChange, ProcessAsync)
	}
}

func TestSliderWant_Initialize(t *testing.T) {
	slider := &SliderWant{Want: Want{
		Metadata: Metadata{ID: "slider-id", Name: "budget-slider", Type: "slider"},
		Spec: WantSpec{Params: map[string]any{
			"default": 5000.0,
			"min":     0.0,
			"max":     10000.0,
			"step":    100.0,
		}},
	}}

	slider.BeginProgressCycle()
	slider.Initialize()
	slider.EndProgressCycle()

	val, _ := slider.GetStateFloat64("value", 0.0)
	assert.Equal(t, 5000.0, val)

	min, _ := slider.GetStateFloat64("min", -1.0)
	assert.Equal(t, 0.0, min)

	max, _ := slider.GetStateFloat64("max", -1.0)
	assert.Equal(t, 10000.0, max)

	step, _ := slider.GetStateFloat64("step", -1.0)
	assert.Equal(t, 100.0, step)

	// target_param is no longer stored in state — propagation is via expose entries.
	tp, _ := slider.GetStateString("target_param", "")
	assert.Equal(t, "", tp, "target_param should not be stored in state")
}

func TestSliderWant_PropagatesValueToParentGoal(t *testing.T) {
	cleanup := makeStateChangeSyncForTest(t)
	defer cleanup()

	// Parent want with "max_budget" declared as a goal-labeled state key.
	parent := &Want{
		Metadata:    Metadata{ID: "parent-id", Name: "parent", Type: "noop"},
		Spec:        WantSpec{Params: map[string]any{}},
		StateLabels: map[string]StateLabel{"max_budget": LabelGoal},
	}

	cb := NewChainBuilder([]*Want{parent})
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	// Register parent so findWantByName works inside the expose handler.
	RegisterWant(parent)
	defer UnregisterWant(parent.Metadata.Name)

	slider := &SliderWant{Want: Want{
		Metadata: Metadata{
			ID:   "slider-id",
			Name: "budget-slider",
			Type: "slider",
			OwnerReferences: []OwnerReference{{
				Kind: "Want", ID: "parent-id", Name: "parent", Controller: true,
			}},
		},
		Spec: WantSpec{
			Params: map[string]any{
				"default": 5000.0,
				"min":     0.0,
				"max":     10000.0,
			},
			Exposes: []ExposeEntry{
				{CurrentState: "value", AsGoal: "max_budget"},
			},
		},
	}}

	// RegisterWant sets up the asGoal expose handler subscription.
	RegisterWant(&slider.Want)
	defer UnregisterWant(slider.Metadata.Name)

	slider.BeginProgressCycle()
	slider.Initialize()
	slider.EndProgressCycle()

	// Progress() calls SetCurrent("value", 5000.0) → StateChangeEvent (sync) → expose handler
	// → parent.SetGoal("max_budget", 5000.0)
	slider.BeginProgressCycle()
	slider.Progress()
	slider.EndProgressCycle()

	goalVal, ok := parent.GetGoal("max_budget")
	require.True(t, ok, "parent goal 'max_budget' should be set")
	assert.Equal(t, 5000.0, goalVal)
}

func TestSliderWant_NoParent_GlobalStateExpose(t *testing.T) {
	cleanup := makeStateChangeSyncForTest(t)
	defer cleanup()

	// GlobalChainBuilder is required for StoreGlobalState / GetGlobalState to work.
	cb := NewChainBuilder([]*Want{})
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	slider := &SliderWant{Want: Want{
		Metadata: Metadata{ID: "slider-id", Name: "orphan-slider", Type: "slider"},
		Spec: WantSpec{
			Params: map[string]any{"default": 42.0},
			// Top-level want: use "as" to write to global state.
			Exposes: []ExposeEntry{
				{CurrentState: "value", As: "some_param"},
			},
		},
	}}

	RegisterWant(&slider.Want)
	defer UnregisterWant(slider.Metadata.Name)

	slider.BeginProgressCycle()
	slider.Initialize()
	slider.EndProgressCycle()

	slider.BeginProgressCycle()
	slider.Progress()
	slider.EndProgressCycle()

	// Top-level expose writes to global STATE (not global parameter).
	val, found := GetGlobalState("some_param")
	assert.True(t, found, "global state 'some_param' should be set via expose")
	assert.Equal(t, 42.0, val)
}

func TestSliderWant_NoExpose_NoPanic(t *testing.T) {
	slider := &SliderWant{Want: Want{
		Metadata: Metadata{ID: "slider-id", Name: "empty-slider", Type: "slider"},
		Spec:     WantSpec{Params: map[string]any{"default": 10.0}},
	}}

	slider.BeginProgressCycle()
	slider.Initialize()
	slider.EndProgressCycle()

	// Should not panic when no expose entries are configured.
	slider.BeginProgressCycle()
	slider.Progress()
	slider.EndProgressCycle()
}

func TestSliderWant_IsNeverAchieved(t *testing.T) {
	s := &SliderWant{}
	assert.False(t, s.IsAchieved())
}
