package main

import (
	"fmt"
	"gochain/chain"
)

// FibonacciSeed represents a fibonacci sequence value with its position
type FibonacciSeed struct {
	Value    int
	Position int
	IsEnd    bool
}

// SeedGenerator provides initial fibonacci seeds (0, 1)
type SeedGenerator struct {
	Node
	MaxCount int
	paths    Paths
}

// NewSeedGenerator creates a new seed generator node
func NewSeedGenerator(metadata Metadata, params map[string]interface{}) *SeedGenerator {
	gen := &SeedGenerator{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		MaxCount: 15,
	}
	
	if c, ok := params["max_count"]; ok {
		if ci, ok := c.(int); ok {
			gen.MaxCount = ci
		} else if cf, ok := c.(float64); ok {
			gen.MaxCount = int(cf)
		}
	}
	
	return gen
}

// InitializePaths initializes the paths for this generator
func (g *SeedGenerator) InitializePaths(inCount, outCount int) {
	g.paths.In = make([]PathInfo, inCount)
	g.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for seed generator
func (g *SeedGenerator) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       0,
		MaxOutputs:      -1,
		NodeType:        "seed_generator",
		Description:     "Fibonacci seed generator",
	}
}

// GetStats returns the stats for this generator
func (g *SeedGenerator) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":   g.Stats.TotalProcessed,
		"average_wait_time": g.Stats.AverageWaitTime,
		"total_wait_time":   g.Stats.TotalWaitTime,
	}
}

// Process processes using enhanced paths
func (g *SeedGenerator) Process(paths Paths) bool {
	g.paths = paths
	return false
}

// GetType returns the node type
func (g *SeedGenerator) GetType() string {
	return "seed_generator"
}

// GetNode returns the embedded Node
func (g *SeedGenerator) GetNode() *Node {
	return &g.Node
}

// CreateFunction returns the generalized chain function for the seed generator
func (g *SeedGenerator) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]
		
		// Send initial seeds: 0 and 1
		out <- FibonacciSeed{Value: 0, Position: 0, IsEnd: false}
		out <- FibonacciSeed{Value: 1, Position: 1, IsEnd: false}
		
		// Send end marker with max count info
		out <- FibonacciSeed{Value: g.MaxCount, Position: -1, IsEnd: true}
		
		g.Stats.TotalProcessed = 2
		fmt.Printf("[SEED] Generated initial seeds: 0, 1 (max_count: %d)\n", g.MaxCount)
		return true
	}
}

// FibonacciComputer computes the next fibonacci number from two inputs
type FibonacciComputer struct {
	Node
	paths Paths
}

// NewFibonacciComputer creates a new fibonacci computer node
func NewFibonacciComputer(metadata Metadata, params map[string]interface{}) *FibonacciComputer {
	return &FibonacciComputer{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
	}
}

// InitializePaths initializes the paths for this computer
func (c *FibonacciComputer) InitializePaths(inCount, outCount int) {
	c.paths.In = make([]PathInfo, inCount)
	c.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for fibonacci computer
func (c *FibonacciComputer) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      -1,
		NodeType:        "fibonacci_computer",
		Description:     "Fibonacci number computer",
	}
}

// GetStats returns the stats for this computer
func (c *FibonacciComputer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":   c.Stats.TotalProcessed,
		"average_wait_time": c.Stats.AverageWaitTime,
		"total_wait_time":   c.Stats.TotalWaitTime,
	}
}

// Process processes using enhanced paths
func (c *FibonacciComputer) Process(paths Paths) bool {
	c.paths = paths
	return false
}

// GetType returns the node type
func (c *FibonacciComputer) GetType() string {
	return "fibonacci_computer"
}

// GetNode returns the embedded Node
func (c *FibonacciComputer) GetNode() *Node {
	return &c.Node
}

