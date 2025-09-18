package main

import (
	"fmt"
	"mywant/src/chain"
	"strings"
	"time"
)

func main() {
	fmt.Println("Runtime Comparison: Stats vs Non-Stats")
	fmt.Println(strings.Repeat("=", 40))

	// Test 1: With stats functionality (1000 packets)
	fmt.Println("\n1. Testing with Stats Functionality")
	fmt.Println(strings.Repeat("-", 30))

	start := time.Now()

	generator := PacketSequence(map[string]interface{}{
		"rate":  2.0,
		"count": 1000,
	})

	queue := NewQueue(map[string]interface{}{
		"service_time": 0.5,
	})

	sink := Goal(map[string]interface{}{})

	ch := chain.C_chain{}
	ch.Add(generator.CreateFunction())
	ch.Add(queue.CreateFunction())
	ch.End(sink.CreateFunction())

	chain.Run()

	elapsed := time.Since(start)

	fmt.Printf("Runtime: %v\n", elapsed)
	fmt.Printf("Generator processed: %d packets\n", generator.Stats.TotalProcessed)
	fmt.Printf("Queue avg wait time: %.3f\n", queue.Stats.AverageWaitTime)
	fmt.Printf("Sink received: %d packets\n", sink.Stats.TotalProcessed)

	// Performance metrics
	packetsPerSecond := float64(generator.Stats.TotalProcessed) / elapsed.Seconds()
	fmt.Printf("Throughput: %.0f packets/second\n", packetsPerSecond)

	fmt.Println("\n" + strings.Repeat("=", 40))
	fmt.Println("STATS FUNCTIONALITY PERFORMANCE SUMMARY")
	fmt.Println(strings.Repeat("=", 40))
	fmt.Printf("Packets processed: %d\n", generator.Stats.TotalProcessed)
	fmt.Printf("Total runtime: %v\n", elapsed)
	fmt.Printf("Average queue wait time: %.3f\n", queue.Stats.AverageWaitTime)
	fmt.Printf("Total queue wait time: %.3f\n", queue.Stats.TotalWaitTime)
	fmt.Printf("Processing throughput: %.0f packets/second\n", packetsPerSecond)
}
