package main

import (
	"log"
)

// GlobalDebugEnabled is a package-level flag for debug logging
var GlobalDebugEnabled bool

// DebugLog logs a message only if debug mode is enabled
func DebugLog(format string, v ...any) {
	if GlobalDebugEnabled {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// DebugLogf is an alias for DebugLog for consistency
func DebugLogf(format string, v ...any) {
	DebugLog(format, v...)
}

// InfoLog logs important informational messages (always shown)
func InfoLog(format string, v ...any) {
	log.Printf("[INFO] "+format, v...)
}
func ErrorLog(format string, v ...any) {
	log.Printf("[ERROR] "+format, v...)
}
