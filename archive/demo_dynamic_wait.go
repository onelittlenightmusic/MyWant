package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🚀 Dynamic Node Addition Demo (Patient Generator)")
	fmt.Println("=================================================")

	// Create initial configuration with patient generator
	config := Config{
		Nodes: []Node{
			{
				Metadata: Metadata{
					Name: "gen-patient",
					Type: "sequence",
					Labels: map[string]string{
						"role": "source",
						"mode": "patient", // Special mode that waits
					},
				},
				Spec: NodeSpec{
					Params: map[string]interface{}{
						"rate":  1.0,
						"count": 50, // Longer run
					},
				},
			},
		},
	}

	builder := NewChainBuilder(config)
	RegisterQNetNodeTypes(builder)

	fmt.Printf("📊 Initial nodes: %d\n", len(config.Nodes))

	// Build and start execution
	err := builder.Build()
	if err != nil {
		fmt.Printf("❌ Build failed: %v\n", err)
		return
	}

	fmt.Println("▶️  Starting execution...")
	go builder.Execute()

	// Wait for execution to start
	time.Sleep(1 * time.Second)

	// Add processing node
	fmt.Println("\n🔧 Adding processor node...")
	processorNode := Node{
		Metadata: Metadata{
			Name: "proc-filter",
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
	} else {
		fmt.Println("✅ Processor added!")
	}

	time.Sleep(2 * time.Second)

	// Add sink node
	fmt.Println("\n🎯 Adding sink node...")
	sinkNode := Node{
		Metadata: Metadata{
			Name: "sink-final",
			Type: "sink",
			Labels: map[string]string{
				"role": "terminal",
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
	} else {
		fmt.Println("✅ Sink added!")
	}

	// Let the chain run and complete
	fmt.Println("\n⏱️  Running for 5 seconds...")
	time.Sleep(5 * time.Second)

	// Show final state
	fmt.Println("\n📈 Final Results:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}

	fmt.Println("\n🎉 Dynamic addition demo completed!")
	fmt.Printf("📊 Total nodes created: %d\n", len(states))
}
