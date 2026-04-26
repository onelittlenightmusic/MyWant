package mywant

import (
	"context"
	"fmt"
	"sync"
)

// paramExposeHandler handles ParameterChangeEvent from upper scope (global or parent want)
// and propagates the value to the local param via UpdateParameter.
// Used for exposes entries with Param field (top-down propagation).
type paramExposeHandler struct {
	want         *Want
	sourceFilter string // "__global__" for top-level, parent name for child wants
	upperKey     string // param name in the upper scope (ExposeEntry.As)
	localKey     string // local param key in this want (ExposeEntry.Param)
}

func (h *paramExposeHandler) GetSubscriberName() string {
	return fmt.Sprintf("%s:param-expose:%s→%s", h.want.Metadata.Name, h.upperKey, h.localKey)
}

func (h *paramExposeHandler) OnEvent(ctx context.Context, event WantEvent) EventResponse {
	pce, ok := event.(*ParameterChangeEvent)
	if !ok {
		return EventResponse{}
	}
	if pce.GetSourceName() != h.sourceFilter || pce.ParamName != h.upperKey {
		return EventResponse{}
	}
	h.want.UpdateParameter(h.localKey, pce.ParamValue)
	return EventResponse{Handled: true}
}

// globalStateExposeHandler handles StateChangeEvent emitted by a top-level want and
// propagates the value directly to global state as a flat key.
// Used for exposes entries with CurrentState field on top-level (no parent) wants.
type globalStateExposeHandler struct {
	want     *Want
	localKey string // state key in this want (ExposeEntry.CurrentState)
	globalKey string // key to store in global state (ExposeEntry.As)
}

func (h *globalStateExposeHandler) GetSubscriberName() string {
	return fmt.Sprintf("%s:global-state-expose:%s→%s", h.want.Metadata.Name, h.localKey, h.globalKey)
}

func (h *globalStateExposeHandler) OnEvent(ctx context.Context, event WantEvent) EventResponse {
	sce, ok := event.(*StateChangeEvent)
	if !ok {
		return EventResponse{}
	}
	if sce.GetSourceName() != h.want.Metadata.Name || sce.StateKey != h.localKey {
		return EventResponse{}
	}
	StoreGlobalState(h.globalKey, sce.StateValue)
	return EventResponse{Handled: true}
}

// currentStateExposeHandler handles StateChangeEvent emitted by this want and
// propagates the value to the parent want's state as a different key.
// Used for exposes entries with CurrentState field (bottom-up propagation).
type currentStateExposeHandler struct {
	want       *Want
	localKey   string // state key in this want (ExposeEntry.CurrentState)
	parentName string // parent want name to push the state to
	parentKey  string // state key in parent (ExposeEntry.As)
}

func (h *currentStateExposeHandler) GetSubscriberName() string {
	return fmt.Sprintf("%s:state-expose:%s→%s.%s", h.want.Metadata.Name, h.localKey, h.parentName, h.parentKey)
}

func (h *currentStateExposeHandler) OnEvent(ctx context.Context, event WantEvent) EventResponse {
	sce, ok := event.(*StateChangeEvent)
	if !ok {
		return EventResponse{}
	}
	if sce.GetSourceName() != h.want.Metadata.Name || sce.StateKey != h.localKey {
		return EventResponse{}
	}
	if h.parentKey == "final_result" {
		// Special convention: set local final_result and propagate to parent/global via MergeParentState.
		h.want.StoreState("final_result", sce.StateValue)
		h.want.MergeParentState(map[string]any{
			"wants": map[string]any{h.want.Metadata.Name: sce.StateValue},
		})
		return EventResponse{Handled: true}
	}
	parent := findWantByName(h.parentName)
	if parent == nil {
		return EventResponse{}
	}
	parent.StoreState(h.parentKey, sce.StateValue)
	return EventResponse{Handled: true}
}

// Global want registry for notification lookup
var (
	wantRegistry      = make(map[string]*Want)
	wantRegistryMutex sync.RWMutex

	// Notification history ring buffer (lock-free, fixed capacity)
	notificationRing = newRingBuffer[StateNotification](1000)
)

// finalResultExposeSubscriberName returns the subscriber name for the auto-generated
// final_result → parent propagation handler (registered when FinalResultField is set).
func finalResultExposeSubscriberName(wantName string) string {
	return fmt.Sprintf("%s:state-expose:final_result→.final_result", wantName)
}

// getControllerParentName returns the parent want's name from OwnerReferences
// without calling GetParentWant (which takes reconcileMutex and can deadlock
// when called from within the reconcile loop).
func getControllerParentName(want *Want) string {
	for _, ref := range want.Metadata.OwnerReferences {
		if ref.Controller && ref.Kind == "Want" {
			return ref.Name
		}
	}
	return ""
}

