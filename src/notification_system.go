package mywant

import (
	"fmt"
	"sync"
)

// Global state listener registry
var (
	stateListenerRegistry = make(map[string]StateUpdateListener)
	stateListenerMutex    sync.RWMutex

	// Parameter change listener registry
	parameterListenerRegistry = make(map[string]ParameterChangeListener)
	parameterListenerMutex    sync.RWMutex

	// Subscription tracking
	stateSubscriptions = make(map[string][]StateSubscription) // wantName -> subscriptions
	subscriptionMutex  sync.RWMutex

	// Want registry for notification lookup
	wantRegistry = make(map[string]*Want)
	wantRegistryMutex sync.RWMutex

	// Notification history for debugging
	notificationHistory = make([]StateNotification, 0, 1000)
	historyMutex       sync.RWMutex

	// Configuration
	maxNotificationHistory = 1000
)

// RegisterStateListener registers a want as capable of receiving notifications
func RegisterStateListener(wantName string, listener StateUpdateListener) {
	stateListenerMutex.Lock()
	defer stateListenerMutex.Unlock()
	stateListenerRegistry[wantName] = listener
	fmt.Printf("[NOTIFICATION] Registered state listener: %s\n", wantName)
}

// UnregisterStateListener removes a want from receiving notifications
func UnregisterStateListener(wantName string) {
	stateListenerMutex.Lock()
	defer stateListenerMutex.Unlock()
	delete(stateListenerRegistry, wantName)
	fmt.Printf("[NOTIFICATION] Unregistered state listener: %s\n", wantName)
}

// RegisterParameterListener registers a want as capable of receiving parameter change notifications
func RegisterParameterListener(wantName string, listener ParameterChangeListener) {
	parameterListenerMutex.Lock()
	defer parameterListenerMutex.Unlock()
	parameterListenerRegistry[wantName] = listener
	fmt.Printf("[NOTIFICATION] Registered parameter listener: %s\n", wantName)
}

// UnregisterParameterListener removes a want from receiving parameter change notifications
func UnregisterParameterListener(wantName string) {
	parameterListenerMutex.Lock()
	defer parameterListenerMutex.Unlock()
	delete(parameterListenerRegistry, wantName)
	fmt.Printf("[NOTIFICATION] Unregistered parameter listener: %s\n", wantName)
}

// RegisterStateSubscriptions registers what state changes a want wants to monitor
func RegisterStateSubscriptions(wantName string, subscriptions []StateSubscription) {
	subscriptionMutex.Lock()
	defer subscriptionMutex.Unlock()
	stateSubscriptions[wantName] = subscriptions
	fmt.Printf("[NOTIFICATION] Registered %d subscriptions for: %s\n", len(subscriptions), wantName)
}

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
	// 1. Owner-child notifications (existing system)
	sendOwnerChildNotifications(notification)

	// 2. Subscription-based notifications (new system)
	sendSubscriptionNotifications(notification)

	// 3. Store in history for debugging
	storeNotificationHistory(notification)
}

// sendParameterNotifications handles parameter change propagation to children (reverse direction)
func sendParameterNotifications(notification StateNotification) {
	// Find all wants that have this want as their owner (i.e., children)
	wantRegistryMutex.RLock()
	defer wantRegistryMutex.RUnlock()

	for _, childWant := range wantRegistry {
		// Check if this child has the notification source as its owner
		for _, ownerRef := range childWant.Metadata.OwnerReferences {
			if ownerRef.Name == notification.SourceWantName && ownerRef.Controller && ownerRef.Kind == "Want" {
				// Create parameter notification for child
				childNotification := notification
				childNotification.TargetWantName = childWant.Metadata.Name
				childNotification.NotificationType = NotificationParameter

				// Deliver parameter change to child
				deliverParameterChangeToWant(childWant.Metadata.Name, childNotification)
				break
			}
		}
	}

	// Store in history for debugging
	storeNotificationHistory(notification)
}

