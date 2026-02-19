package mywant

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// TestGetParentState_NoParent verifies GetParentState returns nil when no parent exists.
func TestGetParentState_NoParent(t *testing.T) {
	child := &Want{
		Metadata: Metadata{Name: "child", Type: "test"},
		State:    make(map[string]any),
	}

	val, ok := child.GetParentState("budget")
	if ok {
		t.Errorf("Expected no parent state, got %v", val)
	}
}

// TestGetParentWant_NoControllerOwner verifies nil when OwnerReferences exist but none is a controller.
func TestGetParentWant_NoControllerOwner(t *testing.T) {
	child := &Want{
		Metadata: Metadata{
			Name: "child",
			Type: "test",
			OwnerReferences: []OwnerReference{
				{Kind: "Want", ID: "parent-id", Controller: false},
			},
		},
		State: make(map[string]any),
	}

	parent := child.GetParentWant()
	if parent != nil {
		t.Error("Expected nil parent when no controller owner exists")
	}
}

// TestParentStateAccess_BudgetScenario tests the travel budget use case:
// Parent Want holds budget and costs; child Wants write their costs and read the budget.
func TestParentStateAccess_BudgetScenario(t *testing.T) {
	parentWant := &Want{
		Metadata: Metadata{
			Name: "travel-coordinator",
			ID:   "travel-coord-id",
			Type: "coordinator",
		},
		Spec:  WantSpec{Params: make(map[string]any)},
		State: make(map[string]any),
	}
	// Set initial budget on parent
	parentWant.StoreState("budget", 5000.0)
	parentWant.StoreState("costs", map[string]any{})

	// Create ChainBuilder with parent want registered
	cb := NewChainBuilder(Config{
		Wants: []*Want{parentWant},
	})
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	// Create child wants with OwnerReferences pointing to parent
	makeChild := func(name string) *Want {
		return &Want{
			Metadata: Metadata{
				Name: name,
				Type: "test",
				OwnerReferences: []OwnerReference{
					{Kind: "Want", ID: "travel-coord-id", Controller: true},
				},
			},
			Spec:  WantSpec{Params: make(map[string]any)},
			State: make(map[string]any),
		}
	}

	flightWant := makeChild("flight")
	hotelWant := makeChild("hotel")
	restaurantWant := makeChild("restaurant")

	// --- Test GetParentState: children can read parent's budget ---
	budget, ok := flightWant.GetParentState("budget")
	if !ok {
		t.Fatal("Flight want failed to read parent budget")
	}
	if budget != 5000.0 {
		t.Errorf("Expected budget 5000, got %v", budget)
	}

	// --- Test StoreParentState: child writes a simple value to parent ---
	flightWant.StoreParentState("status", "booking")
	val, ok := parentWant.GetState("status")
	if !ok || val != "booking" {
		t.Errorf("Expected parent state 'status'='booking', got %v (exists=%v)", val, ok)
	}

	// --- Test MergeParentState: each child merges its cost into parent's costs Dict ---
	flightWant.MergeParentState(map[string]any{
		"costs": map[string]any{"flight": 1200.0},
	})
	hotelWant.MergeParentState(map[string]any{
		"costs": map[string]any{"hotel": 800.0},
	})
	restaurantWant.MergeParentState(map[string]any{
		"costs": map[string]any{"restaurant": 300.0},
	})

	// Verify all costs are merged in parent
	costsVal, ok := parentWant.GetState("costs")
	if !ok {
		t.Fatal("Parent has no costs state")
	}
	costs, ok := costsVal.(map[string]any)
	if !ok {
		t.Fatalf("Expected costs to be map[string]any, got %T", costsVal)
	}
	expectedCosts := map[string]float64{
		"flight":     1200.0,
		"hotel":      800.0,
		"restaurant": 300.0,
	}
	for key, expected := range expectedCosts {
		actual, exists := costs[key]
		if !exists {
			t.Errorf("Missing cost entry '%s'", key)
			continue
		}
		if actual != expected {
			t.Errorf("Cost '%s': expected %v, got %v", key, expected, actual)
		}
	}
}

