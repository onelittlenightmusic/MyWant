package server

import (
	"testing"

	mywant "mywant/engine/src"

	"github.com/stretchr/testify/assert"
)

// TestTargetAchievingPercentageCalculation tests that Target correctly calculates achieving_percentage
func TestTargetAchievingPercentageCalculation(t *testing.T) {
	// Create a Target with metadata and spec using "level 1 approval" type
	metadata := mywant.Metadata{
		Name: "test-target",
		Type: "level 1 approval",
	}
	spec := mywant.WantSpec{
		Params: make(map[string]any),
	}

	target := mywant.NewTarget(metadata, spec)
	target.InitializeSubscriptionSystem()

	// Create child wants
	child1 := &mywant.Want{
		Metadata: mywant.Metadata{Name: "child-1", Type: "test"},
		Spec:     mywant.WantSpec{Params: make(map[string]any)},
		State:    make(map[string]any),
		Status:   mywant.WantStatusIdle,
	}

	child2 := &mywant.Want{
		Metadata: mywant.Metadata{Name: "child-2", Type: "test"},
		Spec:     mywant.WantSpec{Params: make(map[string]any)},
		State:    make(map[string]any),
		Status:   mywant.WantStatusIdle,
	}

	child3 := &mywant.Want{
		Metadata: mywant.Metadata{Name: "child-3", Type: "test"},
		Spec:     mywant.WantSpec{Params: make(map[string]any)},
		State:    make(map[string]any),
		Status:   mywant.WantStatusIdle,
	}

	// Manually add children to Target's childWants
	target.SetChildWants([]*mywant.Want{child1, child2, child3})
	target.SetChildrenCreated(true) // Mark children as created so Progress() will process them

	// Initialize Target's completedChildren map
	completedMap := make(map[string]bool)
	for _, child := range target.GetChildWants() {
		completedMap[child.Metadata.Name] = false
	}
	target.SetCompletedChildren(completedMap)

	// Test 1: Initially, achieving_percentage should be 0
	target.BeginProgressCycle()
	target.Progress()
	target.EndProgressCycle()

	achievingPercentage, exists := target.GetState("achieving_percentage")
	assert.True(t, exists, "achieving_percentage should exist in state")
	assert.Equal(t, 0.0, achievingPercentage, "Initial achieving_percentage should be 0")

	// Test 2: Mark one child as complete (33%)
	completedMap["child-1"] = true
	target.SetCompletedChildren(completedMap)
	target.BeginProgressCycle()
	target.Progress()
	target.EndProgressCycle()

	achievingPercentage, _ = target.GetState("achieving_percentage")
	expectedPercentage := float64(100 / 3) // ~33.33%
	assert.InDelta(t, expectedPercentage, achievingPercentage, 1.0, "achieving_percentage should be ~33% with 1/3 children complete")

	// Test 3: Mark two children as complete (66%)
	completedMap["child-2"] = true
	target.SetCompletedChildren(completedMap)
	target.BeginProgressCycle()
	target.Progress()
	target.EndProgressCycle()

	achievingPercentage, _ = target.GetState("achieving_percentage")
	expectedPercentage = float64(200 / 3) // ~66.66%
	assert.InDelta(t, expectedPercentage, achievingPercentage, 1.0, "achieving_percentage should be ~66% with 2/3 children complete")

	// Test 4: Mark all children as complete (100%)
	completedMap["child-3"] = true
	target.SetCompletedChildren(completedMap)
	target.BeginProgressCycle()
	target.Progress()
	target.EndProgressCycle()

	achievingPercentage, _ = target.GetState("achieving_percentage")
	assert.Equal(t, 100.0, achievingPercentage, "achieving_percentage should be 100% with all children complete")

	// Test 5: Target should reach ACHIEVED status when all children are complete
	assert.Equal(t, mywant.WantStatusAchieved, target.Want.Status, "Target status should be ACHIEVED when all children are complete")
}

// TestTargetAchievedWithFullCompletion tests that when Target reaches ACHIEVED, achieving_percentage is forced to 100
func TestTargetAchievedWithFullCompletion(t *testing.T) {
	metadata := mywant.Metadata{
		Name: "test-target-achieved",
		Type: "level 1 approval",
	}
	spec := mywant.WantSpec{
		Params: make(map[string]any),
	}

	target := mywant.NewTarget(metadata, spec)
	target.InitializeSubscriptionSystem()

	// Create single child
	child := &mywant.Want{
		Metadata: mywant.Metadata{Name: "child-1", Type: "test"},
		Spec:     mywant.WantSpec{Params: make(map[string]any)},
		State:    make(map[string]any),
		Status:   mywant.WantStatusIdle,
	}

	target.SetChildWants([]*mywant.Want{child})
	target.SetChildrenCreated(true) // Mark children as created so Progress() will process them
	completedMap := make(map[string]bool)
	completedMap["child-1"] = false
	target.SetCompletedChildren(completedMap)

	// Progress with no children complete - should be 0%
	target.BeginProgressCycle()
	target.Progress()
	target.EndProgressCycle()

	achievingPercentage, _ := target.GetState("achieving_percentage")
	assert.Equal(t, 0.0, achievingPercentage, "Initially achieving_percentage should be 0")

	// Mark child as complete and progress
	completedMap["child-1"] = true
	target.SetCompletedChildren(completedMap)
	target.BeginProgressCycle()
	target.Progress()
	target.EndProgressCycle()

	// Verify achieving_percentage is 100
	achievingPercentage, _ = target.GetState("achieving_percentage")
	assert.Equal(t, 100.0, achievingPercentage, "achieving_percentage should be 100 when all children complete")

	// Verify status is ACHIEVED
	assert.Equal(t, mywant.WantStatusAchieved, target.Want.Status, "Status should be ACHIEVED")

	// EndProgressCycle should force achieving_percentage to 100 even if status is ACHIEVED
	target.BeginProgressCycle()
	target.EndProgressCycle()

	achievingPercentage, _ = target.GetState("achieving_percentage")
	assert.Equal(t, 100.0, achievingPercentage, "achieving_percentage should remain 100 after EndProgressCycle for ACHIEVED status")
}

