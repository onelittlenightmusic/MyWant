package types

import (
	"context"
	"crypto/md5"
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
	ServerURL             string
	PollInterval          time.Duration
	LastPollTime          time.Time
	LastKnownStatus       string
	StatusChangeHistory   []StatusChange
	LastRecordedStateHash string // Track last recorded state to avoid duplicate history entries
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
// NOTE: This agent runs ONE TIME per ExecuteAgents() call
// The continuous polling loop is handled by the Want's Exec method (FlightWant)
// Individual agents should NOT implement their own polling loops
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

	// Skip monitoring if flight_id is empty (flight cancellation/rebooking in progress)
	if flightIDStr == "" {
		fmt.Printf("[MonitorFlightAPI] Skipping monitoring: flight_id is empty (cancellation/rebooking in progress)\n")
		return nil
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

	// Restore status history from want state for persistence
	// Do NOT clear history - it accumulates across multiple monitoring executions
	if historyI, exists := want.GetState("status_history"); exists {
		if historyStrs, ok := historyI.([]interface{}); ok {
			log.Printf("[MonitorFlightAPI] Restoring %d status history entries from state (interface{})", len(historyStrs))
			for _, entryI := range historyStrs {
				if entry, ok := entryI.(string); ok {
					// Parse history entry format: "HH:MM:SS: OldStatus -> NewStatus (Details)"
					var ts, os, ns, det string
					n, err := fmt.Sscanf(entry, "%s: %s -> %s (%s)", &ts, &os, &ns, &det)
					if err == nil && n == 4 {
						parsedTime, timeErr := time.Parse("15:04:05", ts)
						if timeErr != nil {
							parsedTime = time.Now() // Fallback
						}
						// Only add if not already in history
						found := false
						for _, existing := range m.StatusChangeHistory {
							if existing.OldStatus == os && existing.NewStatus == ns && existing.Details == det {
								found = true
								break
							}
						}
						if !found {
							m.StatusChangeHistory = append(m.StatusChangeHistory, StatusChange{
								Timestamp: parsedTime,
								OldStatus: os,
								NewStatus: ns,
								Details:   det,
							})
						}
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
					// Only add if not already in history
					found := false
					for _, existing := range m.StatusChangeHistory {
						if existing.OldStatus == os && existing.NewStatus == ns && existing.Details == det {
							found = true
							break
						}
					}
					if !found {
						m.StatusChangeHistory = append(m.StatusChangeHistory, StatusChange{
							Timestamp: parsedTime,
							OldStatus: os,
							NewStatus: ns,
							Details:   det,
						})
					}
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

	// Check for status change first (differential history - only record if state changed)
	newStatus := reservation.Status
	oldStatus := m.LastKnownStatus
	hasStateChange := newStatus != oldStatus

	// Calculate hash of current reservation data for differential history
	currentStateJSON, _ := json.Marshal(reservation)
	currentStateHash := fmt.Sprintf("%x", md5.Sum(currentStateJSON))

	// Only update state if state has actually changed (differential history)
	// NOTE: Exec cycle wrapping is handled by the agent execution framework in want_agent.go
	// Individual agents should NOT call BeginExecCycle/EndExecCycle
	if hasStateChange || currentStateHash != m.LastRecordedStateHash {
		// Store flight detail state updates
		want.StoreState("flight_id", reservation.ID)
		want.StoreState("flight_number", reservation.FlightNumber)
		want.StoreState("from", reservation.From)
		want.StoreState("to", reservation.To)
		want.StoreState("departure_time", reservation.DepartureTime.Format(time.RFC3339))
		want.StoreState("arrival_time", reservation.ArrivalTime.Format(time.RFC3339))
		want.StoreState("status_message", reservation.StatusMessage)
		want.StoreState("updated_at", reservation.UpdatedAt.Format(time.RFC3339))

		if hasStateChange {
			fmt.Printf("[MonitorFlightAPI] Status changed: %s -> %s\n", oldStatus, newStatus)

			// Record status change
			statusChange := StatusChange{
				Timestamp: time.Now(),
				OldStatus: oldStatus,
				NewStatus: newStatus,
				Details:   reservation.StatusMessage,
			}
			m.StatusChangeHistory = append(m.StatusChangeHistory, statusChange)

			// Store status change info - use actual newStatus from mock server
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

			// Store all status history in state
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

			m.LastKnownStatus = newStatus

			// Print status progression
			fmt.Printf("[MonitorFlightAPI] FLIGHT %s STATUS PROGRESSION: %s (at %s)\n",
				reservation.ID, newStatus, time.Now().Format("15:04:05"))

			// Update hash after successful commit
			m.LastRecordedStateHash = currentStateHash
			fmt.Printf("[MonitorFlightAPI] State recorded (hash: %s)\n", currentStateHash[:8])
		} else {
			// No status change - don't create history entry, but still update other flight details
			fmt.Printf("[MonitorFlightAPI] Flight details changed but status is still: %s\n", newStatus)
			m.LastRecordedStateHash = currentStateHash
		}
	} else {
		// No state change - skip history entry
		fmt.Printf("[MonitorFlightAPI] No state change detected, skipping history entry\n")
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
