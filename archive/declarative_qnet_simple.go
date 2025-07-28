// Simple queueing network implementation for the declarative chain framework
package main

import (
	"fmt"
	"gochain/chain"
	"math/rand"
)

// Packet represents data flowing through the network
type Packet struct {
	ID   int
	Time float64
}

// IsEnd checks if this is an end-of-stream packet
func (p Packet) IsEnd() bool {
	return p.ID < 0
}

// Generator creates packets
func Generator(rate float64, count int) func(chain.Chan, chain.Chan) bool {
	current := 0
	time := 0.0
	
	return func(_, out chain.Chan) bool {
		if current >= count {
			out <- Packet{-1, 0} // End marker
			return true
		}
		
		current++
		time += rate * rand.ExpFloat64()
		out <- Packet{current, time}
		return false
	}
}

// Queue processes packets with delay
func Queue(serviceTime float64) func(chain.Chan, chain.Chan) bool {
	queueTime := 0.0
	processed := 0
	totalDelay := 0.0
	
	return func(in, out chain.Chan) bool {
		packet := (<-in).(Packet)
		
		if packet.IsEnd() {
			if processed > 0 {
				fmt.Printf("[QUEUE] Service: %.2f, Average Delay: %.2f\n", 
					serviceTime, totalDelay/float64(processed))
			}
			out <- packet
			return true
		}
		
		// Process packet
		if packet.Time > queueTime {
			queueTime = packet.Time
		}
		queueTime += serviceTime * rand.ExpFloat64()
		
		totalDelay += queueTime - packet.Time
		processed++
		
		out <- Packet{packet.ID, queueTime}
		return false
	}
}

// Combiner merges two streams in chronological order
func Combiner() func(chain.Chan, chain.Chan, chain.Chan) bool {
	var packet1, packet2 *Packet
	stream1Done, stream2Done := false, false
	
	return func(in1, in2, out chain.Chan) bool {
		// Read from streams if needed
		if packet1 == nil && !stream1Done {
			p := (<-in1).(Packet)
			if p.IsEnd() {
				stream1Done = true
			} else {
				packet1 = &p
			}
		}
		
		if packet2 == nil && !stream2Done {
			p := (<-in2).(Packet)
			if p.IsEnd() {
				stream2Done = true
			} else {
				packet2 = &p
			}
		}
		
		// Both streams ended
		if stream1Done && stream2Done {
			out <- Packet{-1, 0}
			return true
		}
		
		// Select earliest packet
		var selected *Packet
		if packet1 != nil && packet2 != nil {
			if packet1.Time <= packet2.Time {
				selected = packet1
				packet1 = nil
			} else {
				selected = packet2
				packet2 = nil
			}
		} else if packet1 != nil {
			selected = packet1
			packet1 = nil
		} else if packet2 != nil {
			selected = packet2
			packet2 = nil
		} else {
			return false // Need more data
		}
		
		out <- *selected
		return false
	}
}

// Sink consumes packets
func Sink() func(chain.Chan) bool {
	return func(in chain.Chan) bool {
		packet := (<-in).(Packet)
		return packet.IsEnd()
	}
}

// Register all simple node types
func RegisterSimpleQNetNodeTypes(builder *ChainBuilder) {
	builder.RegisterNodeType("sequence", func(params map[string]interface{}) interface{} {
		rate := params["rate"].(float64)
		// Handle both int and float64 for count
		var count int
		switch v := params["count"].(type) {
		case int:
			count = v
		case float64:
			count = int(v)
		}
		return Generator(rate, count)
	})
	
	builder.RegisterNodeType("queue", func(params map[string]interface{}) interface{} {
		serviceTime := params["service_time"].(float64)
		return Queue(serviceTime)
	})
	
	builder.RegisterNodeType("combiner", func(params map[string]interface{}) interface{} {
		return Combiner()
	})
	
	builder.RegisterNodeType("sink", func(params map[string]interface{}) interface{} {
		return Sink()
	})
}