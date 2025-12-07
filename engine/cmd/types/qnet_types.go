package types

import (
	"fmt"
	"math"
	"math/rand"
	mywant "mywant/engine/src"
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
	Rate                float64
	Count               int
	paths               mywant.Paths
	batchUpdateInterval int     // Batch interval for state history recording
	cycleCount          int     // Track cycles for history recording intervals
	currentTime         float64 // Local state: current simulation time
	currentCount        int     // Local state: current packet count
}

// PacketNumbers creates a new numbers want
func PacketNumbers(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
	gen := &Numbers{
		Want:                mywant.Want{},
		Rate:                1.0,
		Count:               100,
		batchUpdateInterval: 100, // Default: update state every 100 packets
		cycleCount:          0,
	}

	// Initialize base Want fields
	gen.Init(metadata, spec)

	gen.Rate = gen.GetFloatParam("rate", 1.0)
	gen.Count = gen.GetIntParam("count", 100)

	// Allow configurable batch update interval
	gen.batchUpdateInterval = gen.GetIntParam("batch_interval", 100)

	// Set fields for base Want methods
	gen.WantType = "sequence"
	gen.ConnectivityMetadata = mywant.ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       0,
		MaxOutputs:      -1,
		WantType:        "sequence",
		Description:     "Packet generator want",
	}

	return gen
}

// Getmywant.Want returns the embedded mywant.Want
func (g *Numbers) GetWant() *mywant.Want {
	return &g.Want
}

// Exec executes the numbers generator directly with dynamic parameter reading
func (g *Numbers) Exec() bool {
	// Read parameters fresh each cycle - this enables dynamic param changes!
	useDeterministic := g.GetBoolParam("deterministic", false)

	// Read count and rate parameters fresh each cycle
	paramCount := g.GetIntParam("count", g.Count)

	paramRate := g.GetFloatParam("rate", g.Rate)

	// Initialize state map if not present
	if g.State == nil {
		g.State = make(map[string]interface{})
	}

	if g.currentCount >= paramCount {
		// Store generation stats to state (for memory dump)
		g.StoreStateMulti(map[string]interface{}{
			"total_processed":      g.currentCount,
			"average_wait_time":    0.0, // Generators don't have wait time
			"total_wait_time":      0.0,
			"current_time":         g.currentTime,
			"current_count":        g.currentCount,
			"achieving_percentage": 100,
		})

		g.SendPacketMulti(QueuePacket{Num: -1, Time: 0})
		return true
	}
	g.currentCount++

	if useDeterministic {
		// Deterministic inter-arrival time (rate = 1/interval)
		g.currentTime += 1.0 / paramRate
	} else {
		// Exponential inter-arrival time (rate = 1/mean_interval)
		// ExpRand64() returns Exp(1), so divide by rate to get correct mean interval
		g.currentTime += ExpRand64() / paramRate
	}

	// Increment cycle counter for batching history entries
	g.cycleCount++

	// Batch mechanism: only update state history every N packets to reduce history entries
	if g.currentCount%g.batchUpdateInterval == 0 {
		g.StoreStateMulti(map[string]interface{}{
			"current_time":  g.currentTime,
			"current_count": g.currentCount,
		})
	}

	g.SendPacketMulti(QueuePacket{Num: g.currentCount, Time: g.currentTime})
	return false
}

// CalculateAchievingPercentage calculates the progress toward completion for Numbers generator
// Returns (currentCount / targetCount) * 100
func (g *Numbers) CalculateAchievingPercentage() int {
	paramCount := g.GetIntParam("count", g.Count)
	if paramCount <= 0 {
		return 0
	}
	percentage := (g.currentCount * 100) / paramCount
	if percentage > 100 {
		percentage = 100
	}
	return percentage
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
	cycleCount          int // Track cycles for history recording intervals
	// Local persistent state (not in State map, survives across cycles)
	serverFreeTime float64 // When the server will be free
	waitTimeSum    float64 // Accumulated wait time
	processedCount int     // Number of packets processed
}

// NewQueue creates a new queue want
func NewQueue(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
	queue := &Queue{
		Want:                mywant.Want{},
		ServiceTime:         1.0,
		batchUpdateInterval: 100, // Default: update state every 100 packets
		lastBatchCount:      0,
	}

	// Initialize base Want fields
	queue.Init(metadata, spec)

	queue.ServiceTime = queue.GetFloatParam("service_time", 1.0)

	// Allow configurable batch update interval
	queue.batchUpdateInterval = queue.GetIntParam("batch_interval", 100)

	// Set fields for base Want methods
	queue.WantType = "queue"
	queue.ConnectivityMetadata = mywant.ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      -1,
		WantType:        "queue",
		Description:     "Queue processing want",
	}

	return queue
}

