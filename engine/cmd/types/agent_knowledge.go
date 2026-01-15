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

// KnowledgeAgent handles both monitoring and updating knowledge documents
type KnowledgeAgent struct {
	*mywant.DoAgent
}

// NewKnowledgeAgent creates a new KnowledgeAgent
func NewKnowledgeAgent() *KnowledgeAgent {
	baseAgent := mywant.NewBaseAgent(
		"knowledge_agent",
		[]string{"knowledge_monitoring", "knowledge_updating"},
		mywant.DoAgentType,
	)

	agent := &KnowledgeAgent{
		DoAgent: &mywant.DoAgent{
			BaseAgent: *baseAgent,
		},
	}

	agent.DoAgent.Action = func(ctx context.Context, want *mywant.Want) error {
		// Determine which capability is being requested
		for _, req := range want.Spec.Requires {
			switch req {
			case "knowledge_monitoring":
				return agent.monitor(ctx, want)
			case "knowledge_updating":
				return agent.update(ctx, want)
			}
		}
		return fmt.Errorf("no supported capability found in want requirements")
	}

	return agent
}

func (a *KnowledgeAgent) monitor(ctx context.Context, want *mywant.Want) error {
	topic := want.Spec.Params["topic"].(string)
	want.StoreLog("[KNOWLEDGE-AGENT] Monitoring topic: %s", topic)

	goose, err := GetGooseManager(ctx)
	if err != nil {
		return err
	}

	// 1. Search for updates
	searchResult, err := goose.ExecuteViaGoose(ctx, "google_search", map[string]interface{}{
		"query": topic,
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

func (a *KnowledgeAgent) update(ctx context.Context, want *mywant.Want) error {
	topic := want.Spec.Params["topic"].(string)
	path := want.Spec.Params["output_path"].(string)
	depth := want.Spec.Params["depth"].(string)

	updates, _ := want.GetState("discovered_updates")
	updatesJSON, _ := json.MarshalIndent(updates, "", "  ")

	want.StoreLog("[KNOWLEDGE-AGENT] Updating document: %s", path)

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

// RegisterKnowledgeAgents registers the knowledge agents with the registry
func RegisterKnowledgeAgents(registry *mywant.AgentRegistry) {
	if registry == nil {
		return
	}

	// Register capabilities
	registry.RegisterCapability(mywant.Capability{
		Name:  "knowledge_monitoring",
		Gives: []string{"knowledge_monitoring"},
	})
	registry.RegisterCapability(mywant.Capability{
		Name:  "knowledge_updating",
		Gives: []string{"knowledge_updating"},
	})

	// Register agent
	agent := NewKnowledgeAgent()
	registry.RegisterAgent(agent)

	fmt.Printf("[AGENT] KnowledgeAgent registered\n")
}