// TestParentStateAccess_ConcurrentWrites verifies concurrent child writes to parent state are safe.
func TestParentStateAccess_ConcurrentWrites(t *testing.T) {
	parentWant := &Want{
		Metadata: Metadata{
			Name: "parent",
			ID:   "parent-id",
			Type: "coordinator",
		},
		Spec:  WantSpec{Params: make(map[string]any)},
		State: make(map[string]any),
	}
	parentWant.StoreState("costs", map[string]any{})

	cb := NewChainBuilder(Config{
		Wants: []*Want{parentWant},
	})
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	// Spawn 20 child wants that concurrently write to parent's costs
	const numChildren = 20
	var wg sync.WaitGroup
	wg.Add(numChildren)

	for i := 0; i < numChildren; i++ {
		go func(idx int) {
			defer wg.Done()
			child := &Want{
				Metadata: Metadata{
					Name: fmt.Sprintf("child-%d", idx),
					Type: "test",
					OwnerReferences: []OwnerReference{
						{Kind: "Want", ID: "parent-id", Controller: true},
					},
				},
				State: make(map[string]any),
			}
			child.MergeParentState(map[string]any{
				"costs": map[string]any{
					fmt.Sprintf("item-%d", idx): float64(idx * 100),
				},
			})
		}(i)
	}
	wg.Wait()

	// Verify all entries exist
	costsVal, ok := parentWant.GetState("costs")
	if !ok {
		t.Fatal("Parent has no costs state")
	}
	costs, ok := costsVal.(map[string]any)
	if !ok {
		t.Fatalf("Expected costs to be map[string]any, got %T", costsVal)
	}
	if len(costs) != numChildren {
		t.Errorf("Expected %d cost entries, got %d", numChildren, len(costs))
	}
	for i := 0; i < numChildren; i++ {
		key := fmt.Sprintf("item-%d", i)
		val, exists := costs[key]
		if !exists {
			t.Errorf("Missing cost entry '%s'", key)
			continue
		}
		expected := float64(i * 100)
		if val != expected {
			t.Errorf("Cost '%s': expected %v, got %v", key, expected, val)
		}
	}
}

// TestParentStateAccess_Cache verifies that parent want caching works correctly.
func TestParentStateAccess_Cache(t *testing.T) {
	parentWant := &Want{
		Metadata: Metadata{
			Name: "parent",
			ID:   "parent-id",
			Type: "coordinator",
		},
		Spec:  WantSpec{Params: make(map[string]any)},
		State: make(map[string]any),
	}
	parentWant.StoreState("value", "original")

	cb := NewChainBuilder(Config{
		Wants: []*Want{parentWant},
	})
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	child := &Want{
		Metadata: Metadata{
			Name: "child",
			Type: "test",
			OwnerReferences: []OwnerReference{
				{Kind: "Want", ID: "parent-id", Controller: true},
			},
		},
		State: make(map[string]any),
	}

	// First access populates cache
	p1 := child.GetParentWant()
	if p1 == nil {
		t.Fatal("Expected parent want")
	}

	// Second access should return cached pointer
	p2 := child.GetParentWant()
	if p1 != p2 {
		t.Error("Expected cached parent want pointer to be reused")
	}

	// Verify cache invalidation when OwnerReference ID changes
	child.Metadata.OwnerReferences[0].ID = "new-parent-id"
	p3 := child.GetParentWant()
	// new-parent-id doesn't exist, so should return nil
	if p3 != nil {
		t.Error("Expected nil after OwnerReference ID change to non-existent parent")
	}
}

