package types

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	mywant "mywant/engine/src"
)

func init() {
	mywant.RegisterDoAgentType("knowledge_agent",
		[]mywant.Capability{
			mywant.Cap("knowledge_monitoring"),
			mywant.Cap("knowledge_updating"),
		},
		executeKnowledgeAction)
}

// executeKnowledgeAction dispatches to monitor or update based on want requirements
func executeKnowledgeAction(ctx context.Context, want *mywant.Want) error {
	for _, req := range want.Spec.Requires {
		switch req {
		case "knowledge_monitoring":
			return knowledgeMonitor(ctx, want)
		case "knowledge_updating":
			return knowledgeUpdate(ctx, want)
		}
	}
	return fmt.Errorf("no supported capability found in want requirements")
}

func knowledgeMonitor(ctx context.Context, want *mywant.Want) error {
	topic := want.Spec.Params["topic"].(string)
	provider, _ := want.Spec.Params["provider"].(string)
	want.StoreLog("[KNOWLEDGE-AGENT] Monitoring topic: %s (provider: %s)", topic, provider)

	goose, err := GetGooseManager(ctx)
	if err != nil {
		return err
	}

	// 1. Search for updates
	searchResult, err := goose.ExecuteViaGoose(ctx, "google_search", map[string]interface{}{
		"query":    topic,
		"provider": provider,
	})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// 2. Parse facts
	var facts []interface{}
	if resultMap, ok := searchResult.(map[string]interface{}); ok {
		if f, ok := resultMap["facts"].([]interface{}); ok {
			facts = f
		}
	}

	if len(facts) == 0 {
		want.StoreLog("[KNOWLEDGE-AGENT] No new facts found. Knowledge is up to date.")
		want.StoreState("knowledge_status", "fresh")
		want.StoreState("last_sync_time", time.Now().Format(time.RFC3339))
		return nil
	}

	want.StoreLog("[KNOWLEDGE-AGENT] Found %d new facts. Transitioning to updating state.", len(facts))
	want.StoreState("discovered_updates", facts)
	want.StoreState("knowledge_status", "updating")
	return nil
}

func knowledgeUpdate(ctx context.Context, want *mywant.Want) error {
	topic := want.Spec.Params["topic"].(string)
	path := want.Spec.Params["output_path"].(string)
	depth := want.Spec.Params["depth"].(string)
	provider, _ := want.Spec.Params["provider"].(string)

	updates, _ := want.GetState("discovered_updates")
	updatesJSON, _ := json.MarshalIndent(updates, "", "  ")

	want.StoreLog("[KNOWLEDGE-AGENT] Updating document: %s (provider: %s)", path, provider)

	// 1. Read existing content if file exists
	existingContent := ""
	if data, err := os.ReadFile(path); err == nil {
		existingContent = string(data)
	} else {
		want.StoreLog("[KNOWLEDGE-AGENT] File not found, will create new: %s", path)
	}

	goose, err := GetGooseManager(ctx)
	if err != nil {
		return err
	}

	// 2. Synthesize new content
	synthResult, err := goose.ExecuteViaGoose(ctx, "knowledge_synthesize", map[string]interface{}{
		"topic":            topic,
		"existing_content": existingContent,
		"new_facts":        string(updatesJSON),
		"depth":            depth,
		"provider":         provider,
	})
	if err != nil {
		return fmt.Errorf("synthesis failed: %w", err)
	}

	// 3. Extract Markdown from response
	newContent := ""
	if resultMap, ok := synthResult.(map[string]interface{}); ok {
		if text, ok := resultMap["text"].(string); ok {
			newContent = text
		}
	} else if resultStr, ok := synthResult.(string); ok {
		newContent = resultStr
	}

	if newContent == "" {
		return fmt.Errorf("synthesis returned empty content")
	}

	// 4. Ensure directory exists and write file
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	want.StoreLog("[KNOWLEDGE-AGENT] Successfully updated %s", path)
	want.StoreState("knowledge_status", "fresh")
	want.StoreState("last_sync_time", time.Now().Format(time.RFC3339))
	want.StoreState("discovered_updates", nil) // Clear temporary data

	return nil
}
