package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	. "mywant/engine/src"
)

// AgentFlightAPI creates and manages flight reservations via REST API Provides two capabilities from flight_api_agency: - create_flight: Exec() method creates new flight reservations via POST /api/flights - cancel_flight: CancelFlight() method cancels existing flights via DELETE /api/flights/{id}
type AgentFlightAPI struct {
	DoAgent
	ServerURL string
}

// FlightReservation represents a flight booking from the mock server
type FlightReservation struct {
	ID            string    `json:"id"`
	FlightNumber  string    `json:"flight_number"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	DepartureTime time.Time `json:"departure_time"`
	ArrivalTime   time.Time `json:"arrival_time"`
	Status        string    `json:"status"`
	StatusMessage string    `json:"status_message"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type CreateFlightRequest struct {
	FlightNumber  string    `json:"flight_number"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	DepartureTime time.Time `json:"departure_time"`
	ArrivalTime   time.Time `json:"arrival_time"`
}

// NewAgentFlightAPI creates a new flight API agent
func NewAgentFlightAPI(name string, capabilities []string, uses []string, serverURL string) *AgentFlightAPI {
	return &AgentFlightAPI{
		DoAgent:   NewDoAgent(name, capabilities),
		ServerURL: serverURL,
	}
}

// Exec implements the Agent interface and handles both create_flight and cancel_flight actions Reads the "flight_action" state to determine which action to perform This satisfies the agent framework's requirement for Exec() method
func (a *AgentFlightAPI) Exec(ctx context.Context, want *Want) (bool, error) {
	actionVal, exists := want.GetState("flight_action")
	if exists && actionVal != nil {
		action, ok := actionVal.(string)
		if ok {
			switch action {
			case "cancel_flight":
				want.StoreLog("Executing cancel_flight action")
				if err := a.CancelFlight(ctx, want); err != nil {
					return false, err
				}
				// Clear the action after completion
				want.StoreStateForAgent("flight_action", "")
				return false, nil
			case "create_flight":
				want.StoreLog("Executing create_flight action")
				if err := a.CreateFlight(ctx, want); err != nil {
					return false, err
				}
				// Clear the action after completion
				want.StoreStateForAgent("flight_action", "")
				return false, nil
			}
		}
	}

	// Default to create_flight if no action specified
	err := a.CreateFlight(ctx, want)
	return false, err
}

// 2. "created" - after successful API response 3. "confirmed" - when monitor api checks the status Supports two date parameter formats: 1. departure_date: "YYYY-MM-DD" (e.g., "2026-12-20") - converted to 8:00 AM on that date
// 2. departure_time: RFC3339 format - used directly When rebooking (_previous_flight_id exists), generates new flight number and adjusted departure time
func (a *AgentFlightAPI) CreateFlight(ctx context.Context, want *Want) error {
	params := want.Spec.Params
	want.StoreStateForAgent("flight_status", "in process")
	prevFlightID, hasPrevFlight := want.GetState("_previous_flight_id")
	isRebooking := hasPrevFlight && prevFlightID != nil && prevFlightID != ""

	// Extract parameters using ParamExtractor
	extractor := NewParamExtractor(params)
	flightNumber := extractor.String("flight_number", "AA123")

	// For rebooking, generate a different flight number
	if isRebooking {
		baseFlight := flightNumber
		flightNumber = generateRebookingFlightNumber(baseFlight)
		want.StoreLog("Rebooking: Original flight %s -> New flight %s", baseFlight, flightNumber)
	}

	from := extractor.String("from", "New York")
	to := extractor.String("to", "Los Angeles")
	// Generate flight timing (handles both departure_date and departure_time parameters)
	departureDate := extractor.String("departure_date", "")
	departureTimeStr := extractor.String("departure_time", "")

	var departureTime, arrivalTime time.Time
	var err error

	// Prefer departure_time (RFC3339) over departure_date
	if departureTimeStr != "" {
		departureTime, arrivalTime, err = GenerateFlightTiming(departureTimeStr, isRebooking)
	} else if departureDate != "" {
		// Convert departure_date to RFC3339 format (8:00 AM on that date)
		parsedDate, parseErr := time.Parse("2006-01-02", departureDate)
		if parseErr == nil {
			departureTimeStr = parsedDate.Add(8 * time.Hour).Format(time.RFC3339)
			departureTime, arrivalTime, err = GenerateFlightTiming(departureTimeStr, isRebooking)
		} else {
			// Fallback to default
			departureTime, arrivalTime, err = GenerateFlightTiming("", isRebooking)
		}
	} else {
		// No date specified, use default
		departureTime, arrivalTime, err = GenerateFlightTiming("", isRebooking)
	}

	if err != nil {
		return fmt.Errorf("failed to generate flight timing: %w", err)
	}

	if isRebooking {
		want.StoreLog("Rebooking: Adjusted departure time to %s (next available flight)",
			departureTime.Format(time.RFC3339))
	}
	request := CreateFlightRequest{
		FlightNumber:  flightNumber,
		From:          from,
		To:            to,
		DepartureTime: departureTime,
		ArrivalTime:   arrivalTime,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make POST request
	url := fmt.Sprintf("%s/api/flights", a.ServerURL)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create flight: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create flight: status %d, body: %s", resp.StatusCode, string(body))
	}
	var reservation FlightReservation
	if err := json.NewDecoder(resp.Body).Decode(&reservation); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}
	want.StoreStateMultiForAgent(map[string]any{
		"flight_id":      reservation.ID,
		"flight_status":  "created",
		"flight_number":  reservation.FlightNumber,
		"from":           reservation.From,
		"to":             reservation.To,
		"departure_time": reservation.DepartureTime.Format(time.RFC3339),
		"arrival_time":   reservation.ArrivalTime.Format(time.RFC3339),
		"status_message": "Flight reservation created and awaiting confirmation",
		"created_at":     reservation.CreatedAt.Format(time.RFC3339),
		"updated_at":     reservation.UpdatedAt.Format(time.RFC3339),
		"agent_result": FlightSchedule{
			DepartureTime:   reservation.DepartureTime,
			ArrivalTime:     reservation.ArrivalTime,
			FlightType:      "api",
			FlightNumber:    reservation.FlightNumber,
			ReservationName: fmt.Sprintf("Flight %s from %s to %s", reservation.FlightNumber, reservation.From, reservation.To),
		},
	})

	// Record activity description for agent history
	activity := fmt.Sprintf("Flight reservation has been created for %s (Flight %s, %s → %s)",
		reservation.FlightNumber, reservation.FlightNumber, reservation.From, reservation.To)
	want.SetAgentActivity(a.Name, activity)

	want.StoreLog("Created flight reservation: %s (ID: %s, Status: %s)",
		reservation.FlightNumber, reservation.ID, reservation.Status)

	return nil
}

// CancelFlight implements the cancel_flight capability Cancels a flight reservation via DELETE /api/flights/{id} to the mock server
func (a *AgentFlightAPI) CancelFlight(ctx context.Context, want *Want) error {
	flightID, exists := want.GetState("flight_id")
	if !exists {
		return fmt.Errorf("no flight_id found in state")
	}

	flightIDStr, ok := flightID.(string)
	if !ok {
		return fmt.Errorf("flight_id is not a string")
	}
	url := fmt.Sprintf("%s/api/flights/%s", a.ServerURL, flightIDStr)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to cancel flight: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to cancel flight: status %d, body: %s", resp.StatusCode, string(body))
	}
	want.StoreStateMultiForAgent(map[string]any{
		"flight_status":           "canceled",
		"status_message":          "Flight canceled by agent",
		"canceled_at":             time.Now().Format(time.RFC3339),
		"_previous_flight_id":     flightIDStr,
		"_previous_flight_status": "canceled",
		"flight_id":               "",
		"attempted":               false,
		// DO NOT SET agent_result: nil here!
	})

	// Record activity description for agent history
	activity := fmt.Sprintf("Flight reservation has been cancelled (Flight ID: %s)", flightIDStr)
	want.SetAgentActivity(a.Name, activity)

	want.StoreLog("Cancelled flight: %s", flightIDStr)

	return nil
}

// generateRebookingFlightNumber generates an alternative flight number for rebooking
func generateRebookingFlightNumber(baseFlight string) string {
	flightSuffixes := []string{"A", "B", "C", "D", "E"}
	return baseFlight + flightSuffixes[rand.Intn(len(flightSuffixes))]
}
