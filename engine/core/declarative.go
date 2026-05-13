package mywant

import (
	"log"
	"mywant/engine/core/chain"
	"reflect"
	"strings"
	"time"

	want_spec "github.com/onelittlenightmusic/want-spec"
)

// Dict is a convenience alias for map[string]any
// Used throughout the codebase for configuration, state, and parameter dictionaries
type Dict = map[string]any

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
	StateValue       any              `json:"stateValue"`
	PreviousValue    any              `json:"previousValue,omitempty"`
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

// OnDeletable allows wants to perform cleanup when being deleted
type OnDeletable interface {
	OnDelete()
}

// StateSubscription defines what state changes to monitor
type StateSubscription = want_spec.StateSubscription

// ParamEntry represents a single parameter entry in array format
type ParamEntry = want_spec.ParamEntry

// ExposeEntry declares a parameter or state exposure between scope levels.
type ExposeEntry = want_spec.ExposeEntry

// NotificationFilter allows filtering received notifications
type NotificationFilter = want_spec.NotificationFilter

// StateHistoryEntry represents a state change entry in the generic history system
type StateHistoryEntry struct {
	WantName      string    `json:"wantName" yaml:"want_name"`
	StateValue    any       `json:"stateValue" yaml:"state_value"`
	Timestamp     time.Time `json:"timestamp" yaml:"timestamp"`
	ActionByAgent string    `json:"actionByAgent,omitempty" yaml:"action_by_agent,omitempty"`
}

// ParameterUpdate represents a parameter change notification
type ParameterUpdate struct {
	WantName   string    `json:"want_name"`
	ParamName  string    `json:"param_name"`
	ParamValue any       `json:"param_value"`
	Timestamp  time.Time `json:"timestamp"`
}

// AgentExecution represents information about an agent execution
// AgentExecution represents a single immutable event in the agent execution log.
// Each start/completion/failure/termination produces a separate append-only entry.
// ExecutionID links related events (e.g., the "running" start and its "achieved" completion).
type AgentExecution struct {
	ExecutionID   string    `json:"execution_id" yaml:"execution_id"` // UUID linking related events for the same run
	AgentName     string    `json:"agent_name" yaml:"agent_name"`
	AgentType     string    `json:"agent_type" yaml:"agent_type"`
	Timestamp     time.Time `json:"timestamp" yaml:"timestamp"` // Time this event occurred
	Status        string    `json:"status" yaml:"status"`       // "running", "achieved", "failed", "terminated"
	Error         string    `json:"error,omitempty" yaml:"error,omitempty"`
	Activity      string    `json:"activity,omitempty" yaml:"activity,omitempty"`             // Human-readable description of agent action
	ExecutionMode string    `json:"execution_mode,omitempty" yaml:"execution_mode,omitempty"` // local, webhook, rpc
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
	GetData() any
	SetEnded(bool)
}

// PacketHandler defines callbacks for packet processing events
type PacketHandler interface {
	OnEnded(packet Packet) error
}

// BasePacket provides common packet functionality
type BasePacket struct {
	ended bool
	data  any
}

func (p *BasePacket) IsEnded() bool       { return p.ended }
func (p *BasePacket) SetEnded(ended bool) { p.ended = ended }
func (p *BasePacket) GetData() any        { return p.data }

// Progressable represents a want that can execute directly
type Progressable interface {
	Initialize()      // Initialize/reset state before execution begins
	IsAchieved() bool // Returns true when want is complete
	Progress()        // Execute logic (no completion signal)
}

// Failable is an optional extension of Progressable for want types that can
// declare a failed termination condition (e.g. via finalizeWhen.failed).
type Failable interface {
	IsFailed() bool // Returns true when want should terminate with WantStatusFailed
}

