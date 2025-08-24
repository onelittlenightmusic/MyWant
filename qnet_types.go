package main

import (
	"fmt"
	"gochain/chain"
	"math/rand"
	"strconv"
)

// QueuePacket represents data flowing through the chain
type QueuePacket struct {
	Num  int
	Time float64
}

func (p *QueuePacket) isEnded() bool {
	return p.Num < 0
}


// Generator creates packets and sends them downstream
type Generator struct {
	Node
	Rate  float64
	Count int
	paths Paths
}

// PacketSequence creates a new generator node
func PacketSequence(metadata Metadata, spec NodeSpec) *Generator {
	gen := &Generator{
		Node: Node{
			Metadata: metadata,
			Spec:     spec,
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		Rate:  1.0,
		Count: 100,
	}
	
	if r, ok := spec.Params["rate"]; ok {
		if rf, ok := r.(float64); ok {
			gen.Rate = rf
		}
	}
	if c, ok := spec.Params["count"]; ok {
		if ci, ok := c.(int); ok {
			gen.Count = ci
		} else if cf, ok := c.(float64); ok {
			gen.Count = int(cf)
		} else if cs, ok := c.(string); ok {
			if parsed, err := strconv.Atoi(cs); err == nil {
				gen.Count = parsed
			}
		}
	}
	
	return gen
}

// InitializePaths initializes the paths for this generator
func (g *Generator) InitializePaths(inCount, outCount int) {
	g.paths.In = make([]PathInfo, inCount)
	g.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for generator
func (g *Generator) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0, // Generators don't need inputs
		RequiredOutputs: 1, // Must have at least one output
		MaxInputs:       0, // No inputs allowed
		MaxOutputs:      -1, // Unlimited outputs
		NodeType:        "sequence",
		Description:     "Packet generator node",
	}
}

// GetStats returns the stats for this generator
func (g *Generator) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":    g.Stats.TotalProcessed,
		"average_wait_time":  g.Stats.AverageWaitTime,
		"total_wait_time":    g.Stats.TotalWaitTime,
	}
}

// Process processes using enhanced paths (for enhanced node compatibility)
func (g *Generator) Process(paths Paths) bool {
	g.paths = paths
	return false // Not used in current implementation
}

// GetType returns the node type
func (g *Generator) GetType() string {
	return "sequence"
}

// GetNode returns the embedded Node
func (g *Generator) GetNode() *Node {
	return &g.Node
}

// CreateFunction returns the generalized chain function for this generator
func (g *Generator) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	t, j := 0.0, 0
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]
		
		if j >= g.Count {
			// Store generation stats
			g.Stats.TotalProcessed = j
			g.Stats.AverageWaitTime = 0.0 // Generators don't have wait time
			g.Stats.TotalWaitTime = 0.0
			
			out <- QueuePacket{-1, 0}
			fmt.Printf("[GENERATOR] Generated %d packets\n", j)
			return true
		}
		j++
		t += g.Rate * rand.ExpFloat64()
		out <- QueuePacket{j, t}
		return false
	}
}

// Queue processes packets with a service time
type Queue struct {
	Node
	ServiceTime float64
	paths       Paths
}

// NewQueue creates a new queue node
func NewQueue(metadata Metadata, spec NodeSpec) *Queue {
	queue := &Queue{
		Node: Node{
			Metadata: metadata,
			Spec:     spec,
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		ServiceTime: 1.0,
	}
	
	if st, ok := spec.Params["service_time"]; ok {
		if stf, ok := st.(float64); ok {
			queue.ServiceTime = stf
		}
	}
	
	return queue
}

// InitializePaths initializes the paths for this queue
func (q *Queue) InitializePaths(inCount, outCount int) {
	q.paths.In = make([]PathInfo, inCount)
	q.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for queue
func (q *Queue) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  1, // Queues need at least one input
		RequiredOutputs: 1, // Must have at least one output
		MaxInputs:       1, // Only one input supported
		MaxOutputs:      -1, // Unlimited outputs
		NodeType:        "queue",
		Description:     "Queue processing node",
	}
}

// GetStats returns the stats for this queue
func (q *Queue) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":    q.Stats.TotalProcessed,
		"average_wait_time":  q.Stats.AverageWaitTime,
		"total_wait_time":    q.Stats.TotalWaitTime,
	}
}

// Process processes using enhanced paths
func (q *Queue) Process(paths Paths) bool {
	q.paths = paths
	return false // Not used in current implementation
}

// GetType returns the node type
func (q *Queue) GetType() string {
	return "queue"
}

