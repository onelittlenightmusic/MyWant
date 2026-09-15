package server

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	mywant "mywant/engine/core"
)

// A want's new answer is its notification.
//
// There used to be a watcher per kind of news — one polling spotify wants for
// a changed track and composing "♪ <track>" — and nothing at all for the rest:
// a weather want that fetched new weather told nobody. But every want that
// declares a finalResultField already keeps its answers, shaped once, in its
// result history. A new one of those IS the news, for every want alike, so the
// engine announces it (mywant.OnWantOutput) and this records it as the want's
// alert carrying that same entry. The notice, the card's history and the dots
// on the canvas are then three views of one Output, not three data models that
// have to be kept in step.

// RegisterOutputCallback wires the engine's OnWantOutput to the per-want alert
// store and push. Called once during server startup.
func (s *Server) RegisterOutputCallback() {
	mywant.OnWantOutput = func(w *mywant.Want, out mywant.ResultHistoryEntry) {
		defer func() {
			// A notice must never take down the server.
			if r := recover(); r != nil {
				log.Printf("[want-output] recovered: %v", r)
			}
		}()
		if s.notifications == nil || w == nil || w.Metadata.ID == "" {
			return
		}
		msg := outputMessage(out)
		entry := out
		if err := s.notifications.Record(NotificationEntry{
			Message:    msg,
			Kind:       "alert",
			TargetType: "want",
			TargetID:   w.Metadata.ID,
			Output:     &entry,
		}); err != nil {
			log.Printf("[want-output] failed to record alert for %s: %v", w.Metadata.Name, err)
			return
		}
		title := w.Metadata.Name
		if title == "" {
			title = "MyWant"
		}
		s.sendWantPush(w.Metadata.ID, title, msg, "/w/"+w.Metadata.ID)
	}
}

// outputMessage says an answer in one line, for the places that can only show
// text — a push notification, the notice log.
//
// A string answer is its own sentence. A structured one says it with the
// fields people name things by, and only falls back to its JSON when it has
// none of them.
func outputMessage(out mywant.ResultHistoryEntry) string {
	switch v := out.Result.(type) {
	case string:
		return truncateRunes(strings.TrimSpace(v), 120)
	case map[string]any:
		for _, k := range []string{"summary", "title", "name", "datetime", "text"} {
			if s, ok := v[k].(string); ok && strings.TrimSpace(s) != "" {
				return truncateRunes(strings.TrimSpace(s), 120)
			}
		}
	case []any:
		return fmt.Sprintf("%d件", len(v))
	}
	b, err := json.Marshal(out.Result)
	if err != nil {
		return "新しい結果"
	}
	return truncateRunes(string(b), 120)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
