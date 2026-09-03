package server

import (
	"log"
	"strings"
	"sync"
	"time"
)

// Spotify "now playing" → per-want alert.
//
// A small watcher so notifications can be exercised end to end: every few
// seconds it reads each spotify want's current track (its final_result, which
// mirrors current.track_name) and, when that changes to a new non-empty value,
// raises the same per-want alert everything else does — badging the tile and
// pushing to a subscribed /w/<id> home-screen app.
//
// Deliberately poll-based and self-contained: it touches neither the core event
// system nor the spotify plugin, so it is easy to drop later.

const spotifyWatchInterval = 8 * time.Second

type spotifyTrackWatcher struct {
	mu   sync.Mutex
	seen map[string]string // want id → last track we notified about
}

// startSpotifyTrackWatcher launches the background loop. Safe no-op if the
// builder is missing.
func (s *Server) startSpotifyTrackWatcher() {
	if s.globalBuilder == nil {
		return
	}
	w := &spotifyTrackWatcher{seen: map[string]string{}}
	go func() {
		ticker := time.NewTicker(spotifyWatchInterval)
		defer ticker.Stop()
		// Prime once so a track already playing at startup isn't announced.
		w.scan(s, true)
		for range ticker.C {
			w.scan(s, false)
		}
	}()
	log.Printf("[spotify-notify] watching spotify wants for track changes every %s", spotifyWatchInterval)
}

func (w *spotifyTrackWatcher) scan(s *Server, prime bool) {
	defer func() {
		// A panic here must never take down the server.
		if r := recover(); r != nil {
			log.Printf("[spotify-notify] recovered: %v", r)
		}
	}()

	for _, want := range s.globalBuilder.GetAllWantStates() {
		if want == nil || want.Metadata.Type != "spotify" {
			continue
		}
		track := currentSpotifyTrack(want.GetAllState())
		if track == "" {
			continue
		}
		id := want.Metadata.ID

		w.mu.Lock()
		prev, had := w.seen[id]
		changed := track != prev
		if changed {
			w.seen[id] = track
		}
		w.mu.Unlock()

		if prime || !changed || !had {
			continue
		}

		name := want.Metadata.Name
		if name == "" {
			name = "Spotify"
		}
		msg := "♪ " + track
		_ = s.notifications.Record(NotificationEntry{
			Message:    msg,
			Kind:       "alert",
			TargetType: "want",
			TargetID:   id,
		})
		s.sendWantPush(id, name, msg, "/w/"+id)
		log.Printf("[spotify-notify] %s → %q", name, track)
	}
}

// currentSpotifyTrack pulls the track title out of a spotify want's state:
// final_result (the finalResultField mirror), else current.track_name.
func currentSpotifyTrack(state map[string]any) string {
	if v, ok := state["final_result"].(string); ok {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	if cur, ok := state["current"].(map[string]any); ok {
		if v, ok := cur["track_name"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