// CreateFunction returns the generalized chain function for the fibonacci computer
func (c *FibonacciComputer) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	prev := 0
	current := 1
	position := 2
	maxCount := 15
	processed := 0
	initialized := false
	
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) == 0 || len(outputs) == 0 {
			return true
		}
		in := inputs[0]
		out := outputs[0]
		
		seed := (<-in).(FibonacciSeed)
		
		if seed.IsEnd {
			maxCount = seed.Value
			fmt.Printf("[COMPUTER] Received max count: %d\n", maxCount)
			return false
		}
		
		// Initialize with first two seeds
		if !initialized {
			if seed.Position == 0 {
				prev = seed.Value
				return false
			} else if seed.Position == 1 {
				current = seed.Value
				initialized = true
				// Start computing immediately after receiving both seeds
				if position < maxCount {
					next := prev + current
					out <- FibonacciSeed{Value: next, Position: position, IsEnd: false}
					prev = current
					current = next
					position++
					processed++
				}
				return false
			}
		}
		
		// Continue computing based on feedback (seed.Value is the previous computed value)
		if seed.Position >= 0 && position < maxCount {
			next := prev + current  // Use stored prev and current, not the feedback value
			out <- FibonacciSeed{Value: next, Position: position, IsEnd: false}
			
			prev = current
			current = next
			position++
			processed++
		} else if position >= maxCount {
			// Send end signal
			out <- FibonacciSeed{Value: 0, Position: -1, IsEnd: true}
			c.Stats.TotalProcessed = processed
			fmt.Printf("[COMPUTER] Computed %d fibonacci numbers\n", processed)
			return true
		}
		
		return false
	}
}

// FibonacciMerger merges seed values with computed values
type FibonacciMerger struct {
	Node
	paths Paths
}

// NewFibonacciMerger creates a new fibonacci merger node
func NewFibonacciMerger(metadata Metadata, params map[string]interface{}) *FibonacciMerger {
	return &FibonacciMerger{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
	}
}

// InitializePaths initializes the paths for this merger
func (m *FibonacciMerger) InitializePaths(inCount, outCount int) {
	m.paths.In = make([]PathInfo, inCount)
	m.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for fibonacci merger
func (m *FibonacciMerger) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  2,
		RequiredOutputs: 2, // One to computer, one to sink
		MaxInputs:       2,
		MaxOutputs:      -1,
		NodeType:        "fibonacci_merger",
		Description:     "Fibonacci sequence merger",
	}
}

// GetStats returns the stats for this merger
func (m *FibonacciMerger) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":   m.Stats.TotalProcessed,
		"average_wait_time": m.Stats.AverageWaitTime,
		"total_wait_time":   m.Stats.TotalWaitTime,
	}
}

// Process processes using enhanced paths
func (m *FibonacciMerger) Process(paths Paths) bool {
	m.paths = paths
	return false
}

// GetType returns the node type
func (m *FibonacciMerger) GetType() string {
	return "fibonacci_merger"
}

// GetNode returns the embedded Node
func (m *FibonacciMerger) GetNode() *Node {
	return &m.Node
}

// CreateFunction returns the generalized chain function for the fibonacci merger
func (m *FibonacciMerger) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	seedInputClosed := false
	computedInputClosed := false
	processed := 0
	maxCountReceived := false
	
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) < 2 || len(outputs) < 2 {
			return true
		}
		
		seedIn := inputs[0]      // From seed generator
		computedIn := inputs[1]  // From fibonacci computer (feedback loop)
		computerOut := outputs[0] // To fibonacci computer
		sinkOut := outputs[1]     // To sink
		
		// First, handle seed input (non-blocking check)
		select {
		case seed := <-seedIn:
			fibSeed := seed.(FibonacciSeed)
			if fibSeed.IsEnd {
				seedInputClosed = true
				if !maxCountReceived {
					// Forward end signal to computer to set max count
					computerOut <- fibSeed
					maxCountReceived = true
				}
			} else {
				computerOut <- fibSeed  // Send to computer for processing
				sinkOut <- fibSeed      // Send to sink for display
				processed++
			}
			return false
		default:
			// No seed data, continue to check computed input
		}
		
		// Then handle computed input (blocking if no seed data)
		if !seedInputClosed || !computedInputClosed {
			select {
			case computed := <-computedIn:
				fibSeed := computed.(FibonacciSeed)
				if fibSeed.IsEnd {
					computedInputClosed = true
				} else {
					computerOut <- fibSeed  // Feedback to computer
					sinkOut <- fibSeed      // Send to sink for display
					processed++
				}
				return false
			default:
				// No computed data available
				return false
			}
		}
		
		// End when both inputs are closed
		if seedInputClosed && computedInputClosed {
			// Send end signals to outputs
			sinkOut <- FibonacciSeed{Value: 0, Position: -1, IsEnd: true}
			
			m.Stats.TotalProcessed = processed
			fmt.Printf("[MERGER] Merged %d fibonacci values\n", processed)
			return true
		}
		
		return false
	}
}

