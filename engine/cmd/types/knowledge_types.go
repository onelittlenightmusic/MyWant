package types

import (
	"fmt"
	"time"

	. "mywant/engine/src"
)

// KnowledgeLocals holds type-specific local state for KnowledgeWant
type KnowledgeLocals struct {
	Topic           string
	OutputPath      string
	Depth           string
	RefreshInterval time.Duration
}

// KnowledgeWant represents a want that maintains fresh information in a Markdown file
type KnowledgeWant struct {
	Want
}

func (k *KnowledgeWant) GetLocals() *KnowledgeLocals {
	return GetLocals[KnowledgeLocals](&k.Want)
}

func init() {
	RegisterWantImplementation[KnowledgeWant, KnowledgeLocals]("fresh knowledge")
}

// Initialize prepares the Knowledge want for execution
func (k *KnowledgeWant) Initialize() {
	// Get or initialize locals
	locals := k.GetLocals()
	if locals == nil {
		locals = &KnowledgeLocals{}
		k.Locals = locals
	}

	// Parse topic
	locals.Topic = k.GetStringParam("topic", "")
	if locals.Topic == "" {
		k.fail("Missing required parameter 'topic'")
		return
	}

	// Parse output_path
	locals.OutputPath = k.GetStringParam("output_path", "")
	if locals.OutputPath == "" {
		k.fail("Missing required parameter 'output_path'")
		return
	}

	// Parse depth
	locals.Depth = k.GetStringParam("depth", "comprehensive")

	// Parse refresh_interval
	intervalStr := k.GetStringParam("refresh_interval", "24h")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		k.StoreLog("Warning: Invalid refresh_interval format, defaulting to 24h: %v", err)
		interval = 24 * time.Hour
	}
	locals.RefreshInterval = interval

	k.Locals = locals

	// Initial state setup
	if _, exists := k.GetState("knowledge_status"); !exists {
		k.StoreState("knowledge_status", "stale")
	}

	k.StoreLog("[KNOWLEDGE] Knowledge want initialized for topic: %s", locals.Topic)
}

func (k *KnowledgeWant) fail(msg string) {
	k.StoreLog("ERROR: %s", msg)
	k.StoreState("knowledge_status", "failed")
	k.StoreState("error", msg)
	k.Status = "failed"
}

// CalculateAchievingPercentage returns the progress percentage
func (k *KnowledgeWant) CalculateAchievingPercentage() float64 {
	status, _ := k.GetStateString("knowledge_status", "")
	switch status {
	case "stale":
		return 10
	case "updating":
		return 50
	case "fresh":
		return 100
	case "failed":
		return 100
	default:
		return 0
	}
}

// IsAchieved returns true when the knowledge is fresh and the file is up to date
func (k *KnowledgeWant) IsAchieved() bool {
	status, _ := k.GetStateString("knowledge_status", "")
	return status == "fresh"
}

// Progress orchestrates the monitoring and updating of knowledge
func (k *KnowledgeWant) Progress() {
	if k.IsAchieved() {
		// Even if achieved, we check if it's time to refresh
		if k.shouldRefresh() {
			k.StoreLog("[KNOWLEDGE] Refresh interval reached, marking as stale")
			k.StoreState("knowledge_status", "stale")
		} else {
			return
		}
	}

	status, _ := k.GetStateString("knowledge_status", "")

	if status == "stale" {
		k.runMonitor()
	} else if status == "updating" {
		k.runUpdater()
	}
}

func (k *KnowledgeWant) shouldRefresh() bool {
	lastTime, ok := k.GetStateTime("last_sync_time", time.Time{})
	if !ok {
		return true
	}

	locals := k.GetLocals()
	if locals == nil {
		return true
	}

	return time.Since(lastTime) > locals.RefreshInterval
}

func (k *KnowledgeWant) runMonitor() {
	if err := k.ExecuteAgents(); err != nil {
		k.StoreLog("ERROR: KnowledgeMonitor failed: %v", err)
		return
	}

	// The agent should set knowledge_status to 'updating' if new info found,
	// or 'fresh' (and update last_sync_time) if no new info.
}

func (k *KnowledgeWant) runUpdater() {
	if err := k.ExecuteAgents(); err != nil {
		k.StoreLog("ERROR: KnowledgeUpdater failed: %v", err)
		return
	}

	// The agent should set knowledge_status to 'fresh' and update last_sync_time
}
