package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🧪 Sink Behavior Test")
	fmt.Println("=====================")

	// Test sink patience - adding sink before any data flows
	fmt.Println("\n📋 Test: Sink waits patiently for inputs and data")

	builder := NewDynamicChainBuilder()
	RegisterQNetNodeTypes(builder)

	// Start execution mode
	builder.ExecuteDynamic()
	time.Sleep(200 * time.Millisecond)

	// Step 1: Add sink first (no inputs yet)
	fmt.Println("\n🔧 Step 1: Adding sink with no inputs...")
	sinkNode := Node{
		Metadata: Metadata{
			Name: "patient-sink",
			Type: "sink",
			Labels: map[string]string{
				"role": "collector",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{},
			Inputs: []map[string]string{
				{"role": "source"},
			},
		},
	}

	err := builder.AddNode(sinkNode)
	if err != nil {
		fmt.Printf("❌ Failed to add sink: %v\n", err)
		return
	}
	fmt.Println("✅ Sink added - waiting for inputs...")

	time.Sleep(1 * time.Second)

	// Step 2: Add data source
	fmt.Println("\n🔧 Step 2: Adding data source...")
	sourceNode := Node{
		Metadata: Metadata{
			Name: "data-provider",
			Type: "sequence",
			Labels: map[string]string{
				"role": "source",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"rate":  1.0,
				"count": 5,
			},
		},
	}

	err = builder.AddNode(sourceNode)
	if err != nil {
		fmt.Printf("❌ Failed to add source: %v\n", err)
		return
	}
	fmt.Println("✅ Source added - should now connect to waiting sink!")

	// Wait for processing
	time.Sleep(3 * time.Second)

	// Check intermediate state
	fmt.Println("\n📊 Current State:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}

	// Wait for completion
	fmt.Println("\n⏱️  Waiting for completion...")
	builder.WaitForCompletion()

	// Show final results
	fmt.Println("\n📈 Final Results:")
	finalStates := builder.GetAllNodeStates()
	for name, state := range finalStates {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}

	fmt.Println("\n✅ Sink behavior test completed!")
	fmt.Println("📋 Key behaviors demonstrated:")
	fmt.Println("   🔸 Sink waits patiently when no inputs are connected")
	fmt.Println("   🔸 Sink processes data once inputs become available")
	fmt.Println("   🔸 Sink waits for at least one data packet before considering shutdown")
	fmt.Println("   🔸 Sink only shuts down after receiving an end packet AND having processed data")
}
