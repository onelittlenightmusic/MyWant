package mywant

import (
	"context"
	"log"
	"time"
)

// MonitorWant - Example of a want that processes notifications and provides alerting
type MonitorWant struct {
	*BaseNotifiableWant
	AlertThresholds map[string]any
	AlertActions    []string
	Alerts          []AlertRecord
}

// AlertRecord stores information about triggered alerts
type AlertRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	SourceWant string    `json:"sourceWant"`
	StateKey   string    `json:"stateKey"`
	Value      any       `json:"value"`
	Threshold  any       `json:"threshold"`
	AlertType  string    `json:"alertType"`
	Message    string    `json:"message"`
}

// NewMonitorWant creates a new monitor want
func NewMonitorWant(metadata Metadata, spec WantSpec) *MonitorWant {
	want := &Want{
		Metadata: metadata,
		Spec:     spec,
		Status:   WantStatusIdle,
		State:    make(map[string]any),
	}

	baseWant := NewBaseNotifiableWant(want, 200) // Larger buffer for monitor

	monitor := &MonitorWant{
		BaseNotifiableWant: baseWant,
		AlertThresholds:    make(map[string]any),
		AlertActions:       make([]string, 0),
		Alerts:             make([]AlertRecord, 0),
	}

	// Extract alert thresholds from params
	if thresholds, exists := spec.Params["alert_thresholds"]; exists {
		if threshMap, ok := AsMap(thresholds); ok {
			monitor.AlertThresholds = threshMap
		}
	}

	// Extract alert actions from params
	if actions, exists := spec.Params["alert_actions"]; exists {
		if actionList, ok := AsArray(actions); ok {
			for _, action := range actionList {
				if actionStr, ok := AsString(action); ok {
					monitor.AlertActions = append(monitor.AlertActions, actionStr)
				}
			}
		}
	}

	return monitor
}

// Initialize resets state before execution begins
func (mw *MonitorWant) Initialize() {
	// No state reset needed for monitor wants
}

// IsAchieved checks if monitor is complete (always false for continuous monitoring)
func (mw *MonitorWant) IsAchieved() bool {
	return false // Never done - continuous monitoring
}

// Progress implements the Progressable interface
func (mw *MonitorWant) Progress() {
	if _, exists := mw.Want.GetState("start_time"); exists {
		// Already monitoring
		return
	}

	mw.Want.StoreState("start_time", time.Now())
	mw.Want.StoreState("status", "monitoring")
	mw.Want.StoreState("alerts_triggered", 0)
	mw.Want.SetStatus(WantStatusReaching)

	var monitorAgent *MonitorAgent
	if agentRegistry := mw.Want.GetAgentRegistry(); agentRegistry != nil {
		if agent, exists := agentRegistry.GetAgent(mw.Want.Metadata.Name); exists {
			if ma, ok := agent.(*MonitorAgent); ok {
				monitorAgent = ma
			} else {
				log.Printf("⚠️ Monitor %s agent is not a MonitorAgent type", mw.Want.Metadata.Name)
				mw.Want.SetStatus(WantStatusFailed)
				return
			}
		} else {
			log.Printf("⚠️ Monitor %s agent not found in registry", mw.Want.Metadata.Name)
			mw.Want.SetStatus(WantStatusFailed)
			return
		}
	} else {
		log.Printf("⚠️ Monitor %s has no AgentRegistry set", mw.Want.Metadata.Name)
		mw.Want.SetStatus(WantStatusFailed)
		return
	}

	if monitorAgent.Monitor == nil {
		log.Printf("⚠️ Monitor %s agent has no Monitor function set", mw.Want.Metadata.Name)
		mw.Want.SetStatus(WantStatusFailed)
		return
	}

	// Start a goroutine for continuous monitoring
	go func() {
		ticker := time.NewTicker(10 * time.Second) // Use a default poll interval for now
		defer ticker.Stop()

		for {
			select {
			case <-mw.Want.GetStopChannel(): // Stop monitoring if want is stopped
				log.Printf("🛑 Monitor %s stopping continuous monitoring", mw.Want.Metadata.Name)
				mw.Want.SetStatus(WantStatusAchieved) // Mark as completed when stopped
				return
			case <-ticker.C:
				err := monitorAgent.Monitor(context.Background(), mw.Want)
				if err != nil {
					log.Printf("❌ Monitor %s agent execution failed: %v", mw.Want.Metadata.Name, err)
					// Optionally set want status to failed, but continue monitoring mw.Want.SetStatus(WantStatusFailed)
				}
			}
		}
	}()

	log.Printf("🔍 Monitor %s continuous monitoring started", mw.Want.Metadata.Name)
}
func (mw *MonitorWant) GetAlerts() []AlertRecord {
	return mw.Alerts
}

// ClearAlerts clears the alert history
func (mw *MonitorWant) ClearAlerts() {
	mw.Alerts = mw.Alerts[:0]
	mw.Want.StoreState("alerts_triggered", 0)
	mw.Want.StoreState("last_alert", nil)
	mw.Want.StoreState("last_alert_time", nil)
}

// RegisterMonitorWantTypes registers monitor want types with a ChainBuilder
func RegisterMonitorWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("monitor", func(metadata Metadata, spec WantSpec) Progressable {
		monitor := NewMonitorWant(metadata, spec)

		// Register want for lookup
		RegisterWant(monitor.BaseNotifiableWant.Want)

		// Legacy listener/subscription registration removed - Group A events now use unified subscription system

		return monitor
	})

	log.Println("📊 Monitor want types registered")
}
