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

	. "mywant/engine/core"
)

const flightDoAgentName = "agent_flight_api"

func init() {
	RegisterDoAgent(flightDoAgentName, executeFlightAction)
}

// FlightReservation represents a flight booking from the mock server
type FlightReservation struct {
	ID            string    `json:"id"`
	FlightNumber  string    `json:"flight_number"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	DepartureTime time.Time `json:"departure_time"`
	ArrivalTime   time.Time `json:"arrival_time"`
	FlightClass   string    `json:"flight_class"`
	Cost          float64   `json:"cost"`
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
	FlightClass   string    `json:"flight_class"`
}

// executeFlightAction handles both create_flight and cancel_flight actions
func executeFlightAction(ctx context.Context, want *Want) error {
	action := GetPlan(want, "flight_action", "")
	switch action {
	case "cancel_flight":
		want.StoreLog("Executing cancel_flight action")
		if err := cancelFlight(ctx, want); err != nil {
			return err
		}
		want.SetPlan("flight_action", "")
		return nil
	case "create_flight":
		want.StoreLog("Executing create_flight action")
		if err := createFlight(ctx, want); err != nil {
			return err
		}
		want.SetPlan("flight_action", "")
		return nil
	}

	// Default to create_flight if no action specified
	return createFlight(ctx, want)
}

func createFlight(ctx context.Context, want *Want) error {
	serverURL := GetCurrent(want, "server_url", "http://localhost:8090")
	params := want.Spec.Params
	want.SetCurrent("flight_status", "in process")
	prevFlightID := GetCurrent(want, "_previous_flight_id", "")
	isRebooking := prevFlightID != ""

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
	flightClass := extractor.String("flight_type", "economy")
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
		FlightClass:   flightClass,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make POST request
	url := fmt.Sprintf("%s/api/flights", serverURL)
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
	want.SetCurrent("flight_id", reservation.ID)
	want.SetCurrent("flight_status", "created")
	want.SetCurrent("flight_number", reservation.FlightNumber)
	want.SetCurrent("from", reservation.From)
	want.SetCurrent("to", reservation.To)
	want.SetCurrent("departure_time", reservation.DepartureTime.Format(time.RFC3339))
	want.SetCurrent("arrival_time", reservation.ArrivalTime.Format(time.RFC3339))
	want.SetCurrent("status_message", "Flight reservation created and awaiting confirmation")
	want.SetCurrent("created_at", reservation.CreatedAt.Format(time.RFC3339))
	want.SetCurrent("updated_at", reservation.UpdatedAt.Format(time.RFC3339))
	
	want.SetCurrent("agent_result", FlightSchedule{
		DepartureTime:   reservation.DepartureTime,
		ArrivalTime:     reservation.ArrivalTime,
		FlightType:      reservation.FlightClass,
		FlightNumber:    reservation.FlightNumber,
		ReservationName: fmt.Sprintf("Flight %s from %s to %s", reservation.FlightNumber, reservation.From, reservation.To),
		Cost:            reservation.Cost,
	})

	// Record activity description for agent history
	activity := fmt.Sprintf("Flight reservation has been created for %s (Flight %s, %s → %s)",
		reservation.FlightNumber, reservation.FlightNumber, reservation.From, reservation.To)
	want.SetAgentActivity(flightDoAgentName, activity)

	want.StoreLog("Created flight reservation: %s (ID: %s, Status: %s)",
		reservation.FlightNumber, reservation.ID, reservation.Status)

	return nil
}

// cancelFlight cancels a flight reservation via DELETE /api/flights/{id}
func cancelFlight(ctx context.Context, want *Want) error {
	serverURL := GetCurrent(want, "server_url", "http://localhost:8090")
	flightID := GetCurrent(want, "flight_id", "")
	if flightID == "" {
		return fmt.Errorf("no flight_id found in state")
	}

	url := fmt.Sprintf("%s/api/flights/%s", serverURL, flightID)
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
	
	want.SetCurrent("flight_status", "canceled")
	want.SetCurrent("status_message", "Flight canceled by agent")
	want.SetCurrent("canceled_at", time.Now().Format(time.RFC3339))
	want.SetCurrent("_previous_flight_id", flightID)
	want.SetCurrent("_previous_flight_status", "canceled")
	want.SetCurrent("flight_id", "")
	want.SetCurrent("attempted", false)

	// Record activity description for agent history
	activity := fmt.Sprintf("Flight reservation has been cancelled (Flight ID: %s)", flightID)
	want.SetAgentActivity(flightDoAgentName, activity)

	want.StoreLog("Cancelled flight: %s", flightID)

	return nil
}

// generateRebookingFlightNumber generates an alternative flight number for rebooking
func generateRebookingFlightNumber(baseFlight string) string {
	flightSuffixes := []string{"A", "B", "C", "D", "E"}
	return baseFlight + flightSuffixes[rand.Intn(len(flightSuffixes))]
}
