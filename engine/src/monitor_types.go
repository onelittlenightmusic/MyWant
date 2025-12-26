package mywant

import (
	"context"
	"fmt"
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
	log.Printf("🔍 Monitor %s starting continuous monitoring\n", mw.Want.Metadata.Name)
	mw.Want.SetStatus(WantStatusReaching)
	if _, exists := mw.Want.GetState("start_time"); !exists {
		mw.Want.StoreState("start_time", time.Now())
		mw.Want.StoreState("status", "monitoring")
		mw.Want.StoreState("alerts_triggered", 0)
	}
	var monitorAgent *MonitorAgent
	if agentRegistry := mw.Want.GetAgentRegistry(); agentRegistry != nil {
		if agent, exists := agentRegistry.GetAgent(mw.Want.Metadata.Name); exists {
			if ma, ok := agent.(*MonitorAgent); ok {
				monitorAgent = ma
			} else {
				log.Printf("⚠️ Monitor %s agent is not a MonitorAgent type\n", mw.Want.Metadata.Name)
				mw.Want.SetStatus(WantStatusFailed)
				return
			}
		} else {
			log.Printf("⚠️ Monitor %s agent not found in registry\n", mw.Want.Metadata.Name)
			mw.Want.SetStatus(WantStatusFailed)
			return
		}
	} else {
		log.Printf("⚠️ Monitor %s has no AgentRegistry set\n", mw.Want.Metadata.Name)
		mw.Want.SetStatus(WantStatusFailed)
		return
	}

	if monitorAgent.Monitor == nil {
		log.Printf("⚠️ Monitor %s agent has no Monitor function set\n", mw.Want.Metadata.Name)
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
				log.Printf("🛑 Monitor %s stopping continuous monitoring\n", mw.Want.Metadata.Name)
				mw.Want.SetStatus(WantStatusAchieved) // Mark as completed when stopped
				return
			case <-ticker.C:
				err := monitorAgent.Monitor(context.Background(), mw.Want)
				if err != nil {
					log.Printf("❌ Monitor %s agent execution failed: %v\n", mw.Want.Metadata.Name, err)
					// Optionally set want status to failed, but continue monitoring mw.Want.SetStatus(WantStatusFailed)
				}
			}
		}
	}()

	log.Printf("🔍 Monitor %s continuous monitoring started\n", mw.Want.Metadata.Name)
}
func (mw *MonitorWant) handleNotification(notification StateNotification) {
	// Call base implementation first
	mw.BaseNotifiableWant.handleNotification(notification)

	// Custom monitoring logic
	if threshold, exists := mw.AlertThresholds[notification.StateKey]; exists {
		if mw.shouldAlert(notification.StateValue, threshold) {
			mw.triggerAlert(notification, threshold)
		}
	}
	mw.updateMonitoringStats(notification)
}

// shouldAlert determines if an alert should be triggered
func (mw *MonitorWant) shouldAlert(value any, threshold any) bool {
	valueFloat, valueOk := AsFloat(value)
	thresholdFloat, thresholdOk := AsFloat(threshold)

	if valueOk && thresholdOk {
		return valueFloat > thresholdFloat
	}

	// For string comparison, use direct equality
	return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", threshold)
}

// triggerAlert creates and stores an alert
func (mw *MonitorWant) triggerAlert(notification StateNotification, threshold any) {
	alert := AlertRecord{
		Timestamp:  time.Now(),
		SourceWant: notification.SourceWantName,
		StateKey:   notification.StateKey,
		Value:      notification.StateValue,
		Threshold:  threshold,
		AlertType:  "threshold_exceeded",
		Message: fmt.Sprintf("Alert: %s.%s (%v) exceeded threshold (%v)",
			notification.SourceWantName, notification.StateKey,
			notification.StateValue, threshold),
	}
	mw.Alerts = append(mw.Alerts, alert)
	mw.Want.StoreState("last_alert", alert.Message)
	mw.Want.StoreState("last_alert_time", alert.Timestamp)

	// Update alert count
	if count, ok := mw.Want.GetStateInt("alerts_triggered", 0); ok {
		mw.Want.StoreState("alerts_triggered", count+1)
	} else {
		mw.Want.StoreState("alerts_triggered", 1)
	}

	// Log alert
	log.Printf("🚨 ALERT: %s\n", alert.Message)

	// Execute alert actions
	mw.executeAlertActions(alert)
}

// executeAlertActions performs configured alert actions
func (mw *MonitorWant) executeAlertActions(alert AlertRecord) {
	for _, action := range mw.AlertActions {
		switch action {
		case "log":
			log.Printf("📝 Alert logged: %s\n", alert.Message)
		case "store":
			// Alert is already stored in mw.Alerts
		default:
			log.Printf("⚠️  Unknown alert action: %s\n", action)
		}
	}
}

// updateMonitoringStats updates monitoring statistics
func (mw *MonitorWant) updateMonitoringStats(notification StateNotification) {
	// Track notifications per source
	sourceKey := fmt.Sprintf("notifications_from_%s", notification.SourceWantName)
	if count, ok := mw.Want.GetStateInt(sourceKey, 0); ok {
		mw.Want.StoreState(sourceKey, count+1)
	} else {
		mw.Want.StoreState(sourceKey, 1)
	}

	// Track unique sources
	sourcesKey := "monitored_sources"
	var sources []string
	if existingSources, exists := mw.Want.GetState(sourcesKey); exists {
		if sourceList, ok := existingSources.([]string); ok {
			sources = sourceList
		}
	}
	found := false
	for _, source := range sources {
		if source == notification.SourceWantName {
			found = true
			break
		}
	}
	if !found {
		sources = append(sources, notification.SourceWantName)
		mw.Want.StoreState(sourcesKey, sources)
	}
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
