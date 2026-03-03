package mywant

import (
	"context"
	"time"
)

// DispatchThinkerName is the identifier for the dispatching think agent.
const DispatchThinkerName = "dispatch-thinker"

// DispatchRequest represents a request to create a new child want.
// It is stored in the want's state under the "_dispatch_queue" key.
type DispatchRequest struct {
	Action string         `json:"action"`
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
	Series string         `json:"series,omitempty"`
	Version int           `json:"version,omitempty"`
	RequesterID string    `json:"requester_id,omitempty"`
}

// NewDispatchThinker creates a ThinkingAgent that monitors the want's state
// for "_dispatch_queue" and dispatches new child wants accordingly.
func NewDispatchThinker(id string) *ThinkingAgent {
	return NewThinkingAgent(id, 1*time.Second, DispatchThinkerName, func(ctx context.Context, w *Want) error {
		// 1. Check for pending dispatch requests in state
		var queue []DispatchRequest
		queueRaw, exists := w.GetState("_dispatch_queue")
		if !exists || queueRaw == nil {
			return nil
		}

		// Convert from interface{} to []DispatchRequest
		// In a real system, use json.Marshal/Unmarshal or a type-safe map conversion
		switch v := queueRaw.(type) {
		case []DispatchRequest:
			queue = v
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					req := DispatchRequest{
						Action: m["action"].(string),
						Type:   m["type"].(string),
						Name:   m["name"].(string),
						Params: m["params"].(map[string]any),
					}
					if s, ok := m["series"].(string); ok { req.Series = s }
					if ver, ok := m["version"].(float64); ok { req.Version = int(ver) }
					if rID, ok := m["requester_id"].(string); ok { req.RequesterID = rID }
					queue = append(queue, req)
				}
			}
		default:
			return nil
		}

		if len(queue) == 0 {
			return nil
		}

		// 2. Process each request
		for _, req := range queue {
			w.StoreLog("[%s] Dispatching new child want for action '%s' (type: %s)", DispatchThinkerName, req.Action, req.Type)

			childID := GenerateUUID()
			child := &Want{
				Metadata: Metadata{
					ID:   childID,
					Name: req.Name,
					Type: req.Type,
					Labels: map[string]string{
						"action":     req.Action,
						"owner-name": w.Metadata.Name,
					},
					Series:  req.Series,
					Version: req.Version,
				},
				Spec: WantSpec{
					Params: req.Params,
				},
			}

			// If the requester provided their ID, add it as a label for easier resolution
			if req.RequesterID != "" {
				child.Metadata.Labels["itinerary"] = req.RequesterID
			}

			if err := w.AddChildWant(child); err != nil {
				w.StoreLog("[%s] ERROR dispatching child: %v", DispatchThinkerName, err)
				continue
			}
		}

		// 3. Clear the queue
		w.StoreState("_dispatch_queue", []DispatchRequest{})
		return nil
	})
}
