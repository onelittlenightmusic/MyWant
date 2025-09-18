// Enhanced node-based queueing network with unified path management
package main

import (
	"fmt"
	"math/rand"
)

// QNet-specific connectivity metadata definitions
var (
	GeneratorConnectivity = ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 0,
		MaxInputs:       0,
		MaxOutputs:      1,
		NodeType:        "sequence",
		Description:     "Generates packets at specified rate",
	}

	QueueConnectivity = ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      1,
		NodeType:        "queue",
		Description:     "Processes packets with service time delay",
	}

	CombinerConnectivity = ConnectivityMetadata{
		RequiredInputs:  2,
		RequiredOutputs: 1,
		MaxInputs:       -1, // Unlimited inputs
		MaxOutputs:      1,
		NodeType:        "combiner",
		Description:     "Merges multiple input streams in chronological order",
	}

	SinkConnectivity = ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0,
		MaxInputs:       -1, // Can accept multiple inputs
		MaxOutputs:      0,
		NodeType:        "sink",
		Description:     "Consumes packets and provides statistics",
	}
)

// ValidateConnectivity checks if a node's paths match its connectivity requirements
func ValidateConnectivity(node EnhancedBaseNode, inCount, outCount int) error {
	meta := node.GetConnectivityMetadata()

	// Check required inputs
	if inCount < meta.RequiredInputs {
		return fmt.Errorf("node %s requires %d inputs, got %d",
			meta.NodeType, meta.RequiredInputs, inCount)
	}

	// Check required outputs
	if outCount < meta.RequiredOutputs {
		return fmt.Errorf("node %s requires %d outputs, got %d",
			meta.NodeType, meta.RequiredOutputs, outCount)
	}

	// Check maximum inputs
	if meta.MaxInputs >= 0 && inCount > meta.MaxInputs {
		return fmt.Errorf("node %s allows max %d inputs, got %d",
			meta.NodeType, meta.MaxInputs, inCount)
	}

	// Check maximum outputs
	if meta.MaxOutputs >= 0 && outCount > meta.MaxOutputs {
		return fmt.Errorf("node %s allows max %d outputs, got %d",
			meta.NodeType, meta.MaxOutputs, outCount)
	}

	return nil
}

// GetNodeConnectivityInfo returns connectivity info for a given node type
func GetNodeConnectivityInfo(nodeType string) (ConnectivityMetadata, error) {
	switch nodeType {
	case "sequence":
		return GeneratorConnectivity, nil
	case "queue":
		return QueueConnectivity, nil
	case "combiner":
		return CombinerConnectivity, nil
	case "sink":
		return SinkConnectivity, nil
	default:
		return ConnectivityMetadata{}, fmt.Errorf("unknown node type: %s", nodeType)
	}
}

// Packet represents data flowing through the network
type Packet struct {
	ID   int
	Time float64
}

func (p Packet) IsEnd() bool { return p.ID < 0 }

// Enhanced Generator Node
type EnhancedGeneratorNode struct {
	Rate    float64
	Count   int
	current int
	time    float64
	paths   Paths
}

func (g *EnhancedGeneratorNode) GetType() string {
	return "sequence"
}

func (g *EnhancedGeneratorNode) GetConnectivityMetadata() ConnectivityMetadata {
	return GeneratorConnectivity
}

func (g *EnhancedGeneratorNode) InitializePaths(inCount, outCount int) {
	g.paths = Paths{
		In:  make([]PathInfo, inCount),
		Out: make([]PathInfo, outCount),
	}
	// Initialize output path
	if outCount > 0 {
		g.paths.Out[0] = PathInfo{
			Channel: make(chan interface{}, 10),
			Name:    "output",
			Active:  true,
		}
	}
}

func (g *EnhancedGeneratorNode) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"rate":             g.Rate,
		"count":            g.Count,
		"generated":        g.current,
		"input_paths":      g.paths.GetInCount(),
		"output_paths":     g.paths.GetOutCount(),
		"active_in_paths":  g.paths.GetActiveInCount(),
		"active_out_paths": g.paths.GetActiveOutCount(),
	}
}

func (g *EnhancedGeneratorNode) Process(paths *Paths) bool {
	if g.current >= g.Count {
		// Send end marker to all active output paths
		for _, outPath := range paths.Out {
			if outPath.Active {
				outPath.Channel <- Packet{-1, 0}
			}
		}
		fmt.Printf("[GENERATOR] Generated %d packets on %d output paths\n",
			g.current, paths.GetActiveOutCount())
		return true
	}

	g.current++
	g.time += g.Rate * rand.ExpFloat64()
	packet := Packet{g.current, g.time}

	// Send to all active output paths
	for _, outPath := range paths.Out {
		if outPath.Active {
			outPath.Channel <- packet
		}
	}

	return false
}

