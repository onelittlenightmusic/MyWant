package mywant

import (
	"context"
	"fmt"
	"sync"
)

// Global want registry for notification lookup
var (
	wantRegistry      = make(map[string]*Want)
	wantRegistryMutex sync.RWMutex

	// Notification history for debugging
	notificationHistory = make([]StateNotification, 0, 1000)
	historyMutex        sync.RWMutex

	// Configuration
	maxNotificationHistory = 1000
)

// RegisterWant registers a want for notification lookup
func RegisterWant(want *Want) {
	wantRegistryMutex.Lock()
	defer wantRegistryMutex.Unlock()
	wantRegistry[want.Metadata.Name] = want
}

// UnregisterWant removes a want from the registry
func UnregisterWant(wantName string) {
	wantRegistryMutex.Lock()
	defer wantRegistryMutex.Unlock()
	delete(wantRegistry, wantName)
}

// findWantByName retrieves a want from the registry
func findWantByName(wantName string) *Want {
	wantRegistryMutex.RLock()
	defer wantRegistryMutex.RUnlock()
	return wantRegistry[wantName]
}

// sendStateNotifications handles all notification types
func sendStateNotifications(notification StateNotification) {
	// Emit state change through unified subscription system
	emitStateChangeEvent(notification)

	// Send owner-child notifications (child -> parent state updates)
	sendOwnerChildNotifications(notification)

	// Store in history for debugging
	storeNotificationHistory(notification)
}

// emitStateChangeEvent emits a state change through the unified subscription system
func emitStateChangeEvent(notification StateNotification) {
	// Find source want to emit from its subscription system
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}

	// Create StateChangeEvent
	event := &StateChangeEvent{
		BaseEvent: BaseEvent{
			EventType:  EventTypeStateChange,
			SourceName: notification.SourceWantName,
			TargetName: notification.TargetWantName,
			Timestamp:  notification.Timestamp,
			Priority:   0,
		},
		StateKey:      notification.StateKey,
		StateValue:    notification.StateValue,
		PreviousValue: notification.PreviousValue,
	}

	// Emit through subscription system (async)
	want.GetSubscriptionSystem().Emit(context.Background(), event)
}

// sendParameterNotifications handles parameter change propagation to children (reverse direction)
func sendParameterNotifications(notification StateNotification) {
	// Emit through unified subscription system
	emitParameterChangeEvent(notification)

	// Find all wants that have this want as their owner (i.e., children)
	wantRegistryMutex.RLock()
	childWants := make([]*Want, 0)
	for _, childWant := range wantRegistry {
		// Check if this child has the notification source as its owner
		for _, ownerRef := range childWant.Metadata.OwnerReferences {
			if ownerRef.Name == notification.SourceWantName && ownerRef.Controller && ownerRef.Kind == "Want" {
				childWants = append(childWants, childWant)
				break
			}
		}
	}
	wantRegistryMutex.RUnlock()

	// Set child wants to idle for restart (handles parameter change restart)
	for _, childWant := range childWants {
		fmt.Printf("[PARAMETER CHANGE] %s: Parameter %s changed to %v, setting status to idle for restart\n",
			childWant.Metadata.Name, notification.StateKey, notification.StateValue)
		childWant.SetStatus(WantStatusIdle)
	}

	// Store in history for debugging
	storeNotificationHistory(notification)
}

// emitParameterChangeEvent emits a parameter change through the unified subscription system
func emitParameterChangeEvent(notification StateNotification) {
	// Find source want to emit from its subscription system
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}

	// Create ParameterChangeEvent
	event := &ParameterChangeEvent{
		BaseEvent: BaseEvent{
			EventType:  EventTypeParameterChange,
			SourceName: notification.SourceWantName,
			TargetName: notification.TargetWantName,
			Timestamp:  notification.Timestamp,
			Priority:   0,
		},
		ParamName:     notification.StateKey,
		ParamValue:    notification.StateValue,
		PreviousValue: notification.PreviousValue,
	}

	// Emit through subscription system (async)
	want.GetSubscriptionSystem().Emit(context.Background(), event)
}

// sendOwnerChildNotifications handles parent-child notifications (child -> parent)
func sendOwnerChildNotifications(notification StateNotification) {
	// Find the want to get its OwnerReferences
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}

	if len(want.Metadata.OwnerReferences) > 0 {
		for _, ownerRef := range want.Metadata.OwnerReferences {
			if ownerRef.Controller && ownerRef.Kind == "Want" {
				// Emit through unified subscription system
				emitOwnerChildStateEvent(notification, ownerRef.Name)

				// Legacy: Call old Target system for backward compatibility
				// This ensures existing Target behavior continues to work
				notifyParentStateUpdate(ownerRef.Name, want.Metadata.Name,
					notification.StateKey, notification.StateValue)
				break
			}
		}
	}
}

// emitOwnerChildStateEvent emits an owner-child state notification through the unified subscription system
func emitOwnerChildStateEvent(notification StateNotification, ownerName string) {
	// Find source want to emit from its subscription system
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}

	// Create OwnerChildStateEvent
	event := &OwnerChildStateEvent{
		BaseEvent: BaseEvent{
			EventType:  EventTypeOwnerChildState,
			SourceName: notification.SourceWantName,
			TargetName: ownerName,
			Timestamp:  notification.Timestamp,
			Priority:   0,
		},
		StateKey:   notification.StateKey,
		StateValue: notification.StateValue,
	}

	// Emit through subscription system (async)
	want.GetSubscriptionSystem().Emit(context.Background(), event)
}


// storeNotificationHistory stores notification for debugging
func storeNotificationHistory(notification StateNotification) {
	historyMutex.Lock()
	defer historyMutex.Unlock()

	// Add to history
	notificationHistory = append(notificationHistory, notification)

	// Trim history if too long
	if len(notificationHistory) > maxNotificationHistory {
		notificationHistory = notificationHistory[len(notificationHistory)-maxNotificationHistory:]
	}
}

// GetNotificationHistory returns recent notification history for debugging
func GetNotificationHistory(limit int) []StateNotification {
	historyMutex.RLock()
	defer historyMutex.RUnlock()

	if limit <= 0 || limit > len(notificationHistory) {
		limit = len(notificationHistory)
	}

	start := len(notificationHistory) - limit
	if start < 0 {
		start = 0
	}

	result := make([]StateNotification, limit)
	copy(result, notificationHistory[start:])
	return result
}

// ClearNotificationHistory clears the notification history
func ClearNotificationHistory() {
	historyMutex.Lock()
	defer historyMutex.Unlock()
	notificationHistory = notificationHistory[:0]
}

