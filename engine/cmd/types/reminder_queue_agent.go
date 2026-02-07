package types

import (
	"context"
	"encoding/json"
	"fmt"

	mywant "mywant/engine/src"
)

// NewReminderQueueAgent creates a DoAgent that manages reaction queue lifecycle
// This agent creates queues when reminder enters waiting phase and deletes them when completed/failed
func NewReminderQueueAgent() *mywant.DoAgent {
	agent := &mywant.DoAgent{
		BaseAgent: *mywant.NewBaseAgent(
			"reminder_queue_manager",
			[]string{"reminder_queue_management"},
			mywant.DoAgentType,
		),
	}

	agent.Action = func(ctx context.Context, want *mywant.Want) error {
		return manageReactionQueue(ctx, want)
	}

	return agent
}

// manageReactionQueue handles queue creation and deletion based on reminder phase
func manageReactionQueue(ctx context.Context, want *mywant.Want) error {
	// Get current reminder phase
	phase, ok := want.GetStateString("reminder_phase", "")
	if !ok || phase == "" {
		// No phase set yet, nothing to do
		return nil
	}

	// Get HTTP client for API calls
	httpClient := want.GetHTTPClient()
	if httpClient == nil {
		return fmt.Errorf("HTTP client not available - cannot manage reaction queue")
	}

	// Check if we already have a queue ID
	existingQueueID, _ := want.GetStateString("reaction_queue_id", "")

	// Handle phase transitions
	switch phase {
	case ReminderPhaseWaiting, ReminderPhaseReaching:
		// Create queue when entering waiting or reaching phase
		// If we already have a queue ID, delete it first to ensure a new DIFFERENT one is created (avoid reuse)
		if existingQueueID != "" && phase == ReminderPhaseWaiting {
			want.StoreLog("[INFO] Deleting existing reaction queue %s before creating a new one for fresh start", existingQueueID)
			_ = deleteReactionQueue(httpClient, existingQueueID)
			existingQueueID = ""
		}

		if existingQueueID == "" {
			queueID, err := createReactionQueue(httpClient)
			if err != nil {
				want.StoreLog("[ERROR] Failed to create reaction queue for want %s: %v", want.Metadata.ID, err)
				return err
			}

			// Store queue ID in want state
			want.StoreState("reaction_queue_id", queueID)
			want.StoreLog("[INFO] Created reaction queue %s for reminder want %s (phase: %s)", queueID, want.Metadata.ID, phase)
		}

	case ReminderPhaseCompleted, ReminderPhaseFailed:
		// Delete queue when completed or failed (if queue exists)
		if existingQueueID != "" {
			err := deleteReactionQueue(httpClient, existingQueueID)
			if err != nil {
				// Log error but don't fail - queue cleanup is not critical
				want.StoreLog("[WARN] Failed to delete reaction queue %s for want %s: %v", existingQueueID, want.Metadata.ID, err)
			} else {
				want.StoreLog("[INFO] Deleted reaction queue %s for reminder want %s", existingQueueID, want.Metadata.ID)
				// Clear queue ID from state
				want.StoreState("reaction_queue_id", "")
			}
		}
	}

	return nil
}

// createReactionQueue calls POST /api/v1/reactions/ to create a new queue
func createReactionQueue(httpClient *mywant.HTTPClient) (string, error) {
	resp, err := httpClient.POST("/api/v1/reactions/", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create reaction queue: %w", err)
	}

	var result struct {
		QueueID   string `json:"queue_id"`
		CreatedAt string `json:"created_at"`
	}

	if err := httpClient.DecodeJSON(resp, &result); err != nil {
		return "", fmt.Errorf("failed to decode queue creation response: %w", err)
	}

	if result.QueueID == "" {
		return "", fmt.Errorf("queue creation returned empty queue ID")
	}

	return result.QueueID, nil
}

// deleteReactionQueue calls DELETE /api/v1/reactions/{id} to delete a queue
func deleteReactionQueue(httpClient *mywant.HTTPClient, queueID string) error {
	path := fmt.Sprintf("/api/v1/reactions/%s", queueID)
	resp, err := httpClient.DELETE(path)
	if err != nil {
		return fmt.Errorf("failed to delete reaction queue: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		QueueID string `json:"queue_id"`
		Deleted bool   `json:"deleted"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode queue deletion response: %w", err)
	}

	if !result.Deleted {
		return fmt.Errorf("queue deletion failed for queue %s", queueID)
	}

	return nil
}

// RegisterReminderQueueAgent registers the reminder queue management agent
func RegisterReminderQueueAgent(registry *mywant.AgentRegistry) error {
	agent := NewReminderQueueAgent()
	registry.RegisterAgent(agent)
	return nil
}
