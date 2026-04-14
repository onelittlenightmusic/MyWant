package mywant

import (
	"encoding/json"
	"testing"
	"time"
)

// waitWantsAdded waits until all want IDs are registered in the ChainBuilder.
func waitWantsAdded(t *testing.T, cb *ChainBuilder, ids []string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cb.AreWantsAdded(ids) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !cb.AreWantsAdded(ids) {
		t.Fatal("Wants not added to ChainBuilder in time")
	}
}

// getRuntimeWant retrieves the runtime *Want pointer from ChainBuilder by ID.
// After AddWantsAsync, ChainBuilder may create a new pointer, so always use this
// when asserting on state.
func getRuntimeWant(t *testing.T, cb *ChainBuilder, id string) *Want {
	t.Helper()
	w, _, found := cb.FindWantByID(id)
	if !found {
		t.Fatalf("Want %s not found in ChainBuilder", id)
	}
	return w
}

// TestInterpretDirections verifies that InterpretDirections converts directions + direction_map
// into DispatchRequests and writes them to the parent state via ProposeDispatch.
func TestInterpretDirections(t *testing.T) {
	cb := NewChainBuilder(Config{Wants: []*Want{}})
	SetGlobalChainBuilder(cb)
	go cb.reconcileLoop()
	defer cb.Stop()
	time.Sleep(100 * time.Millisecond)

	directionMap := map[string]any{
		"reserve_hotel": map[string]any{
			"type":       "hotel",
			"params":     map[string]any{"hotel_type": "luxury"},
			"sets":       map[string]any{"hotel_reserved": true},
			"cost_field": "hotel_cost",
		},
		"reserve_dinner": map[string]any{
			"type":   "restaurant",
			"params": map[string]any{"restaurant_type": "fine dining"},
			"sets":   map[string]any{"dinner_reserved": true},
		},
	}
	dmBytes, _ := json.Marshal(directionMap)

	parent := &Want{
		Metadata: Metadata{ID: "parent-id", Name: "parent", Type: "noop"},
		Spec:     WantSpec{},
	}
	child := &Want{
		Metadata: Metadata{
			ID:     "child-id",
			Name:   "planner",
			Type:   "itinerary",
			Labels: map[string]string{"child-role": "thinker"},
			OwnerReferences: []OwnerReference{{
				Kind:       "Want",
				ID:         "parent-id",
				Name:       "parent",
				Controller: true,
			}},
		},
		Spec: WantSpec{
			Params: map[string]any{
				"direction_map": string(dmBytes),
			},
		},
	}

	ids, err := cb.AddWantsAsyncWithTracking([]*Want{parent, child})
	if err != nil {
		t.Fatalf("Failed to add wants: %v", err)
	}
	waitWantsAdded(t, cb, ids)

	// Use runtime pointers (ChainBuilder may create new *Want instances)
	rtChild := getRuntimeWant(t, cb, "child-id")
	rtParent := getRuntimeWant(t, cb, "parent-id")

	// parent is "noop" type which has no YAML state definition for desired_dispatch,
	// so we must label it explicitly to allow the governance engine to permit writes.
	if rtParent.StateLabels == nil {
		rtParent.StateLabels = make(map[string]StateLabel)
	}
	rtParent.StateLabels["desired_dispatch"] = LabelPlan
	// rtChild is "itinerary" type: child-role:thinker is auto-applied from itinerary.yaml

	// Set directions in runtime child's plan state
	rtChild.storeState("directions", []string{"reserve_hotel", "reserve_dinner"})

	// Run InterpretDirections
	InterpretDirections(rtChild)

	// Verify desired_dispatch is written to runtime parent's state
	raw, ok := rtParent.getState("desired_dispatch")
	if !ok {
		t.Fatal("Expected 'desired_dispatch' in parent state, but not found")
	}

	var requests []DispatchRequest
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &requests); err != nil {
		t.Fatalf("Failed to unmarshal desired_dispatch: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("Expected 2 DispatchRequests, got %d", len(requests))
	}

	byDir := make(map[string]DispatchRequest)
	for _, r := range requests {
		byDir[r.Direction] = r
	}

	hotel, ok := byDir["reserve_hotel"]
	if !ok {
		t.Fatal("Missing 'reserve_hotel' in desired_dispatch")
	}
	if hotel.Type != "hotel" {
		t.Errorf("Expected type 'hotel', got '%s'", hotel.Type)
	}
	if hotel.CostField != "hotel_cost" {
		t.Errorf("Expected cost_field 'hotel_cost', got '%s'", hotel.CostField)
	}
	if hotel.RequesterID != "child-id" {
		t.Errorf("Expected requester_id 'child-id', got '%s'", hotel.RequesterID)
	}

	dinner, ok := byDir["reserve_dinner"]
	if !ok {
		t.Fatal("Missing 'reserve_dinner' in desired_dispatch")
	}
	if dinner.Type != "restaurant" {
		t.Errorf("Expected type 'restaurant', got '%s'", dinner.Type)
	}

	t.Log("✅ InterpretDirections correctly produced DispatchRequests in parent state")
}

