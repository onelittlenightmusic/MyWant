package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🧪 Dynamic Node Addition Error Handling Test")
	fmt.Println("==============================================")
	
	// Test 1: Adding node to non-executing chain
	fmt.Println("\n📋 Test 1: Adding node to non-executing chain")
	config := Config{
		Nodes: []Node{
			{
				Metadata: Metadata{
					Name: "gen-test",
					Type: "sequence",
					Labels: map[string]string{
						"role": "source",
					},
				},
				Spec: NodeSpec{
					Params: map[string]interface{}{
						"rate":  1.0,
						"count": 5,
					},
				},
			},
		},
	}
	
	builder := NewChainBuilder(config)
	RegisterQNetNodeTypes(builder)
	
	// Try to add node without executing
	testNode := Node{
		Metadata: Metadata{
			Name: "test-fail",
			Type: "queue",
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.5,
			},
			Inputs: []map[string]string{
				{"role": "source"},
			},
		},
	}
	
	err := builder.AddNode(testNode)
	if err != nil {
		fmt.Printf("✅ Expected error: %v\n", err)
	} else {
		fmt.Println("❌ Should have failed - chain not executing")
	}
	
	// Test 2: Adding node with invalid input selector
	fmt.Println("\n📋 Test 2: Adding node with invalid input selector")
	builder.Build()
	go builder.Execute()
	time.Sleep(100 * time.Millisecond)
	
	invalidNode := Node{
		Metadata: Metadata{
			Name: "invalid-input",
			Type: "queue",
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.5,
			},
			Inputs: []map[string]string{
				{"role": "nonexistent"},
			},
		},
	}
	
	err = builder.AddNode(invalidNode)
	if err != nil {
		fmt.Printf("✅ Expected error for invalid selector: %v\n", err)
	} else {
		fmt.Println("❌ Should have failed - invalid input selector")
	}
	
	// Test 3: Adding node with unregistered type
	fmt.Println("\n📋 Test 3: Adding node with unregistered type")
	unregisteredNode := Node{
		Metadata: Metadata{
			Name: "unregistered-type",
			Type: "fake-type",
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{},
			Inputs: []map[string]string{
				{"role": "source"},
			},
		},
	}
	
	err = builder.AddNode(unregisteredNode)
	if err != nil {
		fmt.Printf("✅ Expected error for unregistered type: %v\n", err)
	} else {
		fmt.Println("❌ Should have failed - unregistered node type")
	}
	
	// Test 4: Adding valid node should succeed
	fmt.Println("\n📋 Test 4: Adding valid node")
	validNode := Node{
		Metadata: Metadata{
			Name: "valid-queue",
			Type: "queue",
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.3,
			},
			Inputs: []map[string]string{
				{"role": "source"},
			},
		},
	}
	
	err = builder.AddNode(validNode)
	if err != nil {
		fmt.Printf("❌ Unexpected error: %v\n", err)
	} else {
		fmt.Println("✅ Valid node added successfully")
	}
	
	// Wait for execution
	time.Sleep(2 * time.Second)
	
	// Show final state
	fmt.Println("\n📊 Final State:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}
	
	fmt.Println("\n🎉 Error handling tests completed!")
}