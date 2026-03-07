package types

import (
	"context"
	"encoding/json"
	"fmt"

	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterDoAgent("reminder_queue_manager", manageReactionQueue)
}

// manageReactionQueue handles queue creation and deletion based on reminder phase
func manageReactionQueue(ctx context.Context, want *mywant.Want) error {
	// Get current reminder phase and queue ID using generic accessors
	phase := mywant.GetCurrent(want, "reminder_phase", "")
	existingQueueID := mywant.GetCurrent(want, "reaction_queue_id", "")

	if phase == "" { return nil }

	httpClient := want.GetHTTPClient()
	if httpClient == nil { return fmt.Errorf("no http client") }

	switch phase {
	case ReminderPhaseWaiting, ReminderPhaseReaching:
		if existingQueueID != "" && phase == ReminderPhaseWaiting {
			_ = deleteReactionQueue(httpClient, existingQueueID)
			existingQueueID = ""
		}

		if existingQueueID == "" {
			queueID, err := createReactionQueue(httpClient)
			if err != nil { return err }
			
			// Get reaction type to see if we should set webhook_url
			reactionType := mywant.GetGoal(want, "reaction_type", "internal")
			
			want.SetCurrent("reaction_queue_id", queueID)
			
			// Always set webhook_url if we have a queue ID, as it's the endpoint for reactions
			webhookURL := fmt.Sprintf("/api/v1/reactions/%s", queueID)
			want.SetCurrent("webhook_url", webhookURL)
			
			want.StoreLog("[INFO] Created reaction queue %s (type: %s)", queueID, reactionType)
		}

	case ReminderPhaseCompleted, ReminderPhaseFailed:
		if existingQueueID != "" {
			err := deleteReactionQueue(httpClient, existingQueueID)
			if err == nil {
				want.SetCurrent("reaction_queue_id", "")
				want.StoreLog("[INFO] Deleted reaction queue %s", existingQueueID)
			}
		}
	}

	return nil
}

func createReactionQueue(httpClient *mywant.HTTPClient) (string, error) {
	resp, err := httpClient.POST("/api/v1/reactions/", nil)
	if err != nil { return "", err }
	var result struct { QueueID string `json:"queue_id"` }
	if err := httpClient.DecodeJSON(resp, &result); err != nil { return "", err }
	return result.QueueID, nil
}

func deleteReactionQueue(httpClient *mywant.HTTPClient, queueID string) error {
	path := fmt.Sprintf("/api/v1/reactions/%s", queueID)
	resp, err := httpClient.DELETE(path); if err != nil { return err }
	defer resp.Body.Close()
	var result struct { Deleted bool `json:"deleted"` }
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil { return err }
	if !result.Deleted { return fmt.Errorf("not deleted") }
	return nil
}
