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
	
	node := NewDynamicNode(name, "sequence", processFunc)
	node.AddOutputChannel("out", 50) // Buffer capacity of 50
	return node
}

// Helper function to create a processor node
func createProcessorNode(name string, serviceTime float64) *DynamicNode {
	processFunc := func(inputs map[string]chan *Packet, outputs map[string]*PersistentChannel) error {
		fmt.Printf("[PROC %s] Starting processor with service time %.2f\n", name, serviceTime)
		
		input := inputs["in"]
		output := outputs["out"]
		processed := 0
		
		for {
			select {
			case packet, ok := <-input:
				if !ok {
					fmt.Printf("[PROC %s] Input channel closed\n", name)
					return nil
				}
				
				if packet.IsEOS {
					fmt.Printf("[PROC %s] Received EOS, forwarding and terminating\n", name)
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					err := output.Send(packet, ctx)
					cancel()
					if err != nil {
						return err
					}
					fmt.Printf("[PROC %s] Processed %d packets\n", name, processed)
					return nil
				}
				
				// Simulate processing time
				if serviceTime > 0 {
					delay := time.Duration(serviceTime*1000) * time.Millisecond
					time.Sleep(delay)
				}
				
				// Process and forward packet
				processedData := fmt.Sprintf("processed-%s-%v", name, packet.Data)
				newPacket := NewPacket(processedData, packet.ID)
				
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := output.Send(newPacket, ctx)
				cancel()
				
				if err != nil {
					fmt.Printf("[PROC %s] Failed to send processed packet: %v\n", name, err)
					return err
				}
				
				processed++
				if processed%100 == 0 {
					fmt.Printf("[PROC %s] Processed %d packets\n", name, processed)
				}
			}
		}
	}
	
	node := NewDynamicNode(name, "processor", processFunc)
	node.AddOutputChannel("out", 50)
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

// Helper function to create a combiner node that merges two inputs
func createCombinerNode(name string) *DynamicNode {
	processFunc := func(inputs map[string]chan *Packet, outputs map[string]*PersistentChannel) error {
		fmt.Printf("[COMBINER %s] Starting combiner\n", name)
		
		input1 := inputs["in1"]
		input2 := inputs["in2"]
		output := outputs["out"]
		
		processed := 0
		eosCount := 0
		
		for {
			select {
			case packet, ok := <-input1:
				if !ok {
					input1 = nil
					continue
				}
				
				if packet.IsEOS {
					eosCount++
					if eosCount >= 2 {
						// Forward EOS when both inputs are done
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						err := output.Send(packet, ctx)
						cancel()
						if err != nil {
							return err
						}
						fmt.Printf("[COMBINER %s] Both inputs EOS, forwarding EOS\n", name)
						return nil
					}
					continue
				}
				
				// Forward packet
				combinedData := fmt.Sprintf("combined-%s-%v", name, packet.Data)
				newPacket := NewPacket(combinedData, packet.ID)
				
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := output.Send(newPacket, ctx)
				cancel()
				if err != nil {
					return err
				}
				processed++
				
			case packet, ok := <-input2:
				if !ok {
					input2 = nil
					continue
				}
				
				if packet.IsEOS {
					eosCount++
					if eosCount >= 2 {
						// Forward EOS when both inputs are done
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						err := output.Send(packet, ctx)
						cancel()
						if err != nil {
							return err
						}
						fmt.Printf("[COMBINER %s] Both inputs EOS, forwarding EOS\n", name)
						return nil
					}
					continue
				}
				
				// Forward packet
				combinedData := fmt.Sprintf("combined-%s-%v", name, packet.Data)
				newPacket := NewPacket(combinedData, packet.ID)
				
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := output.Send(newPacket, ctx)
				cancel()
				if err != nil {
					return err
				}
				processed++
			}
			
			if processed%50 == 0 && processed > 0 {
				fmt.Printf("[COMBINER %s] Combined %d packets\n", name, processed)
			}
		}
	}
	
	node := NewDynamicNode(name, "combiner", processFunc)
	node.AddOutputChannel("out", 50)
	return node
}

// Main example demonstrating dynamic chain capabilities
func runDynamicChainExample() {
	fmt.Println("=== Dynamic Chain Execution Example ===")
	
	// Create dynamic chain manager
	manager := NewDynamicChainManager()
	
	// Create initial nodes
	fmt.Println("\n1. Creating initial nodes...")
	gen1 := createGeneratorNode("gen1", 500, 10.0) // Generate 500 packets at 10 Hz
	proc1 := createProcessorNode("proc1", 0.05)     // Process with 50ms delay
	sink1 := createSinkNode("sink1")
	
	// Add initial nodes to manager
	manager.AddNode(gen1, false)
	manager.AddNode(proc1, false)
	manager.AddNode(sink1, true) // sink1 is a sink node
	
	// Connect initial topology: gen1 -> proc1 -> sink1
	fmt.Println("\n2. Connecting initial topology: gen1 -> proc1 -> sink1")
	manager.ConnectNodes("gen1", "out", "proc1", "in", 20)
	manager.ConnectNodes("proc1", "out", "sink1", "in", 20)
	
	// Start the dynamic chain
	fmt.Println("\n3. Starting dynamic chain execution...")
	manager.Start()
	
	// Let it run for a bit
	time.Sleep(2 * time.Second)
	
	// Hot-add a second generator and processor
	fmt.Println("\n4. Hot-adding second generator and processor...")
	gen2 := createGeneratorNode("gen2", 300, 15.0) // Generate 300 packets at 15 Hz
	proc2 := createProcessorNode("proc2", 0.03)     // Process with 30ms delay
	
	manager.AddNode(gen2, false)
	manager.AddNode(proc2, false)
	
	// Connect new path: gen2 -> proc2
	manager.ConnectNodes("gen2", "out", "proc2", "in", 20)
	
	// Let the new nodes start processing
	time.Sleep(1 * time.Second)
	
	// Hot-add a combiner that merges both processors
	fmt.Println("\n5. Hot-adding combiner to merge both processors...")
	combiner := createCombinerNode("combiner1")
	sink2 := createSinkNode("sink2")
	
	manager.AddNode(combiner, false)
	manager.AddNode(sink2, true) // sink2 is also a sink node
	
	// Connect combiner topology: proc1 + proc2 -> combiner -> sink2
	manager.ConnectNodes("proc1", "out", "combiner1", "in1", 20)
	manager.ConnectNodes("proc2", "out", "combiner1", "in2", 20)
	manager.ConnectNodes("combiner1", "out", "sink2", "in", 20)
	
	// Monitor node states periodically
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				fmt.Println("\n📊 Current Node States:")
				states := manager.GetNodeStates()
				for name, state := range states {
					fmt.Printf("  %s: %s\n", name, state)
				}
			}
		}
	}()
	
	// Wait for at least one sink to complete
	fmt.Println("\n6. Waiting for sink completion...")
	completedSink := manager.WaitForCompletion()
	fmt.Printf("\n✅ Sink '%s' completed first!\n", completedSink)
	
	// Let other nodes finish processing
	time.Sleep(2 * time.Second)
	
	// Show final states
	fmt.Println("\n📊 Final Node States:")
	states := manager.GetNodeStates()
	for name, state := range states {
		fmt.Printf("  %s: %s\n", name, state)
	}
	
	// Stop the chain
	fmt.Println("\n7. Stopping dynamic chain...")
	manager.Stop()
	
	fmt.Println("\n✅ Dynamic chain example completed!")
}

// Entry point for the dynamic example
func main() {
	runDynamicChainExample()
}