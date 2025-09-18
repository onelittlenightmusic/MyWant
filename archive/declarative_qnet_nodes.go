// Node-based queueing network implementation with encapsulated functionality
package main

import (
	"fmt"
	"mywant/src/chain"
	"math/rand"
)

// BaseNode defines the common interface for all network nodes
type BaseNode interface {
	Process(in chain.Chan, out chain.Chan) bool
	GetType() string
	GetStats() map[string]interface{}
}

// Packet represents data flowing through the network
type Packet struct {
	ID   int
	Time float64
}

func (p Packet) IsEnd() bool { return p.ID < 0 }

// GeneratorNode creates packets at specified rate
type GeneratorNode struct {
	Rate    float64
	Count   int
	current int
	time    float64
}

func (g *GeneratorNode) GetType() string {
	return "sequence"
}

func (g *GeneratorNode) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"rate":      g.Rate,
		"count":     g.Count,
		"generated": g.current,
	}
}

func (g *GeneratorNode) Process(in chain.Chan, out chain.Chan) bool {
	if g.current >= g.Count {
		out <- Packet{-1, 0} // End marker
		fmt.Printf("[GENERATOR] Generated %d packets\n", g.current)
		return true
	}

	g.current++
	g.time += g.Rate * rand.ExpFloat64()
	out <- Packet{g.current, g.time}
	return false
}

// QueueNode processes packets with service delays
type QueueNode struct {
	ServiceTime float64
	queueTime   float64
	processed   int
	totalDelay  float64
}

func (q *QueueNode) GetType() string {
	return "queue"
}

func (q *QueueNode) GetStats() map[string]interface{} {
	avgDelay := 0.0
	if q.processed > 0 {
		avgDelay = q.totalDelay / float64(q.processed)
	}
	return map[string]interface{}{
		"service_time":  q.ServiceTime,
		"processed":     q.processed,
		"average_delay": avgDelay,
	}
}

func (q *QueueNode) Process(in chain.Chan, out chain.Chan) bool {
	packet := (<-in).(Packet)

	if packet.IsEnd() {
		stats := q.GetStats()
		fmt.Printf("[QUEUE] Service: %.2f, Processed: %d, Avg Delay: %.2f\n",
			q.ServiceTime, q.processed, stats["average_delay"])
		out <- packet
		return true
	}

	// Process packet through queue
	if packet.Time > q.queueTime {
		q.queueTime = packet.Time
	}
	q.queueTime += q.ServiceTime * rand.ExpFloat64()

	q.totalDelay += q.queueTime - packet.Time
	q.processed++

	out <- Packet{packet.ID, q.queueTime}
	return false
}

// CombinerNode merges two streams in chronological order
type CombinerNode struct {
	packet1     *Packet
	packet2     *Packet
	stream1Done bool
	stream2Done bool
	merged      int
}

func (c *CombinerNode) GetType() string {
	return "combiner"
}

func (c *CombinerNode) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"merged_packets": c.merged,
	}
}

func (c *CombinerNode) ProcessTwoStreams(in1, in2, out chain.Chan) bool {
	// Read from streams if needed
	if c.packet1 == nil && !c.stream1Done {
		p := (<-in1).(Packet)
		if p.IsEnd() {
			c.stream1Done = true
		} else {
			c.packet1 = &p
		}
	}

	if c.packet2 == nil && !c.stream2Done {
		p := (<-in2).(Packet)
		if p.IsEnd() {
			c.stream2Done = true
		} else {
			c.packet2 = &p
		}
	}

	// Both streams ended
	if c.stream1Done && c.stream2Done {
		fmt.Printf("[COMBINER] Merged %d packets\n", c.merged)
		out <- Packet{-1, 0}
		return true
	}

	// Select earliest packet
	var selected *Packet
	if c.packet1 != nil && c.packet2 != nil {
		if c.packet1.Time <= c.packet2.Time {
			selected = c.packet1
			c.packet1 = nil
		} else {
			selected = c.packet2
			c.packet2 = nil
		}
	} else if c.packet1 != nil {
		selected = c.packet1
		c.packet1 = nil
	} else if c.packet2 != nil {
		selected = c.packet2
		c.packet2 = nil
	} else {
		return false // Need more data
	}

	c.merged++
	out <- *selected
	return false
}

// Process method for compatibility (single stream)
func (c *CombinerNode) Process(in chain.Chan, out chain.Chan) bool {
	// This shouldn't be called for combiners, but included for interface compliance
	packet := (<-in).(Packet)
	out <- packet
	return packet.IsEnd()
}

// SinkNode consumes packets and provides statistics
type SinkNode struct {
	received int
	lastTime float64
}

func (s *SinkNode) GetType() string {
	return "sink"
}

func (s *SinkNode) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"received":  s.received,
		"last_time": s.lastTime,
	}
}

func (s *SinkNode) Process(in chain.Chan, out chain.Chan) bool {
	packet := (<-in).(Packet)

	if !packet.IsEnd() {
		s.received++
		s.lastTime = packet.Time
		fmt.Printf("Packet %d received at time %.2f\n", packet.ID, packet.Time)
	} else {
		fmt.Printf("[SINK] Total received: %d packets\n", s.received)
	}

	return packet.IsEnd()
}

// Node factory functions that create and return node objects
func CreateGeneratorNode(rate float64, count int) *GeneratorNode {
	return &GeneratorNode{
		Rate:  rate,
		Count: count,
	}
}

func CreateQueueNode(serviceTime float64) *QueueNode {
	return &QueueNode{
		ServiceTime: serviceTime,
	}
}

func CreateCombinerNode() *CombinerNode {
	return &CombinerNode{}
}

func CreateSinkNode() *SinkNode {
	return &SinkNode{}
}

// Adapter functions to convert nodes to chain functions
func NodeToChainFunc(node BaseNode) func(chain.Chan, chain.Chan) bool {
	return func(in, out chain.Chan) bool {
		return node.Process(in, out)
	}
}

func NodeToEndFunc(node BaseNode) func(chain.Chan) bool {
	return func(in chain.Chan) bool {
		return node.Process(in, nil)
	}
}

func CombinerToChainFunc(combiner *CombinerNode) func(chain.Chan, chain.Chan, chain.Chan) bool {
	return func(in1, in2, out chain.Chan) bool {
		return combiner.ProcessTwoStreams(in1, in2, out)
	}
}

// Register node-based types with the chain builder
func RegisterNodeBasedQNetTypes(builder *ChainBuilder) {
	builder.RegisterNodeType("sequence", func(params map[string]interface{}) interface{} {
		rate := params["rate"].(float64)
		var count int
		switch v := params["count"].(type) {
		case int:
			count = v
		case float64:
			count = int(v)
		}
		node := CreateGeneratorNode(rate, count)
		return NodeToChainFunc(node)
	})

	builder.RegisterNodeType("queue", func(params map[string]interface{}) interface{} {
		serviceTime := params["service_time"].(float64)
		node := CreateQueueNode(serviceTime)
		return NodeToChainFunc(node)
	})

	builder.RegisterNodeType("combiner", func(params map[string]interface{}) interface{} {
		node := CreateCombinerNode()
		return CombinerToChainFunc(node)
	})

	builder.RegisterNodeType("sink", func(params map[string]interface{}) interface{} {
		node := CreateSinkNode()
		return NodeToEndFunc(node)
	})
}
