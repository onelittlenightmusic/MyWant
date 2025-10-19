package types

import (
	"fmt"
	"math"
	"math/rand"
	mywant "mywant/engine/src"
	"mywant/engine/src/chain"
)

// QueuePacket represents data flowing through the chain
type QueuePacket struct {
	mywant.BasePacket
	Num  int
	Time float64
}

// IsEnded implements Packet interface with QueuePacket-specific logic
func (p *QueuePacket) IsEnded() bool {
	return p.Num < 0 || p.BasePacket.IsEnded()
}

// GetData returns the packet's queue-specific data
func (p *QueuePacket) GetData() interface{} {
	return struct {
		Num  int
		Time float64
	}{p.Num, p.Time}
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
			// Stats field removed - using State instead
			Status: mywant.WantStatusIdle,
			State:  make(map[string]interface{}),
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
		RequiredInputs:  0,  // Generators don't need using
		RequiredOutputs: 1,  // Must have at least one output
		MaxInputs:       0,  // No using allowed
		MaxOutputs:      -1, // Unlimited outputs
		WantType:        "sequence",
		Description:     "Packet generator want",
	}
}

// GetStats returns the stats for this numbers generator
func (g *Numbers) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return g.State
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

// Exec executes the numbers generator directly with dynamic parameter reading
func (g *Numbers) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Read parameters fresh each cycle - this enables dynamic param changes!
	useDeterministic := false
	if det, ok := g.Spec.Params["deterministic"]; ok {
		if detBool, ok := det.(bool); ok {
			useDeterministic = detBool
		} else if detStr, ok := det.(string); ok {
			useDeterministic = (detStr == "true")
		}
	}

	// Read count and rate parameters fresh each cycle
	currentCount := g.Count // Default fallback
	if c, ok := g.Spec.Params["count"]; ok {
		if ci, ok := c.(int); ok {
			currentCount = ci
		} else if cf, ok := c.(float64); ok {
			currentCount = int(cf)
		}
	}

	currentRate := g.Rate // Default fallback
	if r, ok := g.Spec.Params["rate"]; ok {
		if rf, ok := r.(float64); ok {
			currentRate = rf
		}
	}

	// Initialize state variables if not present
	if g.State == nil {
		g.State = make(map[string]interface{})
	}

	// Get current time and count from state (persistent across calls)
	t, _ := g.State["current_time"].(float64)
	j, _ := g.State["current_count"].(int)

	if len(outputs) == 0 {
		return true
	}
	out := outputs[0]

	if j >= currentCount {
		// Store generation stats
		g.StoreState("total_processed", j)
		g.StoreState("average_wait_time", 0.0) // Generators don't have wait time
		g.StoreState("total_wait_time", 0.0)

		out <- QueuePacket{Num: -1, Time: 0}
		fmt.Printf("[GENERATOR] Generated %d packets\n", j)
		return true
	}
	j++

	if useDeterministic {
		// Deterministic inter-arrival time (rate = 1/interval)
		t += 1.0 / currentRate
	} else {
		// Exponential inter-arrival time (rate = 1/mean_interval)
		// ExpRand64() returns Exp(1), so divide by rate to get correct mean interval
		t += ExpRand64() / currentRate
	}

	// Store state for next call
	g.StoreState("current_time", t)
	g.StoreState("current_count", j)

	out <- QueuePacket{Num: j, Time: t}
	return false
}

// Queue processes packets with a service time
type Queue struct {
	mywant.Want
	ServiceTime float64
	paths       mywant.Paths
	// Batch mechanism for state updates
	batchSize           int
	batchUpdateInterval int
	lastBatchCount      int
}

