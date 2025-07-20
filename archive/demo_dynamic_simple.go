package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🚀 Simple Fully Dynamic Chain Demo")
	fmt.Println("==================================")
	
	// Create an empty dynamic chain builder
	builder := NewDynamicChainBuilder()
	RegisterQNetNodeTypes(builder)
	
	fmt.Println("📊 Starting with completely empty chain")
	
	// Start execution mode without any predefined nodes
	builder.ExecuteDynamic()
	time.Sleep(200 * time.Millisecond)
	
	// Add nodes in sequence to build a simple chain
	fmt.Println("\n🔧 Step 1: Adding source generator...")
	sourceNode := Node{
		Metadata: Metadata{
			Name: "source",
			Type: "dummy-wait-generator",
			Labels: map[string]string{
				"role": "source",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"rate":      0.5,
				"count":     20,
				"wait_time": 500,
			},
		},
	}
	
	err := builder.AddNode(sourceNode)
	if err != nil {
		fmt.Printf("❌ Failed to add source: %v\n", err)
		return
	}
	fmt.Println("✅ Source generator added!")
	
	time.Sleep(1 * time.Second)
	
	fmt.Println("\n🔧 Step 2: Adding processor...")
	processorNode := Node{
		Metadata: Metadata{
			Name: "processor",
			Type: "queue",
			Labels: map[string]string{
				"role": "processor",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.2,
			},
			Inputs: []map[string]string{
				{"role": "source"},
			},
		},
	}
	
	err = builder.AddNode(processorNode)
	if err != nil {
		fmt.Printf("❌ Failed to add processor: %v\n", err)
		return
	}
	fmt.Println("✅ Processor added!")
	
	time.Sleep(2 * time.Second)
	
	fmt.Println("\n🔧 Step 3: Adding sink...")
	sinkNode := Node{
		Metadata: Metadata{
			Name: "sink",
			Type: "sink",
			Labels: map[string]string{
				"role": "sink",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{},
			Inputs: []map[string]string{
				{"role": "processor"},
			},
		},
	}
	
	err = builder.AddNode(sinkNode)
	if err != nil {
		fmt.Printf("❌ Failed to add sink: %v\n", err)
		return
	}
	fmt.Println("✅ Sink added!")
	
	// Show current state
	fmt.Println("\n📊 Current Chain State:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}
	
	// Wait for processing to complete
	fmt.Println("\n⏱️  Waiting for chain to complete...")
	builder.WaitForCompletion()
	
	// Show final results
	fmt.Println("\n📈 Final Results:")
	finalStates := builder.GetAllNodeStates()
	for name, state := range finalStates {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}
	
	fmt.Printf("\n🎉 Dynamic chain completed successfully!\n")
	fmt.Printf("✨ Built entirely through dynamic additions: Source → Processor → Sink\n")
}