// TestParentStateAccess_E2E_BudgetAgents is an end-to-end test simulating the travel budget scenario
// with actual DoAgent execution. Parent coordinator holds budget and costs; three child agents
// (flight, hotel, restaurant) each check the budget and write their cost to the parent's costs Dict.
func TestParentStateAccess_E2E_BudgetAgents(t *testing.T) {
	// 1. Create parent coordinator want with initial budget
	coordinator := &Want{
		Metadata: Metadata{
			Name: "travel-coordinator",
			ID:   "coord-e2e-id",
			Type: "coordinator",
		},
		Spec:  WantSpec{Params: make(map[string]any)},
		State: make(map[string]any),
	}
	coordinator.StoreState("budget", 5000.0)
	coordinator.StoreState("costs", map[string]any{})

	cb := NewChainBuilder(Config{
		Wants: []*Want{coordinator},
	})
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	// 2. Define agent actions that read budget and write costs via parent state
	flightAction := func(ctx context.Context, want *Want) error {
		budget, ok := want.GetParentState("budget")
		if !ok {
			return fmt.Errorf("cannot read parent budget")
		}
		cost := 1200.0
		if budget.(float64) < cost {
			return fmt.Errorf("budget exceeded")
		}
		want.MergeParentState(map[string]any{
			"costs": map[string]any{"flight": cost},
		})
		want.StoreState("booked", true)
		return nil
	}

	hotelAction := func(ctx context.Context, want *Want) error {
		budget, ok := want.GetParentState("budget")
		if !ok {
			return fmt.Errorf("cannot read parent budget")
		}
		cost := 800.0
		if budget.(float64) < cost {
			return fmt.Errorf("budget exceeded")
		}
		want.MergeParentState(map[string]any{
			"costs": map[string]any{"hotel": cost},
		})
		want.StoreState("booked", true)
		return nil
	}

	restaurantAction := func(ctx context.Context, want *Want) error {
		budget, ok := want.GetParentState("budget")
		if !ok {
			return fmt.Errorf("cannot read parent budget")
		}
		cost := 300.0
		if budget.(float64) < cost {
			return fmt.Errorf("budget exceeded")
		}
		want.MergeParentState(map[string]any{
			"costs": map[string]any{"restaurant": cost},
		})
		want.StoreState("booked", true)
		return nil
	}

	// 3. Create child wants with OwnerReferences
	makeChildWant := func(name string) *Want {
		return NewWantWithLocals(
			Metadata{
				Name: name,
				Type: "test",
				OwnerReferences: []OwnerReference{
					{Kind: "Want", ID: "coord-e2e-id", Controller: true},
				},
			},
			WantSpec{Params: make(map[string]any)},
			nil,
			"base",
		)
	}

	flightWant := makeChildWant("flight")
	hotelWant := makeChildWant("hotel")
	restaurantWant := makeChildWant("restaurant")

	// 4. Create DoAgents
	flightAgent := &DoAgent{
		BaseAgent: BaseAgent{
			Name:         "flight_budget_agent",
			Capabilities: []string{"flight_api_agency"},
			Type:         DoAgentType,
			ExecutionConfig: ExecutionConfig{
				Mode: ExecutionModeLocal,
			},
		},
		Action: flightAction,
	}
	hotelAgent := &DoAgent{
		BaseAgent: BaseAgent{
			Name:         "hotel_budget_agent",
			Capabilities: []string{"hotel_agency"},
			Type:         DoAgentType,
			ExecutionConfig: ExecutionConfig{
				Mode: ExecutionModeLocal,
			},
		},
		Action: hotelAction,
	}
	restaurantAgent := &DoAgent{
		BaseAgent: BaseAgent{
			Name:         "restaurant_budget_agent",
			Capabilities: []string{"restaurant_agency"},
			Type:         DoAgentType,
			ExecutionConfig: ExecutionConfig{
				Mode: ExecutionModeLocal,
			},
		},
		Action: restaurantAction,
	}

	// 5. Execute each agent with full progress cycle (simulating real execution)
	type agentExec struct {
		agent *DoAgent
		want  *Want
		name  string
	}
	executions := []agentExec{
		{flightAgent, flightWant, "flight"},
		{hotelAgent, hotelWant, "hotel"},
		{restaurantAgent, restaurantWant, "restaurant"},
	}

	for _, exec := range executions {
		exec.want.BeginProgressCycle()
		_, err := exec.agent.Exec(context.Background(), exec.want)
		if err != nil {
			t.Fatalf("%s agent execution failed: %v", exec.name, err)
		}
		exec.want.EndProgressCycle()

		// Verify child's own state was set
		booked, ok := exec.want.GetState("booked")
		if !ok || booked != true {
			t.Errorf("%s: expected booked=true, got %v", exec.name, booked)
		}
		t.Logf("  %s agent: booked=%v", exec.name, booked)
	}

	// 6. Verify parent coordinator has all costs merged correctly
	costsVal, ok := coordinator.GetState("costs")
	if !ok {
		t.Fatal("Parent coordinator has no costs state")
	}
	costs, ok := costsVal.(map[string]any)
	if !ok {
		t.Fatalf("Expected costs to be map[string]any, got %T", costsVal)
	}

	expectedCosts := map[string]float64{
		"flight":     1200.0,
		"hotel":      800.0,
		"restaurant": 300.0,
	}
	totalCost := 0.0
	for key, expected := range expectedCosts {
		actual, exists := costs[key]
		if !exists {
			t.Errorf("Missing cost entry '%s' in parent costs", key)
			continue
		}
		if actual != expected {
			t.Errorf("Cost '%s': expected %v, got %v", key, expected, actual)
		}
		totalCost += actual.(float64)
	}

	// 7. Verify budget constraint
	budget, _ := coordinator.GetState("budget")
	if totalCost > budget.(float64) {
		t.Errorf("Total cost %.0f exceeds budget %.0f", totalCost, budget.(float64))
	}
	t.Logf("  Budget: %.0f, Total costs: %.0f (flight=%.0f, hotel=%.0f, restaurant=%.0f)",
		budget, totalCost, costs["flight"], costs["hotel"], costs["restaurant"])
}