// TestTargetAchievingPercentageEdgeCases tests edge cases like empty childWants
func TestTargetAchievingPercentageEdgeCases(t *testing.T) {
	metadata := mywant.Metadata{
		Name: "test-target-empty",
		Type: "level 1 approval",
	}
	spec := mywant.WantSpec{
		Params: make(map[string]any),
	}

	target := mywant.NewTarget(metadata, spec)
	target.InitializeSubscriptionSystem()

	// No children
	target.SetChildWants([]*mywant.Want{})
	target.SetChildrenCreated(true) // Mark children as created so Progress() will process them
	target.SetCompletedChildren(make(map[string]bool))

	target.BeginProgressCycle()
	target.Progress()
	target.EndProgressCycle()

	// Should reach ACHIEVED immediately if no children
	assert.Equal(t, mywant.WantStatusAchieved, target.Want.Status, "Target should be ACHIEVED with no children")

	achievingPercentage, _ := target.GetState("achieving_percentage")
	assert.Equal(t, 100.0, achievingPercentage, "achieving_percentage should be 100 with no children to complete")
}

// TestTargetStateConsistency tests that achieving_percentage and status stay consistent
func TestTargetStateConsistency(t *testing.T) {
	metadata := mywant.Metadata{
		Name: "test-target-consistency",
		Type: "level 1 approval",
	}
	spec := mywant.WantSpec{
		Params: make(map[string]any),
	}

	target := mywant.NewTarget(metadata, spec)
	target.InitializeSubscriptionSystem()

	// Create 5 children
	children := make([]*mywant.Want, 0)
	for i := 1; i <= 5; i++ {
		child := &mywant.Want{
			Metadata: mywant.Metadata{Name: "child-" + string(rune('0'+i)), Type: "test"},
			Spec:     mywant.WantSpec{Params: make(map[string]any)},
			State:    make(map[string]any),
			Status:   mywant.WantStatusIdle,
		}
		children = append(children, child)
	}

	target.SetChildWants(children)
	target.SetChildrenCreated(true) // Mark children as created so Progress() will process them

	completedMap := make(map[string]bool)
	for _, child := range target.GetChildWants() {
		completedMap[child.Metadata.Name] = false
	}
	target.SetCompletedChildren(completedMap)

	// Progressively complete children and verify consistency
	for i := 1; i <= 5; i++ {
		// Mark i-th child as complete
		completedMap["child-"+string(rune('0'+i))] = true
		target.SetCompletedChildren(completedMap)

		target.BeginProgressCycle()
		target.Progress()
		target.EndProgressCycle()

		achievingPercentage, _ := target.GetState("achieving_percentage")
		expectedPercentage := float64(i * 100 / 5)

		assert.InDelta(t, expectedPercentage, achievingPercentage, 0.1,
			"achieving_percentage should be %d%% when %d/%d children complete", i*20, i, 5)

		// If all complete, status should be ACHIEVED
		if i == 5 {
			assert.Equal(t, mywant.WantStatusAchieved, target.Want.Status, "Status should be ACHIEVED when all children complete")
			assert.Equal(t, 100.0, achievingPercentage, "achieving_percentage should be exactly 100 when all children complete")
		}
	}
}

// BenchmarkTargetProgressCycle benchmarks the performance of Target.Progress()
func BenchmarkTargetProgressCycle(b *testing.B) {
	metadata := mywant.Metadata{
		Name: "bench-target",
		Type: "level 1 approval",
	}
	spec := mywant.WantSpec{
		Params: make(map[string]any),
	}

	target := mywant.NewTarget(metadata, spec)
	target.InitializeSubscriptionSystem()

	// Create 10 children
	children := make([]*mywant.Want, 0)
	for i := 1; i <= 10; i++ {
		child := &mywant.Want{
			Metadata: mywant.Metadata{Name: "child-bench-" + string(rune('0'+i)), Type: "test"},
			Spec:     mywant.WantSpec{Params: make(map[string]any)},
			State:    make(map[string]any),
			Status:   mywant.WantStatusIdle,
		}
		children = append(children, child)
	}

	target.SetChildWants(children)
	target.SetChildrenCreated(true) // Mark children as created so Progress() will process them

	completedMap := make(map[string]bool)
	for _, child := range target.GetChildWants() {
		completedMap[child.Metadata.Name] = false
	}
	target.SetCompletedChildren(completedMap)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		target.BeginProgressCycle()
		target.Progress()
		target.EndProgressCycle()
	}
}
