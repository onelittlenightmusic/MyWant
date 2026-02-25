package mywant

import (
	"testing"
	"time"
)

type EarlyAchievedWant struct {
	Want
	achieved bool
}

func (m *EarlyAchievedWant) Initialize() {
	m.StoreState("some_field", "some_value")
	m.achieved = true
}

func (m *EarlyAchievedWant) IsAchieved() bool {
	return m.achieved
}

func (m *EarlyAchievedWant) Progress() {
	// Should not be called in Step 3.1 early exit
}

func TestEarlyAchievementFinalResult(t *testing.T) {
	w := &Want{
		Metadata: Metadata{
			Name: "test-early-achieved",
		},
		Spec: WantSpec{
			FinalResultField: "some_field",
		},
		Status: WantStatusReaching,
	}

	ew := &EarlyAchievedWant{
		Want: *w,
	}
	ew.Want.progressable = ew

	// Run progression loop
	getPaths := func() Paths { return Paths{} }
	done := make(chan bool)
	onComplete := func() {
		done <- true
	}

	ew.Want.StartProgressionLoop(getPaths, onComplete)

	select {
	case <-done:
		// Check if final_result is populated
		val, ok := ew.Want.GetState("final_result")
		if !ok || val != "some_value" {
			t.Errorf("Expected final_result to be 'some_value', got %v", val)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out")
	}
}
