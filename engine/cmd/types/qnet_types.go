package types

import (
	"fmt"
	"math"
	"math/rand"
	mywant "mywant/engine/src"
)

// NumbersLocals holds type-specific local state for Numbers want
type NumbersLocals struct {
	Rate                float64
	Count               int
	batchUpdateInterval int
	cycleCount          int
	currentTime         float64
	currentCount        int
}

func (n *NumbersLocals) InitLocals(want *mywant.Want) {
	n.Rate = want.GetFloatParam("rate", 1.0)
	n.Count = want.GetIntParam("count", 100)
	n.batchUpdateInterval = want.GetIntParam("batch_interval", 100)
	n.cycleCount = 0
	n.currentTime = 0.0
	n.currentCount = 0
}

// QueueLocals holds type-specific local state for Queue want
type QueueLocals struct {
	ServiceTime         float64
	batchUpdateInterval int
	lastBatchCount      int
	cycleCount          int
	serverFreeTime      float64
	waitTimeSum         float64
	processedCount      int
}

func (q *QueueLocals) InitLocals(want *mywant.Want) {
	q.ServiceTime = want.GetFloatParam("service_time", 1.0)
	q.batchUpdateInterval = want.GetIntParam("batch_interval", 100)
	q.lastBatchCount = 0
	q.cycleCount = 0
	q.serverFreeTime = 0.0
	q.waitTimeSum = 0.0
	q.processedCount = 0
}

// CombinerLocals holds type-specific local state for Combiner want
type CombinerLocals struct {
	Operation string
}

func (c *CombinerLocals) InitLocals(want *mywant.Want) {
	c.Operation = want.GetStringParam("operation", "merge")
}

// SinkLocals holds type-specific local state for Sink want
type SinkLocals struct {
	Received int
}

func (s *SinkLocals) InitLocals(want *mywant.Want) {
	s.Received = 0
}

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
func (p *QueuePacket) GetData() interface{} {
	return struct {
		Num  int
		Time float64
	}{p.Num, p.Time}
}

// ExpRand64 generates exponentially distributed random numbers with improved precision Uses inverse transform sampling with better numerical stability than standard library
func ExpRand64() float64 {
	// Generate uniform random number in (0,1) avoiding exactly 0 and 1
	u := rand.Float64()
	if u == 0.0 {
		u = math.SmallestNonzeroFloat64
	} else if u == 1.0 {
		u = 1.0 - math.SmallestNonzeroFloat64
	}

	// Use inverse transform: -ln(u) for exponential distribution This provides better distribution than the standard library's algorithm
	return -math.Log(u)
}

// Numbers creates packets and sends them downstream
type Numbers struct {
	mywant.Want
	paths mywant.Paths
}

// PacketNumbers creates a new numbers want
func PacketNumbers(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
	connectivityMetadata := mywant.ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       0,
		MaxOutputs:      -1,
		WantType:        "sequence",
		Description:     "Packet generator want",
	}

	return mywant.NewWant(
		metadata,
		spec,
		func() mywant.WantLocals { return &NumbersLocals{} },
		connectivityMetadata,
		"sequence",
	)
}

// Exec executes the numbers generator directly with dynamic parameter reading
func (g *Numbers) Exec() bool {
	locals, ok := g.Locals.(*NumbersLocals)
	if !ok {
		g.StoreLog("ERROR: Failed to access NumbersLocals from Want.Locals")
		return true
	}

	useDeterministic := g.GetBoolParam("deterministic", false)
	paramCount := g.GetIntParam("count", locals.Count)

	paramRate := g.GetFloatParam("rate", locals.Rate)
	if g.State == nil {
		g.State = make(map[string]interface{})
	}

	if locals.currentCount >= paramCount {
		g.StoreStateMulti(map[string]interface{}{
			"total_processed":      locals.currentCount,
			"average_wait_time":    0.0, // Generators don't have wait time
			"total_wait_time":      0.0,
			"current_time":         locals.currentTime,
			"current_count":        locals.currentCount,
			"achieving_percentage": 100,
		})

		g.SendPacketMulti(QueuePacket{Num: -1, Time: 0})
		return true
	}
	locals.currentCount++

	if useDeterministic {
		// Deterministic inter-arrival time (rate = 1/interval)
		locals.currentTime += 1.0 / paramRate
	} else {
		// Exponential inter-arrival time (rate = 1/mean_interval) ExpRand64() returns Exp(1), so divide by rate to get correct mean interval
		locals.currentTime += ExpRand64() / paramRate
	}
	locals.cycleCount++

	// Batch mechanism: only update state history every N packets to reduce history entries
	if locals.currentCount%locals.batchUpdateInterval == 0 {
		g.StoreStateMulti(map[string]interface{}{
			"current_time":  locals.currentTime,
			"current_count": locals.currentCount,
		})
	}

	g.SendPacketMulti(QueuePacket{Num: locals.currentCount, Time: locals.currentTime})
	return false
}

