package mywant

import (
	"fmt"
	"mywant/engine/src/chain"
	"time"
)

// Re-export chain types for easier access
type Chan = chain.Chan

// NotificationType distinguishes different notification scenarios
type NotificationType string

const (
	NotificationOwnerChild   NotificationType = "owner-child"  // Current Target system (child → parent)
	NotificationSubscription NotificationType = "subscription" // New peer-to-peer (any → any)
	NotificationBroadcast    NotificationType = "broadcast"    // Global notifications (any → all)
	NotificationParameter    NotificationType = "parameter"    // Parameter changes (parent → child)
)

// StateNotification contains complete notification information
type StateNotification struct {
	SourceWantName   string           `json:"sourceWantName"`
	TargetWantName   string           `json:"targetWantName"`
	StateKey         string           `json:"stateKey"`
	StateValue       interface{}      `json:"stateValue"`
	PreviousValue    interface{}      `json:"previousValue,omitempty"`
	Timestamp        time.Time        `json:"timestamp"`
	NotificationType NotificationType `json:"notificationType"`
}

// StateUpdateListener allows wants to receive state change notifications
type StateUpdateListener interface {
	OnStateUpdate(notification StateNotification) error
}

// ParameterChangeListener allows wants to receive parameter change notifications
type ParameterChangeListener interface {
	OnParameterChange(notification StateNotification) error
}

// StateSubscription defines what state changes to monitor
type StateSubscription struct {
	WantName   string   `json:"wantName" yaml:"wantName"`                         // Which want to monitor
	StateKeys  []string `json:"stateKeys,omitempty" yaml:"stateKeys,omitempty"`   // Specific keys (empty = all keys)
	Conditions []string `json:"conditions,omitempty" yaml:"conditions,omitempty"` // Optional conditions like "value > 100"
	BufferSize int      `json:"bufferSize,omitempty" yaml:"bufferSize,omitempty"` // For rate limiting
}

// NotificationFilter allows filtering received notifications
type NotificationFilter struct {
	SourcePattern string   `json:"sourcePattern" yaml:"sourcePattern"`                   // Regex pattern for source names
	StateKeys     []string `json:"stateKeys,omitempty" yaml:"stateKeys,omitempty"`       // Only these keys
	ValuePattern  string   `json:"valuePattern,omitempty" yaml:"valuePattern,omitempty"` // Value conditions
}

// StateHistoryEntry represents a state change entry in the generic history system
type StateHistoryEntry struct {
	WantName   string      `json:"wantName" yaml:"want_name"`
	StateValue interface{} `json:"stateValue" yaml:"state_value"`
	Timestamp  time.Time   `json:"timestamp" yaml:"timestamp"`
}

// ParameterUpdate represents a parameter change notification
type ParameterUpdate struct {
	WantName   string      `json:"want_name"`
	ParamName  string      `json:"param_name"`
	ParamValue interface{} `json:"param_value"`
	Timestamp  time.Time   `json:"timestamp"`
}

// AgentExecution represents information about an agent execution
type AgentExecution struct {
	AgentName string     `json:"agent_name" yaml:"agent_name"`
	AgentType string     `json:"agent_type" yaml:"agent_type"`
	StartTime time.Time  `json:"start_time" yaml:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty" yaml:"end_time,omitempty"`
	Status    string     `json:"status" yaml:"status"` // "running", "achieved", "failed"
	Error     string     `json:"error,omitempty" yaml:"error,omitempty"`
	Activity  string     `json:"activity,omitempty" yaml:"activity,omitempty"` // Description of agent action like "flight reservation has been created"
}

// ParameterUpdateListener represents a want that can receive parameter updates
type ParameterUpdateListener interface {
	OnParameterUpdate(update ParameterUpdate) bool
}

// ChainFunction represents a generalized chain function interface
type ChainFunction interface {
	Exec(using []chain.Chan, outputs []chain.Chan) bool
}

// Packet interface for all packet types in the system
type Packet interface {
	IsEnded() bool
	GetData() interface{}
	SetEnded(bool)
}

// PacketHandler defines callbacks for packet processing events
type PacketHandler interface {
	OnEnded(packet Packet) error
}

// BasePacket provides common packet functionality
type BasePacket struct {
	ended bool
	data  interface{}
}

func (p *BasePacket) IsEnded() bool        { return p.ended }
func (p *BasePacket) SetEnded(ended bool)  { p.ended = ended }
func (p *BasePacket) GetData() interface{} { return p.data }

// ChainWant represents a want that can execute directly
type Executable interface {
	Exec() bool
}

// migrateAllWantsAgentHistory runs agent history migration on all wants
func (cb *ChainBuilder) migrateAllWantsAgentHistory() {
	// Note: This function is called from compilePhase which is already protected by reconcileMutex
	migratedCount := 0
	for _, runtimeWant := range cb.wants {
		if runtimeWant.want != nil {
			if runtimeWant.want.State != nil {
				if _, exists := runtimeWant.want.State["agent_history"]; exists {
					runtimeWant.want.migrateAgentHistoryFromState()
					migratedCount++
				}
			}
		}
	}

	if migratedCount > 0 {
		fmt.Printf("[MIGRATION] Agent history migration completed for %d wants\n", migratedCount)
	}
}

// Config holds the complete declarative configuration
type Config struct {
	Wants []*Want `json:"wants" yaml:"wants"`
}

// PathInfo represents connection information for a single path
type PathInfo struct {
	Channel        chain.Chan
	Name           string
	Active         bool
	TargetWantName string // For output paths, the name of the target want that receives this packet
}

// Paths manages all input and output connections for a want
type Paths struct {
	In  []PathInfo
	Out []PathInfo
}
func (p *Paths) GetInCount() int {
	return len(p.In)
}
func (p *Paths) GetOutCount() int {
	return len(p.Out)
}
func (p *Paths) GetActiveInCount() int {
	count := 0
	for _, path := range p.In {
		if path.Active {
			count++
		}
	}
	return count
}
func (p *Paths) GetActiveOutCount() int {
	count := 0
	for _, path := range p.Out {
		if path.Active {
			count++
		}
	}
	return count
}

// ConnectivityMetadata defines want connectivity requirements and constraints
type ConnectivityMetadata struct {
	RequiredInputs  int
	RequiredOutputs int
	MaxInputs       int // -1 for unlimited
	MaxOutputs      int // -1 for unlimited
	WantType        string
	Description     string
}

// ChangeEventType represents the type of change detected
