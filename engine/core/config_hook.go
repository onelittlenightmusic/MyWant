package mywant

import "fmt"

// configFieldUpdaters holds server-registered callbacks that persist specific
// config.yaml fields, keyed by field name (e.g. "tunnel_url"). Registered once
// at server startup via RegisterConfigFieldUpdater, so want types (e.g.
// dynamic_background, managed_launch/process-based tunnel plugins) can update
// and persist config without an HTTP round-trip to their own server.
var configFieldUpdaters = map[string]func(string) error{}

// RegisterConfigFieldUpdater registers the updater for a single config.yaml field.
func RegisterConfigFieldUpdater(field string, fn func(string) error) {
	configFieldUpdaters[field] = fn
}

// SetConfigField applies a new value to a registered config.yaml field.
// Returns an error if no updater has been registered for that field.
func SetConfigField(field, value string) error {
	fn, ok := configFieldUpdaters[field]
	if !ok {
		return fmt.Errorf("%s updater not registered (server not initialized)", field)
	}
	return fn(value)
}

// SetCanvasBgURL applies a new background image URL through the server-registered updater.
func SetCanvasBgURL(url string) error { return SetConfigField("canvas_bg_url", url) }

// SetTunnelURL applies a newly-captured tunnel URL (e.g. from cloudflared/ngrok)
// through the server-registered updater.
func SetTunnelURL(url string) error { return SetConfigField("tunnel_url", url) }

// SetCACertPath applies a newly-detected CA root cert path (e.g. from a Caddy
// want) through the server-registered updater.
func SetCACertPath(path string) error { return SetConfigField("web_inspector_ca_cert_path", path) }

// SetHTTPSPath applies a certificate-confirmed https:// origin (e.g. from a
// Caddy want) through the server-registered updater.
func SetHTTPSPath(url string) error { return SetConfigField("https_path", url) }