// TestParentStateAccess_E2E_ConcurrentAgents tests concurrent agent execution writing to parent.
func TestParentStateAccess_E2E_ConcurrentAgents(t *testing.T) {
	coordinator := &Want{
		Metadata: Metadata{
			Name: "concurrent-coordinator",
			ID:   "concurrent-coord-id",
			Type: "coordinator",
		},
		Spec:  WantSpec{Params: make(map[string]any)},
		State: make(map[string]any),
	}
	coordinator.StoreState("costs", map[string]any{})

	cb := NewChainBuilder(Config{
		Wants: []*Want{coordinator},
	})
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	// Launch 10 agents concurrently, each writing a different cost key
	const numAgents = 10
	var wg sync.WaitGroup
	wg.Add(numAgents)

	for i := 0; i < numAgents; i++ {
		go func(idx int) {
			defer wg.Done()

			childWant := NewWantWithLocals(
				Metadata{
					Name: fmt.Sprintf("service-%d", idx),
					Type: "test",
					OwnerReferences: []OwnerReference{
						{Kind: "Want", ID: "concurrent-coord-id", Controller: true},
					},
				},
				WantSpec{Params: make(map[string]any)},
				nil,
				"base",
			)

			agent := &DoAgent{
				BaseAgent: BaseAgent{
					Name: fmt.Sprintf("agent-%d", idx),
					Type: DoAgentType,
					ExecutionConfig: ExecutionConfig{
						Mode: ExecutionModeLocal,
					},
				},
				Action: func(ctx context.Context, want *Want) error {
					want.MergeParentState(map[string]any{
						"costs": map[string]any{
							fmt.Sprintf("service-%d", idx): float64((idx + 1) * 100),
						},
					})
					return nil
				},
			}

			childWant.BeginProgressCycle()
			_, err := agent.Exec(context.Background(), childWant)
			if err != nil {
				t.Errorf("Agent %d failed: %v", idx, err)
			}
			childWant.EndProgressCycle()
		}(i)
	}
	wg.Wait()

	// Verify all 10 cost entries are present
	costsVal, ok := coordinator.GetState("costs")
	if !ok {
		t.Fatal("Coordinator has no costs state")
	}
	costs, ok := costsVal.(map[string]any)
	if !ok {
		t.Fatalf("Expected costs to be map[string]any, got %T", costsVal)
	}
	if len(costs) != numAgents {
		t.Errorf("Expected %d cost entries, got %d: %v", numAgents, len(costs), costs)
	}
	for i := 0; i < numAgents; i++ {
		key := fmt.Sprintf("service-%d", i)
		val, exists := costs[key]
		if !exists {
			t.Errorf("Missing cost entry '%s'", key)
			continue
		}
		expected := float64((i + 1) * 100)
		if val != expected {
			t.Errorf("Cost '%s': expected %v, got %v", key, expected, val)
		}
	}
	t.Logf("  All %d concurrent agents wrote costs successfully", numAgents)
}

// TestStoreParentState_NoParentWarning verifies StoreParentState handles no-parent gracefully.
func TestStoreParentState_NoParentWarning(t *testing.T) {
	child := &Want{
		Metadata: Metadata{Name: "orphan", Type: "test"},
		State:    make(map[string]any),
	}

	// Should not panic, just log a warning
	child.StoreParentState("key", "value")
	child.MergeParentState(map[string]any{"key": "value"})
}