// GetNode returns the embedded Node
func (q *Queue) GetNode() *Node {
	return &q.Node
}

// CreateFunction returns the generalized chain function for this queue
func (q *Queue) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	tBuf, tSum := 0.0, 0.0
	nBuf := 0
	
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) == 0 || len(outputs) == 0 {
			return true
		}
		in := inputs[0]
		out := outputs[0]
		
		packet := (<-in).(QueuePacket)
		
		if packet.isEnded() {
			if nBuf > 0 {
				avgWaitTime := tSum / float64(nBuf)
				// Store stats in the Node
				q.Stats.AverageWaitTime = avgWaitTime
				q.Stats.TotalProcessed = nBuf
				q.Stats.TotalWaitTime = tSum
				
				fmt.Printf("[QUEUE %s] Service: %.2f, Processed: %d, Avg Delay: %.3f\n", 
					q.Metadata.Name, q.ServiceTime, nBuf, avgWaitTime)
			}
			out <- packet
			return true
		}
		
		if packet.Time > tBuf {
			tBuf = packet.Time
		}
		tBuf += q.ServiceTime * rand.ExpFloat64()
		
		out <- QueuePacket{packet.Num, tBuf}
		
		waitTime := tBuf - packet.Time
		tSum += waitTime
		nBuf = packet.Num
		return false
	}
}

// Combiner merges multiple input streams
type Combiner struct {
	Node
	Operation string
	paths     Paths
}

// NewCombiner creates a new combiner node
func NewCombiner(metadata Metadata, spec NodeSpec) *Combiner {
	combiner := &Combiner{
		Node: Node{
			Metadata: metadata,
			Spec:     spec,
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		Operation: "merge",
	}
	
	if op, ok := spec.Params["operation"]; ok {
		if ops, ok := op.(string); ok {
			combiner.Operation = ops
		}
	}
	
	return combiner
}

// InitializePaths initializes the paths for this combiner
func (c *Combiner) InitializePaths(inCount, outCount int) {
	c.paths.In = make([]PathInfo, inCount)
	c.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for combiner
func (c *Combiner) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  2, // Combiners need at least two inputs
		RequiredOutputs: 1, // Must have at least one output
		MaxInputs:       -1, // Unlimited inputs
		MaxOutputs:      -1, // Unlimited outputs
		NodeType:        "combiner",
		Description:     "Stream combiner node",
	}
}

// GetStats returns the stats for this combiner
func (c *Combiner) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":    c.Stats.TotalProcessed,
		"average_wait_time":  c.Stats.AverageWaitTime,
		"total_wait_time":    c.Stats.TotalWaitTime,
	}
}

// Process processes using enhanced paths
func (c *Combiner) Process(paths Paths) bool {
	c.paths = paths
	return false // Not used in current implementation
}

// GetType returns the node type
func (c *Combiner) GetType() string {
	return "combiner"
}

// GetNode returns the embedded Node
func (c *Combiner) GetNode() *Node {
	return &c.Node
}

// CreateFunction returns the generalized chain function for this combiner
func (c *Combiner) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	processed := 0
	inputsClosed := make([]bool, 0) // Track which inputs are closed
	packetBuffer := make([]QueuePacket, 0) // Buffer to store packets not yet sent
	
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) == 0 || len(outputs) == 0 {
			return true
		}
		out := outputs[0]
		
		// Initialize closed tracking if needed
		if len(inputsClosed) != len(inputs) {
			inputsClosed = make([]bool, len(inputs))
		}
		
		// Check if all inputs are closed and buffer is empty
		allClosed := true
		for _, closed := range inputsClosed {
			if !closed {
				allClosed = false
				break
			}
		}
		if allClosed && len(packetBuffer) == 0 {
			// Store combiner stats
			c.Stats.TotalProcessed = processed
			c.Stats.AverageWaitTime = 0.0 // Combiners don't add wait time
			c.Stats.TotalWaitTime = 0.0
			
			fmt.Printf("[COMBINER] Operation: %s, Processed %d packets\n", c.Operation, processed)
			out <- QueuePacket{-1, 0} // Send end signal
			return true
		}
		
		// Collect available packets from all open inputs and add to buffer
		for i, input := range inputs {
			if inputsClosed[i] {
				continue // Skip closed inputs
			}
			
			select {
			case packet := <-input:
				qPacket := packet.(QueuePacket)
				if qPacket.isEnded() {
					inputsClosed[i] = true // Mark this input as closed
				} else {
					packetBuffer = append(packetBuffer, qPacket)
				}
			default:
				// No packet available on this input right now
			}
		}
		
		// If buffer is empty and not all inputs closed, wait for at least one packet
		if len(packetBuffer) == 0 && !allClosed {
			// Wait for next packet from any open input
			for i, input := range inputs {
				if inputsClosed[i] {
					continue
				}
				select {
				case packet := <-input:
					qPacket := packet.(QueuePacket)
					if qPacket.isEnded() {
						inputsClosed[i] = true
					} else {
						packetBuffer = append(packetBuffer, qPacket)
					}
					break // Got a packet, process it
				default:
					continue
				}
			}
		}
		
		// Find and send earliest packet from buffer
		if len(packetBuffer) > 0 {
			earliestIdx := 0
			earliestTime := packetBuffer[0].Time
			
			for i, packet := range packetBuffer {
				if packet.Time < earliestTime {
					earliestTime = packet.Time
					earliestIdx = i
				}
			}
			
			// Send the earliest packet
			earliestPacket := packetBuffer[earliestIdx]
			processed++
			out <- earliestPacket
			
			// Remove the sent packet from buffer (preserve order for remaining packets)
			packetBuffer = append(packetBuffer[:earliestIdx], packetBuffer[earliestIdx+1:]...)
		}
		
		return false // Continue processing
	}
}

