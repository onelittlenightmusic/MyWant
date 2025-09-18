package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🚀 Dynamic Fibonacci Demo")
	fmt.Println("=========================")
	fmt.Println("Building fibonacci sequence chain entirely through dynamic additions")

	// Create empty dynamic chain builder
	builder := NewDynamicChainBuilder()
	RegisterFibonacciNodeTypes(builder)

	// Start execution mode with no predefined nodes
	builder.ExecuteDynamic()
	time.Sleep(200 * time.Millisecond)

	// Step 1: Add fibonacci generator
	fmt.Println("\n🔧 Step 1: Adding fibonacci generator...")
	generator := Node{
		Metadata: Metadata{
			Name: "fib-generator",
			Type: "fibonacci_generator",
			Labels: map[string]string{
				"role": "source",
				"type": "fibonacci",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"count": 5,
			},
		},
	}

	err := builder.AddNode(generator)
	if err != nil {
		fmt.Printf("❌ Failed to add generator: %v\n", err)
		return
	}
	fmt.Println("✅ Fibonacci generator added!")

	time.Sleep(500 * time.Millisecond)

	// Step 2: Add fibonacci filter
	fmt.Println("\n🔧 Step 2: Adding fibonacci filter...")
	filter := Node{
		Metadata: Metadata{
			Name: "fib-filter",
			Type: "fibonacci_filter",
			Labels: map[string]string{
				"role": "processor",
				"type": "filter",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"threshold": 100,
			},
			Inputs: []map[string]string{
				{"type": "fibonacci"},
			},
		},
	}

	err = builder.AddNode(filter)
	if err != nil {
		fmt.Printf("❌ Failed to add filter: %v\n", err)
		return
	}
	fmt.Println("✅ Fibonacci filter added!")

	time.Sleep(500 * time.Millisecond)

	// Step 3: Add fibonacci collector sink
	fmt.Println("\n🔧 Step 3: Adding fibonacci collector...")
	collector := Node{
		Metadata: Metadata{
			Name: "fib-collector",
			Type: "fibonacci_sink",
			Labels: map[string]string{
				"role": "sink",
				"type": "collector",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{},
			Inputs: []map[string]string{
				{"type": "filter"},
			},
		},
	}

	err = builder.AddNode(collector)
	if err != nil {
		fmt.Printf("❌ Failed to add collector: %v\n", err)
		return
	}
	fmt.Println("✅ Fibonacci collector added!")

	// Show current chain topology
	fmt.Println("\n📊 Dynamic Fibonacci Topology:")
	fmt.Println("   fib-generator → fib-filter → fib-collector")

	// Show current state
	fmt.Println("\n📈 Current Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}

	// Wait for processing to complete (chain waits for sink)
	fmt.Println("\n⏱️  Waiting for fibonacci sequence to complete...")
	builder.WaitForCompletion()

	// Show final results
	fmt.Println("\n🎯 Final Fibonacci Results:")
	finalStates := builder.GetAllNodeStates()
	totalGenerated := 0
	totalFiltered := 0
	totalCollected := 0

	for name, state := range finalStates {
		processed := int(state.Stats.TotalProcessed)
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), processed)

		switch name {
		case "fib-generator":
			totalGenerated = processed
		case "fib-filter":
			totalFiltered = processed
		case "fib-collector":
			totalCollected = processed
		}
	}

	fmt.Printf("\n📈 Fibonacci Processing Summary:\n")
	fmt.Printf("  🔢 Numbers generated: %d\n", totalGenerated)
	fmt.Printf("  🔍 Numbers filtered: %d\n", totalFiltered)
	fmt.Printf("  🎯 Numbers collected: %d\n", totalCollected)

	fmt.Printf("\n🎉 Dynamic Fibonacci completed!\n")
	fmt.Printf("🏗️  Chain built entirely through %d dynamic additions\n", len(finalStates))
	fmt.Println("✨ Pipeline: Generator → Filter → Collector")
	fmt.Printf("📊 Processing efficiency: %.1f%% (collected %d out of %d generated)\n",
		float64(totalCollected)/float64(totalGenerated)*100, totalCollected, totalGenerated)

	// Dump node memory to YAML file
	fmt.Println("\n📝 Dumping dynamic Fibonacci node memory to YAML...")
	err = builder.dumpNodeMemoryToYAML()
	if err != nil {
		fmt.Printf("❌ Failed to dump node memory: %v\n", err)
	} else {
		fmt.Println("✅ Dynamic Fibonacci node memory dumped successfully!")
	}
}
