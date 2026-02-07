package main

import (
	"fmt"
	_ "mywant/engine/cmd/types"
	mywant "mywant/engine/src"
	"time"
)

func main() {
	fmt.Println("🧪 Testing Parent-Child Lifecycle with Parameter Updates")
	fmt.Println("=========================================================")
	fmt.Println()
	config := mywant.Config{
		Wants: []*mywant.Want{
			{
				Metadata: mywant.Metadata{
					Name: "test-target",
					Type: "target",
					Labels: map[string]string{
						"role": "parent",
					},
				},
				Spec: mywant.WantSpec{
					Params: map[string]any{
						"max_display":  100,
						"count":        50,
						"rate":         10.0,
						"service_time": 0.1,
					},
				},
				Status: mywant.WantStatusIdle,
				State:  make(map[string]any),
			},
		},
	}
	builder := mywant.NewChainBuilder(config)

	// Register want types
	
	mywant.

	fmt.Println("📋 Test Scenario:")
	fmt.Println("1. Start target want (creates children from recipe)")
	fmt.Println("2. Wait for children to complete → parent becomes 'completed'")
	fmt.Println("3. Update parameter on parent")
	fmt.Println("4. Children restart → parent should restart too (completed → idle → running)")
	fmt.Println("5. Children complete again → parent becomes 'completed' again")
	fmt.Println()

	// Start execution in background
	go builder.Execute()

	// Wait for system to initialize
	time.Sleep(1 * time.Second)

	// Monitor parent status
	fmt.Println("📊 Monitoring parent status...")
	for i := 0; i < 20; i++ {
		time.Sleep(1 * time.Second)
		states := builder.GetAllWantStates()
		for name, state := range states {
			if state.Metadata.Type == "target" {
				fmt.Printf("[T+%02ds] Parent '%s': %s\n", i+1, name, state.Status)

				// At 5 seconds, if parent is completed, update parameter
				if i == 4 && state.Status == mywant.WantStatusAchieved {
					fmt.Println("\n🔄 === UPDATING PARAMETER ===")
					if wantPtr, _, found := builder.FindWantByID(name); found {
						exec := wantPtr.GetProgressable()
						if target, ok := exec.(*mywant.Target); ok {
							fmt.Printf("🔄 Changing service_time from 0.1 to 0.05\n")
							target.UpdateParameter("service_time", 0.05)
							fmt.Println("✅ Parameter updated and pushed to children\n")
						}
					}
				}
			}
		}
	}

	// Final status check
	fmt.Println("\n📊 Final Status Report:")
	fmt.Println("=======================")
	states := builder.GetAllWantStates()
	for name, state := range states {
		if state.Metadata.Type == "target" {
			fmt.Printf("🎯 Parent '%s': %s\n", name, state.Status)
			if result, ok := state.State["result"]; ok {
				fmt.Printf("   Result: %v\n", result)
			}
			if childCount, ok := state.State["childCount"]; ok {
				fmt.Printf("   Children: %v\n", childCount)
			}

			fmt.Println("\n📜 State History:")
			if state.History.StateHistory != nil && len(state.History.StateHistory) > 0 {
				fmt.Printf("   Total state changes: %d\n", len(state.History.StateHistory))
			}
		}
	}

	// Count children
	childCount := 0
	for _, state := range states {
		if len(state.Metadata.OwnerReferences) > 0 {
			childCount++
		}
	}
	fmt.Printf("\n   Total children created: %d\n", childCount)

	fmt.Println("\n✅ Test completed!")
	fmt.Println("\n📝 Expected Result:")
	fmt.Println("   - Parent should show 'completed' initially")
	fmt.Println("   - After parameter update, parent should restart")
	fmt.Println("   - Parent should show 'completed' again at the end")
	fmt.Println("   - Parameter history should show the service_time change")
}
