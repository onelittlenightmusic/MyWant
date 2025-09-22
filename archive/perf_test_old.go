package main

import (
	"fmt"
	"math/rand"
	"mywant/src/chain"
	"time"
)

type QueuePacketTuple struct {
	Num  int
	Time float64
}

func (t *QueuePacketTuple) isEnded() bool {
	return t.Num < 0
}

func createOldGenerator(rate float64, count int) func(chain.Chan, chain.Chan) bool {
	t, j := 0.0, 0
	return func(_, out chain.Chan) bool {
		if j++; j >= count {
			out <- QueuePacketTuple{-1, 0}
			return true
		}
		t += rate * rand.ExpFloat64()
		out <- QueuePacketTuple{j, t}
		return false
	}
}

func createOldQueue(serviceTime float64) func(chain.Chan, chain.Chan) bool {
	tBuf, tSum := 0.0, 0.0
	nBuf := 0
	return func(in, out chain.Chan) bool {
		t := (<-in).(QueuePacketTuple)
		if t.isEnded() {
			fmt.Printf("[OLD QUEUE] Service: %.2f, Processed: %d, Avg Delay: %.2f\n",
				serviceTime, nBuf, tSum/float64(nBuf))
			out <- t
			return true
		}
		if t.Time > tBuf {
			tBuf = t.Time
		}
		tBuf += serviceTime * rand.ExpFloat64()
		out <- QueuePacketTuple{t.Num, tBuf}
		tSum += tBuf - t.Time
		nBuf = t.Num
		return false
	}
}

func createOldSink() func(chain.Chan) bool {
	received := 0
	return func(in chain.Chan) bool {
		t := (<-in).(QueuePacketTuple)
		if t.isEnded() {
			fmt.Printf("[OLD SINK] Total received: %d packets\n", received)
			return true
		}
		received++
		return false
	}
}

func main() {
	fmt.Println("Performance Test: Old Implementation")
	fmt.Println("===================================")

	start := time.Now()

	// Create old-style chain
	ch := chain.C_chain{}
	ch.Add(createOldGenerator(3.0, 1000))
	ch.Add(createOldQueue(0.5))
	ch.Add(createOldQueue(0.7))
	ch.End(createOldSink())

	fmt.Println("Starting old implementation...")
	chain.Run()

	elapsed := time.Since(start)
	fmt.Printf("Old implementation completed in: %v\n", elapsed)
}
