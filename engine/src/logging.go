package mywant

import (
	"log"
)

// InfoLog logs informational messages with timestamps
// It's used for all prefixed logs to ensure they display timestamps
func InfoLog(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// ErrorLog logs error messages with timestamps
func ErrorLog(format string, v ...interface{}) {
	log.Printf("[ERROR] "+format, v...)
}
