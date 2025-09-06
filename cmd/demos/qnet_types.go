package main

import (
	"fmt"
	"math"
	"math/rand"
	mywant "mywant/src"
	"mywant/src/chain"
)

// QueuePacket represents data flowing through the chain
type QueuePacket struct {
	Num  int
	Time float64
}

func (p *QueuePacket) isEnded() bool {
	return p.Num < 0
}

// ExpRand64 generates exponentially distributed random numbers with improved precision
// Uses inverse transform sampling with better numerical stability than standard library
func ExpRand64() float64 {
	// Generate uniform random number in (0,1) avoiding exactly 0 and 1
	u := rand.Float64()
	
	// Handle edge cases for better numerical stability
	if u == 0.0 {
		u = math.SmallestNonzeroFloat64
	} else if u == 1.0 {
		u = 1.0 - math.SmallestNonzeroFloat64
	}
	
	// Use inverse transform: -ln(u) for exponential distribution
	// This provides better distribution than the standard library's algorithm
	return -math.Log(u)
}


// Numbers creates packets and sends them downstream
type Numbers struct {
	mywant.Want
	Rate  float64
	Count int
	paths mywant.Paths
}

// PacketNumbers creates a new numbers want
func PacketNumbers(metadata mywant.Metadata, spec mywant.WantSpec) *Numbers {
	gen := &Numbers{
		Want: mywant.Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    mywant.WantStats{},
			Status:   mywant.WantStatusIdle,
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
		}
	}
	
	return gen
}

// InitializePaths initializes the paths for this numbers generator
func (g *Numbers) InitializePaths(inCount, outCount int) {
	g.paths.In = make([]mywant.PathInfo, inCount)
	g.paths.Out = make([]mywant.PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for numbers generator
func (g *Numbers) GetConnectivityMetadata() mywant.ConnectivityMetadata {
	return mywant.ConnectivityMetadata{
		RequiredInputs:  0, // Generators don't need using
		RequiredOutputs: 1, // Must have at least one output
		MaxInputs:       0, // No using allowed
		MaxOutputs:      -1, // Unlimited outputs
		WantType:        "sequence",
		Description:     "Packet generator want",
	}
}

// GetStats returns the stats for this numbers generator
func (g *Numbers) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return g.Stats
}

// Process processes using enhanced paths (for enhanced node compatibility)
func (g *Numbers) Process(paths mywant.Paths) bool {
	g.paths = paths
	return false // Not used in current implementation
}

// GetType returns the want type
func (g *Numbers) GetType() string {
	return "numbers"
}

// Getmywant.Want returns the embedded mywant.Want
func (g *Numbers) GetWant() *mywant.Want {
	return &g.Want
}

// CreateFunction returns the generalized chain function for this numbers generator
func (g *Numbers) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	t, j := 0.0, 0
	
	// Check for deterministic mode in parameters
	useDeterministic := false
	if det, ok := g.Spec.Params["deterministic"]; ok {
		if detBool, ok := det.(bool); ok {
			useDeterministic = detBool
		} else if detStr, ok := det.(string); ok {
			useDeterministic = (detStr == "true")
		}
	}
	
	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]
		
		if j >= g.Count {
			// Store generation stats
			// Initialize stats map if not exists
			if g.Stats == nil {
				g.Stats = make(mywant.WantStats)
			}
			g.Stats["total_processed"] = j
			g.Stats["average_wait_time"] = 0.0 // Generators don't have wait time
			g.Stats["total_wait_time"] = 0.0
			
			out <- QueuePacket{-1, 0}
			fmt.Printf("[GENERATOR] Generated %d packets\n", j)
			return true
		}
		j++
		
		if useDeterministic {
			// Deterministic inter-arrival time (rate = 1/interval)
			t += 1.0 / g.Rate
		} else {
			// Exponential inter-arrival time (rate = 1/mean_interval)
			// ExpRand64() returns Exp(1), so divide by rate to get correct mean interval
			t += ExpRand64() / g.Rate
		}
		
		out <- QueuePacket{j, t}
		return false
	}
}