// NewQueue creates a new queue want
func NewQueue(metadata mywant.Metadata, spec mywant.WantSpec) *Queue {
	queue := &Queue{
		Want: mywant.Want{
			Metadata: metadata,
			Spec:     spec,
			// Stats field removed - using State instead
			Status: mywant.WantStatusIdle,
			State:  make(map[string]interface{}),
		},
		ServiceTime:         1.0,
		batchUpdateInterval: 100, // Default: update state every 100 packets
		lastBatchCount:      0,
	}

	if st, ok := spec.Params["service_time"]; ok {
		if stf, ok := st.(float64); ok {
			queue.ServiceTime = stf
		}
	}

	// Allow configurable batch update interval
	if batchInterval, ok := spec.Params["batch_interval"]; ok {
		if bi, ok := batchInterval.(float64); ok {
			queue.batchUpdateInterval = int(bi)
		} else if bi, ok := batchInterval.(int); ok {
			queue.batchUpdateInterval = bi
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
		RequiredInputs:  1,  // Queues need at least one using
		RequiredOutputs: 1,  // Must have at least one output
		MaxInputs:       1,  // Only one using supported
		MaxOutputs:      -1, // Unlimited outputs
		WantType:        "queue",
		Description:     "Queue processing want",
	}
}

// GetStats returns the stats for this queue
func (q *Queue) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return q.State
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

// Exec executes the queue processing directly with batch mechanism
func (q *Queue) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Using direct Exec approach for dynamic parameter reading
	if q.State == nil {
		q.State = make(map[string]interface{})
	}

	// Initialize persistent state variables
	serverFreeTime, _ := q.State["serverFreeTime"].(float64)
	waitTimeSum, _ := q.State["waitTimeSum"].(float64)
	processedCount, _ := q.State["processedCount"].(int)

	if len(using) == 0 || len(outputs) == 0 {
		return true
	}
	in := using[0]
	out := outputs[0]

	packet := (<-in).(QueuePacket)

	// Check for termination packet and forward it
	if packet.IsEnded() {
		// Always flush batch and store final state when terminating
		q.flushBatch(serverFreeTime, waitTimeSum, processedCount)

		// Trigger OnEnded callback
		if err := q.OnEnded(&packet); err != nil {
			fmt.Printf("[QUEUE] OnEnded callback error: %v\n", err)
		}
		// Forward end signal to next want
		out <- packet
		return true
	}

	// Process the packet...
	arrivalTime := packet.Time
	startServiceTime := math.Max(arrivalTime, serverFreeTime)
	waitTime := startServiceTime - arrivalTime

	// Read service time from parameters (can change dynamically!)
	serviceTime := q.ServiceTime
	if st, ok := q.Spec.Params["service_time"]; ok {
		if stFloat, ok := st.(float64); ok {
			serviceTime = stFloat
		}
	}

	finishTime := startServiceTime + serviceTime
	serverFreeTime = finishTime

	waitTimeSum += waitTime
	processedCount++

	// Store packet-specific info (always, even in batch mode)
	q.StoreState("last_packet_wait_time", waitTime)

	// Batch mechanism: only update statistics every N packets
	if processedCount%q.batchUpdateInterval == 0 {
		q.flushBatch(serverFreeTime, waitTimeSum, processedCount)
		q.lastBatchCount = processedCount
	}

	out <- QueuePacket{Num: packet.Num, Time: finishTime}
	return false
}

// flushBatch commits all accumulated statistics to state
func (q *Queue) flushBatch(serverFreeTime, waitTimeSum float64, processedCount int) {
	// Calculate average wait time
	avgWaitTime := 0.0
	if processedCount > 0 {
		avgWaitTime = waitTimeSum / float64(processedCount)
	}

	// Batch update all statistics at once
	q.StoreState("serverFreeTime", serverFreeTime)
	q.StoreState("waitTimeSum", waitTimeSum)
	q.StoreState("processedCount", processedCount)
	q.StoreState("average_wait_time", avgWaitTime)
	q.StoreState("total_processed", processedCount)
	q.StoreState("total_wait_time", waitTimeSum)
	q.StoreState("current_server_free_time", serverFreeTime)
}

// OnEnded implements PacketHandler interface for packet termination callbacks
func (q *Queue) OnEnded(packet mywant.Packet) error {
	// Extract queue-specific statistics from state
	waitTimeSum, _ := q.State["waitTimeSum"].(float64)
	processedCount, _ := q.State["processedCount"].(int)
	serverFreeTime, _ := q.State["serverFreeTime"].(float64)

	// Calculate final statistics
	avgWaitTime := 0.0
	if processedCount > 0 {
		avgWaitTime = waitTimeSum / float64(processedCount)
	}

	// Store final state
	q.StoreState("average_wait_time", avgWaitTime)
	q.StoreState("total_processed", processedCount)
	q.StoreState("total_wait_time", waitTimeSum)
	q.StoreState("current_server_free_time", serverFreeTime)

	fmt.Printf("[QUEUE] Processed %d packets, avg wait time: %.6f\n", processedCount, avgWaitTime)
	return nil
}

// Combiner merges multiple using streams
type Combiner struct {
	mywant.Want
	Operation string
	paths     mywant.Paths
}