// sendOwnerChildNotifications handles parent-child notifications via generic system
func sendOwnerChildNotifications(notification StateNotification) {
	// Find the want to get its OwnerReferences
	want := findWantByName(notification.SourceWantName)
	if want == nil {
		return
	}

	if len(want.Metadata.OwnerReferences) > 0 {
		for _, ownerRef := range want.Metadata.OwnerReferences {
			if ownerRef.Controller && ownerRef.Kind == "Want" {
				// Create notification for parent
				parentNotification := notification
				parentNotification.TargetWantName = ownerRef.Name
				parentNotification.NotificationType = NotificationOwnerChild

				// Use generic notification system for parents
				deliverNotificationToWant(ownerRef.Name, parentNotification)

				// LEGACY: Also call old Target system for backward compatibility
				// This ensures existing Target behavior continues to work
				notifyParentStateUpdate(ownerRef.Name, want.Metadata.Name,
				                      notification.StateKey, notification.StateValue)
				break
			}
		}
	}
}

// sendSubscriptionNotifications implements the new peer-to-peer system
func sendSubscriptionNotifications(notification StateNotification) {
	subscriptionMutex.RLock()
	defer subscriptionMutex.RUnlock()

	// Find all wants that subscribe to this source want
	for subscriberName, subscriptions := range stateSubscriptions {
		for _, subscription := range subscriptions {
			if subscription.WantName == notification.SourceWantName {
				// Check if this state key is subscribed
				if shouldNotifySubscriber(subscription, notification) {
					notification.TargetWantName = subscriberName
					notification.NotificationType = NotificationSubscription
					deliverNotificationToWant(subscriberName, notification)
				}
			}
		}
	}
}

// shouldNotifySubscriber checks subscription filters
func shouldNotifySubscriber(subscription StateSubscription, notification StateNotification) bool {
	// If no state keys specified, subscribe to all
	if len(subscription.StateKeys) == 0 {
		return true
	}

	// Check if state key is in subscription list
	for _, key := range subscription.StateKeys {
		if key == notification.StateKey {
			return true
		}
	}

	return false
}

// deliverNotificationToWant sends notification to a specific want
func deliverNotificationToWant(wantName string, notification StateNotification) {
	stateListenerMutex.RLock()
	listener, exists := stateListenerRegistry[wantName]
	stateListenerMutex.RUnlock()

	if exists {
		go func() {
			if err := listener.OnStateUpdate(notification); err != nil {
				fmt.Printf("[NOTIFICATION ERROR] Failed to deliver to %s: %v\n", wantName, err)
			}
		}()
	}
}

// deliverParameterChangeToWant sends parameter change notification to a specific want
func deliverParameterChangeToWant(wantName string, notification StateNotification) {
	parameterListenerMutex.RLock()
	listener, exists := parameterListenerRegistry[wantName]
	parameterListenerMutex.RUnlock()

	if exists {
		go func() {
			if err := listener.OnParameterChange(notification); err != nil {
				fmt.Printf("[PARAMETER ERROR] Failed to deliver parameter change to %s: %v\n", wantName, err)
			}
		}()
	} else {
		fmt.Printf("[PARAMETER INFO] No parameter listener registered for %s\n", wantName)
	}
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

// GetRegisteredListeners returns all registered state listeners for debugging
func GetRegisteredListeners() []string {
	stateListenerMutex.RLock()
	defer stateListenerMutex.RUnlock()

	listeners := make([]string, 0, len(stateListenerRegistry))
	for name := range stateListenerRegistry {
		listeners = append(listeners, name)
	}
	return listeners
}

// GetSubscriptions returns all subscriptions for debugging
func GetSubscriptions() map[string][]StateSubscription {
	subscriptionMutex.RLock()
	defer subscriptionMutex.RUnlock()

	result := make(map[string][]StateSubscription)
	for name, subs := range stateSubscriptions {
		result[name] = make([]StateSubscription, len(subs))
		copy(result[name], subs)
	}
	return result
}