// Queue processes packets with a service time
type Queue struct {
	mywant.Want
	ServiceTime float64
	paths       mywant.Paths
}

// NewQueue creates a new queue want
func NewQueue(metadata mywant.Metadata, spec mywant.WantSpec) *Queue {
	queue := &Queue{
		Want: mywant.Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    mywant.WantStats{},
			Status:   mywant.WantStatusIdle,
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
	q.paths.In = make([]mywant.PathInfo, inCount)
	q.paths.Out = make([]mywant.PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for queue
func (q *Queue) GetConnectivityMetadata() mywant.ConnectivityMetadata {
	return mywant.ConnectivityMetadata{
		RequiredInputs:  1, // Queues need at least one using
		RequiredOutputs: 1, // Must have at least one output
		MaxInputs:       1, // Only one using supported
		MaxOutputs:      -1, // Unlimited outputs
		WantType:        "queue",
		Description:     "Queue processing want",
	}
}

// GetStats returns the stats for this queue
func (q *Queue) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return q.Stats
}

// Process processes using enhanced paths
func (q *Queue) Process(paths mywant.Paths) bool {
	q.paths = paths
	return false // Not used in current implementation
}

// GetType returns the want type
func (q *Queue) GetType() string {
	return "queue"
}

// Getmywant.Want returns the embedded mywant.Want
func (q *Queue) GetWant() *mywant.Want {
	return &q.Want
}

// CreateFunction returns the generalized chain function for this queue
func (q *Queue) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	serverFreeTime, waitTimeSum := 0.0, 0.0
	processedCount := 0
	
	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(using) == 0 || len(outputs) == 0 {
			return true
		}
		in := using[0]
		out := outputs[0]
		
		packet := (<-in).(QueuePacket)
		
		if packet.isEnded() {
			if processedCount > 0 {
				avgWaitTime := waitTimeSum / float64(processedCount)
				// Store stats in the mywant.Want
				// Initialize stats map if not exists
				if q.Stats == nil {
					q.Stats = make(mywant.WantStats)
				}
				q.Stats["average_wait_time"] = avgWaitTime
				q.Stats["total_processed"] = processedCount
				q.Stats["total_wait_time"] = waitTimeSum
				
				fmt.Printf("[QUEUE %s] Service: %.2f, Processed: %d, Avg Wait: %.6f\n", 
					q.Metadata.Name, q.ServiceTime, processedCount, avgWaitTime)
			}
			out <- packet
			return true
		}
		
		// Correct M/M/1 queue implementation
		arrivalTime := packet.Time
		serviceStartTime := arrivalTime
		if serverFreeTime > arrivalTime {
			serviceStartTime = serverFreeTime  // Must wait for server
		}
		
		// Calculate actual wait time (time spent waiting, not including service)
		waitTime := serviceStartTime - arrivalTime
		
		// Generate service time (deterministic or exponential)
		useDeterministic := false
		if det, ok := q.Spec.Params["deterministic"]; ok {
			if detBool, ok := det.(bool); ok {
				useDeterministic = detBool
			} else if detStr, ok := det.(string); ok {
				useDeterministic = (detStr == "true")
			}
		}
		
		var serviceTime float64
		if useDeterministic {
			// Deterministic service time (ServiceTime as mean)
			serviceTime = q.ServiceTime
		} else {
			// Exponential service time (ServiceTime as mean service time)
			// ExpRand64() returns Exp(1), so scale to get mean = ServiceTime
			serviceTime = q.ServiceTime * ExpRand64()
		}
		
		// Calculate departure time
		departureTime := serviceStartTime + serviceTime
		
		
		// Update server availability
		serverFreeTime = departureTime
		
		// Send packet with departure time
		out <- QueuePacket{packet.Num, departureTime}
		
		// Debug wait time distribution (commented out to reduce output)
		// if packet.Num <= 10 || waitTime > 10.0 {
		//	fmt.Printf("[DEBUG] Packet %d: wait=%.6f, arrival=%.6f, serverFree=%.6f, service=%.6f, q.ServiceTime=%.6f\n", 
		//		packet.Num, waitTime, arrivalTime, serverFreeTime, serviceTime, q.ServiceTime)
		// }
		
		// Accumulate wait time statistics
		waitTimeSum += waitTime
		processedCount = packet.Num
		
		return false
	}
}

