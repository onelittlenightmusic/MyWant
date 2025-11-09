package mywant

import (
	"log"
)

// DebugLoggingEnabled is a package-level flag for debug logging in the engine
var DebugLoggingEnabled bool

// DebugLog logs a message only if debug mode is enabled
func DebugLog(format string, v ...interface{}) {
	if DebugLoggingEnabled {
		log.Printf("[DEBUG] "+format, v...)
	}
}