// Enhanced Queue Node
type EnhancedQueueNode struct {
	ServiceTime float64
	queueTime   float64
	processed   int
	totalDelay  float64
	paths       Paths
}

func (q *EnhancedQueueNode) GetType() string {
	return "queue"
}

func (q *EnhancedQueueNode) GetConnectivityMetadata() ConnectivityMetadata {
	return QueueConnectivity
}

func (q *EnhancedQueueNode) InitializePaths(inCount, outCount int) {
	q.paths = Paths{
		In:  make([]PathInfo, inCount),
		Out: make([]PathInfo, outCount),
	}
	// Initialize paths
	if inCount > 0 {
		q.paths.In[0] = PathInfo{
			Channel: make(chan interface{}, 10),
			Name:    "input",
			Active:  true,
		}
	}
	if outCount > 0 {
		q.paths.Out[0] = PathInfo{
			Channel: make(chan interface{}, 10),
			Name:    "output",
			Active:  true,
		}
	}
}

func (q *EnhancedQueueNode) GetStats() map[string]interface{} {
	avgDelay := 0.0
	if q.processed > 0 {
		avgDelay = q.totalDelay / float64(q.processed)
	}
	return map[string]interface{}{
		"service_time":     q.ServiceTime,
		"processed":        q.processed,
		"average_delay":    avgDelay,
		"input_paths":      q.paths.GetInCount(),
		"output_paths":     q.paths.GetOutCount(),
		"active_in_paths":  q.paths.GetActiveInCount(),
		"active_out_paths": q.paths.GetActiveOutCount(),
	}
}

func (q *EnhancedQueueNode) Process(paths *Paths) bool {
	// Read from first active input path
	var packet Packet
	for _, inPath := range paths.In {
		if inPath.Active {
			packet = (<-inPath.Channel).(Packet)
			break
		}
	}

	if packet.IsEnd() {
		avgDelay := 0.0
		if q.processed > 0 {
			avgDelay = q.totalDelay / float64(q.processed)
		}
		fmt.Printf("[QUEUE] Service: %.2fs, Processed: %d packets, Average Wait Time: %.3fs, Paths: %dIn/%dOut\n",
			q.ServiceTime, q.processed, avgDelay,
			paths.GetActiveInCount(), paths.GetActiveOutCount())

		// Forward end marker to all active output paths
		for _, outPath := range paths.Out {
			if outPath.Active {
				outPath.Channel <- packet
			}
		}
		return true
	}

	// Process packet through queue
	if packet.Time > q.queueTime {
		q.queueTime = packet.Time
	}
	q.queueTime += q.ServiceTime * rand.ExpFloat64()

	q.totalDelay += q.queueTime - packet.Time
	q.processed++

	processedPacket := Packet{packet.ID, q.queueTime}

	// Send to all active output paths
	for _, outPath := range paths.Out {
		if outPath.Active {
			outPath.Channel <- processedPacket
		}
	}

	return false
}

// Enhanced Combiner Node
type EnhancedCombinerNode struct {
	bufferedPackets []Packet
	streamsEnded    []bool
	merged          int
	paths           Paths
}

func (c *EnhancedCombinerNode) GetType() string {
	return "combiner"
}

func (c *EnhancedCombinerNode) GetConnectivityMetadata() ConnectivityMetadata {
	return CombinerConnectivity
}

func (c *EnhancedCombinerNode) InitializePaths(inCount, outCount int) {
	c.paths = Paths{
		In:  make([]PathInfo, inCount),
		Out: make([]PathInfo, outCount),
	}
	c.bufferedPackets = make([]Packet, inCount)
	c.streamsEnded = make([]bool, inCount)

	// Initialize input paths
	for i := 0; i < inCount; i++ {
		c.paths.In[i] = PathInfo{
			Channel: make(chan interface{}, 10),
			Name:    fmt.Sprintf("input_%d", i),
			Active:  true,
		}
	}

	// Initialize output path
	if outCount > 0 {
		c.paths.Out[0] = PathInfo{
			Channel: make(chan interface{}, 10),
			Name:    "output",
			Active:  true,
		}
	}
}

func (c *EnhancedCombinerNode) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"merged_packets":   c.merged,
		"input_paths":      c.paths.GetInCount(),
		"output_paths":     c.paths.GetOutCount(),
		"active_in_paths":  c.paths.GetActiveInCount(),
		"active_out_paths": c.paths.GetActiveOutCount(),
		"streams_ended":    c.countEndedStreams(),
	}
}

