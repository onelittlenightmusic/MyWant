package main

import (
	"fmt"
	"sync"
	"time"
)

// StatusUpdater manages scheduled status updates for flight reservations
type StatusUpdater struct {
	store   *FlightStore
	timers  map[string][]*time.Timer
	mu      sync.RWMutex
	stopped bool
}

// NewStatusUpdater creates a new status updater
func NewStatusUpdater(store *FlightStore) *StatusUpdater {
	return &StatusUpdater{
		store:  store,
		timers: make(map[string][]*time.Timer),
	}
}

// Start initializes the updater
func (u *StatusUpdater) Start() {
	u.stopped = false
}

// Stop cancels all scheduled updates
func (u *StatusUpdater) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.stopped = true
	for _, timers := range u.timers {
		for _, timer := range timers {
			timer.Stop()
		}
	}
	u.timers = make(map[string][]*time.Timer)
}

// ScheduleUpdates schedules status changes for a flight reservation
// Only schedules detail change and delay transitions for AA100, not for rebooked flights (AA100A, AA100B, etc.)
func (u *StatusUpdater) ScheduleUpdates(reservationID string) {
	// Get the flight to check its flight number first (before locking)
	reservation, exists := u.store.Get(reservationID)

	u.mu.Lock()
	defer u.mu.Unlock()

	if u.stopped {
		return
	}

	if !exists {
		return
	}

	// Only schedule status transitions for the base flight AA100
	// Skip transitions for rebooked flights (AA100A, AA100B, AA100C, etc.)
	if reservation.FlightNumber != "AA100" {
		fmt.Printf("[StatusUpdater] Flight %s (%s): Skipping status transitions (not base flight AA100)\n", reservationID, reservation.FlightNumber)
		u.timers[reservationID] = []*time.Timer{}
		return
	}

	// Schedule first update: details changed after 20 seconds
	timer1 := time.AfterFunc(20*time.Second, func() {
		u.updateToDetailsChanged(reservationID)
	})

	// Schedule second update: delayed after 40 seconds (20 + 20)
	timer2 := time.AfterFunc(40*time.Second, func() {
		u.updateToDelayed(reservationID)
	})

	// Store timers for this reservation
	u.timers[reservationID] = []*time.Timer{timer1, timer2}
	fmt.Printf("[StatusUpdater] Flight %s (AA100): Scheduled status transitions (details_changed@20s, delayed@40s)\n", reservationID)
}

// CancelUpdates cancels all scheduled updates for a reservation
func (u *StatusUpdater) CancelUpdates(reservationID string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if timers, exists := u.timers[reservationID]; exists {
		for _, timer := range timers {
			timer.Stop()
		}
		delete(u.timers, reservationID)
	}
}

// updateToDetailsChanged updates flight status to details changed
func (u *StatusUpdater) updateToDetailsChanged(reservationID string) {
	reservation, exists := u.store.Get(reservationID)
	if !exists {
		return
	}

	// Simulate minor changes to flight details
	reservation.Status = StatusDetailsChanged
	reservation.StatusMessage = "Flight details have been changed. Gate or aircraft type may have changed."
	reservation.UpdatedAt = time.Now()

	u.store.Update(reservation)
	fmt.Printf("[StatusUpdater] Flight %s: Status changed to DETAILS_CHANGED\n", reservationID)
}

// updateToDelayed updates flight status to delayed
func (u *StatusUpdater) updateToDelayed(reservationID string) {
	reservation, exists := u.store.Get(reservationID)
	if !exists {
		return
	}

	// Add one day delay to departure and arrival times
	reservation.DepartureTime = reservation.DepartureTime.Add(24 * time.Hour)
	reservation.ArrivalTime = reservation.ArrivalTime.Add(24 * time.Hour)
	reservation.Status = StatusDelayed
	reservation.StatusMessage = "Flight delayed by one day due to airport incident"
	reservation.UpdatedAt = time.Now()

	u.store.Update(reservation)
	fmt.Printf("[StatusUpdater] Flight %s: Status changed to DELAYED_ONE_DAY\n", reservationID)

	// Clean up timers for this reservation
	u.mu.Lock()
	delete(u.timers, reservationID)
	u.mu.Unlock()
}