// TestDispatchThinkerRealizesDesiredDispatch verifies that DispatchThinker reads
// desired_dispatch from its own state and creates child wants via AddChildWant.
func TestDispatchThinkerRealizesDesiredDispatch(t *testing.T) {
	cb := NewChainBuilder(Config{Wants: []*Want{}})
	SetGlobalChainBuilder(cb)
	go cb.reconcileLoop()
	defer cb.Stop()
	time.Sleep(100 * time.Millisecond)

	parent := &Want{
		Metadata: Metadata{ID: "owner-id", Name: "owner", Type: "noop"},
		Spec:     WantSpec{},
	}
	parent.State.Store("_state_initialized", true)

	if err := cb.AddWantsAsync([]*Want{parent}); err != nil {
		t.Fatalf("Failed to add parent want: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Write desired_dispatch directly to parent's state (as ProposeDispatch would)
	requests := []DispatchRequest{
		{
			Direction:   "reserve_hotel",
			RequesterID: "planner-id",
			Type:        "hotel",
			Params:      map[string]any{"hotel_type": "luxury"},
			Sets:        map[string]any{"hotel_reserved": true},
		},
	}
	parent.storeState("desired_dispatch", requests)

	// Start DispatchThinker on parent (local pointer - DispatchThinker uses this)
	agent := NewDispatchThinker("dispatch-thinker-owner-id")
	if err := parent.AddBackgroundAgent(agent); err != nil {
		t.Fatalf("Failed to start DispatchThinker: %v", err)
	}

	// Wait for DispatchThinker to create the child want
	deadline := time.Now().Add(5 * time.Second)
	var hotelWant *Want
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		for _, w := range cb.GetWants() {
			if w.Metadata.Type == "hotel" && w.Metadata.Name == "reserve_hotel-owner" {
				hotelWant = w
				break
			}
		}
		if hotelWant != nil {
			break
		}
	}

	if hotelWant == nil {
		t.Fatal("Expected DispatchThinker to create a child want of type 'hotel', but none found")
	}
	hotelType, _ := hotelWant.Spec.GetParam("hotel_type")
	if hotelType != "luxury" {
		t.Errorf("Expected hotel_type 'luxury', got '%v'", hotelType)
	}
	if len(hotelWant.Metadata.OwnerReferences) == 0 {
		t.Fatal("Expected hotel want to have owner reference")
	}
	if hotelWant.Metadata.OwnerReferences[0].ID != "owner-id" {
		t.Errorf("Expected owner ID 'owner-id', got '%s'", hotelWant.Metadata.OwnerReferences[0].ID)
	}

	t.Log("✅ DispatchThinker correctly realized desired_dispatch and created child want")
}

// TestProposeDispatchOverwrites verifies that ProposeDispatch overwrites (not appends)
// so that OPA replanning with fewer directions is handled correctly.
func TestProposeDispatchOverwrites(t *testing.T) {
	cb := NewChainBuilder(Config{Wants: []*Want{}})
	SetGlobalChainBuilder(cb)
	go cb.reconcileLoop()
	defer cb.Stop()
	time.Sleep(100 * time.Millisecond)

	parent := &Want{
		Metadata: Metadata{ID: "parent-id2", Name: "parent2", Type: "noop"},
		Spec:     WantSpec{},
	}
	child := &Want{
		Metadata: Metadata{
			ID:     "child-id2",
			Name:   "planner2",
			Type:   "itinerary",
			Labels: map[string]string{"child-role": "thinker"},
			OwnerReferences: []OwnerReference{{
				Kind:       "Want",
				ID:         "parent-id2",
				Name:       "parent2",
				Controller: true,
			}},
		},
		Spec: WantSpec{
			Params: map[string]any{
				"direction_map": `{"a":{"type":"noop"},"b":{"type":"noop"}}`,
			},
		},
	}

	ids, err := cb.AddWantsAsyncWithTracking([]*Want{parent, child})
	if err != nil {
		t.Fatalf("Failed to add wants: %v", err)
	}
	waitWantsAdded(t, cb, ids)

	rtChild := getRuntimeWant(t, cb, "child-id2")
	rtParent := getRuntimeWant(t, cb, "parent-id2")

	// parent is "noop" type which has no YAML state definition for desired_dispatch.
	if rtParent.StateLabels == nil {
		rtParent.StateLabels = make(map[string]StateLabel)
	}
	rtParent.StateLabels["desired_dispatch"] = LabelPlan
	// rtChild is "itinerary" type: child-role:thinker is auto-applied from itinerary.yaml

	// First call: 2 directions
	rtChild.storeState("directions", []string{"a", "b"})
	InterpretDirections(rtChild)

	raw1, _ := rtParent.getState("desired_dispatch")
	b1, _ := json.Marshal(raw1)
	var r1 []DispatchRequest
	json.Unmarshal(b1, &r1)
	if len(r1) != 2 {
		t.Fatalf("Expected 2 requests after first call, got %d", len(r1))
	}

	// Second call: 1 direction (OPA replanned, removed "b")
	rtChild.storeState("directions", []string{"a"})
	InterpretDirections(rtChild)

	raw2, _ := rtParent.getState("desired_dispatch")
	b2, _ := json.Marshal(raw2)
	var r2 []DispatchRequest
	json.Unmarshal(b2, &r2)
	if len(r2) != 1 {
		t.Fatalf("Expected 1 request after replan, got %d (should overwrite, not append)", len(r2))
	}
	if r2[0].Direction != "a" {
		t.Errorf("Expected direction 'a', got '%s'", r2[0].Direction)
	}

	t.Log("✅ ProposeDispatch correctly overwrites on replan")
}