func (c *EnhancedCombinerNode) countEndedStreams() int {
	count := 0
	for _, ended := range c.streamsEnded {
		if ended {
			count++
		}
	}
	return count
}

func (c *EnhancedCombinerNode) Process(paths *Paths) bool {
	// Read from all active input streams that need data
	for i, inPath := range paths.In {
		if inPath.Active && !c.streamsEnded[i] && c.bufferedPackets[i].ID == 0 {
			packet := (<-inPath.Channel).(Packet)
			if packet.IsEnd() {
				c.streamsEnded[i] = true
			} else {
				c.bufferedPackets[i] = packet
			}
		}
	}

	// Check if all streams have ended
	allEnded := true
	for _, ended := range c.streamsEnded {
		if !ended {
			allEnded = false
			break
		}
	}

	if allEnded {
		fmt.Printf("[COMBINER] Merged %d packets from %d input paths\n",
			c.merged, paths.GetActiveInCount())

		// Send end marker to all active output paths
		for _, outPath := range paths.Out {
			if outPath.Active {
				outPath.Channel <- Packet{-1, 0}
			}
		}
		return true
	}

	// Find earliest packet among buffered packets
	earliestIdx := -1
	var earliestTime float64 = -1

	for i, packet := range c.bufferedPackets {
		if !c.streamsEnded[i] && packet.ID > 0 {
			if earliestIdx == -1 || packet.Time < earliestTime {
				earliestIdx = i
				earliestTime = packet.Time
			}
		}
	}

	if earliestIdx == -1 {
		return false // No packets to process
	}

	// Send earliest packet to all active output paths
	selectedPacket := c.bufferedPackets[earliestIdx]
	c.bufferedPackets[earliestIdx] = Packet{} // Clear buffer
	c.merged++

	for _, outPath := range paths.Out {
		if outPath.Active {
			outPath.Channel <- selectedPacket
		}
	}

	return false
}

// Enhanced Sink Node
type EnhancedSinkNode struct {
	received int
	lastTime float64
	paths    Paths
}

func (s *EnhancedSinkNode) GetType() string {
	return "sink"
}

func (s *EnhancedSinkNode) GetConnectivityMetadata() ConnectivityMetadata {
	return SinkConnectivity
}

func (s *EnhancedSinkNode) InitializePaths(inCount, outCount int) {
	s.paths = Paths{
		In:  make([]PathInfo, inCount),
		Out: make([]PathInfo, outCount),
	}

	// Initialize input paths
	for i := 0; i < inCount; i++ {
		s.paths.In[i] = PathInfo{
			Channel: make(chan interface{}, 10),
			Name:    fmt.Sprintf("input_%d", i),
			Active:  true,
		}
	}
}

func (s *EnhancedSinkNode) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"received":         s.received,
		"last_time":        s.lastTime,
		"input_paths":      s.paths.GetInCount(),
		"output_paths":     s.paths.GetOutCount(),
		"active_in_paths":  s.paths.GetActiveInCount(),
		"active_out_paths": s.paths.GetActiveOutCount(),
	}
}

func (s *EnhancedSinkNode) Process(paths *Paths) bool {
	// Read from first active input path
	var packet Packet
	for _, inPath := range paths.In {
		if inPath.Active {
			packet = (<-inPath.Channel).(Packet)
			break
		}
	}

	if !packet.IsEnd() {
		s.received++
		s.lastTime = packet.Time
		// Removed real-time packet monitoring for performance
	} else {
		fmt.Printf("[SINK] Total received: %d packets via %d input paths\n",
			s.received, paths.GetInCount())
	}

	return packet.IsEnd()
}

// Factory functions for enhanced nodes
func CreateEnhancedGeneratorNode(rate float64, count int) *EnhancedGeneratorNode {
	node := &EnhancedGeneratorNode{
		Rate:  rate,
		Count: count,
	}
	node.InitializePaths(0, 1)
	return node
}

func CreateEnhancedQueueNode(serviceTime float64) *EnhancedQueueNode {
	node := &EnhancedQueueNode{
		ServiceTime: serviceTime,
	}
	node.InitializePaths(1, 1)
	return node
}

func CreateEnhancedCombinerNode(inputCount int) *EnhancedCombinerNode {
	node := &EnhancedCombinerNode{}
	node.InitializePaths(inputCount, 1)
	return node
}

func CreateEnhancedSinkNode(inputCount int) *EnhancedSinkNode {
	node := &EnhancedSinkNode{}
	node.InitializePaths(inputCount, 0)
	return node
}
