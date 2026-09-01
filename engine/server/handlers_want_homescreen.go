package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// One want, added to a phone's home screen.
//
// iOS reads a few tags off the /w/<id> page at "Add to Home Screen" time — the
// title, the apple-mobile-web-app-* metas, an apple-touch-icon, and (newer iOS)
// the web manifest. The mywant-gui server rewrites those tags for /w/<id>; this
// endpoint is the manifest they point at — one that names the want and, unlike
// the whole-app manifest, has start_url "/w/<id>" so the icon launches straight
// into that want.
//
// The icon itself is a static PNG shipped with the frontend
// (mywant-gui web/scripts/gen-want-icons.mjs → /want-icons/<key>-<size>.png),
// not rendered here.

// handleWantManifest serves GET /api/v1/wants/{id}/manifest.webmanifest.
func (s *Server) handleWantManifest(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	want, _, found := s.globalBuilder.FindWantByID(id)
	if !found {
		http.NotFound(w, r)
		return
	}
	name := want.Metadata.Name
	if name == "" {
		name = "Want"
	}
	short := name
	if len([]rune(short)) > 14 {
		short = string([]rune(short)[:14])
	}

	manifest := map[string]any{
		"name":             name,
		"short_name":       short,
		"start_url":        "/w/" + id,
		"scope":            "/",
		"display":          "standalone",
		"orientation":      "any",
		"background_color": "#1e293b",
		"theme_color":      "#3b82f6",
		"icons": []map[string]any{
			// Per-category apple-touch-icon is set on the page head by the
			// mywant-gui server / SPA; the manifest carries the generic set so
			// a Chrome "Add to Home Screen" still has an installable icon.
			{"src": "/want-icons/default-192.png", "sizes": "192x192", "type": "image/png", "purpose": "maskable any"},
			{"src": "/want-icons/default-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable any"},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(manifest)
}
