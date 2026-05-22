package debuglog

import (
	"log"
)

// Write writes a debug log entry.
func Write(level, location string, message string, data map[string]any) {
	log.Printf("[%s] %s: %s %v", level, location, message, data)
}
