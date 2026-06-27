package types

import (
	"log"

	. "mywant/engine/core"
)

// ConsumeWebhookAction deduplicates and dispatches a pending webhook_payload for
// user-control want types (button, switch, choice, slider, timer, etc.).
//
// Pattern:
//  1. Read webhook_payload + webhook_received_at from current state.
//  2. Skip if no new event (receivedAt == lastAtKey value or empty).
//  3. Extract "action" and full payload map; call handler.
//  4. If handler returns true (event was handled): update lastAtKey and clear webhook_payload.
//
// Returns true when handler was called and returned true.
func ConsumeWebhookAction(w *Want, lastAtKey string, handler func(action string, payload map[string]any) bool) bool {
	payload, hasPayload := w.GetCurrent("webhook_payload")
	if !hasPayload || payload == nil {
		return false
	}
	receivedAt, _ := w.GetStateString("webhook_received_at", "")
	lastAt, _ := w.GetStateString(lastAtKey, "")
	if receivedAt == "" || receivedAt == lastAt {
		return false
	}
	pm, ok := payload.(map[string]any)
	if !ok {
		log.Printf("[ConsumeWebhookAction] want=%s: webhook_payload is not a map (type=%T) — skipping", w.Metadata.Name, payload)
		return false
	}
	action, _ := pm["action"].(string)
	if !handler(action, pm) {
		log.Printf("[ConsumeWebhookAction] want=%s: handler rejected action=%q", w.Metadata.Name, action)
		return false
	}
	w.StoreState(lastAtKey, receivedAt)
	w.SetCurrent("webhook_payload", nil)
	return true
}
