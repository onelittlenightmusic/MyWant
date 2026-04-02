package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	. "mywant/engine/core"
)

const goalThinkerAgentName = "goal_thinker"

func init() {
	RegisterThinkAgentType(goalThinkerAgentName, []Capability{
		{Name: "goal_thinking", Gives: []string{"goal_thinking"}, Description: "Decomposes a user goal into sub-wants and monitors conversation-based replanning"},
	}, goalThinkerThink)
}

// goalThinkerThink is the ThinkAgent function that implements the Goal Thinker lifecycle.
// It drives the goal want through four phases:
//
//   - decomposing      → call Python script → store proposed_breakdown → create reaction queue → awaiting_approval
//   - awaiting_approval → poll reaction queue → on approved: AddChildWant per item → monitoring; on rejected: decomposing
//   - monitoring       → detect cc_message_count changes → on change: re_planning
//   - re_planning      → call Python script (replan) → update proposed_breakdown → create queue → awaiting_approval
func goalThinkerThink(ctx context.Context, want *Want) error {
	phase := GetCurrent(want, "phase", "decomposing")

	switch phase {
	case "decomposing":
		return goalThinkerDecompose(ctx, want)
	case "awaiting_approval":
		return goalThinkerAwaitApproval(ctx, want)
	case "monitoring":
		return goalThinkerMonitor(ctx, want)
	case "re_planning":
		return goalThinkerReplan(ctx, want)
	default:
		want.DirectLog("[GOAL-THINKER] Unknown phase: %s", phase)
		return nil
	}
}

// goalThinkerDecompose calls the Python script to break down the goal_text into sub-wants.
func goalThinkerDecompose(ctx context.Context, want *Want) error {
	goalText := GetGoal(want, "goal_text", "")
	if goalText == "" {
		want.DirectLog("[GOAL-THINKER] goal_text is empty, skipping decompose")
		return nil
	}

	// Avoid re-running if already in progress (guarded by proposed_breakdown presence)
	existingBreakdown := GetCurrent(want, "proposed_breakdown", []any{})
	if len(existingBreakdown) > 0 {
		// Already decomposed but didn't transition — skip re-running
		return nil
	}

	want.DirectLog("[GOAL-THINKER] Decomposing goal: %s", goalText)

	result, err := callGoalThinkerScript(ctx, map[string]any{
		"phase":     "decompose",
		"goal_text": goalText,
	})
	if err != nil {
		want.DirectLog("[GOAL-THINKER] Script error during decompose: %v", err)
		return nil
	}

	breakdown, _ := result["breakdown"].([]any)
	responseText, _ := result["response_text"].(string)

	want.SetCurrent("proposed_breakdown", breakdown)
	want.SetCurrent("proposed_response", responseText)
	want.DirectLog("[GOAL-THINKER] Decomposed into %d items: %s", len(breakdown), responseText)

	// Create reaction queue for user approval
	httpClient := want.GetHTTPClient()
	if httpClient == nil {
		want.DirectLog("[GOAL-THINKER] No HTTP client available")
		return nil
	}
	queueID, err := createGoalReactionQueue(httpClient)
	if err != nil {
		want.DirectLog("[GOAL-THINKER] Failed to create reaction queue: %v", err)
		return nil
	}

	want.SetCurrent("reaction_queue_id", queueID)
	want.SetCurrent("phase", "awaiting_approval")
	want.SetStatus(WantStatusWaitingUserAction)
	want.DirectLog("[GOAL-THINKER] Reaction queue created: %s, transitioning to awaiting_approval", queueID)

	return nil
}

