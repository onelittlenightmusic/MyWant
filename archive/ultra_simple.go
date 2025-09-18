// Ultra-simple queueing network - minimal code possible
package main

import (
	"fmt"
	"mywant/src/chain"
)

// Data packet
type Data struct {
	ID   int
	Time float64
}

func (d Data) Done() bool { return d.ID < 0 }

// Create packets
func Source(count int) func(chain.Chan, chain.Chan) bool {
	n := 0
	return func(_, out chain.Chan) bool {
		if n >= count {
			out <- Data{-1, 0}
			return true
		}
		n++
		out <- Data{n, float64(n)}
		return false
	}
}

// Process with delay
func Process(delay float64) func(chain.Chan, chain.Chan) bool {
	return func(in, out chain.Chan) bool {
		d := (<-in).(Data)
		if d.Done() {
			fmt.Printf("[PROCESS] Delay: %.1f\n", delay)
			out <- d
			return true
		}
		d.Time += delay
		out <- d
		return false
	}
}

// End point
func End() func(chain.Chan) bool {
	return func(in chain.Chan) bool {
		d := (<-in).(Data)
		if !d.Done() {
			fmt.Printf("Packet %d at time %.1f\n", d.ID, d.Time)
		}
		return d.Done()
	}
}

func main() {
	fmt.Println("Ultra-Simple Network")

	// Manual chain building (no config needed)
	var c chain.C_chain
	c.Add(Source(5))    // Generate 5 packets
	c.Add(Process(1.0)) // Add 1.0 delay
	c.Add(Process(2.0)) // Add 2.0 delay
	c.End(End())        // Print results

	chain.Run()
	fmt.Println("Done!")
}
