package main

import (
	"sync"
	"time"
)

// FlightStatus represents the current status of a flight
type FlightStatus string

const (
	StatusConfirmed      FlightStatus = "confirmed"
	StatusDetailsChanged FlightStatus = "details_changed"
	StatusDelayed        FlightStatus = "delayed_one_day"
	StatusCancelled      FlightStatus = "cancelled"
)

// FlightReservation represents a flight booking
type FlightReservation struct {
	ID            string       `json:"id"`
	FlightNumber  string       `json:"flight_number"`
	From          string       `json:"from"`
	To            string       `json:"to"`
	DepartureTime time.Time    `json:"departure_time"`
	ArrivalTime   time.Time    `json:"arrival_time"`
	Status        FlightStatus `json:"status"`
	StatusMessage string       `json:"status_message"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// FlightStore manages flight reservations in memory
type FlightStore struct {
	mu           sync.RWMutex
	reservations map[string]*FlightReservation
}

// NewFlightStore creates a new flight store
func NewFlightStore() *FlightStore {
	return &FlightStore{
		reservations: make(map[string]*FlightReservation),
	}
}

// Create adds a new flight reservation
func (s *FlightStore) Create(reservation *FlightReservation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reservations[reservation.ID] = reservation
}

// Get retrieves a flight reservation by ID
func (s *FlightStore) Get(id string) (*FlightReservation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reservation, exists := s.reservations[id]
	return reservation, exists
}

// Delete removes a flight reservation
func (s *FlightStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.reservations[id]; exists {
		delete(s.reservations, id)
		return true
	}
	return false
}

// Update modifies a flight reservation
func (s *FlightStore) Update(reservation *FlightReservation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation.UpdatedAt = time.Now()
	s.reservations[reservation.ID] = reservation
}

// List returns all flight reservations
func (s *FlightStore) List() []*FlightReservation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reservations := make([]*FlightReservation, 0, len(s.reservations))
	for _, r := range s.reservations {
		reservations = append(reservations, r)
	}
	return reservations
}