// goalThinkerAwaitApproval polls the reaction queue and transitions based on the user's decision.
func goalThinkerAwaitApproval(ctx context.Context, want *Want) error {
	queueID := GetCurrent(want, "reaction_queue_id", "")
	if queueID == "" {
		want.DirectLog("[GOAL-THINKER] No reaction_queue_id, returning to decomposing")
		want.SetCurrent("phase", "decomposing")
		want.SetCurrent("proposed_breakdown", []any{})
		want.SetCurrent("proposed_response", "")
		return nil
	}

	httpClient := want.GetHTTPClient()
	if httpClient == nil {
		return nil
	}

	path := fmt.Sprintf("/api/v1/reactions/%s", queueID)
	resp, err := httpClient.GET(path)
	if err != nil {
		want.DirectLog("[GOAL-THINKER] Failed to poll reaction queue: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		QueueID   string         `json:"queue_id"`
		Reactions []ReactionData `json:"reactions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		want.DirectLog("[GOAL-THINKER] Failed to decode reaction queue response: %v", err)
		return nil
	}

	if len(result.Reactions) == 0 {
		// No reactions yet — stay in awaiting_approval
		return nil
	}

	reaction := result.Reactions[0]

	// Delete the queue now that we have a reaction
	_ = deleteReactionQueue(httpClient, queueID)
	want.SetCurrent("reaction_queue_id", "")
	want.SetStatus(WantStatusReaching)

	if reaction.Approved {
		want.DirectLog("[GOAL-THINKER] User approved breakdown — spawning child wants")

		breakdown := GetCurrent(want, "proposed_breakdown", []any{})
		for _, item := range breakdown {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if err := spawnChildWantFromBreakdownItem(want, itemMap); err != nil {
				want.DirectLog("[GOAL-THINKER] Failed to spawn child want: %v", err)
			}
		}

		// Track the last cc_message_count for change detection
		currentCount := toInt(GetCurrent(want, "cc_message_count", 0))
		want.StoreState("_last_cc_message_count", currentCount)
		want.SetCurrent("phase", "monitoring")
		want.DirectLog("[GOAL-THINKER] Transitioned to monitoring phase")
	} else {
		want.DirectLog("[GOAL-THINKER] User rejected breakdown — returning to decomposing")
		want.SetCurrent("phase", "decomposing")
		want.SetCurrent("proposed_breakdown", []any{})
		want.SetCurrent("proposed_response", "")
	}

	return nil
}

// goalThinkerMonitor watches cc_message_count for changes that trigger a replan.
func goalThinkerMonitor(ctx context.Context, want *Want) error {
	lastCount := GetState[int](want, "_last_cc_message_count", 0)
	currentCount := toInt(GetCurrent(want, "cc_message_count", 0))

	if currentCount > lastCount {
		want.StoreState("_last_cc_message_count", currentCount)
		want.DirectLog("[GOAL-THINKER] cc_message_count changed (%d → %d), triggering replan", lastCount, currentCount)
		want.SetCurrent("proposed_breakdown", []any{})
		want.SetCurrent("proposed_response", "")
		want.SetCurrent("phase", "re_planning")
	}

	return nil
}

// goalThinkerReplan calls the Python script with conversation history to produce an updated breakdown.
func goalThinkerReplan(ctx context.Context, want *Want) error {
	goalText := GetGoal(want, "goal_text", "")
	ccMessages := GetCurrent(want, "cc_messages", []any{})
	ccResponses := GetCurrent(want, "cc_responses", []any{})

	// Get the latest user message as the modification request
	modificationRequest := ""
	if len(ccMessages) > 0 {
		if lastMsg, ok := ccMessages[len(ccMessages)-1].(map[string]any); ok {
			if text, ok := lastMsg["text"].(string); ok {
				modificationRequest = text
			}
		}
	}

	want.DirectLog("[GOAL-THINKER] Replanning with modification_request: %s", modificationRequest)

	result, err := callGoalThinkerScript(ctx, map[string]any{
		"phase":                "replan",
		"goal_text":            goalText,
		"conversation_history": ccMessages,
		"cc_responses":         ccResponses,
		"modification_request": modificationRequest,
	})
	if err != nil {
		want.DirectLog("[GOAL-THINKER] Script error during replan: %v", err)
		return nil
	}

	breakdown, _ := result["breakdown"].([]any)
	responseText, _ := result["response_text"].(string)

	want.SetCurrent("proposed_breakdown", breakdown)
	want.SetCurrent("proposed_response", responseText)

	// Append the AI response to cc_responses
	existingResponses := GetCurrent(want, "cc_responses", []any{})
	existingResponses = append(existingResponses, map[string]any{"text": responseText})
	if len(existingResponses) > 20 {
		existingResponses = existingResponses[len(existingResponses)-20:]
	}
	want.SetCurrent("cc_responses", existingResponses)

	want.DirectLog("[GOAL-THINKER] Replanned into %d items: %s", len(breakdown), responseText)

	// Create reaction queue for approval
	httpClient := want.GetHTTPClient()
	if httpClient == nil {
		want.DirectLog("[GOAL-THINKER] No HTTP client available")
		return nil
	}
	queueID, err := createGoalReactionQueue(httpClient)
	if err != nil {
		want.DirectLog("[GOAL-THINKER] Failed to create reaction queue: %v", err)
		return nil
	}

	want.SetCurrent("reaction_queue_id", queueID)
	want.SetCurrent("phase", "awaiting_approval")
	want.SetStatus(WantStatusWaitingUserAction)
	want.DirectLog("[GOAL-THINKER] Reaction queue created: %s, transitioning to awaiting_approval", queueID)

	return nil
}

// spawnChildWantFromBreakdownItem creates a child want for a single breakdown item.
// This must only be called from within the ThinkAgent function, not from Progress() or Initialize().
func spawnChildWantFromBreakdownItem(parent *Want, item map[string]any) error {
	name, _ := item["name"].(string)
	wantType, _ := item["type"].(string)
	description, _ := item["description"].(string)
	params, _ := item["params"].(map[string]any)

	if name == "" || wantType == "" {
		return fmt.Errorf("breakdown item missing name or type: %v", item)
	}

	if params == nil {
		params = map[string]any{}
	}
	if description != "" {
		params["want"] = description
	}

	child := &Want{}
	child.Metadata.Name = name
	child.Metadata.Type = wantType
	child.Metadata.Labels = map[string]string{
		"goal_parent": parent.Metadata.Name,
	}
	child.Spec.Params = params

	return parent.AddChildWant(child)
}

// callGoalThinkerScript runs tools/goal_thinker.py with the given input and returns the parsed output.
func callGoalThinkerScript(ctx context.Context, input map[string]any) (map[string]any, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %v", err)
	}

	cmd := exec.CommandContext(ctx, "python3", "tools/goal_thinker.py")
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewReader(data)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("script error: %v\nstderr: %s", err, stderr.String())
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON output: %v\nstdout: %s", err, stdout.String())
	}

	return result, nil
}

// createGoalReactionQueue creates a new reaction queue via the HTTP API.
func createGoalReactionQueue(httpClient *HTTPClient) (string, error) {
	resp, err := httpClient.POST("/api/v1/reactions/", nil)
	if err != nil {
		return "", err
	}
	var result struct {
		QueueID string `json:"queue_id"`
	}
	if err := httpClient.DecodeJSON(resp, &result); err != nil {
		return "", err
	}
	return result.QueueID, nil
}

// toInt converts an interface{} value to int, supporting common numeric types.
func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case float32:
		return int(val)
	default:
		return 0
	}
}