// migrateAllWantsAgentHistory runs agent history migration on all wants
func (cb *ChainBuilder) migrateAllWantsAgentHistory() {
	// Note: This function is called from compilePhase which is already protected by reconcileMutex
	migratedCount := 0
	for _, runtimeWant := range cb.wants {
		if runtimeWant.want != nil {
			if _, exists := runtimeWant.want.getState("agent_history"); exists {
				// Remove legacy agent_history from state
				runtimeWant.want.storeState("agent_history", nil)
				migratedCount++
			}
		}
	}

	if migratedCount > 0 {
		log.Printf("[MIGRATION] Agent history migration completed for %d wants\n", migratedCount)
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
type ConnectivityMetadata = want_spec.ConnectivityMetadata

// RequirePolicy defines connectivity requirements as an enum

// SyncLocalsState synchronizes fields of a Locals struct with the want's state.
// It uses naming conventions (CamelCase -> snake_case) and respects StateLabels
// to determine whether to use Current or Internal state.
func SyncLocalsState(n *Want, locals any, toStruct bool) {
	if locals == nil {
		return
	}

	val := reflect.ValueOf(locals)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		structField := typ.Field(i)

		// 1. Determine the state key
		stateKey := ""
		tag := structField.Tag.Get("mywant")
		if tag != "" {
			parts := strings.Split(tag, ",")
			if len(parts) >= 2 {
				stateKey = parts[1]
			} else {
				stateKey = parts[0]
			}
		}

		if stateKey == "" {
			// Convention: FieldName -> field_name
			stateKey = toSnakeCase(structField.Name)
		}

		// 2. Determine state type (Current vs Internal) based on StateLabels
		// If not in StateLabels, automatically register as internal
		label, exists := n.StateLabels[stateKey]
		if !exists {
			if n.StateLabels == nil {
				n.StateLabels = make(map[string]StateLabel)
			}
			n.StateLabels[stateKey] = LabelInternal
			label = LabelInternal
		}
		if toStruct {
			// Copy from State to Struct ONLY for internal labels.
			// Current, Goal, etc. must be accessed via GetCurrent(), GetGoal() etc.
			if label == LabelInternal {
				if stateVal, ok := n.getState(stateKey); ok {
					setFieldValue(field, stateVal)
				}
			}
		} else {
			// Copy from Struct to State ONLY for internal labels.
			// Goal, Plan, Current, etc. should be updated explicitly via SetCurrent() etc.
			// to avoid accidental overwrites of state that might have been updated by agents
			// or other external entities during the Progress() cycle.
			if label == LabelInternal && field.CanInterface() {
				n.SetInternal(stateKey, field.Interface())
			}
		}
	}
}

func toSnakeCase(str string) string {
	var result strings.Builder
	for i, r := range str {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(rune(strings.ToLower(string(r))[0]))
	}
	// Special handling for acronyms (e.g., PID -> pid, not p_i_d)
	s := result.String()
	s = strings.ReplaceAll(s, "_p_i_d", "_pid")
	s = strings.ReplaceAll(s, "_u_r_l", "_url")
	return s
}

func setFieldValue(field reflect.Value, value any) {
	if !field.CanSet() {
		return
	}

	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return
	}

	// Basic type conversion
	if val.Type().AssignableTo(field.Type()) {
		field.Set(val)
		return
	}

	// Handle pointer to value
	if field.Kind() == reflect.Ptr && val.Type().AssignableTo(field.Type().Elem()) {
		ptr := reflect.New(field.Type().Elem())
		ptr.Elem().Set(val)
		field.Set(ptr)
		return
	}

	// For basic types, handle common conversions (e.g. float64 from JSON -> int)
	if field.Kind() == reflect.Int {
		field.SetInt(int64(ToInt(value, 0)))
		return
	}
	if field.Kind() == reflect.String {
		field.SetString(ToString(value, ""))
		return
	}
	if field.Kind() == reflect.Bool {
		field.SetBool(ToBool(value, false))
		return
	}
	if field.Kind() == reflect.Map && field.Type().Key().Kind() == reflect.String {
		// Handle map[string]any assignment
		if m, ok := value.(map[string]any); ok {
			field.Set(reflect.ValueOf(m))
			return
		}
	}
}
