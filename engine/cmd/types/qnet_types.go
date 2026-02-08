package types

import (
	"fmt"
	"math"
	"math/rand"
	mywant "mywant/engine/src"
	"time"
)

func init() {
	mywant.RegisterWantImplementation[Numbers, NumbersLocals]("numbers")
	mywant.RegisterWantImplementation[Numbers, NumbersLocals]("qnet numbers")
	mywant.RegisterWantImplementation[Queue, QueueLocals]("queue")
	mywant.RegisterWantImplementation[Queue, QueueLocals]("qnet queue")
	mywant.RegisterWantImplementation[Combiner, CombinerLocals]("combiner")
	mywant.RegisterWantImplementation[Combiner, CombinerLocals]("qnet combiner")
}

// NumbersLocals holds type-specific local state for Numbers want
type NumbersLocals struct {
	Rate                float64
	Count               int
	batchUpdateInterval int
	cycleCount          int
	currentTime         float64
	currentCount        int
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

// CombinerLocals holds type-specific local state for Combiner want
type CombinerLocals struct {
	Operation string
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
func (p *QueuePacket) GetData() any {
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
}

func (g *Numbers) GetLocals() *NumbersLocals {
	return mywant.GetLocals[NumbersLocals](&g.Want)
}

// Initialize resets state before execution begins
func (g *Numbers) Initialize() {
	// No state reset needed for queue wants
}

// IsAchieved checks if numbers generator is complete (all packets sent)
func (g *Numbers) IsAchieved() bool {
	locals := g.GetLocals()
	if locals == nil {
		return false
	}
	paramCount := g.GetIntParam("count", locals.Count)
	isAchieved := locals.currentCount >= paramCount
	if isAchieved {
		g.StoreLog("[NUMBERS-ISACHIEVED] Complete: currentCount=%d, paramCount=%d", locals.currentCount, paramCount)
	}
	return isAchieved
}

// Progress executes the numbers generator directly with dynamic parameter reading
func (g *Numbers) Progress() {
	locals := mywant.CheckLocalsInitialized[NumbersLocals](&g.Want)

	useDeterministic := g.GetBoolParam("deterministic", false)
	paramCount := g.GetIntParam("count", locals.Count)

	// Initialize batchUpdateInterval if not set
	if locals.batchUpdateInterval == 0 {
		locals.batchUpdateInterval = g.GetIntParam("batch_interval", 100)
	}

	paramRate := g.GetFloatParam("rate", locals.Rate)

	// Check if we're already done before generating more packets
	if locals.currentCount >= paramCount {
		g.StoreLog("[NUMBERS-EXEC] Already complete: currentCount=%d >= paramCount=%d, sending DONE signal", locals.currentCount, paramCount)
		g.ProvideDone()
		return
	}

	// Initial delay for the very first packet to ensure subscribers (Queues) are connected
	if locals.currentCount == 0 {
		time.Sleep(1000 * time.Millisecond)
		g.StoreLog("[NUMBERS-EXEC] Starting generation after initial sync delay")
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
		achievingPercentage := 0
		if paramCount > 0 {
			achievingPercentage = (locals.currentCount * 100) / paramCount
			if achievingPercentage > 100 {
				achievingPercentage = 100
			}
		}
		g.StoreLog("[NUMBERS-EXEC] Progress: currentCount=%d/%d (%.1f%%)", locals.currentCount, paramCount, float64(locals.currentCount)*100/float64(paramCount))
		g.StoreStateMulti(mywant.Dict{
			"total_processed":      locals.currentCount,
			"average_wait_time":    0.0, // Generators don't have wait time
			"total_wait_time":      0.0,
			"current_time":         locals.currentTime,
			"current_count":        locals.currentCount,
			"achieving_percentage": achievingPercentage,
		})
	}

	g.Provide(QueuePacket{Num: locals.currentCount, Time: locals.currentTime})

	// Check if this was the last packet
	if locals.currentCount >= paramCount {
		g.StoreLog("[NUMBERS-EXEC] Last packet sent: currentCount=%d >= paramCount=%d", locals.currentCount, paramCount)
		g.StoreStateMulti(mywant.Dict{
			"achieving_percentage": 100,
			"final_result":         fmt.Sprintf("Generated %d packets", paramCount),
		})

		g.ProvideDone()
	}
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
}

func (q *Queue) GetLocals() *QueueLocals {
	return mywant.GetLocals[QueueLocals](&q.Want)
}

// NewQueue creates a new queue want
// Initialize resets state before execution begins
func (q *Queue) Initialize() {
	// No state reset needed for queue wants
}

// IsAchieved checks if queue is complete (end signal received)
func (q *Queue) IsAchieved() bool {
	locals := q.GetLocals()
	if locals == nil {
		return false
	}
	completed, _ := q.GetStateBool("completed", false)
	if completed {
		q.StoreLog("[QUEUE-ISACHIEVED] Completed! Shifting to achieved")
	}
	return completed
}

// Progress executes the queue processing directly with batch mechanism
func (q *Queue) Progress() {
	locals := mywant.CheckLocalsInitialized[QueueLocals](&q.Want)

	// Initialize missing fields if needed
	if locals.batchUpdateInterval == 0 {
		locals.batchUpdateInterval = q.GetIntParam("batch_interval", 100)
	}
	if locals.ServiceTime == 0 {
		locals.ServiceTime = q.GetFloatParam("service_time", 1.0)
	}

	_, i, done, ok := q.Use(100) // Use 100ms timeout instead of forever
	if !ok {
		// No packet received (timeout) - skip this cycle
		return
	}

	if done {
		q.StoreLog("[QUEUE-EXEC] Received DONE signal")
	} else if i != nil {
		packet := i.(QueuePacket)
		if packet.Num%locals.batchUpdateInterval == 0 || packet.Num <= 5 {
			q.StoreLog("[QUEUE-EXEC] Received packet #%d, time=%.2f", packet.Num, packet.Time)
		}
	}

	// Check for end signal
	if done {
		// Always flush batch and store final state when terminating
		q.flushBatch(locals)

		// Trigger OnEnded callback
		if err := q.OnEnded(nil, locals); err != nil {
			q.StoreLog("OnEnded callback error: %v", err)
		}
		// Forward end signal to next want
		q.ProvideDone()
		q.StoreLog("[QUEUE-EXEC] Setting completed=true")
		q.StoreState("completed", true)
		return
	}

	packet := i.(QueuePacket)
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

	q.Provide(QueuePacket{Num: packet.Num, Time: finishTime})
}

// flushBatch commits all accumulated statistics to state
func (q *Queue) flushBatch(locals *QueueLocals) {
	// Calculate average wait time
	avgWaitTime := 0.0
	if locals.processedCount > 0 {
		avgWaitTime = locals.waitTimeSum / float64(locals.processedCount)
	}

	// Calculate achieving percentage
	achievingPercentage := 0
	totalCount := q.GetIntParam("count", -1)
	if totalCount > 0 && locals.processedCount > 0 {
		achievingPercentage = (locals.processedCount * 100) / totalCount
		if achievingPercentage > 100 {
			achievingPercentage = 100
		}
	} else if locals.processedCount > 0 {
		achievingPercentage = 50 // Partial progress for streaming without count
	}

	// Batch update all statistics at once
	q.StoreStateMulti(mywant.Dict{
		"serverFreeTime":           locals.serverFreeTime,
		"waitTimeSum":              locals.waitTimeSum,
		"processedCount":           locals.processedCount,
		"average_wait_time":        avgWaitTime,
		"total_processed":          locals.processedCount,
		"total_wait_time":          locals.waitTimeSum,
		"current_server_free_time": locals.serverFreeTime,
		"achieving_percentage":     achievingPercentage,
	})
}

// OnEnded implements PacketHandler interface for packet termination callbacks
func (q *Queue) OnEnded(packet mywant.Packet, locals *QueueLocals) error {
	// Calculate final statistics from local persistent state
	avgWaitTime := 0.0
	if locals.processedCount > 0 {
		avgWaitTime = locals.waitTimeSum / float64(locals.processedCount)
	}
	q.StoreStateMulti(mywant.Dict{
		"average_wait_time":        avgWaitTime,
		"total_processed":          locals.processedCount,
		"total_wait_time":          locals.waitTimeSum,
		"current_server_free_time": locals.serverFreeTime,
		"achieving_percentage":     100,
		"final_result":             fmt.Sprintf("%.2f", avgWaitTime),
	})

	return nil
}

// CalculateAchievingPercentage calculates the progress toward completion for Queue
// Uses processedCount / totalCount * 100 if count parameter is provided
// Otherwise, returns 100 when complete, 50 during processing, 0 when idle
func (q *Queue) CalculateAchievingPercentage() int {
	locals := q.GetLocals()
	if locals == nil {
		return 0
	}

	// Check if count parameter is provided
	totalCount := q.GetIntParam("count", -1)

	// If count is provided and processedCount is available, calculate percentage
	if totalCount > 0 && locals.processedCount > 0 {
		percentage := (locals.processedCount * 100) / totalCount
		if percentage > 100 {
			percentage = 100
		}
		return percentage
	}

	// Fallback: returns 100 when termination is received, 50 during processing
	completed, _ := q.GetStateBool("completed", false)
	if completed {
		return 100
	}

	// For streaming queue without count parameter, indicate partial progress
	if locals.processedCount > 0 {
		return 50 // Indicate partial progress while streaming
	}
	return 0
}

// Combiner merges multiple using streams
type Combiner struct {
	mywant.Want
}

func (c *Combiner) GetLocals() *CombinerLocals {
	return mywant.GetLocals[CombinerLocals](&c.Want)
}

// Initialize resets state before execution begins
func (c *Combiner) Initialize() {
	// No state reset needed for queue wants
}

// IsAchieved checks if combiner is complete (end signal received)
func (c *Combiner) IsAchieved() bool {
	locals := c.GetLocals()
	if locals == nil {
		return false
	}
	completed, _ := c.GetStateBool("completed", false)
	return completed
}

// Progress executes the combiner directly
func (c *Combiner) Progress() {
	locals := mywant.CheckLocalsInitialized[CombinerLocals](&c.Want)

	processed, _ := c.GetStateInt("processed", 0)
	if c.GetInCount() == 0 || c.GetOutCount() == 0 {
		return
	}

	// Receive from any input channel (wait for data from any source)
	_, i, done, ok := c.UseForever()
	if !ok {
		return
	}

	if done {
		// Trigger OnEnded callback
		if err := c.OnEnded(nil, locals); err != nil {
			c.StoreLog("OnEnded callback error: %v", err)
		}
		// Forward end signal to next want
		c.ProvideDone()
		c.StoreState("completed", true)
		return
	}

	packet := i.(QueuePacket)
	processed++
	c.Provide(packet)

	c.StoreState("processed", processed)
}

// OnEnded implements PacketHandler interface for Combiner termination callbacks
func (c *Combiner) OnEnded(packet mywant.Packet, locals *CombinerLocals) error {
	// Extract combiner-specific statistics from state
	processed, _ := c.GetStateInt("processed", 0)
	c.StoreStateMulti(mywant.Dict{
		"total_processed":   processed,
		"average_wait_time": 0.0, // Combiners don't add wait time
		"total_wait_time":   0.0,
		"final_result":      fmt.Sprintf("Combined %d packets", processed),
	})

	return nil
}
