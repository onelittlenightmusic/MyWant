package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🚀 Fast Dynamic QNet Demo")
	fmt.Println("========================")
	fmt.Println("Building queueing network with quick processing")
	
	// Create empty dynamic chain builder
	builder := NewDynamicChainBuilder()
	RegisterQNetNodeTypes(builder)
	
	// Start execution mode with no predefined nodes
	builder.ExecuteDynamic()
	time.Sleep(100 * time.Millisecond)
	
	// Step 1: Add primary generator (fast)
	fmt.Println("\n🔧 Step 1: Adding fast primary generator...")
	primaryGen := Node{
		Metadata: Metadata{
			Name: "gen-primary",
			Type: "dummy-wait-generator",
			Labels: map[string]string{
				"role":   "sequence",
				"stream": "primary",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"rate":      10.0,
				"count":     20,
				"wait_time": 50,
			},
		},
	}
	
	err := builder.AddNode(primaryGen)
	if err != nil {
		fmt.Printf("❌ Failed to add primary generator: %v\n", err)
		return
	}
	fmt.Println("✅ Primary generator added!")
	
	time.Sleep(200 * time.Millisecond)
	
	// Step 2: Add secondary generator (fast)
	fmt.Println("\n🔧 Step 2: Adding fast secondary generator...")
	secondaryGen := Node{
		Metadata: Metadata{
			Name: "gen-secondary",
			Type: "dummy-wait-generator",
			Labels: map[string]string{
				"role":   "sequence",
				"stream": "secondary",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"rate":      8.0,
				"count":     15,
				"wait_time": 60,
			},
		},
	}
	
	err = builder.AddNode(secondaryGen)
	if err != nil {
		fmt.Printf("❌ Failed to add secondary generator: %v\n", err)
		return
	}
	fmt.Println("✅ Secondary generator added!")
	
	time.Sleep(200 * time.Millisecond)
	
	// Step 3: Add primary queue
	fmt.Println("\n🔧 Step 3: Adding primary queue...")
	primaryQueue := Node{
		Metadata: Metadata{
			Name: "queue-primary",
			Type: "queue",
			Labels: map[string]string{
				"role":  "queue",
				"stage": "primary",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.2,
			},
			Inputs: []map[string]string{
				{"stream": "primary"},
			},
		},
	}
	
	err = builder.AddNode(primaryQueue)
	if err != nil {
		fmt.Printf("❌ Failed to add primary queue: %v\n", err)
		return
	}
	fmt.Println("✅ Primary queue added!")
	
	time.Sleep(200 * time.Millisecond)
	
	// Step 4: Add secondary queue
	fmt.Println("\n🔧 Step 4: Adding secondary queue...")
	secondaryQueue := Node{
		Metadata: Metadata{
			Name: "queue-secondary",
			Type: "queue",
			Labels: map[string]string{
				"role":  "queue",
				"stage": "secondary",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.3,
			},
			Inputs: []map[string]string{
				{"stream": "secondary"},
			},
		},
	}
	
	err = builder.AddNode(secondaryQueue)
	if err != nil {
		fmt.Printf("❌ Failed to add secondary queue: %v\n", err)
		return
	}
	fmt.Println("✅ Secondary queue added!")
	
	time.Sleep(200 * time.Millisecond)
	
	// Step 5: Add combiner
	fmt.Println("\n🔧 Step 5: Adding stream combiner...")
	combiner := Node{
		Metadata: Metadata{
			Name: "combiner-main",
			Type: "combiner",
			Labels: map[string]string{
				"role":      "combiner",
				"operation": "merge",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"operation": "merge",
			},
			Inputs: []map[string]string{
				{"stage": "primary"},
				{"stage": "secondary"},
			},
		},
	}
	
	err = builder.AddNode(combiner)
	if err != nil {
		fmt.Printf("❌ Failed to add combiner: %v\n", err)
		return
	}
	fmt.Println("✅ Stream combiner added!")
	
	time.Sleep(200 * time.Millisecond)
	
	// Step 6: Add collector sink directly (skip final queue for speed)
	fmt.Println("\n🔧 Step 6: Adding data collector...")
	collector := Node{
		Metadata: Metadata{
			Name: "collector-end",
			Type: "sink",
			Labels: map[string]string{
				"role": "sink",
				"type": "collector",
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{},
			Inputs: []map[string]string{
				{"operation": "merge"},
			},
		},
	}
	
	err = builder.AddNode(collector)
	if err != nil {
		fmt.Printf("❌ Failed to add collector: %v\n", err)
		return
	}
	fmt.Println("✅ Data collector added!")
	
	// Show current chain topology
	fmt.Println("\n📊 Fast Dynamic QNet Topology:")
	fmt.Println("   gen-primary → queue-primary ↘")
	fmt.Println("                                 combiner → collector")
	fmt.Println("   gen-secondary → queue-secondary ↗")
	
	// Wait a bit for processing
	fmt.Println("\n⏱️  Processing for 5 seconds...")
	time.Sleep(5 * time.Second)
	
	// Show intermediate state
	fmt.Println("\n📈 Intermediate Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}
	
	// Wait for processing to complete
	fmt.Println("\n🏁 Finishing up...")
	time.Sleep(3 * time.Second)
	
	// Stop and show final results
	builder.Stop()
	
	fmt.Println("\n🎯 Final Fast QNet Results:")
	finalStates := builder.GetAllNodeStates()
	totalPackets := 0
	for name, state := range finalStates {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
		if name == "collector-end" {
			totalPackets = int(state.Stats.TotalProcessed)
		}
	}
	
	fmt.Printf("\n🎉 Fast Dynamic QNet completed!\n")
	fmt.Printf("📦 Total packets processed: %d\n", totalPackets)
	fmt.Printf("🏗️  Network built entirely through %d dynamic additions\n", len(finalStates))
	fmt.Println("✨ Fast topology: Primary & Secondary → Queues → Combiner → Collector")
}