// Getmywant.Want returns the embedded mywant.Want
func (q *Queue) GetWant() *mywant.Want {
	return &q.Want
}

// Exec executes the queue processing directly with batch mechanism
func (q *Queue) Exec() bool {
	// Using direct Exec approach for dynamic parameter reading
	if q.State == nil {
		q.State = make(map[string]interface{})
	}

	// Local persistent state variables are used instead of State map
	// This ensures they persist across cycles without batching interference

	// Get input channel
	in, connectionAvailable := q.GetInputChannel(0)
	if !connectionAvailable {
		return true
	}

	packet := (<-in).(QueuePacket)

	// Check for termination packet and forward it
	if packet.IsEnded() {
		// Always flush batch and store final state when terminating
		q.flushBatch()

		// Trigger OnEnded callback
		if err := q.OnEnded(&packet); err != nil {
			q.StoreLog(fmt.Sprintf("OnEnded callback error: %v", err))
		}
		// Forward end signal to next want
		q.SendPacketMulti(packet)
		return true
	}

	// Process the packet...
	arrivalTime := packet.Time
	startServiceTime := math.Max(arrivalTime, q.serverFreeTime)
	waitTime := startServiceTime - arrivalTime

	// Read service time from parameters (can change dynamically!)
	serviceTime := q.GetFloatParam("service_time", q.ServiceTime)

	finishTime := startServiceTime + serviceTime
	q.serverFreeTime = finishTime

	q.waitTimeSum += waitTime
	q.processedCount++

	// Increment cycle counter for batching history entries
	q.cycleCount++

	// Batch mechanism: only update statistics every N packets
	if q.processedCount%q.batchUpdateInterval == 0 {
		// Store packet-specific info only at batch intervals to reduce history entries
		q.StoreState("last_packet_wait_time", waitTime)
		q.flushBatch()
		q.lastBatchCount = q.processedCount
	}

	q.SendPacketMulti(QueuePacket{Num: packet.Num, Time: finishTime})
	return false
}

// flushBatch commits all accumulated statistics to state
func (q *Queue) flushBatch() {
	// Calculate average wait time
	avgWaitTime := 0.0
	if q.processedCount > 0 {
		avgWaitTime = q.waitTimeSum / float64(q.processedCount)
	}

	// Batch update all statistics at once
	q.StoreStateMulti(map[string]interface{}{
		"serverFreeTime":           q.serverFreeTime,
		"waitTimeSum":              q.waitTimeSum,
		"processedCount":           q.processedCount,
		"average_wait_time":        avgWaitTime,
		"total_processed":          q.processedCount,
		"total_wait_time":          q.waitTimeSum,
		"current_server_free_time": q.serverFreeTime,
	})
}

// OnEnded implements PacketHandler interface for packet termination callbacks
func (q *Queue) OnEnded(packet mywant.Packet) error {
	// Calculate final statistics from local persistent state
	avgWaitTime := 0.0
	if q.processedCount > 0 {
		avgWaitTime = q.waitTimeSum / float64(q.processedCount)
	}

	// Store final state
	q.StoreStateMulti(map[string]interface{}{
		"average_wait_time":        avgWaitTime,
		"total_processed":          q.processedCount,
		"total_wait_time":          q.waitTimeSum,
		"current_server_free_time": q.serverFreeTime,
		"achieving_percentage":     100,
	})

	return nil
}

// CalculateAchievingPercentage calculates the progress toward completion for Queue
// For streaming queue, returns 100 when complete (all packets processed)
// During streaming, this is calculated indirectly through packet count tracking
func (q *Queue) CalculateAchievingPercentage() int {
	// Queue is a streaming processor - returns 100 when termination is received
	// The percentage is implicitly tracked by processedCount during streaming
	completed, _ := q.GetStateBool("completed", false)
	if completed {
		return 100
	}
	// For streaming queue, we can't easily determine total expected packets
	// So we return percentage based on whether any packets have been processed
	if q.processedCount > 0 {
		// Streaming mode: return 100 only when complete
		return 50 // Indicate partial progress while streaming
	}
	return 0
}

// Combiner merges multiple using streams
type Combiner struct {
	mywant.Want
	Operation string
	paths     mywant.Paths
}

