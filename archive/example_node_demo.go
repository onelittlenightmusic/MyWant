package main

import (
	"fmt"
	"gochain/chain"
)

func main() {
	fmt.Println("Node-Based Network Demonstration")
	fmt.Println("================================")
	
	// Create nodes directly (no configuration needed)
	gen1 := CreateGeneratorNode(1.5, 3)
	gen2 := CreateGeneratorNode(2.0, 3) 
	queue1 := CreateQueueNode(0.8)
	queue2 := CreateQueueNode(1.2)
	combiner := CreateCombinerNode()
	finalQueue := CreateQueueNode(0.5)
	sink := CreateSinkNode()
	
	fmt.Printf("Created nodes:\n")
	fmt.Printf("- Generator 1: %v\n", gen1.GetStats())
	fmt.Printf("- Generator 2: %v\n", gen2.GetStats())
	fmt.Printf("- Queue 1: %v\n", queue1.GetStats())
	fmt.Printf("- Queue 2: %v\n", queue2.GetStats())
	fmt.Printf("- Combiner: %v\n", combiner.GetStats())
	fmt.Printf("- Final Queue: %v\n", finalQueue.GetStats())
	fmt.Printf("- Sink: %v\n", sink.GetStats())
	
	// Build chains manually to demonstrate node objects
	var c1, c2 chain.C_chain
	
	// Chain 1: gen1 -> queue1
	c1.Add(NodeToChainFunc(gen1))
	c1.Add(NodeToChainFunc(queue1))
	
	// Chain 2: gen2 -> queue2  
	c2.Add(NodeToChainFunc(gen2))
	c2.Add(NodeToChainFunc(queue2))
	
	// Chain 3: combine c1 and c2, then final processing
	c1.Merge(c2, CombinerToChainFunc(combiner))
	c1.Add(NodeToChainFunc(finalQueue))
	c1.End(NodeToEndFunc(sink))
	
	fmt.Println("\nRunning simulation...")
	chain.Run()
	
	// Show final statistics
	fmt.Println("\nFinal Node Statistics:")
	fmt.Printf("- Generator 1: %v\n", gen1.GetStats())
	fmt.Printf("- Generator 2: %v\n", gen2.GetStats()) 
	fmt.Printf("- Queue 1: %v\n", queue1.GetStats())
	fmt.Printf("- Queue 2: %v\n", queue2.GetStats())
	fmt.Printf("- Combiner: %v\n", combiner.GetStats())
	fmt.Printf("- Final Queue: %v\n", finalQueue.GetStats())
	fmt.Printf("- Sink: %v\n", sink.GetStats())
}