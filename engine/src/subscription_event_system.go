package mywant

import (
	"context"
	"log"
	"sync"
	"time"
)

// Global subscription system shared across all wants
var globalSubscriptionSystem *UnifiedSubscriptionSystem
var globalSubscriptionSystemOnce sync.Once

func GetGlobalSubscriptionSystem() *UnifiedSubscriptionSystem {
	globalSubscriptionSystemOnce.Do(func() {
		globalSubscriptionSystem = NewUnifiedSubscriptionSystem()
	})
	return globalSubscriptionSystem
}

// EventType identifies the type of event
type EventType string

const (
	// Group A: Async notifications (post-execution)
	EventTypeStateChange     EventType = "state_change"
	EventTypeParameterChange EventType = "parameter_change"
	EventTypeOwnerChildState EventType = "owner_child_state"

	// Group B: Sync control (pre-execution) - for future use
	EventTypePreExecution EventType = "pre_execution"
	EventTypeMonitorAgent EventType = "monitor_agent"
	EventTypeChannelEnd   EventType = "channel_end"
	EventTypeStatusChange EventType = "status_change"
	EventTypeProcessEnd   EventType = "process_end"

	// Group C: Blocking coordination
	EventTypeOwnerCompletion EventType = "owner_completion"
	EventTypeChannelSync     EventType = "channel_sync"
)

// ExecutionControl defines how events control want execution
type ExecutionControl string

const (
	ExecutionContinue  ExecutionControl = "continue"  // Proceed with execution
	ExecutionSkip      ExecutionControl = "skip"      // Skip this cycle
	ExecutionTerminate ExecutionControl = "terminate" // Stop want permanently
	ExecutionBlock     ExecutionControl = "block"     // Wait for condition
	ExecutionRestart   ExecutionControl = "restart"   // Restart from beginning
)

type ProcessingMode int

const (
	ProcessAsync ProcessingMode = iota // Fire and forget - Group A
	ProcessSync                        // Must process before Exec() - Group B
	ProcessBlock                       // Wait for response - Group C
)

// WantEvent is the unified interface for all event types
type WantEvent interface {
	GetEventType() EventType
	GetSourceName() string
	GetTargetName() string
	GetTimestamp() time.Time
	GetPriority() int
}

// EventResponse contains the result of event handling
type EventResponse struct {
	ExecutionControl ExecutionControl // For Group B/C
	Handled          bool             // Whether event was processed
	Error            error            // Any error during handling
}

// EventSubscription is the unified interface for event handlers
type EventSubscription interface {
	OnEvent(ctx context.Context, event WantEvent) EventResponse
	GetSubscriberName() string
}

// BaseEvent provides common event fields
type BaseEvent struct {
	EventType  EventType
	SourceName string
	TargetName string
	Timestamp  time.Time
	Priority   int
}

func (e *BaseEvent) GetEventType() EventType { return e.EventType }
func (e *BaseEvent) GetSourceName() string   { return e.SourceName }
func (e *BaseEvent) GetTargetName() string   { return e.TargetName }
func (e *BaseEvent) GetTimestamp() time.Time { return e.Timestamp }
func (e *BaseEvent) GetPriority() int        { return e.Priority }

// StateChangeEvent represents a state change notification (Group A)
type StateChangeEvent struct {
	BaseEvent
	StateKey      string
	StateValue    any
	PreviousValue any
}

// ParameterChangeEvent represents a parameter change notification (Group A)
type ParameterChangeEvent struct {
	BaseEvent
	ParamName     string
	ParamValue    any
	PreviousValue any
}

// OwnerChildStateEvent represents child->parent state notification (Group A)
type OwnerChildStateEvent struct {
	BaseEvent
	StateKey   string
	StateValue any
}

// OwnerCompletionEvent represents child completion notification to parent (Group C)
type OwnerCompletionEvent struct {
	BaseEvent
	ChildName string
}

// PreExecutionEvent represents pre-execution control event (Group B)
type PreExecutionEvent struct {
	BaseEvent
	ExecutionContext map[string]any
}

