package types

import (
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[KnowledgeWant, KnowledgeLocals]("fresh knowledge")
}

// KnowledgeLocals holds type-specific local state for KnowledgeWant
type KnowledgeLocals struct {
	Topic           string
	OutputPath      string
	Depth           string
	Provider        string
	RefreshInterval time.Duration

	// State fields (auto-synced)
	KnowledgeStatus   string    `mywant:"current,knowledge_status"`
	ContentHash       string    `mywant:"internal,content_hash"`
	DiscoveredUpdates []any     `mywant:"internal,discovered_updates"`
	LastSyncTime      string    `mywant:"current,last_sync_time"`
	Error             string    `mywant:"current,error"`
}

// KnowledgeWant represents a want that maintains fresh information in a Markdown file
type KnowledgeWant struct {
	Want
}

func (k *KnowledgeWant) GetLocals() *KnowledgeLocals {
	return CheckLocalsInitialized[KnowledgeLocals](&k.Want)
}

// Initialize prepares the Knowledge want for execution
func (k *KnowledgeWant) Initialize() {
	// Get locals (guaranteed to be initialized by framework)
	locals := k.GetLocals()

	// Parse and validate required parameters using ConfigError pattern
	locals.Topic = k.GetStringParam("topic", "")
	if locals.Topic == "" {
		k.SetConfigError("topic", "Missing required parameter 'topic'")
		return
	}

	// Parse output_path
	locals.OutputPath = k.GetStringParam("output_path", "")
	if locals.OutputPath == "" {
		k.SetConfigError("output_path", "Missing required parameter 'output_path'")
		return
	}

	// Parse depth
	locals.Depth = k.GetStringParam("depth", "comprehensive")

	// Parse provider
	locals.Provider = k.GetStringParam("provider", "")

	// Parse refresh_interval
	intervalStr := k.GetStringParam("refresh_interval", "24h")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		k.StoreLog("Warning: Invalid refresh_interval format, defaulting to 24h: %v", err)
		interval = 24 * time.Hour
	}
	locals.RefreshInterval = interval

	// Initial state setup
	if locals.KnowledgeStatus == "" {
		locals.KnowledgeStatus = "stale"
	}

	k.StoreLog("[KNOWLEDGE] Knowledge want initialized for topic: %s", locals.Topic)
}

func (k *KnowledgeWant) fail(msg string) {
	locals := k.GetLocals()
	k.StoreLog("ERROR: %s", msg)
	locals.KnowledgeStatus = "failed"
	locals.Error = msg
	k.Status = "failed"
}

// CalculateAchievingPercentage returns the progress percentage
func (k *KnowledgeWant) CalculateAchievingPercentage() float64 {
	locals := k.GetLocals()
	switch locals.KnowledgeStatus {
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
	return k.GetLocals().KnowledgeStatus == "fresh"
}

// Progress orchestrates the monitoring and updating of knowledge
func (k *KnowledgeWant) Progress() {
	locals := k.GetLocals()
	k.SetPredefined("achieving_percentage", k.CalculateAchievingPercentage())

	if k.IsAchieved() {
		// Even if achieved, we check if it's time to refresh
		if k.shouldRefresh() {
			k.StoreLog("[KNOWLEDGE] Refresh interval reached, marking as stale")
			locals.KnowledgeStatus = "stale"
		} else {
			return
		}
	}

	if locals.KnowledgeStatus == "stale" {
		k.runMonitor()
	} else if locals.KnowledgeStatus == "updating" {
		k.runUpdater()
	}
}

func (k *KnowledgeWant) shouldRefresh() bool {
	locals := k.GetLocals()
	if locals.LastSyncTime == "" {
		return true
	}
	lastTime, err := time.Parse(time.RFC3339, locals.LastSyncTime)
	if err != nil {
		return true
	}

	return time.Since(lastTime) > locals.RefreshInterval
}

func (k *KnowledgeWant) runMonitor() {
	if err := k.ExecuteAgents(); err != nil {
		k.StoreLog("ERROR: KnowledgeMonitor failed: %v", err)
		return
	}
}

func (k *KnowledgeWant) runUpdater() {
	if err := k.ExecuteAgents(); err != nil {
		k.StoreLog("ERROR: KnowledgeUpdater failed: %v", err)
		return
	}
}