// FibonacciSink collects and displays fibonacci sequence
type FibonacciSink struct {
	Node
	Sequence []int
	paths    Paths
}

// NewFibonacciSink creates a new fibonacci sink node
func NewFibonacciSink(metadata Metadata, params map[string]interface{}) *FibonacciSink {
	return &FibonacciSink{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		Sequence: make([]int, 0),
	}
}

// InitializePaths initializes the paths for this sink
func (s *FibonacciSink) InitializePaths(inCount, outCount int) {
	s.paths.In = make([]PathInfo, inCount)
	s.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for fibonacci sink
func (s *FibonacciSink) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0,
		MaxInputs:       1,
		MaxOutputs:      0,
		NodeType:        "fibonacci_sink",
		Description:     "Fibonacci sequence collector",
	}
}

// GetStats returns the stats for this sink
func (s *FibonacciSink) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":   s.Stats.TotalProcessed,
		"average_wait_time": s.Stats.AverageWaitTime,
		"total_wait_time":   s.Stats.TotalWaitTime,
		"sequence_length":   len(s.Sequence),
	}
}

// Process processes using enhanced paths
func (s *FibonacciSink) Process(paths Paths) bool {
	s.paths = paths
	return false
}

// GetType returns the node type
func (s *FibonacciSink) GetType() string {
	return "fibonacci_sink"
}

// GetNode returns the embedded Node
func (s *FibonacciSink) GetNode() *Node {
	return &s.Node
}

// CreateFunction returns the generalized chain function for the fibonacci sink
func (s *FibonacciSink) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) == 0 {
			return true
		}
		in := inputs[0]
		
		seed := (<-in).(FibonacciSeed)
		
		if seed.IsEnd {
			s.Stats.TotalProcessed = len(s.Sequence)
			
			fmt.Printf("\n🔢 Fibonacci Sequence (Loop Architecture):\n")
			for i, val := range s.Sequence {
				if i > 0 {
					fmt.Print(" ")
				}
				fmt.Print(val)
			}
			fmt.Printf("\n\n📊 Total numbers collected: %d\n", len(s.Sequence))
			return true
		}
		
		s.Sequence = append(s.Sequence, seed.Value)
		return false
	}
}

// RegisterFibonacciLoopNodeTypes registers the fibonacci loop node types with a ChainBuilder
func RegisterFibonacciLoopNodeTypes(builder *ChainBuilder) {
	// Register seed generator type
	builder.RegisterNodeType("seed_generator", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewSeedGenerator(metadata, params)
	})
	
	// Register fibonacci computer type
	builder.RegisterNodeType("fibonacci_computer", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewFibonacciComputer(metadata, params)
	})
	
	// Register fibonacci merger type
	builder.RegisterNodeType("fibonacci_merger", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewFibonacciMerger(metadata, params)
	})
	
	// Register fibonacci sink type
	builder.RegisterNodeType("fibonacci_sink", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewFibonacciSink(metadata, params)
	})
}