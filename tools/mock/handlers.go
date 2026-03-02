package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// Server handles HTTP requests for flight reservations
type Server struct {
	store   *FlightStore
	updater *StatusUpdater
}

// NewServer creates a new server instance
func NewServer() *Server {
	store := NewFlightStore()
	updater := NewStatusUpdater(store)
	updater.Start()

	return &Server{
		store:   store,
		updater: updater,
	}
}

// CreateFlightRequest represents the request body for creating a flight
type CreateFlightRequest struct {
	FlightNumber  string    `json:"flight_number"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	DepartureTime time.Time `json:"departure_time"`
	ArrivalTime   time.Time `json:"arrival_time"`
	FlightClass   string    `json:"flight_class"`
}

// flightClassCost returns a randomized cost for the given flight class
func flightClassCost(class string) float64 {
	switch class {
	case "business":
		return math.Round((1200.0+rand.Float64()*1300.0)*100) / 100
	case "first":
		return math.Round((3000.0+rand.Float64()*5000.0)*100) / 100
	default: // economy
		return math.Round((300.0+rand.Float64()*500.0)*100) / 100
	}
}

// CreateFlight handles POST /api/flights
func (s *Server) CreateFlight(w http.ResponseWriter, r *http.Request) {
	var req CreateFlightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FlightNumber == "" || req.From == "" || req.To == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	flightClass := req.FlightClass
	if flightClass == "" {
		flightClass = "economy"
	}
	now := time.Now()
	reservation := &FlightReservation{
		ID:            uuid.New().String(),
		FlightNumber:  req.FlightNumber,
		From:          req.From,
		To:            req.To,
		DepartureTime: req.DepartureTime,
		ArrivalTime:   req.ArrivalTime,
		FlightClass:   flightClass,
		Cost:          flightClassCost(flightClass),
		Status:        StatusConfirmed,
		StatusMessage: "Flight reservation confirmed",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	s.store.Create(reservation)

	// Schedule status updates for this reservation
	s.updater.ScheduleUpdates(reservation.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(reservation)
}

// GetFlight handles GET /api/flights/{id}
func (s *Server) GetFlight(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	reservation, exists := s.store.Get(id)
	if !exists {
		http.Error(w, "Flight reservation not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reservation)
}

// DeleteFlight handles DELETE /api/flights/{id}
func (s *Server) DeleteFlight(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Cancel any scheduled updates
	s.updater.CancelUpdates(id)

	if !s.store.Delete(id) {
		http.Error(w, "Flight reservation not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Flight reservation cancelled successfully",
		"id":      id,
	})
}

// ListFlights handles GET /api/flights
func (s *Server) ListFlights(w http.ResponseWriter, r *http.Request) {
	reservations := s.store.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reservations)
}

// HealthCheck handles GET /health
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}
