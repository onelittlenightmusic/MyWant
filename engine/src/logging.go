package mywant

import (
	"log"
)

// InfoLog logs informational messages with timestamps It's used for all prefixed logs to ensure they display timestamps
func InfoLog(format string, v ...any) {
	log.Printf(format, v...)
}
func ErrorLog(format string, v ...any) {
	log.Printf("[ERROR] "+format, v...)
}
