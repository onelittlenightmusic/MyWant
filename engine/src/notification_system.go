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
func findWantByName(wantName string) *Want {
	wantRegistryMutex.RLock()
	defer wantRegistryMutex.RUnlock()
	return wantRegistry[wantName]
}
func sendStateNotifications(notification StateNotification) {
	// Emit state change through unified subscription system
	emitStateChangeEvent(notification)
	sendOwnerChildNotifications(notification)
	storeNotificationHistory(notification)
}

// emitStateChangeEvent emits a state change through the unified subscription system
func emitStateChangeEvent(notification StateNotification) {
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}
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
func sendParameterNotifications(notification StateNotification) {
	// Emit through unified subscription system
	emitParameterChangeEvent(notification)
	wantRegistryMutex.RLock()
	childWants := make([]*Want, 0)
	for _, childWant := range wantRegistry {
		for _, ownerRef := range childWant.Metadata.OwnerReferences {
			if ownerRef.Name == notification.SourceWantName && ownerRef.Controller && ownerRef.Kind == "Want" {
				childWants = append(childWants, childWant)
				break
			}
		}
	}
	wantRegistryMutex.RUnlock()
	for _, childWant := range childWants {
		fmt.Printf("[PARAMETER CHANGE] %s: Parameter %s changed to %v, setting status to idle for restart\n",
			childWant.Metadata.Name, notification.StateKey, notification.StateValue)
		childWant.SetStatus(WantStatusIdle)
	}
	storeNotificationHistory(notification)
}

// emitParameterChangeEvent emits a parameter change through the unified subscription system
func emitParameterChangeEvent(notification StateNotification) {
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}
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
func sendOwnerChildNotifications(notification StateNotification) {
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}

	if len(want.Metadata.OwnerReferences) > 0 {
		for _, ownerRef := range want.Metadata.OwnerReferences {
			if ownerRef.Controller && ownerRef.Kind == "Want" {
				// Emit through unified subscription system
				emitOwnerChildStateEvent(notification, ownerRef.Name)
				break
			}
		}
	}
}

// emitOwnerChildStateEvent emits an owner-child state notification through the unified subscription system
func emitOwnerChildStateEvent(notification StateNotification, ownerName string) {
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}
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
func storeNotificationHistory(notification StateNotification) {
	historyMutex.Lock()
	defer historyMutex.Unlock()
	notificationHistory = append(notificationHistory, notification)

	// Trim history if too long
	if len(notificationHistory) > maxNotificationHistory {
		notificationHistory = notificationHistory[len(notificationHistory)-maxNotificationHistory:]
	}
}
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
