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
	var phase string
	var existingQueueID string

	// Get current reminder phase and queue ID
	want.GetStateMulti(mywant.Dict{
		"reminder_phase":    &phase,
		"reaction_queue_id": &existingQueueID,
	})

	// Prefer GCP current if available
	if p, ok := want.GetCurrent("phase"); ok && p != nil { phase = p.(string) }
	if q, ok := want.GetCurrent("reaction_queue_id"); ok && q != nil { existingQueueID = q.(string) }

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
			want.SetCurrent("reaction_queue_id", queueID)
			want.StoreLog("[INFO] Created reaction queue %s", queueID)
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
