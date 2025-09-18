package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("Performance Test: New Implementation")
	fmt.Println("===================================")

	start := time.Now()

	// Create simple chain: gen -> queue1 -> queue2 -> sink
	gen := CreateEnhancedGeneratorNode(3.0, 1000)
	queue1 := CreateEnhancedQueueNode(0.5)
	queue2 := CreateEnhancedQueueNode(0.7)
	sink := CreateEnhancedSinkNode(1)

	// Set up simple linear chain
	gen1ToQueue1 := make(chan interface{}, 10)
	queue1ToQueue2 := make(chan interface{}, 10)
	queue2ToSink := make(chan interface{}, 10)

	genPaths := &Paths{
		Out: []PathInfo{{Channel: gen1ToQueue1, Name: "to_queue1", Active: true}},
	}
	queue1Paths := &Paths{
		In:  []PathInfo{{Channel: gen1ToQueue1, Name: "from_gen", Active: true}},
		Out: []PathInfo{{Channel: queue1ToQueue2, Name: "to_queue2", Active: true}},
	}
	queue2Paths := &Paths{
		In:  []PathInfo{{Channel: queue1ToQueue2, Name: "from_queue1", Active: true}},
		Out: []PathInfo{{Channel: queue2ToSink, Name: "to_sink", Active: true}},
	}
	sinkPaths := &Paths{
		In: []PathInfo{{Channel: queue2ToSink, Name: "from_queue2", Active: true}},
	}

	// Pre-compute active paths for performance
	genPaths.PrecomputeActivePaths()
	queue1Paths.PrecomputeActivePaths()
	queue2Paths.PrecomputeActivePaths()
	sinkPaths.PrecomputeActivePaths()

	fmt.Println("Starting new implementation...")

	// Run pipeline
	go func() {
		for !gen.Process(genPaths) {
		}
	}()
	go func() {
		for !queue1.Process(queue1Paths) {
		}
	}()
	go func() {
		for !queue2.Process(queue2Paths) {
		}
	}()
	go func() {
		for !sink.Process(sinkPaths) {
		}
	}()

	// Wait for completion by checking sink
	for sink.GetStats()["received"].(int) < 1000 {
		time.Sleep(1 * time.Millisecond)
	}

	elapsed := time.Since(start)
	fmt.Printf("New implementation completed in: %v\n", elapsed)

	// Show final stats
	fmt.Printf("Generator: %v\n", gen.GetStats())
	fmt.Printf("Queue1: %v\n", queue1.GetStats())
	fmt.Printf("Queue2: %v\n", queue2.GetStats())
	fmt.Printf("Sink: %v\n", sink.GetStats())
}
