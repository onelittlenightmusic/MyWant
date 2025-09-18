package main

import (
	"fmt"
)

func main() {
	fmt.Println("Connectivity Definitions Demonstration")
	fmt.Println("=====================================")

	// Show that connectivity definitions are domain-specific
	fmt.Println("\n1. DOMAIN-SPECIFIC CONNECTIVITY DEFINITIONS:")
	fmt.Println("============================================")

	nodeTypes := []string{"sequence", "queue", "combiner", "sink"}

	for _, nodeType := range nodeTypes {
		if meta, err := GetNodeConnectivityInfo(nodeType); err == nil {
			fmt.Printf("\n%s:\n", nodeType)
			fmt.Printf("  Required: %dIn/%dOut\n", meta.RequiredInputs, meta.RequiredOutputs)
			fmt.Printf("  Maximum:  %dIn/%dOut\n", meta.MaxInputs, meta.MaxOutputs)
			fmt.Printf("  Description: %s\n", meta.Description)
		}
	}

	// Demonstrate connectivity validation
	fmt.Println("\n2. CONNECTIVITY VALIDATION:")
	fmt.Println("===========================")

	// Create test nodes
	gen := CreateEnhancedGeneratorNode(1.0, 5)
	queue := CreateEnhancedQueueNode(0.5)
	combiner := CreateEnhancedCombinerNode(3) // 3 inputs
	sink := CreateEnhancedSinkNode(1)

	testCases := []struct {
		node     EnhancedBaseNode
		inCount  int
		outCount int
		name     string
	}{
		{gen, 0, 1, "Generator (valid)"},
		{gen, 1, 1, "Generator (invalid - has input)"},
		{queue, 1, 1, "Queue (valid)"},
		{queue, 0, 1, "Queue (invalid - no input)"},
		{queue, 2, 1, "Queue (invalid - too many inputs)"},
		{combiner, 3, 1, "Combiner (valid)"},
		{combiner, 1, 1, "Combiner (invalid - too few inputs)"},
		{sink, 1, 0, "Sink (valid)"},
		{sink, 1, 1, "Sink (invalid - has output)"},
	}

	for _, test := range testCases {
		err := ValidateConnectivity(test.node, test.inCount, test.outCount)
		status := "✓ VALID"
		if err != nil {
			status = fmt.Sprintf("✗ INVALID: %s", err.Error())
		}
		fmt.Printf("%-30s %s\n", test.name, status)
	}

	// Show predefined connectivity constants
	fmt.Println("\n3. PREDEFINED CONNECTIVITY CONSTANTS:")
	fmt.Println("====================================")

	fmt.Printf("GeneratorConnectivity: %+v\n", GeneratorConnectivity)
	fmt.Printf("QueueConnectivity:     %+v\n", QueueConnectivity)
	fmt.Printf("CombinerConnectivity:  %+v\n", CombinerConnectivity)
	fmt.Printf("SinkConnectivity:      %+v\n", SinkConnectivity)

	// Demonstrate framework vs domain separation
	fmt.Println("\n4. FRAMEWORK vs DOMAIN SEPARATION:")
	fmt.Println("==================================")

	fmt.Println("Framework Types (declarative.go):")
	fmt.Println("  ✓ PathInfo - Generic path information")
	fmt.Println("  ✓ Paths - Generic path container")
	fmt.Println("  ✓ ConnectivityMetadata - Generic metadata structure")
	fmt.Println("  ✓ EnhancedBaseNode - Generic node interface")
	fmt.Println("  ✓ PathSpec/PathsSpec - Generic configuration")

	fmt.Println("\nDomain Types (declarative_qnet_paths.go):")
	fmt.Println("  ✓ GeneratorConnectivity - QNet-specific generator constraints")
	fmt.Println("  ✓ QueueConnectivity - QNet-specific queue constraints")
	fmt.Println("  ✓ CombinerConnectivity - QNet-specific combiner constraints")
	fmt.Println("  ✓ SinkConnectivity - QNet-specific sink constraints")
	fmt.Println("  ✓ ValidateConnectivity() - QNet-specific validation")
	fmt.Println("  ✓ GetNodeConnectivityInfo() - QNet-specific lookup")

	fmt.Println("\n5. BENEFITS OF THIS SEPARATION:")
	fmt.Println("===============================")
	fmt.Println("✓ Framework provides reusable path management")
	fmt.Println("✓ Domain defines specific node constraints")
	fmt.Println("✓ Easy to add new domains with different connectivity rules")
	fmt.Println("✓ Validation logic is domain-specific and flexible")
	fmt.Println("✓ Connectivity metadata is self-documenting")
	fmt.Println("✓ Type safety ensures correct node usage")
}
