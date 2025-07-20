package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🚀 Dynamic Prime Sieve Demo")
	fmt.Println("===========================")
	fmt.Println("Building prime number sieve entirely through dynamic additions")
	
	// Create empty dynamic chain builder
	builder := NewDynamicChainBuilder()
	RegisterPrimeNodeTypes(builder)
	
	// Start execution mode with no predefined nodes
	builder.ExecuteDynamic()
	time.Sleep(200 * time.Millisecond)
	
	// Step 1: Add number generator
	fmt.Println("\n🔧 Step 1: Adding number generator...")
	generator := Node{
		Metadata: Metadata{
			Name: "number-generator",
			Type: "prime_generator",
			Labels: map[string]string{
				"role": "source",
				"type": "numbers",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"start": 2,
				"end":   10,
			},
		},
	}
	
	err := builder.AddNode(generator)
	if err != nil {
		fmt.Printf("❌ Failed to add generator: %v\n", err)
		return
	}
	fmt.Println("✅ Number generator added!")
	
	time.Sleep(500 * time.Millisecond)
	
	// Step 2: Add prime filter
	fmt.Println("\n🔧 Step 2: Adding prime filter...")
	filter := Node{
		Metadata: Metadata{
			Name: "prime-filter",
			Type: "prime_filter",
			Labels: map[string]string{
				"role": "processor",
				"type": "filter",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{},
			Inputs: []map[string]string{
				{"type": "numbers"},
			},
		},
	}
	
	err = builder.AddNode(filter)
	if err != nil {
		fmt.Printf("❌ Failed to add filter: %v\n", err)
		return
	}
	fmt.Println("✅ Prime filter added!")
	
	time.Sleep(500 * time.Millisecond)
	
	// Step 3: Add prime collector sink
	fmt.Println("\n🔧 Step 3: Adding prime collector...")
	collector := Node{
		Metadata: Metadata{
			Name: "prime-collector",
			Type: "prime_sink",
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
	fmt.Println("✅ Prime collector added!")
	
	// Show current chain topology
	fmt.Println("\n📊 Dynamic Prime Sieve Topology:")
	fmt.Println("   number-generator → prime-filter → prime-collector")
	
	// Show current state
	fmt.Println("\n📈 Current Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}
	
	// Wait for processing to complete (chain waits for sink)
	fmt.Println("\n⏱️  Waiting for prime sieve to complete...")
	
	// Add timeout mechanism to force completion for demonstration
	done := make(chan bool, 1)
	go func() {
		builder.WaitForCompletion()
		done <- true
	}()
	
	go func() {
		time.Sleep(2 * time.Second)
		fmt.Println("\n⏰ Forcing completion after 2 seconds for demonstration...")
		builder.Stop()
		done <- true
	}()
	
	<-done
	
	// Show final results
	fmt.Println("\n🎯 Final Prime Sieve Results:")
	finalStates := builder.GetAllNodeStates()
	totalNumbers := 0
	totalFiltered := 0
	totalPrimes := 0
	
	for name, state := range finalStates {
		processed := int(state.Stats.TotalProcessed)
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), processed)
		
		switch name {
		case "number-generator":
			totalNumbers = processed
		case "prime-filter":
			totalFiltered = processed
		case "prime-collector":
			totalPrimes = processed
		}
	}
	
	fmt.Printf("\n📈 Prime Processing Summary:\n")
	fmt.Printf("  🔢 Numbers generated: %d\n", totalNumbers)
	fmt.Printf("  🔍 Numbers filtered: %d\n", totalFiltered)
	fmt.Printf("  🎯 Primes found: %d\n", totalPrimes)
	
	fmt.Printf("\n🎉 Dynamic Prime Sieve completed!\n")
	fmt.Printf("🏗️  Sieve built entirely through %d dynamic additions\n", len(finalStates))
	fmt.Println("✨ Pipeline: Generator → Filter → Collector")
	fmt.Printf("🧮 Prime efficiency: %.1f%% (found %d primes out of %d numbers)\n", 
		float64(totalPrimes)/float64(totalNumbers)*100, totalPrimes, totalNumbers)
	
	// Dump node memory to YAML file
	fmt.Println("\n📝 Dumping dynamic Prime Sieve node memory to YAML...")
	err = builder.dumpNodeMemoryToYAML()
	if err != nil {
		fmt.Printf("❌ Failed to dump node memory: %v\n", err)
	} else {
		fmt.Println("✅ Dynamic Prime Sieve node memory dumped successfully!")
	}
}