// MonitorAgentEvent represents agent monitoring event (Group B)
type MonitorAgentEvent struct {
	BaseEvent
	AgentName   string
	MonitorData map[string]any
}
type ProcessEndEvent struct {
	BaseEvent
	FinalState map[string]any
	Success    bool
}

// StatusChangeEvent represents status change notification (Group B)
type StatusChangeEvent struct {
	BaseEvent
	OldStatus WantStatus
	NewStatus WantStatus
}

// ChannelEndEvent represents channel lifecycle event (Group B)
type ChannelEndEvent struct {
	BaseEvent
	ChannelName string
	Direction   string // "input" or "output"
}

// UnifiedSubscriptionSystem manages all event subscriptions and delivery
type UnifiedSubscriptionSystem struct {
	// Subscription storage: eventType -> list of subscriptions
	subscriptions  map[EventType][]EventSubscription
	processingMode map[EventType]ProcessingMode

	// Priority ordering for sync processing
	syncPriority map[EventType]int

	// Mutex for thread-safe access
	mutex sync.RWMutex

	// Enable/disable logging
	enableLogging bool
}

// NewUnifiedSubscriptionSystem creates a new subscription system
func NewUnifiedSubscriptionSystem() *UnifiedSubscriptionSystem {
	uss := &UnifiedSubscriptionSystem{
		subscriptions:  make(map[EventType][]EventSubscription),
		processingMode: make(map[EventType]ProcessingMode),
		syncPriority:   make(map[EventType]int),
		enableLogging:  false, // Disable by default for clean output
	}
	uss.SetProcessingMode(EventTypeStateChange, ProcessAsync)
	uss.SetProcessingMode(EventTypeParameterChange, ProcessAsync)
	uss.SetProcessingMode(EventTypeOwnerChildState, ProcessAsync)

	// Group B: Sync control (for future use)
	uss.SetProcessingMode(EventTypePreExecution, ProcessSync)
	uss.SetProcessingMode(EventTypeMonitorAgent, ProcessSync)
	uss.SetProcessingMode(EventTypeChannelEnd, ProcessSync)
	uss.SetProcessingMode(EventTypeStatusChange, ProcessSync)
	uss.SetProcessingMode(EventTypeProcessEnd, ProcessSync)

	// Group C: Blocking coordination - Changed to Async to prevent deadlocks
	uss.SetProcessingMode(EventTypeOwnerCompletion, ProcessAsync)
	uss.SetProcessingMode(EventTypeChannelSync, ProcessBlock)

	return uss
}
func (uss *UnifiedSubscriptionSystem) SetProcessingMode(eventType EventType, mode ProcessingMode) {
	uss.mutex.Lock()
	defer uss.mutex.Unlock()
	uss.processingMode[eventType] = mode
}

// Subscribe registers a subscription for an event type
func (uss *UnifiedSubscriptionSystem) Subscribe(
	eventType EventType,
	subscription EventSubscription,
) {
	uss.mutex.Lock()
	defer uss.mutex.Unlock()

	uss.subscriptions[eventType] = append(uss.subscriptions[eventType], subscription)

	// DEBUG: Always log OwnerCompletion subscriptions
	subscriberName := subscription.GetSubscriberName()
	if eventType == EventTypeOwnerCompletion {
		log.Printf("[SUBSCRIPTION] REGISTERED: %s subscribed to %s events (now %d subscribers)\n",
			subscriberName, eventType, len(uss.subscriptions[eventType]))
	}

	if uss.enableLogging {
		log.Printf("[SUBSCRIPTION] %s subscribed to %s events\n",
			subscriberName, eventType)
	}
}

// Unsubscribe removes a subscription
func (uss *UnifiedSubscriptionSystem) Unsubscribe(eventType EventType, subscriberName string) {
	uss.mutex.Lock()
	defer uss.mutex.Unlock()

	subs := uss.subscriptions[eventType]
	for i, sub := range subs {
		if sub.GetSubscriberName() == subscriberName {
			uss.subscriptions[eventType] = append(subs[:i], subs[i+1:]...)
			if uss.enableLogging {
				log.Printf("[SUBSCRIPTION] %s unsubscribed from %s events\n",
					subscriberName, eventType)
			}
			return
		}
	}
}

