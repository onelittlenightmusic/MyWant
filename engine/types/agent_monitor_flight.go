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

	. "mywant/engine/core"
)

const flightMonitorAgentName = "monitor_flight_api"

func init() {
	RegisterMonitorAgentType(flightMonitorAgentName,
		[]Capability{Cap("flight_api_agency")},
		monitorFlightStatus)
}

// monitorFlightStatus polls the mock server for flight status updates.
// All state is read from/written to want state — no struct fields needed.
// Polling interval is managed by AddMonitoringAgent; this function runs once per cycle.
func monitorFlightStatus(ctx context.Context, want *Want) error {
	flightID, ok := want.GetStateString("flight_id", "")
	if !ok || flightID == "" {
		return fmt.Errorf("no flight_id found in state - flight not created yet")
	}

	serverURL := want.GetStringParam("server_url", "http://localhost:8090")
	agentName := "flight-monitor-" + flightID

	// Restore last known status from want state
	lastKnownStatus, _ := want.GetStateString("flight_status", "unknown")

	// Restore status history from want state
	var statusChangeHistory []StatusChange
	if historyI, exists := want.GetState("status_history"); exists {
		if historyStrs, ok := historyI.([]any); ok {
			for _, entryI := range historyStrs {
				if entry, ok := entryI.(string); ok {
					if parsed, ok := parseStatusHistoryEntry(entry); ok {
						statusChangeHistory = append(statusChangeHistory, parsed)
					}
				}
			}
		} else if historyStrs, ok := historyI.([]string); ok {
			for _, entry := range historyStrs {
				if parsed, ok := parseStatusHistoryEntry(entry); ok {
					statusChangeHistory = append(statusChangeHistory, parsed)
				}
			}
		}
	}

	// Restore last recorded state hash from want state
	lastRecordedStateHash, _ := want.GetStateString("_monitor_state_hash", "")

	url := fmt.Sprintf("%s/api/flights/%s", serverURL, flightID)
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
	oldStatus := lastKnownStatus
	hasStateChange := newStatus != oldStatus

	// Calculate hash of current reservation data for differential history
	currentStateJSON, _ := json.Marshal(reservation)
	currentStateHash := fmt.Sprintf("%x", md5.Sum(currentStateJSON))

	// Only update state if state has actually changed (differential history)
	if hasStateChange || currentStateHash != lastRecordedStateHash {
		updates := Dict{
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
			want.StoreLog("Status changed: %s -> %s", oldStatus, newStatus)

			// Record status change
			statusChange := StatusChange{
				Timestamp: time.Now(),
				OldStatus: oldStatus,
				NewStatus: newStatus,
				Details:   reservation.StatusMessage,
			}
			statusChangeHistory = append(statusChangeHistory, statusChange)

			updates["flight_status"] = newStatus
			updates["status_changed"] = true
			updates["status_changed_at"] = time.Now().Format(time.RFC3339)
			updates["status_change_history_count"] = len(statusChangeHistory)

			// Record activity description for agent history
			activity := fmt.Sprintf("Flight status updated: %s → %s for flight %s (%s)",
				oldStatus, newStatus, reservation.FlightNumber, reservation.StatusMessage)
			want.SetAgentActivity(agentName, activity)
			schedule := FlightSchedule{
				DepartureTime:   reservation.DepartureTime,
				ArrivalTime:     reservation.ArrivalTime,
				FlightNumber:    reservation.FlightNumber,
				ReservationName: fmt.Sprintf("Flight %s from %s to %s", reservation.FlightNumber, reservation.From, reservation.To),
			}
			updates["agent_result"] = schedule
			want.StoreLog("[PACKET-SEND] Flight schedule packet: FlightNumber=%s, From=%s, To=%s, Status=%s",
				schedule.FlightNumber, reservation.From, reservation.To, newStatus)
			statusHistoryStrs := make([]string, 0)
			for _, change := range statusChangeHistory {
				historyEntry := fmt.Sprintf("%s: %s -> %s (%s)",
					change.Timestamp.Format("15:04:05"),
					change.OldStatus,
					change.NewStatus,
					change.Details)
				statusHistoryStrs = append(statusHistoryStrs, historyEntry)
			}
			updates["status_history"] = statusHistoryStrs

			// Print status progression
			want.StoreLog("FLIGHT %s STATUS PROGRESSION: %s (at %s)",
				reservation.ID, newStatus, time.Now().Format("15:04:05"))

			// Update hash after successful commit
			updates["_monitor_state_hash"] = currentStateHash
			want.StoreLog("State recorded (hash: %s)", currentStateHash[:8])
		} else {
			// No status change - update hash only
			updates["_monitor_state_hash"] = currentStateHash
		}
		// Use StoreStateMultiForAgent for background agent updates (separate from Want progress cycle)
		// Mark this as MonitorAgent for history tracking
		updates["action_by_agent"] = "MonitorAgent"
		want.StoreStateMultiForAgent(updates)
	}

	return nil
}

// parseStatusHistoryEntry parses a status history entry string
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

// findFirstColon finds the first colon in a string
func findFirstColon(s string) int {
	for i, ch := range s {
		if ch == ':' {
			return i
		}
	}
	return -1
}