// CalculateAchievingPercentage calculates the progress toward completion for Numbers generator Returns (currentCount / targetCount) * 100
func (g *Numbers) CalculateAchievingPercentage() int {
	locals, ok := g.Locals.(*NumbersLocals)
	if !ok {
		return 0
	}

	paramCount := g.GetIntParam("count", locals.Count)
	if paramCount <= 0 {
		return 0
	}
	percentage := (locals.currentCount * 100) / paramCount
	if percentage > 100 {
		percentage = 100
	}
	return percentage
}

// Queue processes packets with a service time
type Queue struct {
	mywant.Want
	paths mywant.Paths
}

// NewQueue creates a new queue want
func NewQueue(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
	connectivityMetadata := mywant.ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      -1,
		WantType:        "queue",
		Description:     "Queue processing want",
	}

	return mywant.NewWant(
		metadata,
		spec,
		func() mywant.WantLocals { return &QueueLocals{} },
		connectivityMetadata,
		"queue",
	)
}

// Exec executes the queue processing directly with batch mechanism
func (q *Queue) Exec() bool {
	locals, ok := q.Locals.(*QueueLocals)
	if !ok {
		q.StoreLog("ERROR: Failed to access QueueLocals from Want.Locals")
		return true
	}

	if q.State == nil {
		q.State = make(map[string]interface{})
	}

	_, i, ok := q.ReceiveFromAnyInputChannel(100)
	if !ok {
		return false
	}

	packet := i.(QueuePacket)
	if packet.IsEnded() {
		// Always flush batch and store final state when terminating
		q.flushBatch(locals)

		// Trigger OnEnded callback
		if err := q.OnEnded(&packet, locals); err != nil {
			q.StoreLog(fmt.Sprintf("OnEnded callback error: %v", err))
		}
		// Forward end signal to next want
		q.SendPacketMulti(packet)
		return true
	}
	arrivalTime := packet.Time
	startServiceTime := math.Max(arrivalTime, locals.serverFreeTime)
	waitTime := startServiceTime - arrivalTime
	serviceTime := q.GetFloatParam("service_time", locals.ServiceTime)

	finishTime := startServiceTime + serviceTime
	locals.serverFreeTime = finishTime

	locals.waitTimeSum += waitTime
	locals.processedCount++
	locals.cycleCount++

	// Batch mechanism: only update statistics every N packets
	if locals.processedCount%locals.batchUpdateInterval == 0 {
		q.StoreState("last_packet_wait_time", waitTime)
		q.flushBatch(locals)
		locals.lastBatchCount = locals.processedCount
	}

	q.SendPacketMulti(QueuePacket{Num: packet.Num, Time: finishTime})
	return false
}

// flushBatch commits all accumulated statistics to state
func (q *Queue) flushBatch(locals *QueueLocals) {
	// Calculate average wait time
	avgWaitTime := 0.0
	if locals.processedCount > 0 {
		avgWaitTime = locals.waitTimeSum / float64(locals.processedCount)
	}

	// Batch update all statistics at once
	q.StoreStateMulti(map[string]interface{}{
		"serverFreeTime":           locals.serverFreeTime,
		"waitTimeSum":              locals.waitTimeSum,
		"processedCount":           locals.processedCount,
		"average_wait_time":        avgWaitTime,
		"total_processed":          locals.processedCount,
		"total_wait_time":          locals.waitTimeSum,
		"current_server_free_time": locals.serverFreeTime,
	})
}

// OnEnded implements PacketHandler interface for packet termination callbacks
func (q *Queue) OnEnded(packet mywant.Packet, locals *QueueLocals) error {
	// Calculate final statistics from local persistent state
	avgWaitTime := 0.0
	if locals.processedCount > 0 {
		avgWaitTime = locals.waitTimeSum / float64(locals.processedCount)
	}
	q.StoreStateMulti(map[string]interface{}{
		"average_wait_time":        avgWaitTime,
		"total_processed":          locals.processedCount,
		"total_wait_time":          locals.waitTimeSum,
		"current_server_free_time": locals.serverFreeTime,
		"achieving_percentage":     100,
	})

	return nil
}

