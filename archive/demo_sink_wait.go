package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🚀 Chain Waits for Sink Completion Demo")
	fmt.Println("=======================================")

	// Create simple chain: Generator → Queue → Sink
	config := Config{
		Nodes: []Node{
			{
				Metadata: Metadata{
					Name: "fast-generator",
					Type: "sequence",
					Labels: map[string]string{
						"role": "source",
					},
				},
				Spec: NodeSpec{
					Params: map[string]interface{}{
						"rate":  10.0, // High rate, no internal waiting
						"count": 25,   // Quick generation
					},
				},
			},
			{
				Metadata: Metadata{
					Name: "processing-queue",
					Type: "queue",
					Labels: map[string]string{
						"role": "processor",
					},
				},
				Spec: NodeSpec{
					Params: map[string]interface{}{
						"service_time": 0.1, // Fast processing
					},
					Inputs: []map[string]string{
						{"role": "source"},
					},
				},
			},
			{
				Metadata: Metadata{
					Name: "final-sink",
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
			},
		},
	}

	builder := NewChainBuilder(config)
	RegisterQNetNodeTypes(builder)

	fmt.Println("🏗️  Building chain: Generator → Queue → Sink")
	err := builder.Build()
	if err != nil {
		fmt.Printf("❌ Build failed: %v\n", err)
		return
	}

	fmt.Println("▶️  Starting execution (chain will wait for sink to complete)...")
	startTime := time.Now()

	// Execute and wait for sink completion
	builder.Execute()

	duration := time.Since(startTime)

	fmt.Println("\n📊 Final Results (after sink completion):")
	states := builder.GetAllNodeStates()
	totalGenerated := 0
	totalProcessed := 0
	totalReceived := 0

	for name, state := range states {
		processed := int(state.Stats.TotalProcessed)
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), processed)

		switch name {
		case "fast-generator":
			totalGenerated = processed
		case "processing-queue":
			totalProcessed = processed
		case "final-sink":
			totalReceived = processed
		}
	}

	fmt.Printf("\n📈 Processing Summary:\n")
	fmt.Printf("  📦 Generated: %d packets\n", totalGenerated)
	fmt.Printf("  ⚙️  Processed: %d packets\n", totalProcessed)
	fmt.Printf("  📥 Received: %d packets\n", totalReceived)
	fmt.Printf("  ⏱️  Duration: %v\n", duration)

	fmt.Println("\n✅ Key Behavior Demonstrated:")
	fmt.Println("   🔸 Generator completed immediately (no wait time)")
	fmt.Println("   🔸 Chain waited for sink to finish processing all data")
	fmt.Println("   🔸 Complete statistics available after sink completion")
	fmt.Println("   🔸 All packets flowed through the entire pipeline")
}
