package mywant

import (
	"fmt"
	"time"
)

// BaseNotifiableWant provides default notification handling for any want type
type BaseNotifiableWant struct {
	Want               *Want
	NotificationBuffer chan StateNotification
	BufferSize         int
	isRunning          bool
}

// NewBaseNotifiableWant creates a new base notifiable want
func NewBaseNotifiableWant(want *Want, bufferSize int) *BaseNotifiableWant {
	if bufferSize <= 0 {
		bufferSize = 100 // Default buffer size
	}

	return &BaseNotifiableWant{
		Want:               want,
		NotificationBuffer: make(chan StateNotification, bufferSize),
		BufferSize:         bufferSize,
		isRunning:          false,
	}
}

// OnStateUpdate provides default implementation of StateUpdateListener
func (bnw *BaseNotifiableWant) OnStateUpdate(notification StateNotification) error {
	// Non-blocking send to buffer
	select {
	case bnw.NotificationBuffer <- notification:
		return nil
	default:
		return fmt.Errorf("notification buffer full for want %s", bnw.Want.Metadata.Name)
	}
}

// StartNotificationProcessing starts processing notifications in a goroutine
func (bnw *BaseNotifiableWant) StartNotificationProcessing() {
	if bnw.isRunning {
		return
	}
	bnw.isRunning = true

	go func() {
		for bnw.isRunning {
			select {
			case notification := <-bnw.NotificationBuffer:
				bnw.handleNotification(notification)
			case <-time.After(100 * time.Millisecond):
				// Periodic check to allow graceful shutdown
				continue
			}
		}
	}()
}

// StopNotificationProcessing stops processing notifications
func (bnw *BaseNotifiableWant) StopNotificationProcessing() {
	bnw.isRunning = false
	close(bnw.NotificationBuffer)
}

// handleNotification can be overridden by specific want types
func (bnw *BaseNotifiableWant) handleNotification(notification StateNotification) {
	fmt.Printf("[NOTIFICATION] %s received: %s.%s = %v (type: %s)\n",
		bnw.Want.Metadata.Name, notification.SourceWantName,
		notification.StateKey, notification.StateValue, notification.NotificationType)

	// Store notification in want's state for inspection
	notificationKey := fmt.Sprintf("last_notification_%s_%s", notification.SourceWantName, notification.StateKey)
	bnw.Want.StoreState(notificationKey, notification)

	// Store timestamp of last notification
	bnw.Want.StoreState("last_notification_time", notification.Timestamp)

	// Keep count of notifications received
	countKey := "notification_count"
	if count, exists := bnw.Want.GetState(countKey); exists {
		if c, ok := count.(int); ok {
			bnw.Want.StoreState(countKey, c+1)
		} else {
			bnw.Want.StoreState(countKey, 1)
		}
	} else {
		bnw.Want.StoreState(countKey, 1)
	}
}

// GetWant returns the underlying want for Executable interface compatibility
func (bnw *BaseNotifiableWant) GetWant() *Want {
	return bnw.Want
}

// GetNotificationBufferSize returns current buffer usage
func (bnw *BaseNotifiableWant) GetNotificationBufferSize() int {
	return len(bnw.NotificationBuffer)
}

// GetNotificationBufferCapacity returns buffer capacity
func (bnw *BaseNotifiableWant) GetNotificationBufferCapacity() int {
	return bnw.BufferSize
}

// IsNotificationProcessingActive returns whether notification processing is running
func (bnw *BaseNotifiableWant) IsNotificationProcessingActive() bool {
	return bnw.isRunning
}
