package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Packet represents a data packet with metadata for dynamic chains
type Packet struct {
	Data      interface{}
	Timestamp int64
	ID        uint64
	IsEOS     bool // End of stream marker
}

// NewPacket creates a new packet with timestamp and ID
func NewPacket(data interface{}, id uint64) *Packet {
	return &Packet{
		Data:      data,
		Timestamp: time.Now().UnixNano(),
		ID:        id,
		IsEOS:     false,
	}
}

// NewEOSPacket creates an end-of-stream packet
func NewEOSPacket(id uint64) *Packet {
	return &Packet{
		Data:      nil,
		Timestamp: time.Now().UnixNano(),
		ID:        id,
		IsEOS:     true,
	}
}

// PersistentChannel manages buffered channels with packet history
type PersistentChannel struct {
	buffer      []*Packet
	capacity    int
	mutex       sync.RWMutex
	notEmpty    *sync.Cond
	notFull     *sync.Cond
	closed      bool
	subscribers []chan *Packet
	subMutex    sync.RWMutex
}

// NewPersistentChannel creates a new persistent channel with given capacity
func NewPersistentChannel(capacity int) *PersistentChannel {
	pc := &PersistentChannel{
		buffer:      make([]*Packet, 0, capacity),
		capacity:    capacity,
		subscribers: make([]chan *Packet, 0),
	}
	pc.notEmpty = sync.NewCond(&pc.mutex)
	pc.notFull = sync.NewCond(&pc.mutex)
	return pc
}

// Send adds a packet to the buffer with backpressure handling
func (pc *PersistentChannel) Send(packet *Packet, ctx context.Context) error {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	// Wait for space if buffer is full
	for len(pc.buffer) >= pc.capacity && !pc.closed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			pc.notFull.Wait()
		}
	}

	if pc.closed {
		return fmt.Errorf("channel is closed")
	}

	// Add packet to buffer
	pc.buffer = append(pc.buffer, packet)

	// Notify subscribers
	pc.subMutex.RLock()
	for _, sub := range pc.subscribers {
		select {
		case sub <- packet:
		default:
			// Subscriber channel is full, skip
		}
	}
	pc.subMutex.RUnlock()

	pc.notEmpty.Broadcast()
	return nil
}

// Subscribe creates a new subscriber channel for this persistent channel
func (pc *PersistentChannel) Subscribe(bufferSize int) chan *Packet {
	pc.subMutex.Lock()
	defer pc.subMutex.Unlock()

	subscriber := make(chan *Packet, bufferSize)
	pc.subscribers = append(pc.subscribers, subscriber)

	// Send all historical packets to new subscriber
	pc.mutex.RLock()
	go func() {
		defer pc.mutex.RUnlock()
		for _, packet := range pc.buffer {
			select {
			case subscriber <- packet:
			default:
				// Subscriber buffer full, they'll miss some history
				break
			}
		}
	}()

	return subscriber
}

// Close closes the persistent channel
func (pc *PersistentChannel) Close() {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	pc.closed = true
	pc.notEmpty.Broadcast()
	pc.notFull.Broadcast()

	// Close all subscriber channels
	pc.subMutex.Lock()
	for _, sub := range pc.subscribers {
		close(sub)
	}
	pc.subscribers = nil
	pc.subMutex.Unlock()
}

// GetHistory returns a copy of all buffered packets
func (pc *PersistentChannel) GetHistory() []*Packet {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()

	history := make([]*Packet, len(pc.buffer))
	copy(history, pc.buffer)
	return history
}

// DynamicNodeState represents the state of a dynamic node
type DynamicNodeState int

const (
	StateIdle DynamicNodeState = iota
	StateRunning
	StateWaiting
	StateFinalized
	StateFailed
)

