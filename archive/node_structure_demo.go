package main

import (
	"fmt"
	"reflect"
)

// Complete Node Type Structure Documentation
func main() {
	fmt.Println("Node Type Structure in GoChain")
	fmt.Println("==============================")

	// 1. Base Interface
	fmt.Println("\n1. BASE INTERFACE:")
	fmt.Println("==================")
	fmt.Println(`
type BaseNode interface {
    Process(in chain.Chan, out chain.Chan) bool  // Main processing logic
    GetType() string                             // Node type identifier  
    GetStats() map[string]interface{}           // Runtime statistics
}`)

	// 2. Concrete Node Types
	fmt.Println("\n2. CONCRETE NODE TYPES:")
	fmt.Println("=======================")

	// Generator Node
	fmt.Println("\nGeneratorNode:")
	fmt.Println("--------------")
	gen := CreateGeneratorNode(2.0, 10)
	genType := reflect.TypeOf(gen).Elem()
	fmt.Printf("Type: %s\n", genType.Name())
	fmt.Println("Fields:")
	for i := 0; i < genType.NumField(); i++ {
		field := genType.Field(i)
		fmt.Printf("  %s %s  // %s\n", field.Name, field.Type, getFieldDescription(field.Name))
	}
	fmt.Printf("Methods: Process(), GetType(), GetStats()\n")
	fmt.Printf("Stats: %+v\n", gen.GetStats())

	// Queue Node
	fmt.Println("\nQueueNode:")
	fmt.Println("----------")
	queue := CreateQueueNode(1.5)
	queueType := reflect.TypeOf(queue).Elem()
	fmt.Printf("Type: %s\n", queueType.Name())
	fmt.Println("Fields:")
	for i := 0; i < queueType.NumField(); i++ {
		field := queueType.Field(i)
		fmt.Printf("  %s %s  // %s\n", field.Name, field.Type, getFieldDescription(field.Name))
	}
	fmt.Printf("Methods: Process(), GetType(), GetStats()\n")
	fmt.Printf("Stats: %+v\n", queue.GetStats())

	// Combiner Node
	fmt.Println("\nCombinerNode:")
	fmt.Println("-------------")
	combiner := CreateCombinerNode()
	combinerType := reflect.TypeOf(combiner).Elem()
	fmt.Printf("Type: %s\n", combinerType.Name())
	fmt.Println("Fields:")
	for i := 0; i < combinerType.NumField(); i++ {
		field := combinerType.Field(i)
		fmt.Printf("  %s %s  // %s\n", field.Name, field.Type, getFieldDescription(field.Name))
	}
	fmt.Printf("Methods: Process(), ProcessTwoStreams(), GetType(), GetStats()\n")
	fmt.Printf("Stats: %+v\n", combiner.GetStats())

	// Sink Node
	fmt.Println("\nSinkNode:")
	fmt.Println("---------")
	sink := CreateSinkNode()
	sinkType := reflect.TypeOf(sink).Elem()
	fmt.Printf("Type: %s\n", sinkType.Name())
	fmt.Println("Fields:")
	for i := 0; i < sinkType.NumField(); i++ {
		field := sinkType.Field(i)
		fmt.Printf("  %s %s  // %s\n", field.Name, field.Type, getFieldDescription(field.Name))
	}
	fmt.Printf("Methods: Process(), GetType(), GetStats()\n")
	fmt.Printf("Stats: %+v\n", sink.GetStats())

	// 3. Supporting Types
	fmt.Println("\n3. SUPPORTING TYPES:")
	fmt.Println("====================")

	fmt.Println("\nPacket:")
	fmt.Println("-------")
	packet := Packet{ID: 1, Time: 5.5}
	packetType := reflect.TypeOf(packet)
	fmt.Printf("Type: %s\n", packetType.Name())
	fmt.Println("Fields:")
	for i := 0; i < packetType.NumField(); i++ {
		field := packetType.Field(i)
		fmt.Printf("  %s %s  // %s\n", field.Name, field.Type, getFieldDescription(field.Name))
	}
	fmt.Printf("Methods: IsEnd() bool\n")
	fmt.Printf("Example: %+v, IsEnd: %t\n", packet, packet.IsEnd())

	// 4. Factory Functions
	fmt.Println("\n4. FACTORY FUNCTIONS:")
	fmt.Println("=====================")
	fmt.Println(`
CreateGeneratorNode(rate float64, count int) *GeneratorNode
CreateQueueNode(serviceTime float64) *QueueNode  
CreateCombinerNode() *CombinerNode
CreateSinkNode() *SinkNode`)

	// 5. Adapter Functions
	fmt.Println("\n5. ADAPTER FUNCTIONS:")
	fmt.Println("=====================")
	fmt.Println(`
NodeToChainFunc(node BaseNode) func(chain.Chan, chain.Chan) bool
NodeToEndFunc(node BaseNode) func(chain.Chan) bool
CombinerToChainFunc(combiner *CombinerNode) func(chain.Chan, chain.Chan, chain.Chan) bool`)

	// 6. Type Hierarchy
	fmt.Println("\n6. TYPE HIERARCHY:")
	fmt.Println("==================")
	fmt.Println(`
BaseNode (interface)
├── GeneratorNode (struct)
├── QueueNode (struct) 
├── CombinerNode (struct)
└── SinkNode (struct)

Packet (struct)
├── ID int
├── Time float64
└── IsEnd() bool`)

	// 7. Memory Layout Example
	fmt.Println("\n7. MEMORY LAYOUT EXAMPLE:")
	fmt.Println("=========================")
	showMemoryLayout()
}

func getFieldDescription(fieldName string) string {
	descriptions := map[string]string{
		"Rate":        "Packet generation rate (avg inter-arrival time)",
		"Count":       "Total number of packets to generate",
		"current":     "Current number of packets generated",
		"time":        "Current simulation time",
		"ServiceTime": "Average service time per packet",
		"queueTime":   "Current queue departure time",
		"processed":   "Number of packets processed",
		"totalDelay":  "Cumulative queueing delay",
		"packet1":     "Buffered packet from first input stream",
		"packet2":     "Buffered packet from second input stream",
		"stream1Done": "Flag indicating first stream has ended",
		"stream2Done": "Flag indicating second stream has ended",
		"merged":      "Number of packets merged so far",
		"received":    "Number of packets received",
		"lastTime":    "Timestamp of last received packet",
		"ID":          "Unique packet identifier",
		"Time":        "Packet timestamp",
	}
	if desc, exists := descriptions[fieldName]; exists {
		return desc
	}
	return "Field description"
}

func showMemoryLayout() {
	gen := CreateGeneratorNode(2.0, 100)
	queue := CreateQueueNode(1.5)

	fmt.Printf("GeneratorNode size: %d bytes\n", reflect.TypeOf(*gen).Size())
	fmt.Printf("QueueNode size: %d bytes\n", reflect.TypeOf(*queue).Size())
	fmt.Printf("Packet size: %d bytes\n", reflect.TypeOf(Packet{}).Size())

	fmt.Println("\nMemory alignment:")
	fmt.Printf("GeneratorNode alignment: %d bytes\n", reflect.TypeOf(*gen).Align())
	fmt.Printf("QueueNode alignment: %d bytes\n", reflect.TypeOf(*queue).Align())
	fmt.Printf("Packet alignment: %d bytes\n", reflect.TypeOf(Packet{}).Align())
}