func NewCombiner(metadata mywant.Metadata, spec mywant.WantSpec) *Combiner {
	combiner := &Combiner{
		Want: mywant.Want{
			Metadata: metadata,
			Spec:     spec,
			// Stats field removed - using State instead
			Status: mywant.WantStatusIdle,
			State:  make(map[string]interface{}),
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
		RequiredInputs:  2,  // Combiners need at least two using
		RequiredOutputs: 1,  // Must have at least one output
		MaxInputs:       -1, // Unlimited using
		MaxOutputs:      -1, // Unlimited outputs
		WantType:        "combiner",
		Description:     "Stream combiner want",
	}
}

// GetStats returns the stats for this combiner
func (c *Combiner) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return c.State
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

// Exec executes the combiner directly
func (c *Combiner) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// Initialize state if needed
	if c.State == nil {
		c.State = make(map[string]interface{})
	}

	// Get persistent state
	processed, _ := c.State["processed"].(int)

	if len(using) == 0 || len(outputs) == 0 {
		return true
	}
	out := outputs[0]

	// Simple combiner: just forward all packets from all inputs
	for _, in := range using {
		select {
		case packet, ok := <-in:
			if !ok {
				continue
			}
			qp := packet.(QueuePacket)

			// Check for termination packet and forward it
			if qp.IsEnded() {
				// Trigger OnEnded callback
				if err := c.OnEnded(&qp); err != nil {
					fmt.Printf("[COMBINER] OnEnded callback error: %v\n", err)
				}
				// Forward end signal to next want
				out <- qp
				return true
			}

			processed++
			out <- qp
		default:
			// No data available on this channel right now
		}
	}

	c.StoreState("processed", processed)
	return false
}

// OnEnded implements PacketHandler interface for Combiner termination callbacks
func (c *Combiner) OnEnded(packet mywant.Packet) error {
	// Extract combiner-specific statistics from state
	processed, _ := c.State["processed"].(int)

	// Store final state
	c.StoreState("total_processed", processed)
	c.StoreState("average_wait_time", 0.0) // Combiners don't add wait time
	c.StoreState("total_wait_time", 0.0)

	fmt.Printf("[COMBINER] Processed %d packets\n", processed)
	return nil
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
			// Stats field removed - using State instead
			Status: mywant.WantStatusIdle,
			State:  make(map[string]interface{}),
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
		RequiredInputs:  1,  // Sinks need at least one using
		RequiredOutputs: 0,  // Sinks don't need outputs
		MaxInputs:       -1, // Unlimited using
		MaxOutputs:      0,  // No outputs allowed
		WantType:        "sink",
		Description:     "Data sink/collector want",
	}
}

// GetStats returns the stats for this sink
func (s *Sink) GetStats() map[string]interface{} {
	// Stats are now dynamic, just return the map directly
	return s.State
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

// Exec executes the sink directly
func (s *Sink) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	// If no using configured, this sink shouldn't run
	if len(using) == 0 {
		return true
	}
	in := using[0]

	// Block waiting for data from using channel
	packet := (<-in).(QueuePacket)

	// Check for termination packet
	if packet.IsEnded() {
		// Trigger OnEnded callback
		if err := s.OnEnded(&packet); err != nil {
			fmt.Printf("[SINK] OnEnded callback error: %v\n", err)
		}
		return true
	}

	s.Received++
	return false // Continue waiting for more packets
}

// OnEnded implements PacketHandler interface for Sink termination callbacks
func (s *Sink) OnEnded(packet mywant.Packet) error {
	// Store final state
	s.StoreState("total_processed", s.Received)
	s.StoreState("average_wait_time", 0.0) // Sinks don't add wait time
	s.StoreState("total_wait_time", 0.0)

	fmt.Printf("[SINK] Received %d packets\n", s.Received)
	return nil
}

// RegisterQNetWantTypes registers the qnet-specific want types with a mywant.ChainBuilder
func RegisterQNetWantTypes(builder *mywant.ChainBuilder) {
	// Register numbers generator type - return the enhanced want itself for validation
	builder.RegisterWantType("qnet numbers", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return PacketNumbers(metadata, spec)
	})

	// Register queue type - return the enhanced want itself for validation
	builder.RegisterWantType("qnet queue", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return NewQueue(metadata, spec)
	})

	// Register combiner type - return the enhanced want itself for validation
	builder.RegisterWantType("qnet combiner", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return NewCombiner(metadata, spec)
	})

	// Register sink type - return the enhanced want itself for validation
	builder.RegisterWantType("qnet sink", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return Goal(metadata, spec)
	})

	// Register collector type (alias for sink) - return the enhanced want itself for validation
	builder.RegisterWantType("qnet collector", func(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
		return Goal(metadata, spec)
	})
}