// Combiner merges multiple using streams
type Combiner struct {
	mywant.Want
	Operation string
	paths     mywant.Paths
}

// NewCombiner creates a new combiner want
func NewCombiner(metadata mywant.Metadata, spec mywant.WantSpec) *Combiner {
	combiner := &Combiner{
		Want: mywant.Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    mywant.WantStats{},
			Status:   mywant.WantStatusIdle,
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
	c.paths.In = make([]mywant.PathInfo, inCount)
	c.paths.Out = make([]mywant.PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for combiner
func (c *Combiner) GetConnectivityMetadata() mywant.ConnectivityMetadata {
	return mywant.ConnectivityMetadata{
		RequiredInputs:  2, // Combiners need at least two using
		RequiredOutputs: 1, // Must have at least one output
		MaxInputs:       -1, // Unlimited using
		MaxOutputs:      -1, // Unlimited outputs
		WantType:        "combiner",
		Description:     "Stream combiner want",
	}
}

// GetStats returns the stats for this combiner
func (c *Combiner) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return c.Stats
}

// Process processes using enhanced paths
func (c *Combiner) Process(paths mywant.Paths) bool {
	c.paths = paths
	return false // Not used in current implementation
}

// GetType returns the want type
func (c *Combiner) GetType() string {
	return "combiner"
}

// Getmywant.Want returns the embedded mywant.Want
func (c *Combiner) GetWant() *mywant.Want {
	return &c.Want
}

// CreateFunction returns the generalized chain function for this combiner
func (c *Combiner) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	processed := 0
	usingsClosed := make([]bool, 0) // Track which using are closed
	packetBuffer := make([]QueuePacket, 0) // Buffer to store packets not yet sent
	stuckCount := 0 // Track iterations with no progress
	
	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(using) == 0 || len(outputs) == 0 {
			return true
		}
		out := outputs[0]
		
		// Initialize closed tracking if needed
		if len(usingsClosed) != len(using) {
			usingsClosed = make([]bool, len(using))
		}
		
		// Check if all using are closed and buffer is empty
		allClosed := true
		for _, closed := range usingsClosed {
			if !closed {
				allClosed = false
				break
			}
		}
		if allClosed && len(packetBuffer) == 0 {
			// Store combiner stats
			// Initialize stats map if not exists
			if c.Stats == nil {
				c.Stats = make(mywant.WantStats)
			}
			c.Stats["total_processed"] = processed
			c.Stats["average_wait_time"] = 0.0 // Combiners don't add wait time
			c.Stats["total_wait_time"] = 0.0
			
			fmt.Printf("[COMBINER] Operation: %s, Processed %d packets\n", c.Operation, processed)
			out <- QueuePacket{-1, 0} // Send end signal
			return true
		}
		
		// Collect available packets from all open using and add to buffer
		madeProgress := false
		for i, input := range using {
			if usingsClosed[i] {
				continue // Skip closed using
			}
			
			select {
			case packet := <-input:
				qPacket := packet.(QueuePacket)
				if qPacket.isEnded() {
					usingsClosed[i] = true // Mark this using as closed
					madeProgress = true
				} else {
					packetBuffer = append(packetBuffer, qPacket)
					madeProgress = true
				}
			default:
				// No packet available on this using right now
			}
		}
		
		// If buffer is empty and not all using closed, and no progress was made,
		// try a blocking read with timeout behavior
		if len(packetBuffer) == 0 && !allClosed && !madeProgress {
			stuckCount++
			
			// If we've been stuck too many times, force termination
			if stuckCount > 10000 {
				fmt.Printf("[COMBINER] Warning: Terminating due to apparent deadlock after %d stuck iterations\n", stuckCount)
				// Mark all remaining inputs as closed to force termination
				for i := range usingsClosed {
					usingsClosed[i] = true
				}
				// Send end signal
				// Initialize stats map if not exists
				if c.Stats == nil {
					c.Stats = make(mywant.WantStats)
				}
				c.Stats["total_processed"] = processed
				out <- QueuePacket{-1, 0}
				return true
			}
		} else if madeProgress {
			stuckCount = 0 // Reset stuck counter when progress is made
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
	mywant.Want
	Received int
	paths    mywant.Paths
}

// Goal creates a new sink want
func Goal(metadata mywant.Metadata, spec mywant.WantSpec) *Sink {
	return &Sink{
		Want: mywant.Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    mywant.WantStats{},
			Status:   mywant.WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		Received: 0,
	}
}

// InitializePaths initializes the paths for this sink
func (s *Sink) InitializePaths(inCount, outCount int) {
	s.paths.In = make([]mywant.PathInfo, inCount)
	s.paths.Out = make([]mywant.PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for sink
func (s *Sink) GetConnectivityMetadata() mywant.ConnectivityMetadata {
	return mywant.ConnectivityMetadata{
		RequiredInputs:  1, // Sinks need at least one using
		RequiredOutputs: 0, // Sinks don't need outputs
		MaxInputs:       -1, // Unlimited using
		MaxOutputs:      0, // No outputs allowed
		WantType:        "sink",
		Description:     "Data sink/collector want",
	}
}

// GetStats returns the stats for this sink
func (s *Sink) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return s.Stats
}

// Process processes using enhanced paths
func (s *Sink) Process(paths mywant.Paths) bool {
	s.paths = paths
	return false // Not used in current implementation
}

// GetType returns the want type
func (s *Sink) GetType() string {
	return "sink"
}

// Getmywant.Want returns the embedded mywant.Want
func (s *Sink) GetWant() *mywant.Want {
	return &s.Want
}

// CreateFunction returns the generalized chain function for this sink
func (s *Sink) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	return func(using []chain.Chan, outputs []chain.Chan) bool {
		// If no using configured, this sink shouldn't run
		if len(using) == 0 {
			return true
		}
		in := using[0]
		
		// Block waiting for data from using channel
		packet := (<-in).(QueuePacket)
		
		if packet.isEnded() {
			// Store sink stats
			// Initialize stats map if not exists
			if s.Stats == nil {
				s.Stats = make(mywant.WantStats)
			}
			s.Stats["total_processed"] = s.Received
			s.Stats["average_wait_time"] = 0.0 // Sinks don't add wait time
			s.Stats["total_wait_time"] = 0.0
			
			fmt.Printf("[SINK] Total received: %d packets\n", s.Received)
			return true
		}
		
		s.Received++
		return false
	}
}

// RegisterQNetWantTypes registers the qnet-specific want types with a mywant.ChainBuilder
func RegisterQNetWantTypes(builder *mywant.ChainBuilder) {
	// Register numbers generator type - return the enhanced want itself for validation
	builder.RegisterWantType("numbers", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return PacketNumbers(metadata, spec)
	})
	
	// Register queue type - return the enhanced want itself for validation
	builder.RegisterWantType("queue", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return NewQueue(metadata, spec)
	})
	
	// Register combiner type - return the enhanced want itself for validation
	builder.RegisterWantType("combiner", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return NewCombiner(metadata, spec)
	})
	
	// Register sink type - return the enhanced want itself for validation
	builder.RegisterWantType("sink", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return Goal(metadata, spec)
	})
	
	// Register collector type (alias for sink) - return the enhanced want itself for validation
	builder.RegisterWantType("collector", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return Goal(metadata, spec)
	})
}