func (s DynamicNodeState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	case StateWaiting:
		return "waiting"
	case StateFinalized:
		return "finalized"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// DynamicNode represents a node in the dynamic chain system
type DynamicNode struct {
	Name       string
	Type       string
	State      DynamicNodeState
	stateMutex sync.RWMutex

	// Channels
	outputChannels map[string]*PersistentChannel
	inputSubs      map[string]chan *Packet
	channelMutex   sync.RWMutex

	// Processing
	processFunc func(inputs map[string]chan *Packet, outputs map[string]*PersistentChannel) error
	ctx         context.Context
	cancel      context.CancelFunc

	// Statistics
	packetsProcessed uint64
	lastActivity     time.Time
	statsMutex       sync.RWMutex
}

// NewDynamicNode creates a new dynamic node
func NewDynamicNode(name, nodeType string, processFunc func(inputs map[string]chan *Packet, outputs map[string]*PersistentChannel) error) *DynamicNode {
	ctx, cancel := context.WithCancel(context.Background())

	return &DynamicNode{
		Name:           name,
		Type:           nodeType,
		State:          StateIdle,
		outputChannels: make(map[string]*PersistentChannel),
		inputSubs:      make(map[string]chan *Packet),
		processFunc:    processFunc,
		ctx:            ctx,
		cancel:         cancel,
		lastActivity:   time.Now(),
	}
}

// AddOutputChannel adds a persistent output channel to the node
func (dn *DynamicNode) AddOutputChannel(name string, capacity int) {
	dn.channelMutex.Lock()
	defer dn.channelMutex.Unlock()

	dn.outputChannels[name] = NewPersistentChannel(capacity)
}

// ConnectInput connects this node's input to another node's output channel
func (dn *DynamicNode) ConnectInput(inputName string, outputChannel *PersistentChannel, bufferSize int) {
	dn.channelMutex.Lock()
	defer dn.channelMutex.Unlock()

	// Subscribe to the output channel
	subscriber := outputChannel.Subscribe(bufferSize)
	dn.inputSubs[inputName] = subscriber
}

// SetState updates the node's state thread-safely
func (dn *DynamicNode) SetState(state DynamicNodeState) {
	dn.stateMutex.Lock()
	defer dn.stateMutex.Unlock()
	dn.State = state
}

// GetState returns the current node state
func (dn *DynamicNode) GetState() DynamicNodeState {
	dn.stateMutex.RLock()
	defer dn.stateMutex.RUnlock()
	return dn.State
}

// Start begins processing for this node
func (dn *DynamicNode) Start() error {
	dn.SetState(StateRunning)

	go func() {
		defer func() {
			if dn.GetState() == StateRunning {
				dn.SetState(StateFinalized)
			}
		}()

		// Execute the processing function
		err := dn.processFunc(dn.inputSubs, dn.outputChannels)
		if err != nil {
			dn.SetState(StateFailed)
			fmt.Printf("[ERROR] Node %s failed: %v\n", dn.Name, err)
		}
	}()

	return nil
}

// Stop gracefully stops the node
func (dn *DynamicNode) Stop() {
	dn.cancel()
	dn.SetState(StateFinalized)

	// Close all output channels
	dn.channelMutex.Lock()
	for _, channel := range dn.outputChannels {
		channel.Close()
	}
	dn.channelMutex.Unlock()
}

// UpdateStats updates processing statistics
func (dn *DynamicNode) UpdateStats() {
	dn.statsMutex.Lock()
	defer dn.statsMutex.Unlock()

	dn.packetsProcessed++
	dn.lastActivity = time.Now()
}

// GetStats returns current statistics
func (dn *DynamicNode) GetStats() (uint64, time.Time) {
	dn.statsMutex.RLock()
	defer dn.statsMutex.RUnlock()

	return dn.packetsProcessed, dn.lastActivity
}

// DynamicChainManager manages the dynamic chain execution
type DynamicChainManager struct {
	nodes      map[string]*DynamicNode
	sinkNodes  map[string]*DynamicNode
	nodesMutex sync.RWMutex

	// Execution control
	running  bool
	runMutex sync.RWMutex

	// Completion tracking
	completedSinks map[string]bool
	sinkMutex      sync.RWMutex
	completion     chan string // Sink name that completed
}

// NewDynamicChainManager creates a new dynamic chain manager
func NewDynamicChainManager() *DynamicChainManager {
	return &DynamicChainManager{
		nodes:          make(map[string]*DynamicNode),
		sinkNodes:      make(map[string]*DynamicNode),
		completedSinks: make(map[string]bool),
		completion:     make(chan string, 10),
	}
}

// AddNode adds a node to the dynamic chain
func (dcm *DynamicChainManager) AddNode(node *DynamicNode, isSink bool) error {
	dcm.nodesMutex.Lock()
	defer dcm.nodesMutex.Unlock()

	if _, exists := dcm.nodes[node.Name]; exists {
		return fmt.Errorf("node %s already exists", node.Name)
	}

	dcm.nodes[node.Name] = node

	if isSink {
		dcm.sinkNodes[node.Name] = node
	}

	// If system is already running, start the node immediately
	dcm.runMutex.RLock()
	if dcm.running {
		node.Start()
		fmt.Printf("[DYNAMIC] Hot-added node %s (type: %s)\n", node.Name, node.Type)
	}
	dcm.runMutex.RUnlock()

	return nil
}

// ConnectNodes connects output of one node to input of another
func (dcm *DynamicChainManager) ConnectNodes(outputNodeName, outputChannelName, inputNodeName, inputChannelName string, bufferSize int) error {
	dcm.nodesMutex.RLock()
	defer dcm.nodesMutex.RUnlock()

	outputNode, exists := dcm.nodes[outputNodeName]
	if !exists {
		return fmt.Errorf("output node %s not found", outputNodeName)
	}

	inputNode, exists := dcm.nodes[inputNodeName]
	if !exists {
		return fmt.Errorf("input node %s not found", inputNodeName)
	}

	// Get the output channel
	outputNode.channelMutex.RLock()
	outputChannel, exists := outputNode.outputChannels[outputChannelName]
	outputNode.channelMutex.RUnlock()

	if !exists {
		return fmt.Errorf("output channel %s not found in node %s", outputChannelName, outputNodeName)
	}

	// Connect input
	inputNode.ConnectInput(inputChannelName, outputChannel, bufferSize)

	fmt.Printf("[DYNAMIC] Connected %s:%s -> %s:%s\n",
		outputNodeName, outputChannelName, inputNodeName, inputChannelName)

	return nil
}

// Start begins execution of the dynamic chain
func (dcm *DynamicChainManager) Start() {
	dcm.runMutex.Lock()
	dcm.running = true
	dcm.runMutex.Unlock()

	fmt.Println("[DYNAMIC] Starting dynamic chain execution")

	// Start all existing nodes
	dcm.nodesMutex.RLock()
	for _, node := range dcm.nodes {
		node.Start()
		fmt.Printf("[DYNAMIC] Started node %s (type: %s)\n", node.Name, node.Type)
	}
	dcm.nodesMutex.RUnlock()

	// Start monitoring sink completion
	go dcm.monitorSinkCompletion()
}

// monitorSinkCompletion monitors sink nodes for completion
func (dcm *DynamicChainManager) monitorSinkCompletion() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		dcm.runMutex.RLock()
		if !dcm.running {
			dcm.runMutex.RUnlock()
			return
		}
		dcm.runMutex.RUnlock()

		select {
		case <-ticker.C:
			dcm.checkSinkCompletion()
		}
	}
}

