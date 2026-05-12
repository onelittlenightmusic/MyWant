package types

import (
	"context"
	"encoding/json"
	"fmt"
	. "mywant/engine/core"
)

const userReactionMonitorAgentName = "user_reaction_monitor"

func init() {
	RegisterWithInit(func() {
		RegisterMonitorAgent(userReactionMonitorAgentName, pollUserReactions)
	})
}

func pollUserReactions(ctx context.Context, want *Want) (bool, error) {
	status := want.GetStatus()

	// Stop only on terminal states; keep running while Idle (waiting for reaching_time)
	switch status {
	case WantStatusAchieved, WantStatusAchievedWithWarning,
		WantStatusFailed, WantStatusTerminated,
		WantStatusModuleError, WantStatusConfigError, WantStatusCancelled:
		return true, nil
	}

	// Only poll the reaction queue when in the active reaction phase
	if status != WantStatusReaching && status != WantStatusWaitingUserAction {
		return false, nil
	}

	err := monitorUserReactions(ctx, want)
	if err != nil {
		return false, err
	}

	userReaction := GetCurrent(want, "user_reaction", map[string]any{})
	if len(userReaction) > 0 {
		if _, ok := userReaction["approved"].(bool); ok {
			want.StoreLog("[MONITOR] Valid reaction received, stopping monitor")
			return true, nil
		}
	}

	return false, nil
}

func monitorUserReactions(ctx context.Context, want *Want) error {
	if want.Metadata.Type != "reminder" {
		return nil
	}

	status := want.GetStatus()
	requireReaction := GetGoal(want, "require_reaction", false)
	queueID := GetCurrent(want, "reaction_queue_id", "")

	if status != WantStatusReaching && status != WantStatusWaitingUserAction {
		return nil
	}
	if !requireReaction || queueID == "" {
		return nil
	}

	httpClient := want.GetHTTPClient()
	if httpClient == nil {
		return nil
	}

	path := fmt.Sprintf("/api/v1/reactions/%s", queueID)
	resp, err := httpClient.GET(path)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		QueueID   string         `json:"queue_id"`
		Reactions []ReactionData `json:"reactions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	if len(result.Reactions) == 0 {
		return nil
	}

	reaction := result.Reactions[0]
	reactionData := map[string]any{
		"approved":  reaction.Approved,
		"comment":   reaction.Comment,
		"timestamp": reaction.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
	}

	want.SetCurrent("user_reaction", reactionData)
	want.SetCurrent("action_by_agent", "MonitorAgent")

	if reaction.Approved {
		want.StoreLog("User approved reminder reaction")
	} else {
		want.StoreLog("User rejected reminder reaction")
	}

	return nil
}
