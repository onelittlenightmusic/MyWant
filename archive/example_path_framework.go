package main

import (
	"fmt"
	"reflect"
)

func main() {
	fmt.Println("Path Framework Integration Demonstration")
	fmt.Println("======================================")

	// Demonstrate that path types are now part of the core declarative framework
	fmt.Println("\n1. CORE PATH TYPES (from declarative.go):")
	fmt.Println("==========================================")

	// Create path structures using core types
	pathInfo := PathInfo{
		Channel: make(chan interface{}, 10),
		Name:    "example_path",
		Active:  true,
	}

	paths := Paths{
		In:  []PathInfo{pathInfo},
		Out: []PathInfo{pathInfo},
	}

	connectivity := ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 1,
		MaxInputs:       2,
		MaxOutputs:      1,
		NodeType:        "example",
		Description:     "Example node for demonstration",
	}

	fmt.Printf("PathInfo: %+v\n", pathInfo)
	fmt.Printf("Paths: InCount=%d, OutCount=%d, ActiveIn=%d, ActiveOut=%d\n",
		paths.GetInCount(), paths.GetOutCount(),
		paths.GetActiveInCount(), paths.GetActiveOutCount())
	fmt.Printf("ConnectivityMetadata: %+v\n", connectivity)

	// Show enhanced metadata structure
	fmt.Println("\n2. ENHANCED METADATA STRUCTURE:")
	fmt.Println("===============================")

	metadata := Metadata{
		Name:         "example-node",
		Type:         "example",
		Labels:       map[string]string{"role": "demo"},
		Connectivity: connectivity,
	}

	fmt.Printf("Enhanced Metadata: %+v\n", metadata)

	// Show new path specification in NodeSpec
	fmt.Println("\n3. PATH SPECIFICATION IN CONFIG:")
	fmt.Println("=================================")

	pathSpec := PathSpec{
		Name:     "input_stream",
		Selector: map[string]string{"role": "source"},
		Active:   true,
	}

	pathsSpec := PathsSpec{
		Inputs:  []PathSpec{pathSpec},
		Outputs: []PathSpec{{Name: "output_stream", Active: true}},
	}

	nodeSpec := NodeSpec{
		Params: map[string]interface{}{"rate": 2.0},
		Paths:  pathsSpec,
	}

	fmt.Printf("PathSpec: %+v\n", pathSpec)
	fmt.Printf("PathsSpec: %+v\n", pathsSpec)
	fmt.Printf("NodeSpec with Paths: %+v\n", nodeSpec)

	// Show complete node structure
	fmt.Println("\n4. COMPLETE ENHANCED NODE:")
	fmt.Println("==========================")

	node := Node{
		Metadata: metadata,
		Spec:     nodeSpec,
	}

	fmt.Printf("Complete Node: %+v\n", node)

	// Demonstrate framework integration
	fmt.Println("\n5. FRAMEWORK INTEGRATION:")
	fmt.Println("=========================")

	config := Config{
		Nodes: []Node{node},
	}

	fmt.Printf("Config with Enhanced Nodes: %d nodes\n", len(config.Nodes))
	fmt.Printf("First node connectivity: %+v\n", config.Nodes[0].Metadata.Connectivity)

	// Show type information
	fmt.Println("\n6. TYPE INFORMATION:")
	fmt.Println("====================")

	fmt.Printf("PathInfo type: %s\n", reflect.TypeOf(pathInfo).Name())
	fmt.Printf("Paths type: %s\n", reflect.TypeOf(paths).Name())
	fmt.Printf("ConnectivityMetadata type: %s\n", reflect.TypeOf(connectivity).Name())
	fmt.Printf("EnhancedBaseNode interface methods:\n")
	fmt.Println("  - Process(paths *Paths) bool")
	fmt.Println("  - GetType() string")
	fmt.Println("  - GetStats() map[string]interface{}")
	fmt.Println("  - GetConnectivityMetadata() ConnectivityMetadata")
	fmt.Println("  - InitializePaths(inCount, outCount int)")

	fmt.Println("\n7. BENEFITS OF FRAMEWORK INTEGRATION:")
	fmt.Println("=====================================")
	fmt.Println("✓ Path types available to ALL domain implementations")
	fmt.Println("✓ Consistent connectivity metadata across domains")
	fmt.Println("✓ Unified configuration format for paths")
	fmt.Println("✓ Reusable path management utilities")
	fmt.Println("✓ Enhanced base node interface for all domains")
	fmt.Println("✓ Legacy configuration support maintained")
}
