package main

import (
	"fmt"
	"gochain/chain"
	"math/rand"
	"time"
)

// Simple packet structure
type QueuePacket struct {
	Num  int
	Time float64
}

func (p *QueuePacket) isEnded() bool {
	return p.Num < 0
}

// Node types for simple creation
type NodeType string

const (
	GeneratorNode NodeType = "generator"
	QueueNode     NodeType = "queue"
	SinkNode      NodeType = "sink"
)

// Simple node configuration
type SimpleNode struct {
	Type   NodeType
	Params map[string]interface{}
}

// Create simple generator function
func createGenerator(params map[string]interface{}) func(chain.Chan, chain.Chan) bool {
	rate := 1.0
	count := 100
	
	if r, ok := params["rate"]; ok {
		if rf, ok := r.(float64); ok {
			rate = rf
		}
	}
	if c, ok := params["count"]; ok {
		if ci, ok := c.(int); ok {
			count = ci
		} else if cf, ok := c.(float64); ok {
			count = int(cf)
		}
	}
	
	t, j := 0.0, 0
	return func(_, out chain.Chan) bool {
		if j++; j >= count {
			out <- QueuePacket{-1, 0}
			fmt.Printf("[GENERATOR] Generated %d packets\n", j-1)
			return true
		}
		t += rate * rand.ExpFloat64()
		out <- QueuePacket{j, t}
		return false
	}
}

// Create simple queue function
func createQueue(params map[string]interface{}) func(chain.Chan, chain.Chan) bool {
	serviceTime := 1.0
	queueName := "QUEUE"
	
	if st, ok := params["service_time"]; ok {
		if stf, ok := st.(float64); ok {
			serviceTime = stf
		}
	}
	if name, ok := params["name"]; ok {
		if nameStr, ok := name.(string); ok {
			queueName = nameStr
		}
	}
	
	tBuf, tSum := 0.0, 0.0
	nBuf := 0
	
	return func(in, out chain.Chan) bool {
		packet := (<-in).(QueuePacket)
		
		if packet.isEnded() {
			if nBuf > 0 {
				fmt.Printf("[%s] Service: %.2f, Processed: %d, Avg Wait Time: %.3f\n", 
					queueName, serviceTime, nBuf, tSum/float64(nBuf))
			}
			out <- packet
			return true
		}
		
		if packet.Time > tBuf {
			tBuf = packet.Time
		}
		tBuf += serviceTime * rand.ExpFloat64()
		
		out <- QueuePacket{packet.Num, tBuf}
		
		tSum += tBuf - packet.Time
		nBuf = packet.Num
		return false
	}
}

// Create simple sink function
func createSink(params map[string]interface{}) func(chain.Chan) bool {
	received := 0
	
	return func(in chain.Chan) bool {
		packet := (<-in).(QueuePacket)
		
		if packet.isEnded() {
			fmt.Printf("[SINK] Total received: %d packets\n", received)
			return true
		}
		
		received++
		return false
	}
}

// Simple node factory
func CreateNode(nodeType NodeType, params map[string]interface{}) interface{} {
	switch nodeType {
	case GeneratorNode:
		return createGenerator(params)
	case QueueNode:
		return createQueue(params)
	case SinkNode:
		return createSink(params)
	default:
		panic(fmt.Sprintf("Unknown node type: %s", nodeType))
	}
}

func main() {
	fmt.Println("Simple QNet Node Creation")
	fmt.Println("========================")
	
	// Create simple linear chain: Generator -> Queue -> Sink
	ch := chain.C_chain{}
	
	// Add generator
	genParams := map[string]interface{}{
		"rate":  2.0,
		"count": 1000,
	}
	genFunc := CreateNode(GeneratorNode, genParams).(func(chain.Chan, chain.Chan) bool)
	ch.Add(genFunc)
	
	// Add queue
	queueParams := map[string]interface{}{
		"service_time": 0.5,
	}
	queueFunc := CreateNode(QueueNode, queueParams).(func(chain.Chan, chain.Chan) bool)
	ch.Add(queueFunc)
	
	// Add sink
	sinkParams := map[string]interface{}{}
	sinkFunc := CreateNode(SinkNode, sinkParams).(func(chain.Chan) bool)
	ch.End(sinkFunc)
	
	fmt.Println("Running simple chain...")
	
	// Measure runtime
	startTime := time.Now()
	chain.Run()
	endTime := time.Now()
	
	duration := endTime.Sub(startTime)
	
	fmt.Println("Simple QNet completed successfully!")
	fmt.Printf("Runtime: %v\n", duration)
	fmt.Printf("Runtime (milliseconds): %.2f ms\n", float64(duration.Nanoseconds())/1000000.0)
	fmt.Printf("Runtime (seconds): %.6f s\n", duration.Seconds())
}