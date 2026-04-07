package types

import (
	"testing"

	. "mywant/engine/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSliderWant_Initialize(t *testing.T) {
	slider := &SliderWant{Want: Want{
		Metadata: Metadata{ID: "slider-id", Name: "budget-slider", Type: "slider"},
		Spec: WantSpec{Params: map[string]any{
			"target_param": "max_budget",
			"default":      5000.0,
			"min":          0.0,
			"max":          10000.0,
			"step":         100.0,
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

	tp, _ := slider.GetStateString("target_param", "")
	assert.Equal(t, "max_budget", tp)
}

func TestSliderWant_PropagatesValueToParent(t *testing.T) {
	parent := &Want{
		Metadata: Metadata{ID: "parent-id", Name: "parent", Type: "noop"},
		Spec:     WantSpec{Params: map[string]any{"max_budget": 1000.0}},
	}

	// Set up ChainBuilder so GetParentWant can find the parent via FindWantByID
	cb := NewChainBuilder(Config{Wants: []*Want{parent}})
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	slider := &SliderWant{Want: Want{
		Metadata: Metadata{
			ID:   "slider-id",
			Name: "budget-slider",
			Type: "slider",
			OwnerReferences: []OwnerReference{{
				Kind: "Want", ID: "parent-id", Name: "parent", Controller: true,
			}},
		},
		Spec: WantSpec{Params: map[string]any{
			"target_param": "max_budget",
			"default":      5000.0,
			"min":          0.0,
			"max":          10000.0,
		}},
	}}

	slider.BeginProgressCycle()
	slider.Initialize()
	slider.EndProgressCycle()

	slider.BeginProgressCycle()
	slider.Progress()
	slider.EndProgressCycle()

	// FindWantByID returns the config want (parent) since reconcile loop isn't running
	rtParent, _, found := cb.FindWantByID("parent-id")
	require.True(t, found)
	paramVal, ok := rtParent.Spec.GetParam("max_budget")
	assert.True(t, ok)
	assert.Equal(t, 5000.0, paramVal)
}

func TestSliderWant_NoParent_SetsGlobalParameter(t *testing.T) {
	slider := &SliderWant{Want: Want{
		Metadata: Metadata{ID: "slider-id", Name: "orphan-slider", Type: "slider"},
		Spec: WantSpec{Params: map[string]any{
			"target_param": "some_param",
			"default":      42.0,
		}},
	}}

	slider.BeginProgressCycle()
	slider.Initialize()
	slider.EndProgressCycle()

	slider.BeginProgressCycle()
	slider.Progress()
	slider.EndProgressCycle()

	// Should propagate to global parameter
	val, found := GetGlobalParameter("some_param")
	assert.True(t, found)
	assert.Equal(t, 42.0, val)
}

func TestSliderWant_EmptyTargetParam(t *testing.T) {
	slider := &SliderWant{Want: Want{
		Metadata: Metadata{ID: "slider-id", Name: "empty-slider", Type: "slider"},
		Spec:     WantSpec{Params: map[string]any{"default": 10.0}},
	}}

	slider.BeginProgressCycle()
	slider.Initialize()
	slider.EndProgressCycle()

	// Should not panic when target_param is empty
	slider.BeginProgressCycle()
	slider.Progress()
	slider.EndProgressCycle()
}

func TestSliderWant_IsNeverAchieved(t *testing.T) {
	s := &SliderWant{}
	assert.False(t, s.IsAchieved())
}