// CalculateAchievingPercentage calculates the progress toward completion for Queue For streaming queue, returns 100 when complete (all packets processed) During streaming, this is calculated indirectly through packet count tracking
func (q *Queue) CalculateAchievingPercentage() int {
	locals, ok := q.Locals.(*QueueLocals)
	if !ok {
		return 0
	}
	// Queue is a streaming processor - returns 100 when termination is received The percentage is implicitly tracked by processedCount during streaming
	completed, _ := q.GetStateBool("completed", false)
	if completed {
		return 100
	}
	// For streaming queue, we can't easily determine total expected packets So we return percentage based on whether any packets have been processed
	if locals.processedCount > 0 {
		// Streaming mode: return 100 only when complete
		return 50 // Indicate partial progress while streaming
	}
	return 0
}

// Combiner merges multiple using streams
type Combiner struct {
	mywant.Want
	paths mywant.Paths
}

func NewCombiner(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
	connectivityMetadata := mywant.ConnectivityMetadata{
		RequiredInputs:  2,
		RequiredOutputs: 1,
		MaxInputs:       -1,
		MaxOutputs:      -1,
		WantType:        "combiner",
		Description:     "Stream combiner want",
	}

	return mywant.NewWant(
		metadata,
		spec,
		func() mywant.WantLocals { return &CombinerLocals{} },
		connectivityMetadata,
		"combiner",
	)
}

// Exec executes the combiner directly
func (c *Combiner) Exec() bool {
	locals, ok := c.Locals.(*CombinerLocals)
	if !ok {
		c.StoreLog("ERROR: Failed to access CombinerLocals from Want.Locals")
		return true
	}

	if c.State == nil {
		c.State = make(map[string]interface{})
	}
	processed, _ := c.State["processed"].(int)
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
			if qp.IsEnded() {
				// Trigger OnEnded callback
				if err := c.OnEnded(&qp, locals); err != nil {
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
func (c *Combiner) OnEnded(packet mywant.Packet, locals *CombinerLocals) error {
	// Extract combiner-specific statistics from state
	processed, _ := c.State["processed"].(int)
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
	paths mywant.Paths
}

// Goal creates a new sink want
func Goal(metadata mywant.Metadata, spec mywant.WantSpec) interface{} {
	connectivityMetadata := mywant.ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0,
		MaxInputs:       -1,
		MaxOutputs:      0,
		WantType:        "sink",
		Description:     "Data sink/collector want",
	}

	return mywant.NewWant(
		metadata,
		spec,
		func() mywant.WantLocals { return &SinkLocals{} },
		connectivityMetadata,
		"sink",
	)
}

// Exec executes the sink directly
func (s *Sink) Exec() bool {
	locals, ok := s.Locals.(*SinkLocals)
	if !ok {
		s.StoreLog("ERROR: Failed to access SinkLocals from Want.Locals")
		return true
	}

	completed, _ := s.GetStateBool("completed", false)
	if completed {
		return true
	}

	for {
		_, i, ok := s.ReceiveFromAnyInputChannel(100)
		if !ok {
			// No packet received, but we don't want to end the sink. The sink should wait until a termination packet is received. We return false to continue the execution loop. If we return true, the sink will be marked as completed and will not process any more packets.
			// The timeout in ReceiveFromAnyInputChannel helps to not block the execution loop forever. if the timeout is reached, the loop will continue to the next iteration.
			return false
		}

		packet := i.(QueuePacket)
		if packet.IsEnded() {
			s.StoreState("completed", true)
			// Trigger OnEnded callback
			if err := s.OnEnded(&packet, locals); err != nil {
				s.StoreLog(fmt.Sprintf("OnEnded callback error: %v", err))
			}
			return true
		}

		locals.Received++
	}
}

// OnEnded implements PacketHandler interface for Sink termination callbacks
func (s *Sink) OnEnded(packet mywant.Packet, locals *SinkLocals) error {
	s.StoreStateMulti(map[string]interface{}{
		"total_processed":      locals.Received,
		"average_wait_time":    0.0, // Sinks don't add wait time
		"total_wait_time":      0.0,
		"achieving_percentage": 100,
	})

	return nil
}

// CalculateAchievingPercentage calculates the progress toward completion for Sink Returns 100 when all packets have been collected (completion)
func (s *Sink) CalculateAchievingPercentage() int {
	locals, ok := s.Locals.(*SinkLocals)
	if !ok {
		return 0
	}
	completed, _ := s.GetStateBool("completed", false)
	if completed {
		return 100
	}
	// While streaming, indicate partial progress
	if locals.Received > 0 {
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