// checkSinkCompletion checks if any sink has reached finalized state
func (dcm *DynamicChainManager) checkSinkCompletion() {
	dcm.sinkMutex.Lock()
	defer dcm.sinkMutex.Unlock()

	for sinkName, sink := range dcm.sinkNodes {
		if !dcm.completedSinks[sinkName] && sink.GetState() == StateFinalized {
			dcm.completedSinks[sinkName] = true
			fmt.Printf("[DYNAMIC] Sink %s completed\n", sinkName)

			select {
			case dcm.completion <- sinkName:
			default:
			}
		}
	}
}

// WaitForCompletion waits until at least one sink completes
func (dcm *DynamicChainManager) WaitForCompletion() string {
	return <-dcm.completion
}

// Stop gracefully stops the dynamic chain
func (dcm *DynamicChainManager) Stop() {
	dcm.runMutex.Lock()
	dcm.running = false
	dcm.runMutex.Unlock()

	fmt.Println("[DYNAMIC] Stopping dynamic chain execution")

	// Stop all nodes
	dcm.nodesMutex.RLock()
	for _, node := range dcm.nodes {
		node.Stop()
	}
	dcm.nodesMutex.RUnlock()
}

// GetNodeStates returns current states of all nodes
func (dcm *DynamicChainManager) GetNodeStates() map[string]DynamicNodeState {
	dcm.nodesMutex.RLock()
	defer dcm.nodesMutex.RUnlock()

	states := make(map[string]DynamicNodeState)
	for name, node := range dcm.nodes {
		states[name] = node.GetState()
	}
	return states
}
