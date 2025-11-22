package main

import (
	"fmt"
	"mywant/engine/cmd/types"
	mywant "mywant/engine/src"
	"time"
)

func main() {
	fmt.Println("🧪 Testing Parent-Child Lifecycle with Parameter Updates")
	fmt.Println("=========================================================")
	fmt.Println()

	// Load config with target want
	config, err := mywant.LoadConfigFromYAML("config/config-qnet-owner.yaml")
	if err != nil {
		fmt.Printf("❌ Error loading config: %v\n", err)
		return
	}

	// Create chain builder
	builder := mywant.NewChainBuilder(config)

	// Register want types
	types.RegisterQNetWantTypes(builder)
	mywant.RegisterOwnerWantTypes(builder)

	fmt.Println("📋 Test Plan:")
	fmt.Println("1. Start target want with children")
	fmt.Println("2. Wait for all children to complete (parent should become completed)")
	fmt.Println("3. Update parameter on parent (should push to children)")
	fmt.Println("4. Children should restart and parent should restart too")
	fmt.Println("5. Wait for children to complete again")
	fmt.Println("6. Parent should become completed again")
	fmt.Println()

	// Start execution in background
	go builder.Execute()

	// Wait for initial execution to complete
	fmt.Println("⏳ Waiting 5 seconds for initial completion...")
	time.Sleep(5 * time.Second)

	// Check parent status
	states := builder.GetAllWantStates()
	for name, state := range states {
		if state.Metadata.Type == "target" {
			fmt.Printf("📊 Parent '%s' status after first round: %s\n", name, state.Status)

			// Update a parameter on the target
			fmt.Printf("\n🔄 Updating parameter on parent '%s'...\n", name)
			if runtimeWant, exists := builder.GetWant(name); exists {
				if target, ok := runtimeWant.(*mywant.Target); ok {
					target.UpdateParameter("service_time", 0.05)
					fmt.Println("✅ Parameter updated and pushed to children")
				}
			}
		}
	}

	// Wait for parameter update to propagate and children to restart
	fmt.Println("\n⏳ Waiting 8 seconds for parameter update and second round...")
	time.Sleep(8 * time.Second)

	// Check final states
	fmt.Println("\n📊 Final States:")
	states = builder.GetAllWantStates()
	for name, state := range states {
		status := state.Status
		if state.Metadata.Type == "target" {
			fmt.Printf("🎯 Parent '%s': %s\n", name, status)
			if result, ok := state.State["result"]; ok {
				fmt.Printf("   Result: %v\n", result)
			}
		} else if len(state.Metadata.OwnerReferences) > 0 {
			processed := 0
			if state.State != nil {
				if val, ok := state.State["total_processed"]; ok {
					if intVal, ok := val.(int); ok {
						processed = intVal
					}
				}
			}
			fmt.Printf("   Child '%s': %s (processed: %d)\n", name, status, processed)
		}
	}

	fmt.Println("\n✅ Test completed!")
	fmt.Println("\n📝 Expected behavior:")
	fmt.Println("   - Parent should be 'completed' after first round")
	fmt.Println("   - After parameter update, parent should restart (idle → running)")
	fmt.Println("   - Parent should be 'completed' again after second round")
}
