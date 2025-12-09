package types

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
		PollInterval:        5 * time.Second, // Poll flight status every 5 seconds to reduce API calls
		LastKnownStatus:     "unknown",
		StatusChangeHistory: make([]StatusChange, 0),
	}
}

// Exec polls the mock server for flight status updates NOTE: This agent runs ONE TIME per ExecuteAgents() call The continuous polling loop is handled by the Want's Exec method (FlightWant) Individual agents should NOT implement their own polling loops
func (m *MonitorFlightAPI) Exec(ctx context.Context, want *Want) error {
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
		want.StoreLog("Skipping monitoring: flight_id is empty (cancellation/rebooking in progress)")
		return nil
	}
	now := time.Now()
	if !m.LastPollTime.IsZero() && now.Sub(m.LastPollTime) < m.PollInterval {
		// Skip this polling cycle - wait for PollInterval to elapse
		return nil
	}

	// Record this poll time for next interval check
	m.LastPollTime = now

	// Restore last known status from want state for persistence across execution cycles
	if lastStatus, exists := want.GetState("flight_status"); exists {
		if lastStatusStr, ok := lastStatus.(string); ok {
			m.LastKnownStatus = lastStatusStr
		}
	} else {
		m.LastKnownStatus = "unknown" // Default if not found in state
	}

	// Restore status history from want state for persistence Do NOT clear history - it accumulates across multiple monitoring executions
	if historyI, exists := want.GetState("status_history"); exists {
		if historyStrs, ok := historyI.([]interface{}); ok {
			for _, entryI := range historyStrs {
				if entry, ok := entryI.(string); ok {
					if parsed, ok := parseStatusHistoryEntry(entry); ok {
						// Only add if not already in history
						found := false
						for _, existing := range m.StatusChangeHistory {
							if existing.OldStatus == parsed.OldStatus && existing.NewStatus == parsed.NewStatus && existing.Details == parsed.Details {
								found = true
								break
							}
						}
						if !found {
							m.StatusChangeHistory = append(m.StatusChangeHistory, parsed)
						}
					}
				}
			}
		} else if historyStrs, ok := historyI.([]string); ok {
			for _, entry := range historyStrs {
				if parsed, ok := parseStatusHistoryEntry(entry); ok {
					// Only add if not already in history
					found := false
					for _, existing := range m.StatusChangeHistory {
						if existing.OldStatus == parsed.OldStatus && existing.NewStatus == parsed.NewStatus && existing.Details == parsed.Details {
							found = true
							break
						}
					}
					if !found {
						m.StatusChangeHistory = append(m.StatusChangeHistory, parsed)
					}
				}
			}
		}
	}
	url := fmt.Sprintf("%s/api/flights/%s", m.ServerURL, flightIDStr)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get flight status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get flight: status %d, body: %s", resp.StatusCode, string(body))
	}
	var reservation FlightReservation
	if err := json.NewDecoder(resp.Body).Decode(&reservation); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}
	newStatus := reservation.Status
	oldStatus := m.LastKnownStatus
	hasStateChange := newStatus != oldStatus

	// Calculate hash of current reservation data for differential history
	currentStateJSON, _ := json.Marshal(reservation)
	currentStateHash := fmt.Sprintf("%x", md5.Sum(currentStateJSON))

	// Only update state if state has actually changed (differential history) NOTE: Exec cycle wrapping is handled by the agent execution framework in want_agent.go Individual agents should NOT call BeginExecCycle/EndExecCycle
	if hasStateChange || currentStateHash != m.LastRecordedStateHash {
		updates := map[string]interface{}{
			"flight_id":      reservation.ID,
			"flight_number":  reservation.FlightNumber,
			"from":           reservation.From,
			"to":             reservation.To,
			"departure_time": reservation.DepartureTime.Format(time.RFC3339),
			"arrival_time":   reservation.ArrivalTime.Format(time.RFC3339),
			"status_message": reservation.StatusMessage,
			"updated_at":     reservation.UpdatedAt.Format(time.RFC3339),
		}

		if hasStateChange {
			want.StoreLog(fmt.Sprintf("Status changed: %s -> %s", oldStatus, newStatus))

			// Record status change
			statusChange := StatusChange{
				Timestamp: time.Now(),
				OldStatus: oldStatus,
				NewStatus: newStatus,
				Details:   reservation.StatusMessage,
			}
			m.StatusChangeHistory = append(m.StatusChangeHistory, statusChange)

			updates["flight_status"] = newStatus
			updates["status_changed"] = true
			updates["status_changed_at"] = time.Now().Format(time.RFC3339)
			updates["status_change_history_count"] = len(m.StatusChangeHistory)

			// Record activity description for agent history
			activity := fmt.Sprintf("Flight status updated: %s → %s for flight %s (%s)",
				oldStatus, newStatus, reservation.FlightNumber, reservation.StatusMessage)
			want.SetAgentActivity(m.Name, activity)
			schedule := FlightSchedule{
				DepartureTime:   reservation.DepartureTime,
				ArrivalTime:     reservation.ArrivalTime,
				FlightNumber:    reservation.FlightNumber,
				ReservationName: fmt.Sprintf("Flight %s from %s to %s", reservation.FlightNumber, reservation.From, reservation.To),
			}
			updates["agent_result"] = schedule
			want.StoreLog(fmt.Sprintf("[PACKET-SEND] Flight schedule packet: FlightNumber=%s, From=%s, To=%s, Status=%s",
				schedule.FlightNumber, reservation.From, reservation.To, newStatus))
			statusHistoryStrs := make([]string, 0)
			for _, change := range m.StatusChangeHistory {
				historyEntry := fmt.Sprintf("%s: %s -> %s (%s)",
					change.Timestamp.Format("15:04:05"),
					change.OldStatus,
					change.NewStatus,
					change.Details)
				statusHistoryStrs = append(statusHistoryStrs, historyEntry)
			}
			updates["status_history"] = statusHistoryStrs

			m.LastKnownStatus = newStatus

			// Print status progression
			want.StoreLog(fmt.Sprintf("FLIGHT %s STATUS PROGRESSION: %s (at %s)",
				reservation.ID, newStatus, time.Now().Format("15:04:05")))

			// Update hash after successful commit
			m.LastRecordedStateHash = currentStateHash
			want.StoreLog(fmt.Sprintf("State recorded (hash: %s)", currentStateHash[:8]))
		} else {
			// No status change - don't create history entry, but still update other flight details Removed verbose log: "Flight details changed but status is still: ..."
			m.LastRecordedStateHash = currentStateHash
		}
		want.StoreStateMulti(updates)
	}

	return nil
}
func (m *MonitorFlightAPI) GetStatusChangeHistory() []StatusChange {
	return m.StatusChangeHistory
}