// Emit sends an event to all subscribers based on processing mode
func (uss *UnifiedSubscriptionSystem) Emit(ctx context.Context, event WantEvent) []EventResponse {
	eventType := event.GetEventType()

	uss.mutex.RLock()
	mode := uss.processingMode[eventType]
	subscribers := uss.subscriptions[eventType]
	uss.mutex.RUnlock()

	if len(subscribers) == 0 {
		// DEBUG: Log missing subscribers for owner completion events
		if eventType == EventTypeOwnerCompletion {
			log.Printf("[SUBSCRIPTION] WARNING: No subscribers for %s event from %s to %s\n",
				eventType, event.GetSourceName(), event.GetTargetName())
		}
		return nil
	}

	// DEBUG: Log emitting OwnerCompletion events
	if eventType == EventTypeOwnerCompletion {
		log.Printf("[SUBSCRIPTION] EMITTING %s to %d subscribers (from %s to %s, mode=%v)\n",
			eventType, len(subscribers), event.GetSourceName(), event.GetTargetName(), mode)
		for i, sub := range subscribers {
			log.Printf("  [%d] %s\n", i, sub.GetSubscriberName())
		}
	}

	switch mode {
	case ProcessAsync:
		uss.emitAsync(ctx, event, subscribers)
		return nil
	case ProcessSync:
		return uss.emitSync(ctx, event, subscribers)
	case ProcessBlock:
		return uss.emitBlock(ctx, event, subscribers)
	default:
		return nil
	}
}

// emitAsync sends events asynchronously (fire and forget) - Group A
func (uss *UnifiedSubscriptionSystem) emitAsync(ctx context.Context, event WantEvent, subs []EventSubscription) {
	for _, sub := range subs {
		go func(s EventSubscription) {
			response := s.OnEvent(ctx, event)
			if response.Error != nil && uss.enableLogging {
				log.Printf("[SUBSCRIPTION ERROR] Async handler %s failed: %v\n",
					s.GetSubscriberName(), response.Error)
			}
		}(sub)
	}
}

// emitSync sends events synchronously and collects responses - Group B
func (uss *UnifiedSubscriptionSystem) emitSync(ctx context.Context, event WantEvent, subs []EventSubscription) []EventResponse {
	responses := make([]EventResponse, 0, len(subs))
	for _, sub := range subs {
		response := sub.OnEvent(ctx, event)
		responses = append(responses, response)
		if response.Error != nil && uss.enableLogging {
			log.Printf("[SUBSCRIPTION ERROR] Sync handler %s failed: %v\n",
				sub.GetSubscriberName(), response.Error)
		}
	}
	return responses
}

// emitBlock sends events with blocking semantics - Group C
func (uss *UnifiedSubscriptionSystem) emitBlock(ctx context.Context, event WantEvent, subs []EventSubscription) []EventResponse {
	responses := make([]EventResponse, 0, len(subs))
	for _, sub := range subs {
		response := sub.OnEvent(ctx, event)
		responses = append(responses, response)
		if response.Error != nil && uss.enableLogging {
			log.Printf("[SUBSCRIPTION ERROR] Block handler %s failed: %v\n",
				sub.GetSubscriberName(), response.Error)
		}
	}
	return responses
}
func (uss *UnifiedSubscriptionSystem) GetSubscriberCount(eventType EventType) int {
	uss.mutex.RLock()
	defer uss.mutex.RUnlock()
	return len(uss.subscriptions[eventType])
}

// EnableLogging enables debug logging for subscription system
func (uss *UnifiedSubscriptionSystem) EnableLogging(enable bool) {
	uss.mutex.Lock()
	defer uss.mutex.Unlock()
	uss.enableLogging = enable
}
