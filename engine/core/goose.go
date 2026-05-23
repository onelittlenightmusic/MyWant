package mywant

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// GooseExecutor is the interface for delegating operations to the Goose AI runtime.
type GooseExecutor interface {
	ExecuteViaGoose(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error)
}

// generateRecommendations calls the injected GooseExecutor to get want recommendations
// based on the current conversation history.
func (im *InteractionManager) generateRecommendations(ctx context.Context, history []ConversationMessage, interactCtx *InteractContext) ([]Recommendation, error) {
	var latestMessage string
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			latestMessage = history[i].Content
			break
		}
	}

	historyJSON, err := json.Marshal(history)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	params := map[string]interface{}{
		"message":              latestMessage,
		"conversation_history": string(historyJSON),
	}
	if interactCtx != nil {
		if interactCtx.Provider != "" {
			params["provider"] = interactCtx.Provider
		}
		if len(interactCtx.Categories) > 0 {
			params["categories"] = interactCtx.Categories
		}
		params["preferRecipes"] = interactCtx.PreferRecipes
	}

	result, err := im.gooseExecutor.ExecuteViaGoose(ctx, "interact_recommend", params)
	if err != nil {
		return nil, fmt.Errorf("Goose execution failed: %w", err)
	}

	recommendations, err := parseRecommendationsFromGoose(result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse recommendations: %w", err)
	}
	return recommendations, nil
}

// parseRecommendationsFromGoose extracts a []Recommendation from the raw Goose response.
func parseRecommendationsFromGoose(result interface{}) ([]Recommendation, error) {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	recsInterface, ok := resultMap["recommendations"]
	if !ok {
		if text, ok := resultMap["text"].(string); ok {
			return nil, fmt.Errorf("no recommendations found in result. AI response: %s", text)
		}
		return nil, fmt.Errorf("no recommendations found in result (keys found: %v)", reflect.ValueOf(resultMap).MapKeys())
	}

	recsJSON, err := json.Marshal(fixUsingFieldFormat(recsInterface))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal recommendations: %w", err)
	}

	var recommendations []Recommendation
	if err := json.Unmarshal(recsJSON, &recommendations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recommendations: %w", err)
	}
	return recommendations, nil
}

// fixUsingFieldFormat recursively normalises the "using" field in Goose responses
// from an object to an array so it can be unmarshalled into []UsingSelector.
func fixUsingFieldFormat(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		fixed := make(map[string]interface{})
		for key, val := range v {
			if key == "using" {
				fixed[key] = normalizeUsingField(val)
			} else {
				fixed[key] = fixUsingFieldFormat(val)
			}
		}
		return fixed
	case []interface{}:
		fixed := make([]interface{}, len(v))
		for i, item := range v {
			fixed[i] = fixUsingFieldFormat(item)
		}
		return fixed
	default:
		return data
	}
}

func normalizeUsingField(val interface{}) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		return []interface{}{v}
	case []interface{}:
		return v
	default:
		return []interface{}{}
	}
}