func NewCombiner(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
	combiner := &Combiner{
		Want:      mywant.Want{},
		Operation: "merge",
	}

	// Initialize base Want fields
	combiner.Init(metadata, spec)

	combiner.Operation = combiner.GetStringParam("operation", "merge")

	// Set fields for base Want methods
	combiner.WantType = "combiner"
	combiner.ConnectivityMetadata = mywant.ConnectivityMetadata{
		RequiredInputs:  2,
		RequiredOutputs: 1,
		MaxInputs:       -1,
		MaxOutputs:      -1,
		WantType:        "combiner",
		Description:     "Stream combiner want",
	}

	return combiner
}

// Getmywant.Want returns the embedded mywant.Want
func (c *Combiner) GetWant() *mywant.Want {
	return &c.Want
}

// Exec executes the combiner directly
func (c *Combiner) Exec() bool {
	// Initialize state if needed
	if c.State == nil {
		c.State = make(map[string]interface{})
	}

	// Get persistent state
	processed, _ := c.State["processed"].(int)

	// Validate channels are available
	if c.paths.GetInCount() == 0 || c.paths.GetOutCount() == 0 {
		return true
	}

	// Simple combiner: just forward all packets from all inputs
	for i := 0; i < c.paths.GetInCount(); i++ {
		select {
		case packet, ok := <-c.paths.In[i].Channel:
			if !ok {
				continue
			}
			qp := packet.(QueuePacket)

			// Check for termination packet and forward it
			if qp.IsEnded() {
				// Trigger OnEnded callback
				if err := c.OnEnded(&qp); err != nil {
					c.StoreLog(fmt.Sprintf("OnEnded callback error: %v", err))
				}
				// Forward end signal to next want
				c.SendPacketMulti(qp)
				return true
			}

			processed++
			c.SendPacketMulti(qp)
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
	c.StoreStateMulti(map[string]interface{}{
		"total_processed":   processed,
		"average_wait_time": 0.0, // Combiners don't add wait time
		"total_wait_time":   0.0,
	})

	return nil
}

// Sink collects and terminates the packet stream
type Sink struct {
	mywant.Want
	Received int
	paths    mywant.Paths
}

// Goal creates a new sink want
func Goal(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
	sink := &Sink{
		Want:     mywant.Want{},
		Received: 0,
	}

	// Initialize base Want fields
	sink.Init(metadata, spec)

	// Set fields for base Want methods
	sink.WantType = "sink"
	sink.ConnectivityMetadata = mywant.ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0,
		MaxInputs:       -1,
		MaxOutputs:      0,
		WantType:        "sink",
		Description:     "Data sink/collector want",
	}

	return sink
}

// Getmywant.Want returns the embedded mywant.Want
func (s *Sink) GetWant() *mywant.Want {
	return &s.Want
}

// Exec executes the sink directly
func (s *Sink) Exec() bool {
	// Validate input channel is available
	in, connectionAvailable := s.GetFirstInputChannel()
	if !connectionAvailable {
		return true
	}

	// Check if already completed using persistent state
	completed, _ := s.GetStateBool("completed", false)
	if completed {
		return true
	}

	// Block waiting for data from using channel
	packet := (<-in).(QueuePacket)

	// Check for termination packet
	if packet.IsEnded() {
		// Mark as completed in persistent state
		s.StoreState("completed", true)
		// Trigger OnEnded callback
		if err := s.OnEnded(&packet); err != nil {
			s.StoreLog(fmt.Sprintf("OnEnded callback error: %v", err))
		}
		return true
	}

	s.Received++
	return false // Continue waiting for more packets
}

// OnEnded implements PacketHandler interface for Sink termination callbacks
func (s *Sink) OnEnded(packet mywant.Packet) error {
	// Store final state
	s.StoreStateMulti(map[string]interface{}{
		"total_processed":      s.Received,
		"average_wait_time":    0.0, // Sinks don't add wait time
		"total_wait_time":      0.0,
		"achieving_percentage": 100,
	})

	return nil
}

// CalculateAchievingPercentage calculates the progress toward completion for Sink
// Returns 100 when all packets have been collected (completion)
func (s *Sink) CalculateAchievingPercentage() int {
	completed, _ := s.GetStateBool("completed", false)
	if completed {
		return 100
	}
	// While streaming, indicate partial progress
	if s.Received > 0 {
		return 50
	}
	return 0
}

// RegisterQNetWantTypes registers the qnet-specific want types with a mywant.ChainBuilder
func RegisterQNetWantTypes(builder *mywant.ChainBuilder) {
	builder.RegisterWantType("qnet numbers", PacketNumbers)
	builder.RegisterWantType("qnet queue", NewQueue)
	builder.RegisterWantType("qnet combiner", NewCombiner)
	builder.RegisterWantType("qnet sink", Goal)
	// Register collector type (alias for sink)
	builder.RegisterWantType("qnet collector", Goal)
}
