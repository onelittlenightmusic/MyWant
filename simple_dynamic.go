package main

import (
	"fmt"
	"time"
	"context"
)

// Helper function to create a generator node
func createGeneratorNode(name string, count int, rate float64) *DynamicNode {
	processFunc := func(inputs map[string]chan *Packet, outputs map[string]*PersistentChannel) error {
		fmt.Printf("[GEN %s] Starting to generate %d packets\n", name, count)
		
		output := outputs["out"]
		packetID := uint64(0)
		
		for i := 0; i < count; i++ {
			// Simulate processing time based on rate
			if rate > 0 {
				delay := time.Duration(1.0/rate*1000) * time.Millisecond
				time.Sleep(delay)
			}
			
			// Create and send packet
			packet := NewPacket(fmt.Sprintf("data-%s-%d", name, i), packetID)
			packetID++
			
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := output.Send(packet, ctx)
			cancel()
			
			if err != nil {
				fmt.Printf("[GEN %s] Failed to send packet %d: %v\n", name, i, err)
				return err
			}
		}
		
		// Send end-of-stream packet
		eosPacket := NewEOSPacket(packetID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := output.Send(eosPacket, ctx)
		cancel()
		
		if err != nil {
			fmt.Printf("[GEN %s] Failed to send EOS packet: %v\n", name, err)
			return err
		}
		
		fmt.Printf("[GEN %s] Generated %d packets + EOS\n", name, count)
		return nil
	}
	
	node := NewDynamicNode(name, "generator", processFunc)
	node.AddOutputChannel("out", 50) // Buffer capacity of 50
	return node
}

// Helper function to create a sink node
func createSinkNode(name string) *DynamicNode {
	processFunc := func(inputs map[string]chan *Packet, outputs map[string]*PersistentChannel) error {
		fmt.Printf("[SINK %s] Starting sink\n", name)
		
		input := inputs["in"]
		received := 0
		
		for {
			select {
			case packet, ok := <-input:
				if !ok {
					fmt.Printf("[SINK %s] Input channel closed\n", name)
					return nil
				}
				
				if packet.IsEOS {
					fmt.Printf("[SINK %s] Received EOS packet - FINALIZED\n", name)
					fmt.Printf("[SINK %s] Total received: %d packets\n", name, received)
					return nil
				}
				
				received++
				if received%100 == 0 {
					fmt.Printf("[SINK %s] Received %d packets\n", name, received)
				}
			}
		}
	}
	
	node := NewDynamicNode(name, "sink", processFunc)
	return node
}

func testSimpleDynamicChain() {
	fmt.Println("=== Simple Dynamic Chain Test ===")
	
	// Create manager
	manager := NewDynamicChainManager()
	
	// Create simple generator (only 5 packets)
	gen := createGeneratorNode("test-gen", 5, 2.0) // 5 packets at 2 Hz
	sink := createSinkNode("test-sink")
	
	// Add nodes
	manager.AddNode(gen, false)
	manager.AddNode(sink, true)
	
	// Connect
	manager.ConnectNodes("test-gen", "out", "test-sink", "in", 10)
	
	// Start
	fmt.Println("Starting simple chain...")
	manager.Start()
	
	// Wait for completion with timeout
	fmt.Println("Waiting for completion...")
	select {
	case completedSink := <-manager.completion:
		fmt.Printf("✅ Sink '%s' completed!\n", completedSink)
	case <-time.After(30 * time.Second):
		fmt.Println("❌ Test timed out")
	}
	
	// Show final states
	fmt.Println("Final states:")
	for name, state := range manager.GetNodeStates() {
		fmt.Printf("  %s: %s\n", name, state)
	}
	
	manager.Stop()
}

func main() {
	testSimpleDynamicChain()
}