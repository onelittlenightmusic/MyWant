package mywant

import (
	"context"
	"fmt"
	"sync"
)

// exposeAsHandler handles ParameterChangeEvent from upper scope (global or parent want)
// and propagates the value to the local param via UpdateParameter.
type exposeAsHandler struct {
	want         *Want
	sourceFilter string // "__global__" for top-level, parent name for child wants
	exposeAs     string // the key name in the upper scope
	localKey     string // the local param key in this want
}

func (h *exposeAsHandler) GetSubscriberName() string {
	return fmt.Sprintf("%s:expose:%s→%s", h.want.Metadata.Name, h.exposeAs, h.localKey)
}

func (h *exposeAsHandler) OnEvent(ctx context.Context, event WantEvent) EventResponse {
	pce, ok := event.(*ParameterChangeEvent)
	if !ok {
		return EventResponse{}
	}
	if pce.GetSourceName() != h.sourceFilter || pce.ParamName != h.exposeAs {
		return EventResponse{}
	}
	h.want.UpdateParameter(h.localKey, pce.ParamValue)
	return EventResponse{Handled: true}
}

// Global want registry for notification lookup
var (
	wantRegistry      = make(map[string]*Want)
	wantRegistryMutex sync.RWMutex

	// Notification history ring buffer (lock-free, fixed capacity)
	notificationRing = newRingBuffer[StateNotification](1000)
)

// RegisterWant registers a want for notification lookup
func RegisterWant(want *Want) {
	wantRegistryMutex.Lock()
	wantRegistry[want.Metadata.Name] = want
	wantRegistryMutex.Unlock()

	// Subscribe for exposeAs param propagation
	for localKey, exposeAs := range want.Spec.ParamExposes {
		parent := want.GetParentWant()
		var sourceFilter string
		if parent == nil {
			sourceFilter = "__global__"
		} else {
			sourceFilter = parent.Metadata.Name
		}
		handler := &exposeAsHandler{
			want:         want,
			sourceFilter: sourceFilter,
			exposeAs:     exposeAs,
			localKey:     localKey,
		}
		want.GetSubscriptionSystem().Subscribe(EventTypeParameterChange, handler)
	}
}

// UnregisterWant removes a want from the registry
func UnregisterWant(wantName string) {
	wantRegistryMutex.Lock()
	want := wantRegistry[wantName]
	delete(wantRegistry, wantName)
	wantRegistryMutex.Unlock()

	if want == nil {
		return
	}
	// Unsubscribe exposeAs handlers
	for localKey, exposeAs := range want.Spec.ParamExposes {
		subscriberName := fmt.Sprintf("%s:expose:%s→%s", wantName, exposeAs, localKey)
		want.GetSubscriptionSystem().Unsubscribe(EventTypeParameterChange, subscriberName)
	}
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
		DebugLog("[PARAMETER CHANGE] %s: Parameter %s changed to %v, restarting execution\n",
			childWant.Metadata.Name, notification.StateKey, notification.StateValue)
		childWant.RestartWant()
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
	notificationRing.Append(notification)
}

func GetNotificationHistory(limit int) []StateNotification {
	return notificationRing.Snapshot(limit)
}

// ClearNotificationHistory clears the notification history.
func ClearNotificationHistory() {
	notificationRing.Clear()
}

// GetRegisteredListeners returns the list of subscriber names currently registered
// in the global unified subscription system. This is useful for debugging and
// for demo code to introspect active listeners.
func GetRegisteredListeners() []string {
	uss := GetGlobalSubscriptionSystem()
	uss.mutex.RLock()
	defer uss.mutex.RUnlock()

	seen := make(map[string]bool)
	names := make([]string, 0)
	for _, subs := range uss.subscriptions {
		for _, s := range subs {
			name := s.GetSubscriberName()
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// GetSubscriptions returns a map of subscriber -> StateSubscriptions declared
// on each registered want. This is intended for demo and debugging purposes.
func GetSubscriptions() map[string][]StateSubscription {
	wantRegistryMutex.RLock()
	defer wantRegistryMutex.RUnlock()

	result := make(map[string][]StateSubscription)
	for _, w := range wantRegistry {
		if len(w.Spec.StateSubscriptions) > 0 {
			// copy to avoid sharing underlying slice
			subsCopy := make([]StateSubscription, len(w.Spec.StateSubscriptions))
			copy(subsCopy, w.Spec.StateSubscriptions)
			result[w.Metadata.Name] = subsCopy
		}
	}
	return result
}
