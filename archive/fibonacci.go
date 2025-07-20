package main

import (
	"fmt"
	"gochain/chain"
)

// FibonacciGenerator generates fibonacci sequence numbers
type FibonacciGenerator struct {
	Node
	Count int
	paths Paths
}

// NewFibonacciGenerator creates a new fibonacci generator node
func NewFibonacciGenerator(metadata Metadata, params map[string]interface{}) *FibonacciGenerator {
	gen := &FibonacciGenerator{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		Count: 20,
	}
	
	if c, ok := params["count"]; ok {
		if ci, ok := c.(int); ok {
			gen.Count = ci
		} else if cf, ok := c.(float64); ok {
			gen.Count = int(cf)
		}
	}
	
	return gen
}

// CreateFunction returns the generalized chain function for the generator
func (g *FibonacciGenerator) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]
		a, b := 0, 1
		for i := 0; i < g.Count; i++ {
			out <- a
			a, b = b, a+b
		}
		close(out)
		return true
	}
}

// GetNode returns the underlying Node
func (g *FibonacciGenerator) GetNode() *Node {
	return &g.Node
}

// FibonacciFilter filters fibonacci numbers based on criteria
type FibonacciFilter struct {
	Node
	MinValue int
	MaxValue int
	paths    Paths
}

// NewFibonacciFilter creates a new fibonacci filter node
func NewFibonacciFilter(metadata Metadata, params map[string]interface{}) *FibonacciFilter {
	filter := &FibonacciFilter{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		MinValue: 0,
		MaxValue: 1000000,
	}
	
	if min, ok := params["min_value"]; ok {
		if mini, ok := min.(int); ok {
			filter.MinValue = mini
		} else if minf, ok := min.(float64); ok {
			filter.MinValue = int(minf)
		}
	}
	
	if max, ok := params["max_value"]; ok {
		if maxi, ok := max.(int); ok {
			filter.MaxValue = maxi
		} else if maxf, ok := max.(float64); ok {
			filter.MaxValue = int(maxf)
		}
	}
	
	return filter
}

// CreateFunction returns the generalized chain function for the filter
func (f *FibonacciFilter) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) == 0 || len(outputs) == 0 {
			return true
		}
		in := inputs[0]
		out := outputs[0]
		
		for i := range in {
			if val, ok := i.(int); ok {
				// Filter based on min/max values
				if val >= f.MinValue && val <= f.MaxValue {
					out <- val
				}
				f.Stats.TotalProcessed++
			}
		}
		close(out)
		return true
	}
}

// GetNode returns the underlying Node
func (f *FibonacciFilter) GetNode() *Node {
	return &f.Node
}

// FibonacciSink collects and displays fibonacci numbers
type FibonacciSink struct {
	Node
	numbers []int
	paths   Paths
}

// NewFibonacciSink creates a new fibonacci sink node
func NewFibonacciSink(metadata Metadata, params map[string]interface{}) *FibonacciSink {
	sink := &FibonacciSink{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		numbers: make([]int, 0),
	}
	
	return sink
}

// CreateFunction returns the generalized chain function for the sink
func (s *FibonacciSink) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) == 0 {
			return true
		}
		in := inputs[0]
		
		fmt.Println("Fibonacci numbers:")
		for i := range in {
			if val, ok := i.(int); ok {
				s.numbers = append(s.numbers, val)
				fmt.Printf("%d ", val)
				s.Stats.TotalProcessed++
			}
		}
		fmt.Printf("\n\nTotal fibonacci numbers collected: %d\n", len(s.numbers))
		s.StoreState("numbers", s.numbers)
		s.StoreState("count", len(s.numbers))
		return true
	}
}

// GetNode returns the underlying Node
func (s *FibonacciSink) GetNode() *Node {
	return &s.Node
}

// RegisterFibonacciNodeTypes registers the fibonacci-specific node types with a ChainBuilder
func RegisterFibonacciNodeTypes(builder *ChainBuilder) {
	builder.RegisterNodeType("fibonacci_generator", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewFibonacciGenerator(metadata, params)
	})
	
	builder.RegisterNodeType("fibonacci_filter", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewFibonacciFilter(metadata, params)
	})
	
	builder.RegisterNodeType("fibonacci_sink", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewFibonacciSink(metadata, params)
	})
}

func main() {
	fmt.Println("Fibonacci Sequence Demo")
	fmt.Println("=======================")
	
	// Create a configuration for fibonacci sequence
	config := Config{
		Nodes: []Node{
			{
				Metadata: Metadata{
					Name: "generator",
					Type: "fibonacci_generator",
					Labels: map[string]string{
						"role": "source",
					},
				},
				Spec: NodeSpec{
					Params: map[string]interface{}{
						"count": 15,
					},
				},
			},
			{
				Metadata: Metadata{
					Name: "filter",
					Type: "fibonacci_filter",
					Labels: map[string]string{
						"role": "filter",
					},
				},
				Spec: NodeSpec{
					Params: map[string]interface{}{
						"min_value": 1,
						"max_value": 100,
					},
					Inputs: []map[string]string{
						{"role": "source"},
					},
				},
			},
			{
				Metadata: Metadata{
					Name: "sink",
					Type: "fibonacci_sink",
					Labels: map[string]string{
						"role": "collector",
					},
				},
				Spec: NodeSpec{
					Params: map[string]interface{}{},
					Inputs: []map[string]string{
						{"role": "filter"},
					},
				},
			},
		},
	}
	
	fmt.Printf("Created configuration with %d nodes\n", len(config.Nodes))
	
	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register fibonacci node types
	RegisterFibonacciNodeTypes(builder)
	
	fmt.Println("\nBuilding fibonacci chain...")
	err := builder.Build()
	if err != nil {
		fmt.Printf("❌ Build failed: %v\n", err)
		return
	}
	
	fmt.Println("✅ Build successful!")
	
	fmt.Println("\nExecuting fibonacci sequence...")
	builder.Execute()
	
	fmt.Println("\n📊 Final Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n",
			name, state.Status, state.Stats.TotalProcessed)
	}
	
	fmt.Println("\n✅ Fibonacci sequence execution completed!")
}