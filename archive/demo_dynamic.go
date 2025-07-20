package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("Dynamic Node Addition Demo")
	fmt.Println("==========================")
	
	// Create initial configuration with just a generator
	config := Config{
		Nodes: []Node{
			{
				Metadata: Metadata{
					Name: "gen-primary",
					Type: "generator",
					Labels: map[string]string{
						"role": "source",
						"stream": "primary",
					},
				},
				Spec: NodeSpec{
					Params: map[string]interface{}{
						"rate":  2.0,
						"count": 20,
					},
				},
			},
		},
	}
	
	fmt.Printf("Started with %d nodes\n", len(config.Nodes))
	
	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register qnet node types
	RegisterQNetNodeTypes(builder)
	
	fmt.Println("\nBuilding initial chain...")
	err := builder.Build()
	if err != nil {
		fmt.Printf("❌ Build failed: %v\n", err)
		return
	}
	
	// Start execution in a separate goroutine
	fmt.Println("✅ Starting execution...")
	go builder.Execute()
	
	// Wait a moment for execution to start
	time.Sleep(500 * time.Millisecond)
	
	fmt.Println("\n🔄 Adding queue node dynamically...")
	// Add a queue node dynamically
	queueNode := Node{
		Metadata: Metadata{
			Name: "queue-dynamic",
			Type: "queue",
			Labels: map[string]string{
				"role": "processor",
				"stage": "dynamic",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.5,
			},
			Inputs: []map[string]string{
				{"role": "source", "stream": "primary"},
			},
		},
	}
	
	err = builder.AddNode(queueNode)
	if err != nil {
		fmt.Printf("❌ Failed to add queue node: %v\n", err)
	} else {
		fmt.Println("✅ Queue node added successfully!")
	}
	
	// Wait another moment
	time.Sleep(1 * time.Second)
	
	fmt.Println("\n🔄 Adding sink node dynamically...")
	// Add a sink node
	sinkNode := Node{
		Metadata: Metadata{
			Name: "sink-dynamic", 
			Type: "sink",
			Labels: map[string]string{
				"role": "terminal",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{},
			Inputs: []map[string]string{
				{"role": "processor", "stage": "dynamic"},
			},
		},
	}
	
	err = builder.AddNode(sinkNode)
	if err != nil {
		fmt.Printf("❌ Failed to add sink node: %v\n", err)
	} else {
		fmt.Println("✅ Sink node added successfully!")
	}
	
	// Wait for execution to complete
	fmt.Println("\n⏳ Waiting for execution to complete...")
	time.Sleep(5 * time.Second)
	
	fmt.Println("\n📊 Final Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n",
			name, state.Status, state.Stats.TotalProcessed)
	}
	
	fmt.Println("\n✅ Dynamic node addition demo completed!")
}