// Sink collects and terminates the packet stream
type Sink struct {
	Node
	Received int
	paths    Paths
}

// Goal creates a new sink node
func Goal(metadata Metadata, spec NodeSpec) *Sink {
	return &Sink{
		Node: Node{
			Metadata: metadata,
			Spec:     spec,
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		Received: 0,
	}
}

// InitializePaths initializes the paths for this sink
func (s *Sink) InitializePaths(inCount, outCount int) {
	s.paths.In = make([]PathInfo, inCount)
	s.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for sink
func (s *Sink) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  1, // Sinks need at least one input
		RequiredOutputs: 0, // Sinks don't need outputs
		MaxInputs:       -1, // Unlimited inputs
		MaxOutputs:      0, // No outputs allowed
		NodeType:        "sink",
		Description:     "Data sink/collector node",
	}
}

// GetStats returns the stats for this sink
func (s *Sink) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":    s.Stats.TotalProcessed,
		"average_wait_time":  s.Stats.AverageWaitTime,
		"total_wait_time":    s.Stats.TotalWaitTime,
	}
}

// Process processes using enhanced paths
func (s *Sink) Process(paths Paths) bool {
	s.paths = paths
	return false // Not used in current implementation
}

// GetType returns the node type
func (s *Sink) GetType() string {
	return "sink"
}

// GetNode returns the embedded Node
func (s *Sink) GetNode() *Node {
	return &s.Node
}

// CreateFunction returns the generalized chain function for this sink
func (s *Sink) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		// If no inputs configured, this sink shouldn't run
		if len(inputs) == 0 {
			return true
		}
		in := inputs[0]
		
		// Block waiting for data from input channel
		packet := (<-in).(QueuePacket)
		
		if packet.isEnded() {
			// Store sink stats
			s.Stats.TotalProcessed = s.Received
			s.Stats.AverageWaitTime = 0.0 // Sinks don't add wait time
			s.Stats.TotalWaitTime = 0.0
			
			fmt.Printf("[SINK] Total received: %d packets\n", s.Received)
			return true
		}
		
		s.Received++
		return false
	}
}

// RegisterQNetNodeTypes registers the qnet-specific node types with a ChainBuilder
func RegisterQNetNodeTypes(builder *ChainBuilder) {
	// Register generator type - return the enhanced node itself for validation
	builder.RegisterNodeType("sequence", func(metadata Metadata, spec NodeSpec) interface{} {
		return PacketSequence(metadata, spec)
	})
	
	// Register queue type - return the enhanced node itself for validation
	builder.RegisterNodeType("queue", func(metadata Metadata, spec NodeSpec) interface{} {
		return NewQueue(metadata, spec)
	})
	
	// Register combiner type - return the enhanced node itself for validation
	builder.RegisterNodeType("combiner", func(metadata Metadata, spec NodeSpec) interface{} {
		return NewCombiner(metadata, spec)
	})
	
	// Register sink type - return the enhanced node itself for validation
	builder.RegisterNodeType("sink", func(metadata Metadata, spec NodeSpec) interface{} {
		return Goal(metadata, spec)
	})
	
	// Register collector type (alias for sink) - return the enhanced node itself for validation
	builder.RegisterNodeType("collector", func(metadata Metadata, spec NodeSpec) interface{} {
		return Goal(metadata, spec)
	})
}

