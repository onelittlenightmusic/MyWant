package mywant

import (
	"sync"
	"testing"
)

// TestWantStateConcurrency tests the concurrent read/write safety of the new
// sync.Map-based state management.
func TestWantStateConcurrency(t *testing.T) {
	want := &Want{}
	want.Init() // Initializes State sync.Map and other fields

	// Define some labels for testing
	want.StateLabels = map[string]StateLabel{
		"current_counter": LabelCurrent,
		"goal_status":     LabelGoal,
		"internal_flag":   LabelInternal,
	}

	var wg sync.WaitGroup
	iterations := 1000

	// Concurrent writes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want.SetCurrent("current_counter", i)
			if i%10 == 0 {
				want.SetGoal("goal_status", "in_progress")
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok := want.GetCurrent("current_counter")
			if !ok {
				// This might be ok if the read happens before the first write
			}
			_, _ = want.GetGoal("goal_status")
		}()
	}

	wg.Wait()

	// Final check
	val, ok := want.GetCurrent("current_counter")
	if !ok {
		t.Errorf("Expected to get a value for 'current_counter', but got none")
	}
	if val.(int) >= iterations {
		t.Errorf("Counter value is unexpectedly high: got %d", val.(int))
	}

	goalVal, ok := want.GetGoal("goal_status")
	if !ok {
		t.Errorf("Expected to get a value for 'goal_status', but got none")
	}
	if goalVal.(string) != "in_progress" {
		t.Errorf("Expected goal_status to be 'in_progress', but got %s", goalVal.(string))
	}

	t.Logf("Concurrency test finished. Final counter value: %v", val)
}
