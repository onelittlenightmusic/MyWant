package types

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	. "mywant/engine/src"
)

// MonitorFlightAPI extends MonitorAgent to poll flight status from mock server
type MonitorFlightAPI struct {
	MonitorAgent
	ServerURL           string
	PollInterval        time.Duration
	LastPollTime        time.Time
	LastKnownStatus     string
	StatusChangeHistory []StatusChange
}

// StatusChange represents a status change event
type StatusChange struct {
	Timestamp time.Time
	OldStatus string
	NewStatus string
	Details   string
}

// NewMonitorFlightAPI creates a new flight API monitor agent
func NewMonitorFlightAPI(name string, capabilities []string, uses []string, serverURL string) *MonitorFlightAPI {
	return &MonitorFlightAPI{
		MonitorAgent: MonitorAgent{
			BaseAgent: BaseAgent{
				Name:         name,
				Capabilities: capabilities,
				Uses:         uses,
				Type:         MonitorAgentType,
			},
		},
		ServerURL:           serverURL,
		PollInterval:        10 * time.Second,
		LastKnownStatus:     "unknown",
		StatusChangeHistory: make([]StatusChange, 0),
	}
}

// Exec polls the mock server for flight status updates
func (m *MonitorFlightAPI) Exec(ctx context.Context, want *Want) error {
	// Get flight ID from state (set by AgentFlightAPI)
	flightID, exists := want.GetState("flight_id")
	if !exists {
		return fmt.Errorf("no flight_id found in state - flight not created yet")
	}

	flightIDStr, ok := flightID.(string)
	if !ok {
		return fmt.Errorf("flight_id is not a string")
	}

	fmt.Printf("[MonitorFlightAPI] Polling flight status for ID: %s\n", flightIDStr)

	// Restore last known status from want state for persistence across execution cycles
	if lastStatus, exists := want.GetState("flight_status"); exists {
		if lastStatusStr, ok := lastStatus.(string); ok {
			m.LastKnownStatus = lastStatusStr
		}
	} else {
		m.LastKnownStatus = "unknown" // Default if not found in state
	}
	fmt.Printf("[MonitorFlightAPI] Current m.LastKnownStatus: %s\n", m.LastKnownStatus)

	// Restore status history from want state for persistence across execution cycles
	// Clear current history to avoid duplicates if this is a re-execution
	m.StatusChangeHistory = make([]StatusChange, 0)
	if historyI, exists := want.GetState("status_history"); exists {
		if historyStrs, ok := historyI.([]interface{}); ok {
			log.Printf("[MonitorFlightAPI] Restoring %d status history entries from state (interface{})", len(historyStrs))
			for _, entryI := range historyStrs {
				if entry, ok := entryI.(string); ok {
					// The format string needs to match exactly what was stored
					// Example: "15:04:05: confirmed -> details_changed (Flight details updated)"
					// We need to parse the timestamp, old status, new status, and details
					// This is a simplified parsing, assuming the format "HH:MM:SS: OldStatus -> NewStatus (Details)"
					// A more robust solution would involve storing StatusChange objects as JSON
					// For now, let's try to parse the string
					var ts, os, ns, det string
					n, err := fmt.Sscanf(entry, "%s: %s -> %s (%s)", &ts, &os, &ns, &det)
					if err == nil && n == 4 {
						parsedTime, timeErr := time.Parse("15:04:05", ts)
						if timeErr != nil {
							parsedTime = time.Now() // Fallback
						}
						m.StatusChangeHistory = append(m.StatusChangeHistory, StatusChange{
							Timestamp: parsedTime,
							OldStatus: os,
							NewStatus: ns,
							Details:   det,
						})
					} else {
						log.Printf("[MonitorFlightAPI] Failed to parse history entry (interface{}): %s, Error: %v, n=%d\n", entry, err, n)
					}
				}
			}
		} else if historyStrs, ok := historyI.([]string); ok {
			log.Printf("[MonitorFlightAPI] Restoring %d status history entries from state ([]string)", len(historyStrs))
			for _, entry := range historyStrs {
				var ts, os, ns, det string
				n, err := fmt.Sscanf(entry, "%s: %s -> %s (%s)", &ts, &os, &ns, &det)
				if err == nil && n == 4 {
					parsedTime, timeErr := time.Parse("15:04:05", ts)
					if timeErr != nil {
						parsedTime = time.Now() // Fallback
					}
					m.StatusChangeHistory = append(m.StatusChangeHistory, StatusChange{
						Timestamp: parsedTime,
						OldStatus: os,
						NewStatus: ns,
						Details:   det,
					})
				} else {
					log.Printf("[MonitorFlightAPI] Failed to parse history entry ([]string): %s, Error: %v, n=%d\n", entry, err, n)
				}
			}
		}
	}

	// GET the flight details
	url := fmt.Sprintf("%s/api/flights/%s", m.ServerURL, flightIDStr)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get flight status: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get flight: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var reservation FlightReservation
	if err := json.NewDecoder(resp.Body).Decode(&reservation); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	// Batch flight detail state updates (in case any change)
	{
		want.BeginExecCycle()
		want.StoreState("flight_id", reservation.ID)
		want.StoreState("flight_number", reservation.FlightNumber)
		want.StoreState("from", reservation.From)
		want.StoreState("to", reservation.To)
		want.StoreState("departure_time", reservation.DepartureTime.Format(time.RFC3339))
		want.StoreState("arrival_time", reservation.ArrivalTime.Format(time.RFC3339))
		want.StoreState("status_message", reservation.StatusMessage)
		want.StoreState("updated_at", reservation.UpdatedAt.Format(time.RFC3339))
		want.EndExecCycle()
	}

	// Check for status change
	newStatus := reservation.Status
	oldStatus := m.LastKnownStatus

	if newStatus != oldStatus {
		fmt.Printf("[MonitorFlightAPI] Status changed: %s -> %s\n", oldStatus, newStatus)

		// Record status change
		statusChange := StatusChange{
			Timestamp: time.Now(),
			OldStatus: oldStatus,
			NewStatus: newStatus,
			Details:   reservation.StatusMessage,
		}
		m.StatusChangeHistory = append(m.StatusChangeHistory, statusChange)

		// Batch all status change state updates into a single history entry
		{
			want.BeginExecCycle()
			// Store status change info - all batched together
			want.StoreState("flight_status", newStatus)
			want.StoreState("status_changed", true)
			want.StoreState("status_changed_at", time.Now().Format(time.RFC3339))
			want.StoreState("status_change_history_count", len(m.StatusChangeHistory))

			// Store complete flight info when status changes
			schedule := FlightSchedule{
				DepartureTime:   reservation.DepartureTime,
				ArrivalTime:     reservation.ArrivalTime,
				FlightNumber:    reservation.FlightNumber,
				ReservationName: fmt.Sprintf("Flight %s from %s to %s", reservation.FlightNumber, reservation.From, reservation.To),
			}
			want.StoreState("agent_result", schedule)

			// Store all status history in state - also batched
			statusHistoryStrs := make([]string, 0)
			for _, change := range m.StatusChangeHistory {
				historyEntry := fmt.Sprintf("%s: %s -> %s (%s)",
					change.Timestamp.Format("15:04:05"),
					change.OldStatus,
					change.NewStatus,
					change.Details)
				statusHistoryStrs = append(statusHistoryStrs, historyEntry)
			}
			want.StoreState("status_history", statusHistoryStrs)
			want.EndExecCycle()
		}

		m.LastKnownStatus = newStatus

		// Print status progression
		fmt.Printf("[MonitorFlightAPI] FLIGHT %s STATUS PROGRESSION: %s (at %s)\n",
			reservation.ID, newStatus, time.Now().Format("15:04:05"))
	} else {
		// No status change - still batch the status field updates
		{
			want.BeginExecCycle()
			want.StoreState("flight_status", newStatus)
			want.StoreState("status_changed", false)
			want.EndExecCycle()
		}
	}

	fmt.Printf("[MonitorFlightAPI] Polling complete - Current status: %s\n", newStatus)

	return nil
}

// GetStatusChangeHistory returns the history of status changes
func (m *MonitorFlightAPI) GetStatusChangeHistory() []StatusChange {
	return m.StatusChangeHistory
}

// WasStatusChanged checks if status has changed since last check
func (m *MonitorFlightAPI) WasStatusChanged() bool {
	return len(m.StatusChangeHistory) > 0
}
