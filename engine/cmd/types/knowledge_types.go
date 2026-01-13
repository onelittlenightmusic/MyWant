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

// NewKnowledgeWant creates a new KnowledgeWant
func NewKnowledgeWant(want *Want) *KnowledgeWant {
	return &KnowledgeWant{Want: *want}
}

// Initialize prepares the Knowledge want for execution
func (k *KnowledgeWant) Initialize() {
	k.StoreLog("[KNOWLEDGE] Initializing Knowledge want: %s\n", k.Metadata.Name)

	locals := &KnowledgeLocals{}

	// Parse topic
	topic, ok := k.Spec.Params["topic"]
	if !ok || topic == "" {
		k.fail("Missing required parameter 'topic'")
		return
	}
	locals.Topic = fmt.Sprintf("%v", topic)

	// Parse output_path
	path, ok := k.Spec.Params["output_path"]
	if !ok || path == "" {
		k.fail("Missing required parameter 'output_path'")
		return
	}
	locals.OutputPath = fmt.Sprintf("%v", path)

	// Parse depth
	depth, ok := k.Spec.Params["depth"]
	if !ok {
		depth = "comprehensive"
	}
	locals.Depth = fmt.Sprintf("%v", depth)

	// Parse refresh_interval
	intervalStr, ok := k.Spec.Params["refresh_interval"]
	if !ok {
		intervalStr = "24h"
	}
	interval, err := time.ParseDuration(fmt.Sprintf("%v", intervalStr))
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

	k.StoreLog("[KNOWLEDGE] Knowledge want initialized for topic: %s\n", locals.Topic)
}

func (k *KnowledgeWant) fail(msg string) {
	k.StoreLog(fmt.Sprintf("ERROR: %s", msg))
	k.StoreState("knowledge_status", "failed")
	k.StoreState("error", msg)
	k.Status = "failed"
}

// CalculateAchievingPercentage returns the progress percentage
func (k *KnowledgeWant) CalculateAchievingPercentage() float64 {
	status, exists := k.GetState("knowledge_status")
	if !exists {
		return 0
	}

	statusStr := fmt.Sprintf("%v", status)
	switch statusStr {
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
	status, exists := k.GetState("knowledge_status")
	if !exists {
		return false
	}
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

	status, _ := k.GetState("knowledge_status")
	
	if status == "stale" {
		k.runMonitor()
	} else if status == "updating" {
		k.runUpdater()
	}
}

func (k *KnowledgeWant) shouldRefresh() bool {
	lastSync, exists := k.GetState("last_sync_time")
	if !exists || lastSync == "" {
		return true
	}

	lastTime, err := time.Parse(time.RFC3339, fmt.Sprintf("%v", lastSync))
	if err != nil {
		return true
	}

	locals := k.getLocals()
	if locals == nil {
		return true
	}

	return time.Since(lastTime) > locals.RefreshInterval
}

func (k *KnowledgeWant) runMonitor() {
	k.StoreLog("[KNOWLEDGE] Running KnowledgeMonitor to search for updates...")
	k.Spec.Requires = []string{"knowledge_monitoring"}
	
	if err := k.ExecuteAgents(); err != nil {
		k.StoreLog("ERROR: KnowledgeMonitor failed: %v", err)
		return
	}

	// The agent should set knowledge_status to 'updating' if new info found,
	// or 'fresh' (and update last_sync_time) if no new info.
}

func (k *KnowledgeWant) runUpdater() {
	k.StoreLog("[KNOWLEDGE] Running KnowledgeUpdater to synthesize and save results...")
	k.Spec.Requires = []string{"knowledge_updating"}

	if err := k.ExecuteAgents(); err != nil {
		k.StoreLog("ERROR: KnowledgeUpdater failed: %v", err)
		return
	}

	// The agent should set knowledge_status to 'fresh' and update last_sync_time
}

func (k *KnowledgeWant) getLocals() *KnowledgeLocals {
	if k.Locals == nil {
		return nil
	}
	locals, ok := k.Locals.(*KnowledgeLocals)
	if !ok {
		return nil
	}
	return locals
}

// RegisterKnowledgeWantType registers the knowledge want type with the builder
func RegisterKnowledgeWantType(builder *ChainBuilder) {
	builder.RegisterWantType("knowledge", func(metadata Metadata, spec WantSpec) Progressable {
		want := &Want{
			Metadata: metadata,
			Spec:     spec,
		}
		want.Init()
		return NewKnowledgeWant(want)
	})
}
