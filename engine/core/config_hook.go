package mywant

import "fmt"

// canvasBgURLUpdater is a server-registered callback that persists canvas_bg_url changes.
// Set once at server startup via RegisterCanvasBgURLUpdater.
var canvasBgURLUpdater func(string) error

// RegisterCanvasBgURLUpdater lets the server layer register a function that updates
// and persists canvas_bg_url in the server config. Called once at startup.
func RegisterCanvasBgURLUpdater(fn func(string) error) {
	canvasBgURLUpdater = fn
}

// SetCanvasBgURL applies a new background image URL through the server-registered updater.
// Returns an error if no updater has been registered.
func SetCanvasBgURL(url string) error {
	if canvasBgURLUpdater == nil {
		return fmt.Errorf("canvas_bg_url updater not registered (server not initialized)")
	}
	return canvasBgURLUpdater(url)
}
