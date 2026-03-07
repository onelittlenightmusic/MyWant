package types

import (
	"context"
	"fmt"
	. "mywant/engine/core"
)

func init() {
	RegisterDoAgent("user_reaction_do", performAutoApproval)
}

func performAutoApproval(ctx context.Context, want *Want) error {
	want.StoreLog("[SILENCER:DO] Starting auto-approval check")
	
	reactionID := GetInternal(want, "target_reaction_id", "")
	
	if reactionID == "" {
		want.StoreLog("[SILENCER:DO] No target reaction ID found")
		return nil
	}

	httpClient := want.GetHTTPClient()
	if httpClient == nil { return fmt.Errorf("no http client") }

	requestBody := map[string]any{
		"approved": true,
		"comment":  fmt.Sprintf("Auto-approved by Silencer for '%s'", want.Metadata.Name),
	}

	path := fmt.Sprintf("/api/v1/reactions/%s", reactionID)
	resp, err := httpClient.PUT(path, requestBody)
	if err != nil { return err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 { return fmt.Errorf("status %d", resp.StatusCode) }

	want.StoreLog("[SILENCER:DO] Approved reaction %s", reactionID)
	want.SetInternal("target_reaction_id", "")

	return nil
}
