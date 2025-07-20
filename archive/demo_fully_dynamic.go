package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🚀 Fully Dynamic Chain Configuration Demo")
	fmt.Println("=========================================")
	
	// Create an empty dynamic chain builder
	builder := NewDynamicChainBuilder()
	RegisterQNetNodeTypes(builder)
	
	fmt.Println("📊 Starting with completely empty chain")
	
	// Start execution mode without any predefined nodes
	builder.ExecuteDynamic()
	
	// Wait a moment to ensure execution mode is active
	time.Sleep(200 * time.Millisecond)
	
	// Step 1: Add a patient generator node (no dependencies)
	fmt.Println("\n🔧 Step 1: Adding patient generator node...")
	generatorNode := Node{
		Metadata: Metadata{
			Name: "data-source",
			Type: "dummy-wait-generator",
			Labels: map[string]string{
				"role": "primary-source",
				"type": "data",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"rate":      1.0,
				"count":     25,
				"wait_time": 200,
			},
		},
	}
	
	err := builder.AddNode(generatorNode)
	if err != nil {
		fmt.Printf("❌ Failed to add generator: %v\n", err)
		return
	}
	fmt.Println("✅ Generator added successfully!")
	
	// Wait for generator to start producing
	time.Sleep(500 * time.Millisecond)
	
	// Step 2: Add a processing queue
	fmt.Println("\n🔧 Step 2: Adding processing queue...")
	queueNode := Node{
		Metadata: Metadata{
			Name: "processor-queue",
			Type: "queue",
			Labels: map[string]string{
				"role": "processor",
				"stage": "primary",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.3,
			},
			Inputs: []map[string]string{
				{"role": "primary-source"},
			},
		},
	}
	
	err = builder.AddNode(queueNode)
	if err != nil {
		fmt.Printf("❌ Failed to add queue: %v\n", err)
		return
	}
	fmt.Println("✅ Processing queue added successfully!")
	
	// Wait for processing to occur
	time.Sleep(1 * time.Second)
	
	// Step 3: Add another patient generator for secondary stream
	fmt.Println("\n🔧 Step 3: Adding secondary patient generator...")
	secondGenerator := Node{
		Metadata: Metadata{
			Name: "data-source-2",
			Type: "dummy-wait-generator",
			Labels: map[string]string{
				"role": "secondary-source",
				"type": "data",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"rate":      1.0,
				"count":     15,
				"wait_time": 250,
			},
		},
	}
	
	err = builder.AddNode(secondGenerator)
	if err != nil {
		fmt.Printf("❌ Failed to add secondary generator: %v\n", err)
		return
	}
	fmt.Println("✅ Secondary generator added successfully!")
	
	// Step 4: Add a combiner to merge both streams
	fmt.Println("\n🔧 Step 4: Adding stream combiner...")
	combinerNode := Node{
		Metadata: Metadata{
			Name: "stream-combiner",
			Type: "combiner",
			Labels: map[string]string{
				"role": "aggregator",
				"operation": "merge",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"operation": "merge",
			},
			Inputs: []map[string]string{
				{"role": "processor"},
				{"role": "secondary-source"},
			},
		},
	}
	
	err = builder.AddNode(combinerNode)
	if err != nil {
		fmt.Printf("❌ Failed to add combiner: %v\n", err)
		return
	}
	fmt.Println("✅ Stream combiner added successfully!")
	
	// Step 5: Add final sink
	fmt.Println("\n🔧 Step 5: Adding final sink...")
	sinkNode := Node{
		Metadata: Metadata{
			Name: "data-collector",
			Type: "sink",
			Labels: map[string]string{
				"role": "terminal",
				"type": "collector",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{},
			Inputs: []map[string]string{
				{"role": "aggregator"},
			},
		},
	}
	
	err = builder.AddNode(sinkNode)
	if err != nil {
		fmt.Printf("❌ Failed to add sink: %v\n", err)
		return
	}
	fmt.Println("✅ Final sink added successfully!")
	
	// Step 6: Let the dynamically built chain run
	fmt.Println("\n⏱️  Letting the dynamically built chain complete...")
	time.Sleep(3 * time.Second)
	
	// Step 7: Show intermediate state
	fmt.Println("\n📊 Intermediate Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}
	
	// Step 8: Wait for full completion
	fmt.Println("\n🏁 Waiting for chain completion...")
	builder.WaitForCompletion()
	
	// Step 9: Show final state
	fmt.Println("\n📈 Final Node States:")
	finalStates := builder.GetAllNodeStates()
	for name, state := range finalStates {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}
	
	fmt.Printf("\n🎉 Fully dynamic chain configuration completed!\n")
	fmt.Printf("📊 Total nodes dynamically added: %d\n", len(finalStates))
	fmt.Println("✨ Chain built entirely through dynamic node additions!")
}