// WasStatusChanged checks if status has changed since last check
func (m *MonitorFlightAPI) WasStatusChanged() bool {
	return len(m.StatusChangeHistory) > 0
}
func parseStatusHistoryEntry(entry string) (StatusChange, bool) {
	colonIdx := findFirstColon(entry)
	if colonIdx < 0 || colonIdx+2 >= len(entry) {
		return StatusChange{}, false
	}

	// Extract timestamp part (before first colon)
	timestampStr := entry[:colonIdx]
	rest := strings.TrimSpace(entry[colonIdx+1:])
	arrowIdx := strings.Index(rest, " -> ")
	if arrowIdx < 0 {
		return StatusChange{}, false
	}

	// Extract old status (after colon, before arrow)
	oldStatus := strings.TrimSpace(rest[:arrowIdx])
	afterArrow := strings.TrimSpace(rest[arrowIdx+4:])
	parenIdx := strings.Index(afterArrow, "(")
	if parenIdx < 0 {
		return StatusChange{}, false
	}

	// Extract new status (after arrow, before parenthesis)
	newStatus := strings.TrimSpace(afterArrow[:parenIdx])

	// Extract details (inside parentheses)
	detailsPart := strings.TrimSpace(afterArrow[parenIdx:])
	if len(detailsPart) < 2 || !strings.HasPrefix(detailsPart, "(") || !strings.HasSuffix(detailsPart, ")") {
		return StatusChange{}, false
	}
	details := strings.TrimSpace(detailsPart[1 : len(detailsPart)-1])
	parsedTime, err := time.Parse("15:04:05", timestampStr)
	if err != nil {
		parsedTime = time.Now() // Fallback
	}

	return StatusChange{
		Timestamp: parsedTime,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Details:   details,
	}, true
}
func findFirstColon(s string) int {
	for i, ch := range s {
		if ch == ':' {
			return i
		}
	}
	return -1
}