// RegisterWant registers a want for notification lookup and sets up expose subscriptions.
// NOTE: This is called from addWant inside the reconcile loop while reconcileMutex
// is held. Must NOT call GetParentWant/FindWantByID (they take reconcileMutex → deadlock).
// Use getControllerParentName instead.
func RegisterWant(want *Want) {
	wantRegistryMutex.Lock()
	wantRegistry[want.Metadata.Name] = want
	wantRegistryMutex.Unlock()

	parentName := getControllerParentName(want)

	// Auto-register handler for FinalResultField shorthand:
	// When final_result state changes, propagate to parent/global via MergeParentState.
	// (EndProgressCycle handles the source→final_result copy with dot-notation and zero-value skipping.)
	if want.Spec.FinalResultField != "" {
		handler := &currentStateExposeHandler{
			want:       want,
			localKey:   "final_result",
			parentName: parentName,
			parentKey:  "final_result",
		}
		want.GetSubscriptionSystem().Subscribe(EventTypeStateChange, handler)
	}

	// Subscribe handlers for each expose entry
	for _, entry := range want.Spec.Exposes {
		if entry.Param != "" {
			// Top-down: receive param from upper scope (global or parent)
			var sourceFilter string
			if parentName == "" {
				sourceFilter = "__global__"
			} else {
				sourceFilter = parentName
			}
			handler := &paramExposeHandler{
				want:         want,
				sourceFilter: sourceFilter,
				upperKey:     entry.As,
				localKey:     entry.Param,
			}
			want.GetSubscriptionSystem().Subscribe(EventTypeParameterChange, handler)
		} else if entry.CurrentState != "" {
			// Bottom-up: push state to parent when local state changes.
			// "final_result" is handled specially via MergeParentState, so it works for top-level wants too.
			if parentName == "" && entry.As != "final_result" {
				// Top-level want: expose directly to global state as a flat key.
				handler := &globalStateExposeHandler{
					want:      want,
					localKey:  entry.CurrentState,
					globalKey: entry.As,
				}
				want.GetSubscriptionSystem().Subscribe(EventTypeStateChange, handler)
				continue
			}
			handler := &currentStateExposeHandler{
				want:       want,
				localKey:   entry.CurrentState,
				parentName: parentName,
				parentKey:  entry.As,
			}
			want.GetSubscriptionSystem().Subscribe(EventTypeStateChange, handler)
		}
	}
}

// UnregisterWant removes a want from the registry and cleans up expose subscriptions.
func UnregisterWant(wantName string) {
	wantRegistryMutex.Lock()
	want := wantRegistry[wantName]
	delete(wantRegistry, wantName)
	wantRegistryMutex.Unlock()

	if want == nil {
		return
	}
	// Unsubscribe auto-generated FinalResultField handler
	if want.Spec.FinalResultField != "" {
		want.GetSubscriptionSystem().Unsubscribe(EventTypeStateChange, finalResultExposeSubscriberName(wantName))
	}
	// Unsubscribe all expose handlers
	parentName := getControllerParentName(want)
	for _, entry := range want.Spec.Exposes {
		if entry.Param != "" {
			subscriberName := fmt.Sprintf("%s:param-expose:%s→%s", wantName, entry.As, entry.Param)
			want.GetSubscriptionSystem().Unsubscribe(EventTypeParameterChange, subscriberName)
		} else if entry.CurrentState != "" {
			if parentName == "" && entry.As != "final_result" {
				subscriberName := fmt.Sprintf("%s:global-state-expose:%s→%s", wantName, entry.CurrentState, entry.As)
				want.GetSubscriptionSystem().Unsubscribe(EventTypeStateChange, subscriberName)
				continue
			}
			subscriberName := fmt.Sprintf("%s:state-expose:%s→%s.%s", wantName, entry.CurrentState, parentName, entry.As)
			want.GetSubscriptionSystem().Unsubscribe(EventTypeStateChange, subscriberName)
		}
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
		// Skip wants whose target_param matches the changed key — they are the
		// *source* of the change (e.g. a slider propagating its value to the parent).
		// Restarting the source would reset its value and trigger an infinite loop.
		if targetParam, ok := childWant.GetParameter("target_param"); ok {
			if targetParam == notification.StateKey {
				DebugLog("[PARAMETER CHANGE] %s: skipping restart (source of change via target_param=%s)\n",
					childWant.Metadata.Name, notification.StateKey)
				continue
			}
		}
		// Skip wants that receive this param via exposes — the paramExposeHandler
		// subscription already calls UpdateParameter on them, and their background
		// ThinkAgent picks up the new value on its next tick. Restarting would
		// stop the ThinkAgent goroutine unnecessarily.
		hasExposeForKey := false
		for _, entry := range childWant.Spec.Exposes {
			if entry.Param != "" && entry.As == notification.StateKey {
				hasExposeForKey = true
				break
			}
		}
		if hasExposeForKey {
			DebugLog("[PARAMETER CHANGE] %s: skipping restart (receives %s via exposes)\n",
				childWant.Metadata.Name, notification.StateKey)
			continue
		}
		// Skip wants that don't have the changed key bound in their own Spec.Params.
		// Only restart wants that actually use this parameter — e.g. if "budget"
		// changes, only restart wants whose params include "budget", not all children.
		if _, hasBoundParam := childWant.GetParameter(notification.StateKey); !hasBoundParam {
			DebugLog("[PARAMETER CHANGE] %s: skipping restart (param %s not bound)\n",
				childWant.Metadata.Name, notification.StateKey)
			continue
		}
		DebugLog("[PARAMETER CHANGE] %s: Parameter %s changed to %v, restarting execution\n",
			childWant.Metadata.Name, notification.StateKey, notification.StateValue)
		cb := GetGlobalChainBuilder()
		if cb != nil {
			if err := cb.RestartWant(childWant.Metadata.ID); err != nil {
				childWant.RestartWant() // fallback: set idle without reconcile trigger
			}
		} else {
			childWant.RestartWant()
		}
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
