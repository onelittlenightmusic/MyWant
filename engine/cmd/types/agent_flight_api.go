package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "mywant/engine/src"
)

// AgentFlightAPI creates and manages flight reservations via REST API
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

// CreateFlightRequest represents the request to create a flight
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
		DoAgent: DoAgent{
			BaseAgent: BaseAgent{
				Name:         name,
				Capabilities: capabilities,
				Uses:         []string{},
				Type:         DoAgentType,
			},
		},
		ServerURL: serverURL,
	}
}

// Exec creates a flight reservation via POST API
func (a *AgentFlightAPI) Exec(ctx context.Context, want *Want) error {
	// Get flight parameters from want params
	params := want.Spec.Params

	flightNumber, _ := params["flight_number"].(string)
	if flightNumber == "" {
		flightNumber = "AA123"
	}

	from, _ := params["from"].(string)
	if from == "" {
		from = "New York"
	}

	to, _ := params["to"].(string)
	if to == "" {
		to = "Los Angeles"
	}

	// Parse departure and arrival times
	departureTimeStr, _ := params["departure_time"].(string)
	departureTime, err := time.Parse(time.RFC3339, departureTimeStr)
	if err != nil {
		// Default to tomorrow morning
		departureTime = time.Now().AddDate(0, 0, 1).Truncate(24 * time.Hour).Add(8 * time.Hour)
	}

	arrivalTimeStr, _ := params["arrival_time"].(string)
	arrivalTime, err := time.Parse(time.RFC3339, arrivalTimeStr)
	if err != nil {
		// Default to 3.5 hours after departure
		arrivalTime = departureTime.Add(3*time.Hour + 30*time.Minute)
	}

	// Create flight request
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

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create flight: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var reservation FlightReservation
	if err := json.NewDecoder(resp.Body).Decode(&reservation); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	// Store reservation in want state
	// NOTE: Exec cycle wrapping is handled by the agent execution framework in want_agent.go
	// Individual agents should NOT call BeginExecCycle/EndExecCycle
	want.StoreState("flight_id", reservation.ID)
	want.StoreState("flight_status", reservation.Status)
	want.StoreState("flight_number", reservation.FlightNumber)
	want.StoreState("from", reservation.From)
	want.StoreState("to", reservation.To)
	want.StoreState("departure_time", reservation.DepartureTime.Format(time.RFC3339))
	want.StoreState("arrival_time", reservation.ArrivalTime.Format(time.RFC3339))
	want.StoreState("status_message", reservation.StatusMessage)
	want.StoreState("created_at", reservation.CreatedAt.Format(time.RFC3339))
	want.StoreState("updated_at", reservation.UpdatedAt.Format(time.RFC3339))
	want.StoreState("agent_result", FlightSchedule{
		DepartureTime:   reservation.DepartureTime,
		ArrivalTime:     reservation.ArrivalTime,
		FlightType:      "api",
		FlightNumber:    reservation.FlightNumber,
		ReservationName: fmt.Sprintf("Flight %s from %s to %s", reservation.FlightNumber, reservation.From, reservation.To),
	})

	fmt.Printf("[AgentFlightAPI] Created flight reservation: %s (ID: %s, Status: %s)\n",
		reservation.FlightNumber, reservation.ID, reservation.Status)

	return nil
}

// CancelFlight cancels a flight reservation via DELETE API
func (a *AgentFlightAPI) CancelFlight(ctx context.Context, want *Want) error {
	// Get flight ID from state
	flightID, exists := want.GetState("flight_id")
	if !exists {
		return fmt.Errorf("no flight_id found in state")
	}

	flightIDStr, ok := flightID.(string)
	if !ok {
		return fmt.Errorf("flight_id is not a string")
	}

	// Create DELETE request
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

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to cancel flight: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Update state
	// NOTE: Exec cycle wrapping is handled by the agent execution framework
	want.StoreState("flight_status", "cancelled")
	want.StoreState("status_message", "Flight cancelled by agent")
	want.StoreState("cancelled_at", time.Now().Format(time.RFC3339))

	fmt.Printf("[AgentFlightAPI] Cancelled flight: %s\n", flightIDStr)

	return nil
}
