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
	RegisterMonitorAgent(flightMonitorAgentName, monitorFlightStatus)
}

func monitorFlightStatus(ctx context.Context, want *Want) (bool, error) {
	flightID := GetCurrent(want, "flight_id", "")
	if flightID == "" {
		return false, fmt.Errorf("no flight_id found in state - flight not created yet")
	}

	serverURL := want.GetStringParam("server_url", "http://localhost:8090")
	agentName := "flight-monitor-" + flightID

	lastKnownStatus := GetCurrent(want, "status", "unknown")

	var statusChangeHistory []StatusChange
	historyStrs := GetCurrent(want, "status_history", []string{})
	for _, entry := range historyStrs {
		if parsed, ok := parseStatusHistoryEntry(entry); ok {
			statusChangeHistory = append(statusChangeHistory, parsed)
		}
	}

	lastHashStr := GetCurrent(want, "monitor_state_hash", "")

	url := fmt.Sprintf("%s/api/flights/%s", serverURL, flightID)
	resp, err := http.Get(url)
	if err != nil {
		return false, fmt.Errorf("failed to get flight status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("failed to get flight: status %d, body: %s", resp.StatusCode, string(body))
	}
	var reservation FlightReservation
	if err := json.NewDecoder(resp.Body).Decode(&reservation); err != nil {
		return false, fmt.Errorf("failed to decode response: %v", err)
	}
	newStatus := reservation.Status
	oldStatus := lastKnownStatus
	hasStateChange := newStatus != oldStatus

	currentStateJSON, _ := json.Marshal(reservation)
	currentStateHash := fmt.Sprintf("%x", md5.Sum(currentStateJSON))

	if hasStateChange || currentStateHash != lastHashStr {
		want.SetCurrent("flight_id", reservation.ID)
		want.SetCurrent("flight_number", reservation.FlightNumber)
		want.SetCurrent("from", reservation.From)
		want.SetCurrent("to", reservation.To)
		want.SetCurrent("departure_time", reservation.DepartureTime.Format(time.RFC3339))
		want.SetCurrent("arrival_time", reservation.ArrivalTime.Format(time.RFC3339))
		want.SetCurrent("message", reservation.StatusMessage)
		want.SetCurrent("updated_at", reservation.UpdatedAt.Format(time.RFC3339))
		want.SetCurrent("status", newStatus)

		if hasStateChange {
			want.StoreLog("Status changed: %s -> %s", oldStatus, newStatus)

			statusChange := StatusChange{
				Timestamp: time.Now(),
				OldStatus: oldStatus,
				NewStatus: newStatus,
				Details:   reservation.StatusMessage,
			}
			statusChangeHistory = append(statusChangeHistory, statusChange)

			want.SetPredefined("status_changed", true)
			want.SetCurrent("status_changed_at", time.Now().Format(time.RFC3339))

			activity := fmt.Sprintf("Flight status updated: %s → %s for flight %s (%s)",
				oldStatus, newStatus, reservation.FlightNumber, reservation.StatusMessage)
			want.SetAgentActivity(agentName, activity)
			
			schedule := FlightSchedule{
				DepartureTime:   reservation.DepartureTime,
				ArrivalTime:     reservation.ArrivalTime,
				FlightNumber:    reservation.FlightNumber,
				ReservationName: fmt.Sprintf("Flight %s from %s to %s", reservation.FlightNumber, reservation.From, reservation.To),
			}
			want.SetPredefined("final_result", schedule)
			
			statusHistoryStrs := make([]string, 0)
			for _, change := range statusChangeHistory {
				historyEntry := fmt.Sprintf("%s: %s -> %s (%s)",
					change.Timestamp.Format("15:04:05"),
					change.OldStatus,
					change.NewStatus,
					change.Details)
				statusHistoryStrs = append(statusHistoryStrs, historyEntry)
			}
			want.SetCurrent("status_history", statusHistoryStrs)
		}
		
		want.SetCurrent("monitor_state_hash", currentStateHash)

		// Stop monitoring if confirmed or cancelled
		if newStatus == "confirmed" || newStatus == "cancelled" {
			return true, nil
		}
	}

	return false, nil
}

func parseStatusHistoryEntry(entry string) (StatusChange, bool) {
	colonIdx := findFirstColon(entry)
	if colonIdx < 0 || colonIdx+2 >= len(entry) { return StatusChange{}, false }
	timestampStr := entry[:colonIdx]
	rest := strings.TrimSpace(entry[colonIdx+1:])
	arrowIdx := strings.Index(rest, " -> ")
	if arrowIdx < 0 { return StatusChange{}, false }
	oldStatus := strings.TrimSpace(rest[:arrowIdx])
	afterArrow := strings.TrimSpace(rest[arrowIdx+4:])
	parenIdx := strings.Index(afterArrow, "(")
	if parenIdx < 0 { return StatusChange{}, false }
	newStatus := strings.TrimSpace(afterArrow[:parenIdx])
	detailsPart := strings.TrimSpace(afterArrow[parenIdx:])
	if len(detailsPart) < 2 || !strings.HasPrefix(detailsPart, "(") || !strings.HasSuffix(detailsPart, ")") {
		return StatusChange{}, false
	}
	details := strings.TrimSpace(detailsPart[1 : len(detailsPart)-1])
	parsedTime, err := time.Parse("15:04:05", timestampStr)
	if err != nil { parsedTime = time.Now() }
	return StatusChange{Timestamp: parsedTime, OldStatus: oldStatus, NewStatus: newStatus, Details: details}, true
}

func findFirstColon(s string) int {
	for i, ch := range s {
		if ch == ':' { return i }
	}
	return -1
}
