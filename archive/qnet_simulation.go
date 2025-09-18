package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

func main() {
	fmt.Println("Enhanced QNet Simulation")
	fmt.Println("=======================")

	// Create network topology: Gen -> Queue1 -> Combiner <- Queue2 <- Gen2 -> Sink
	gen1 := CreateEnhancedGeneratorNode(1.0, 5) // Generate 5 packets at rate 1.0
	gen2 := CreateEnhancedGeneratorNode(1.5, 4) // Generate 4 packets at rate 1.5
	queue1 := CreateEnhancedQueueNode(0.8)      // Service time 0.8
	queue2 := CreateEnhancedQueueNode(1.2)      // Service time 1.2
	combiner := CreateEnhancedCombinerNode(2)   // Merge 2 streams
	sink := CreateEnhancedSinkNode(1)           // Collect results

	// Display network topology
	fmt.Println("\nNetwork Topology:")
	fmt.Println("================")
	fmt.Println("Gen1(1.0,5) -> Queue1(0.8) \\")
	fmt.Println("                              -> Combiner -> Sink")
	fmt.Println("Gen2(1.5,4) -> Queue2(1.2) /")

	// Set up paths manually (in real implementation, this would be done by configuration)
	fmt.Println("\nSetting up network paths...")

	// Gen1 -> Queue1
	gen1ToQueue1 := make(chan interface{}, 10)
	gen1Paths := &Paths{
		Out: []PathInfo{{Channel: gen1ToQueue1, Name: "to_queue1", Active: true}},
	}
	queue1Paths := &Paths{
		In:  []PathInfo{{Channel: gen1ToQueue1, Name: "from_gen1", Active: true}},
		Out: []PathInfo{{Channel: make(chan interface{}, 10), Name: "to_combiner", Active: true}},
	}

	// Gen2 -> Queue2
	gen2ToQueue2 := make(chan interface{}, 10)
	gen2Paths := &Paths{
		Out: []PathInfo{{Channel: gen2ToQueue2, Name: "to_queue2", Active: true}},
	}
	queue2Paths := &Paths{
		In:  []PathInfo{{Channel: gen2ToQueue2, Name: "from_gen2", Active: true}},
		Out: []PathInfo{{Channel: make(chan interface{}, 10), Name: "to_combiner", Active: true}},
	}

	// Queue1,Queue2 -> Combiner
	combinerPaths := &Paths{
		In: []PathInfo{
			{Channel: queue1Paths.Out[0].Channel, Name: "from_queue1", Active: true},
			{Channel: queue2Paths.Out[0].Channel, Name: "from_queue2", Active: true},
		},
		Out: []PathInfo{{Channel: make(chan interface{}, 10), Name: "to_sink", Active: true}},
	}

	// Combiner -> Sink
	sinkPaths := &Paths{
		In: []PathInfo{{Channel: combinerPaths.Out[0].Channel, Name: "from_combiner", Active: true}},
	}

	fmt.Println("✓ Network paths configured")

	// Start simulation
	fmt.Println("\nStarting simulation...")
	var wg sync.WaitGroup

	// Start all nodes as goroutines
	wg.Add(6)

	// Generator 1
	go func() {
		defer wg.Done()
		fmt.Println("[GEN1] Starting...")
		for !gen1.Process(gen1Paths) {
			time.Sleep(100 * time.Millisecond)
		}
		fmt.Println("[GEN1] Finished")
	}()

	// Generator 2
	go func() {
		defer wg.Done()
		fmt.Println("[GEN2] Starting...")
		for !gen2.Process(gen2Paths) {
			time.Sleep(100 * time.Millisecond)
		}
		fmt.Println("[GEN2] Finished")
	}()

	// Queue 1
	go func() {
		defer wg.Done()
		fmt.Println("[QUEUE1] Starting...")
		for !queue1.Process(queue1Paths) {
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Println("[QUEUE1] Finished")
	}()

	// Queue 2
	go func() {
		defer wg.Done()
		fmt.Println("[QUEUE2] Starting...")
		for !queue2.Process(queue2Paths) {
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Println("[QUEUE2] Finished")
	}()

	// Combiner
	go func() {
		defer wg.Done()
		fmt.Println("[COMBINER] Starting...")
		for !combiner.Process(combinerPaths) {
			time.Sleep(30 * time.Millisecond)
		}
		fmt.Println("[COMBINER] Finished")
	}()

	// Sink
	go func() {
		defer wg.Done()
		fmt.Println("[SINK] Starting...")
		for !sink.Process(sinkPaths) {
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Println("[SINK] Finished")
	}()

	// Wait for simulation to complete
	wg.Wait()

	// Display final statistics
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("SIMULATION COMPLETE - Final Statistics")
	fmt.Println(strings.Repeat("=", 50))

	nodes := []struct {
		name string
		node EnhancedBaseNode
	}{
		{"Generator1", gen1},
		{"Generator2", gen2},
		{"Queue1", queue1},
		{"Queue2", queue2},
		{"Combiner", combiner},
		{"Sink", sink},
	}

	for _, n := range nodes {
		stats := n.node.GetStats()
		meta := n.node.GetConnectivityMetadata()
		fmt.Printf("\n%s (%s):\n", n.name, meta.NodeType)
		for key, value := range stats {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	fmt.Println("\nNetwork Performance Summary:")
	fmt.Println("===========================")
	gen1Stats := gen1.GetStats()
	gen2Stats := gen2.GetStats()
	sinkStats := sink.GetStats()

	totalGenerated := gen1Stats["generated"].(int) + gen2Stats["generated"].(int)
	totalReceived := sinkStats["received"].(int)

	fmt.Printf("Total Packets Generated: %d\n", totalGenerated)
	fmt.Printf("Total Packets Received:  %d\n", totalReceived)
	fmt.Printf("Packet Loss Rate:        %.2f%%\n",
		float64(totalGenerated-totalReceived)/float64(totalGenerated)*100)

	if combinerStats := combiner.GetStats(); combinerStats["merged_packets"].(int) > 0 {
		fmt.Printf("Combiner Efficiency:     %.2f%% (merged %d packets)\n",
			float64(combinerStats["merged_packets"].(int))/float64(totalGenerated)*100,
			combinerStats["merged_packets"].(int))
	}
}
